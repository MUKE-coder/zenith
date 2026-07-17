import { Ridgeline } from "./ridgeline";

/**
 * A stylized analytics dashboard panel for the hero — the product's own visual
 * language (summary tiles + the ridgeline), rendered as a glassy floating card.
 */
export function DashboardMock() {
  return (
    <div className="relative">
      {/* Depth: an offset panel behind the main one. */}
      <div className="absolute -right-4 -top-4 hidden h-full w-full rounded-2xl border border-line bg-surface/40 sm:block" />

      <div className="relative overflow-hidden rounded-2xl border border-line-strong bg-surface shadow-2xl shadow-black/40">
        {/* Window chrome. */}
        <div className="flex items-center gap-2 border-b border-line px-4 py-3">
          <span className="size-2.5 rounded-full bg-line-strong" />
          <span className="size-2.5 rounded-full bg-line-strong" />
          <span className="size-2.5 rounded-full bg-line-strong" />
          <span className="ml-3 font-mono text-xs text-subtle">acme.com/analytics-dashboard</span>
          <span className="ml-auto inline-flex items-center gap-1.5 rounded-full border border-line px-2 py-0.5 text-[11px] text-muted">
            <span className="size-1.5 animate-pulse rounded-full bg-positive" />
            live
          </span>
        </div>

        <div className="space-y-5 p-5">
          {/* Summary tiles. */}
          <div className="grid grid-cols-3 gap-3">
            {[
              { label: "Visitors", value: "12,847", delta: "+18%" },
              { label: "Pageviews", value: "48.1K", delta: "+12%" },
              { label: "Sessions", value: "15.3K", delta: "+9%" },
            ].map((s) => (
              <div key={s.label} className="rounded-xl border border-line bg-bg-2 p-3">
                <p className="text-[11px] uppercase tracking-wide text-subtle">{s.label}</p>
                <p className="mt-1 font-mono text-xl font-semibold text-text tabular-nums">
                  {s.value}
                </p>
                <p className="mt-0.5 text-[11px] font-medium text-positive">{s.delta}</p>
              </div>
            ))}
          </div>

          {/* The ridgeline. */}
          <div className="rounded-xl border border-line bg-bg-2 p-4">
            <div className="mb-2 flex items-center justify-between">
              <p className="text-xs font-medium text-muted">Traffic</p>
              <p className="font-mono text-[11px] text-subtle">last 30 days</p>
            </div>
            <Ridgeline className="h-32 w-full" />
          </div>

          {/* A breakdown row. */}
          <div className="grid grid-cols-2 gap-3 text-xs">
            <div className="rounded-xl border border-line bg-bg-2 p-3">
              <p className="mb-2 text-[11px] uppercase tracking-wide text-subtle">Top pages</p>
              {[
                ["/", "5,210"],
                ["/pricing", "3,102"],
                ["/blog", "2,011"],
              ].map(([path, n]) => (
                <div key={path} className="flex items-center justify-between py-0.5">
                  <span className="font-mono text-muted">{path}</span>
                  <span className="font-mono text-subtle tabular-nums">{n}</span>
                </div>
              ))}
            </div>
            <div className="rounded-xl border border-line bg-bg-2 p-3">
              <p className="mb-2 text-[11px] uppercase tracking-wide text-subtle">Referrers</p>
              {[
                ["news.ycombinator.com", "1,204"],
                ["google.com", "890"],
                ["x.com", "410"],
              ].map(([src, n]) => (
                <div key={src} className="flex items-center justify-between py-0.5">
                  <span className="truncate font-mono text-muted">{src}</span>
                  <span className="ml-2 font-mono text-subtle tabular-nums">{n}</span>
                </div>
              ))}
            </div>
          </div>
        </div>
      </div>
    </div>
  );
}
