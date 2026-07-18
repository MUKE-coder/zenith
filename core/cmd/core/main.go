// Command core runs the Zenith core service: event ingestion, the analytics
// query API, auth, and the developer console.
package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/zenith/core/internal/account"
	"github.com/zenith/core/internal/auth"
	"github.com/zenith/core/internal/config"
	"github.com/zenith/core/internal/geo"
	zhttp "github.com/zenith/core/internal/http"
	"github.com/zenith/core/internal/scheduler"
	"github.com/zenith/core/internal/storage"
	"github.com/zenith/core/internal/storage/duckdb"
	"github.com/zenith/core/internal/storage/sqlite"
	"github.com/zenith/core/internal/visitor"
)

// provisionAdmin creates the first developer account from the environment.
//
// It never overwrites an existing account, so leaving the variables set across
// restarts is safe: a restart cannot reset the developer's password back to
// whatever the environment happens to say.
func provisionAdmin(ctx context.Context, cfg config.Config, app storage.AppStore, log *slog.Logger) error {
	if cfg.AdminEmail == "" && cfg.AdminPassword == "" {
		// Not configured. If nobody can log in yet, say so once rather than
		// leaving the operator to discover it at the login screen.
		n, err := app.CountUsers(ctx)
		if err != nil {
			return err
		}
		if n == 0 {
			log.Warn("no accounts exist yet: set ZENITH_ADMIN_EMAIL and " +
				"ZENITH_ADMIN_PASSWORD, or run `go run ./cmd/seed`")
		}
		return nil
	}

	if cfg.AdminEmail == "" || cfg.AdminPassword == "" {
		return errors.New("set both ZENITH_ADMIN_EMAIL and ZENITH_ADMIN_PASSWORD, or neither")
	}

	created, err := account.Provision(ctx, app, cfg.AdminEmail, cfg.AdminPassword)
	if err != nil {
		return err
	}
	if created {
		log.Info("created developer account", "email", cfg.AdminEmail)
	}
	return nil
}

func main() {
	// The container healthcheck re-execs this binary rather than shelling out to
	// curl, which the slim runtime image deliberately does not ship.
	healthcheck := flag.Bool("healthcheck", false, "probe the local /health endpoint and exit")
	flag.Parse()

	log := slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelInfo}))

	if *healthcheck {
		if err := probeHealth(); err != nil {
			fmt.Fprintln(os.Stderr, err)
			os.Exit(1)
		}
		return
	}

	if err := run(log); err != nil {
		log.Error("fatal", "err", err)
		os.Exit(1)
	}
}

// probeHealth is the container healthcheck: hit /health, exit non-zero unless
// it reports 200.
func probeHealth() error {
	cfg, err := config.Load()
	if err != nil {
		return err
	}

	client := &http.Client{Timeout: 5 * time.Second}
	url := fmt.Sprintf("http://127.0.0.1:%d/health", cfg.Port)

	resp, err := client.Get(url)
	if err != nil {
		return fmt.Errorf("healthcheck: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("healthcheck: status %d", resp.StatusCode)
	}
	return nil
}

func run(log *slog.Logger) error {
	cfg, err := config.Load()
	if err != nil {
		return err
	}

	// Signal-aware from the start, so a Ctrl-C during a slow DB open still exits.
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	events, err := duckdb.Open(ctx, cfg.EventsDBPath)
	if err != nil {
		return err
	}
	defer events.Close()
	log.Info("event store ready", "path", cfg.EventsDBPath)

	app, err := sqlite.Open(ctx, cfg.AppDBPath)
	if err != nil {
		return err
	}
	defer app.Close()

	if err := app.Migrate(ctx); err != nil {
		return err
	}
	log.Info("app store ready", "path", cfg.AppDBPath)

	if cfg.EphemeralSecret {
		log.Warn("ZENITH_JWT_SECRET is not set: generated a temporary signing " +
			"secret. Every restart will sign everyone out. Development only.")
	}

	issuer, err := auth.NewIssuer(cfg.JWTSecret, cfg.TokenTTL)
	if err != nil {
		return err
	}

	if err := provisionAdmin(ctx, cfg, app, log); err != nil {
		return err
	}

	// Revoked tokens are only useful until they expire on their own.
	if n, err := app.DeleteExpiredRevokedTokens(ctx); err != nil {
		// Not fatal: a full denylist is harmless, just untidy.
		log.Warn("could not sweep expired tokens", "err", err)
	} else if n > 0 {
		log.Info("swept expired tokens", "count", n)
	}

	// The salt behind this lives in memory only and rotates daily, so a
	// restart starts a new one and today's visitors may be counted twice.
	// That is the deliberate cost of never writing the salt down.
	visitors, err := visitor.NewHasher()
	if err != nil {
		return err
	}

	countries, err := geo.Open(cfg.GeoIPDBPath)
	geoReady := err == nil && cfg.GeoIPDBPath != ""
	if err != nil {
		// A configured but unreadable database is not a reason to refuse to
		// start. Country is a nice-to-have; ingestion is not -- and a
		// deployment that downloads the file alongside core should never be
		// one failed download away from collecting nothing.
		log.Warn("geo database unavailable: country will be unknown",
			"path", cfg.GeoIPDBPath, "error", err)
		countries = geo.Unavailable{}
	}
	defer countries.Close()

	switch {
	case cfg.GeoIPDBPath == "":
		log.Info("no geo database configured: country will be unknown " +
			"(set ZENITH_GEOIP_DB to a country .mmdb to enable it)")
	case geoReady:
		log.Info("geo database ready", "path", cfg.GeoIPDBPath)
	}

	// Monthly reports. Built in rather than an external cron: a self-hosted
	// product that needs a crontab entry to do half its job is one most people
	// will deploy half-broken.
	reporter := scheduler.NewReporter(app, events, log)
	if cfg.ResendEndpoint != "" {
		log.Warn("sending email through a custom endpoint, not Resend",
			"endpoint", cfg.ResendEndpoint)
		reporter.SetEndpoint(cfg.ResendEndpoint)
	}

	reports := scheduler.New(reporter, log)
	if err := reports.Start(); err != nil {
		return err
	}
	defer reports.Stop()

	srv := &http.Server{
		Addr: fmt.Sprintf(":%d", cfg.Port),
		Handler: zhttp.New(zhttp.Deps{
			Events:       events,
			App:          app,
			Issuer:       issuer,
			Visitors:     visitors,
			Geo:          countries,
			Log:          log,
			Reporter:     reporter,
			DashboardDir: cfg.DashboardDir,
		}).Routes(),
		ReadHeaderTimeout: 10 * time.Second,
	}

	errCh := make(chan error, 1)
	go func() {
		log.Info("core listening", "addr", srv.Addr)
		if err := srv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			errCh <- err
		}
	}()

	select {
	case err := <-errCh:
		return err
	case <-ctx.Done():
		log.Info("shutting down")
	}

	shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	return srv.Shutdown(shutdownCtx)
}
