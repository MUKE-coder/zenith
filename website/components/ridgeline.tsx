/**
 * The daily traffic ridgeline — Zenith's signature.
 *
 * A crisp single-hue line with a faint accent glow beneath, reading like an
 * altitude profile climbing toward a peak. Pure SVG, deterministic, no client
 * JS. The same visual language the product's dashboard uses.
 */

// A hand-tuned series that rises with a couple of dips to a clear peak.
const DATA = [8, 10, 9, 14, 12, 18, 16, 22, 20, 28, 34, 31, 40, 52, 48, 64, 78];

const W = 560;
const H = 240;
const PAD = 12;

function buildPath() {
  const max = Math.max(...DATA);
  const stepX = (W - PAD * 2) / (DATA.length - 1);
  const points = DATA.map((v, i) => {
    const x = PAD + i * stepX;
    const y = H - PAD - (v / max) * (H - PAD * 2);
    return [x, y] as const;
  });

  // A smooth catmull-rom-ish curve through the points.
  let d = `M ${points[0][0]} ${points[0][1]}`;
  for (let i = 0; i < points.length - 1; i++) {
    const [x0, y0] = points[Math.max(0, i - 1)];
    const [x1, y1] = points[i];
    const [x2, y2] = points[i + 1];
    const [x3, y3] = points[Math.min(points.length - 1, i + 2)];
    const c1x = x1 + (x2 - x0) / 6;
    const c1y = y1 + (y2 - y0) / 6;
    const c2x = x2 - (x3 - x1) / 6;
    const c2y = y2 - (y3 - y1) / 6;
    d += ` C ${c1x} ${c1y}, ${c2x} ${c2y}, ${x2} ${y2}`;
  }
  const peak = points[points.length - 1];
  return { line: d, area: `${d} L ${peak[0]} ${H} L ${points[0][0]} ${H} Z`, peak };
}

export function Ridgeline({ className = "" }: { className?: string }) {
  const { line, area, peak } = buildPath();

  return (
    <svg
      viewBox={`0 0 ${W} ${H}`}
      className={className}
      fill="none"
      role="img"
      aria-label="A traffic chart climbing to a peak"
      preserveAspectRatio="none"
    >
      <defs>
        <linearGradient id="ridge-glow" x1="0" y1="0" x2="0" y2="1">
          <stop offset="0%" stopColor="var(--accent)" stopOpacity="0.28" />
          <stop offset="100%" stopColor="var(--accent)" stopOpacity="0" />
        </linearGradient>
      </defs>

      {/* Faint horizontal gridlines. */}
      {[0.25, 0.5, 0.75].map((f) => (
        <line
          key={f}
          x1={PAD}
          x2={W - PAD}
          y1={H - PAD - f * (H - PAD * 2)}
          y2={H - PAD - f * (H - PAD * 2)}
          stroke="var(--border)"
          strokeWidth="1"
        />
      ))}

      <path d={area} fill="url(#ridge-glow)" />
      <path
        d={line}
        stroke="var(--accent-bright)"
        strokeWidth="2.5"
        strokeLinecap="round"
        strokeLinejoin="round"
      />

      {/* The current-day accent tick + dot at the peak. */}
      <line x1={peak[0]} x2={peak[0]} y1={PAD} y2={H - PAD} stroke="var(--accent)" strokeWidth="1" strokeOpacity="0.35" />
      <circle cx={peak[0]} cy={peak[1]} r="4" fill="var(--accent-bright)" />
      <circle cx={peak[0]} cy={peak[1]} r="8" fill="var(--accent-bright)" fillOpacity="0.18" />
    </svg>
  );
}
