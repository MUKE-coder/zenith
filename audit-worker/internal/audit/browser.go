package audit

import (
	"context"
	"encoding/json"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/chromedp/cdproto/runtime"
	"github.com/chromedp/chromedp"
)

// UserAgent identifies the auditor.
//
// Honest about being a bot, and it names the product so a site owner reading
// their logs can tell what it was. It also means Zenith's own tracking treats
// an audit as a bot and does not count it as a visitor.
const UserAgent = "Mozilla/5.0 (compatible; ZenithAuditBot/1.0; +https://github.com/zenith)"

// pageTimeout caps rendering one page.
//
// Generous enough for a slow page on a slow site, short enough that one
// unresponsive URL cannot hold the whole audit.
const pageTimeout = 30 * time.Second

// settleDelay is how long to wait after load for late paint and layout shift.
//
// LCP and CLS are not final at load: an image decoding or a font swapping
// afterwards changes both. Sampling immediately would report numbers better
// than the ones a visitor actually experiences.
const settleDelay = 1500 * time.Millisecond

// Browser renders pages.
type Browser struct {
	allocCtx context.Context
	cancel   context.CancelFunc
}

// NewBrowser starts a headless Chromium.
//
// One browser process for the whole run: starting Chromium costs about a
// second, and paying that per page would dominate the audit.
func NewBrowser(ctx context.Context, chromePath string) (*Browser, error) {
	opts := append(chromedp.DefaultExecAllocatorOptions[:],
		chromedp.UserAgent(UserAgent),
		chromedp.WindowSize(1440, 900),
		// The container runs as an unprivileged user with no sandbox support.
		// Safe here because the alternative -- root -- is worse, and the pages
		// rendered are the developer's own clients'.
		chromedp.NoSandbox,
		chromedp.DisableGPU,
		chromedp.Flag("disable-dev-shm-usage", true),
	)

	if chromePath != "" {
		opts = append(opts, chromedp.ExecPath(chromePath))
	}

	allocCtx, cancel := chromedp.NewExecAllocator(ctx, opts...)

	// A throwaway page proves Chromium actually starts. Without it the first
	// real failure surfaces mid-audit as a confusing per-page error.
	probeCtx, probeCancel := chromedp.NewContext(allocCtx)
	defer probeCancel()

	probe, probeTimeout := context.WithTimeout(probeCtx, 30*time.Second)
	defer probeTimeout()

	if err := chromedp.Run(probe); err != nil {
		cancel()
		return nil, fmt.Errorf("audit: could not start Chromium: %w", err)
	}

	return &Browser{allocCtx: allocCtx, cancel: cancel}, nil
}

// Close shuts the browser down.
func (b *Browser) Close() { b.cancel() }

// vitalsCollectMs is how long the observers are given to report.
//
// buffered:true replays entries synchronously on the next task, so this only
// has to outlast a tick. It is not a settle window -- settleDelay already did
// that before this script runs.
const vitalsCollectMs = 300

// extractScript pulls everything the checks need out of a rendered page.
//
// One evaluation rather than a dozen round trips: each chromedp call is a CDP
// message, and this runs once per page across a whole site.
//
// It returns a Promise, because LCP and CLS cannot be read synchronously.
// They are not in the performance timeline -- getEntriesByType returns nothing
// for either -- and are only reachable through a PerformanceObserver with
// buffered:true, which replays the entries recorded during load. Reading them
// the obvious way silently reports 0 for every page, which looks exactly like
// a fast site.
// extractScript is the template; VITALS_COLLECT_MS is substituted at init.
// Go raw strings do not interpolate, so a ${...} here would reach the browser
// verbatim and fail to parse -- taking every page's render down with it.
var extractScript = strings.ReplaceAll(extractTemplate,
	"VITALS_COLLECT_MS", strconv.Itoa(vitalsCollectMs))

const extractTemplate = `(async () => {
  const text = (el) => (el ? (el.textContent || '').trim() : '');
  const attr = (sel, name) => {
    const el = document.querySelector(sel);
    return el ? (el.getAttribute(name) || '') : '';
  };

  // Observe first, then wait: buffered:true delivers what already happened.
  const vitals = await new Promise((resolve) => {
    let lcp = 0;
    let cls = 0;
    const observers = [];

    const observe = (type, handle) => {
      try {
        const observer = new PerformanceObserver((list) => {
          for (const entry of list.getEntries()) handle(entry);
        });
        observer.observe({ type, buffered: true });
        observers.push(observer);
      } catch (e) {
        // An engine without this entry type. The rest still stands.
      }
    };

    // The last LCP candidate wins: the metric is the largest paint, and later
    // ones supersede earlier ones.
    observe('largest-contentful-paint', (entry) => { lcp = entry.startTime; });
    observe('layout-shift', (entry) => {
      // A shift the visitor caused by interacting is not a defect.
      if (!entry.hadRecentInput) cls += entry.value;
    });

    setTimeout(() => {
      for (const observer of observers) observer.disconnect();
      resolve({ lcp, cls });
    }, VITALS_COLLECT_MS);
  });

  const images = [...document.images];
  const links = [...document.querySelectorAll('a[href]')]
    .map((a) => a.href)
    .filter((h) => h.startsWith('http'));

  const jsonld = [...document.querySelectorAll('script[type="application/ld+json"]')]
    .map((s) => s.textContent || '')
    .filter((s) => s.trim() !== '');

  const nav = performance.getEntriesByType('navigation')[0] || {};
  const paint = performance.getEntriesByType('paint') || [];
  const fcp = paint.find((p) => p.name === 'first-contentful-paint');

  return {
    title: document.title || '',
    description: attr('meta[name="description"]', 'content'),
    canonical: attr('link[rel="canonical"]', 'href'),
    robots: attr('meta[name="robots"]', 'content'),
    h1s: [...document.querySelectorAll('h1')].map(text).filter(Boolean),
    headings: [...document.querySelectorAll('h1,h2,h3,h4,h5,h6')].map((h) => h.tagName.toLowerCase()),
    imagesTotal: images.length,
    imagesNoAlt: images.filter((img) => !img.hasAttribute('alt') || img.alt.trim() === '').length,
    links: [...new Set(links)],
    jsonld,
    vitals: {
      ttfb_ms: nav.responseStart || 0,
      fcp_ms: fcp ? fcp.startTime : 0,
      lcp_ms: vitals.lcp,
      cls: vitals.cls,
      dcl_ms: nav.domContentLoadedEventEnd || 0,
      load_ms: nav.loadEventEnd || 0,
    },
  };
})()`

// extracted mirrors extractScript's return value.
type extracted struct {
	Title       string   `json:"title"`
	Description string   `json:"description"`
	Canonical   string   `json:"canonical"`
	Robots      string   `json:"robots"`
	H1s         []string `json:"h1s"`
	Headings    []string `json:"headings"`
	ImagesTotal int      `json:"imagesTotal"`
	ImagesNoAlt int      `json:"imagesNoAlt"`
	Links       []string `json:"links"`
	JSONLD      []string `json:"jsonld"`
	Vitals      Vitals   `json:"vitals"`
}

// Render loads a page and extracts what the checks need.
func (b *Browser) Render(ctx context.Context, pageURL string) (Page, error) {
	// A fresh tab per page: state left behind by the last page -- a service
	// worker, a modal, a stuck timer -- would otherwise leak into the next
	// page's numbers.
	tabCtx, cancelTab := chromedp.NewContext(b.allocCtx)
	defer cancelTab()

	timeoutCtx, cancelTimeout := context.WithTimeout(tabCtx, pageTimeout)
	defer cancelTimeout()

	var raw []byte

	err := chromedp.Run(timeoutCtx,
		chromedp.Navigate(pageURL),
		// Ready, not loaded: waiting for every third-party script to finish
		// would time out on pages that work fine for people.
		chromedp.WaitReady("body", chromedp.ByQuery),
		chromedp.Sleep(settleDelay),
		// The script is async, so the promise has to be awaited -- otherwise
		// what comes back is an unresolved Promise object rather than the page.
		chromedp.Evaluate(extractScript, &raw,
			func(p *runtime.EvaluateParams) *runtime.EvaluateParams {
				return p.WithAwaitPromise(true)
			}),
	)
	if err != nil {
		if ctx.Err() != nil {
			return Page{}, ctx.Err()
		}
		return Page{}, fmt.Errorf("could not render %s: %w", pageURL, err)
	}

	var data extracted
	if err := json.Unmarshal(raw, &data); err != nil {
		return Page{}, fmt.Errorf("could not read %s: %w", pageURL, err)
	}

	return Page{
		URL:         pageURL,
		Title:       data.Title,
		Description: data.Description,
		Canonical:   data.Canonical,
		Robots:      data.Robots,
		H1s:         data.H1s,
		Headings:    data.Headings,
		ImagesTotal: data.ImagesTotal,
		ImagesNoAlt: data.ImagesNoAlt,
		Links:       data.Links,
		JSONLD:      data.JSONLD,
		Vitals:      data.Vitals,
	}, nil
}
