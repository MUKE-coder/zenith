/** Shared site constants and content. */

export const GITHUB_URL = "https://github.com/MUKE-coder/zenith";
export const NPM_URL = "https://www.npmjs.com/package/zenith-analytics";

export const TECH = ["Go", "DuckDB", "SQLite", "React", "Next.js", "Docker"];

export type Feature = {
  title: string;
  description: string;
  icon: string; // lucide icon name, resolved in the component
};

export const FEATURES: Feature[] = [
  {
    title: "Cookieless tracking",
    description:
      "A daily-rotating visitor hash — no cookies, no fingerprinting, no persistent identifier. GDPR-clean, so no consent banner.",
    icon: "shield-check",
  },
  {
    title: "Multi-site console",
    description:
      "Deploy once. Every site you manage reports into one place. Switch between clients from a single dashboard.",
    icon: "layout-grid",
  },
  {
    title: "Domain-native dashboards",
    description:
      "Your client reads their analytics at theirsite.com/analytics-dashboard — password-gated, same-origin, native to their brand.",
    icon: "globe",
  },
  {
    title: "SEO audits",
    description:
      "On-demand, headless-Chromium audits: titles, meta, links, structured data, and Core Web Vitals from the real render.",
    icon: "search-check",
  },
  {
    title: "Monthly email reports",
    description:
      "Each client is emailed last month's numbers automatically on the 1st. No cron to run — the scheduler is built in.",
    icon: "mail",
  },
  {
    title: "Self-hosted, open-core",
    description:
      "One docker compose up. Two Go services, DuckDB for events, SQLite for app data. Your data stays on your box.",
    icon: "server",
  },
];

export type Stat = { label: string; value: string; unit?: string };

// Illustrative aggregate numbers for the marketing stats band.
export const STATS: Stat[] = [
  { label: "Tracking script", value: "1", unit: "KB, cookieless" },
  { label: "Sites per deploy", value: "∞" },
  { label: "Consent banners", value: "0" },
  { label: "Tests, end to end", value: "320" },
];
