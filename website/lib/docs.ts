/** The documentation navigation tree. Order here is the sidebar order. */

export type DocLink = { title: string; slug: string };
export type DocSection = { title: string; links: DocLink[] };

export const DOC_NAV: DocSection[] = [
  {
    title: "Getting started",
    links: [
      { title: "Overview", slug: "" },
      { title: "Quickstart", slug: "quickstart" },
      { title: "The two keys", slug: "keys" },
    ],
  },
  {
    title: "Install in your app",
    links: [
      { title: "Next.js", slug: "nextjs" },
      { title: "Tracking & events", slug: "tracking" },
      { title: "Domain-native dashboard", slug: "dashboard" },
      { title: "Any framework", slug: "script" },
    ],
  },
  {
    title: "Run the server",
    links: [
      { title: "Self-hosting", slug: "self-hosting" },
      { title: "Monthly reports", slug: "reports" },
      { title: "SEO audits", slug: "audits" },
      { title: "Configuration", slug: "config" },
    ],
  },
];

/** Flat, ordered list — used for prev/next navigation. */
export const DOC_ORDER: DocLink[] = DOC_NAV.flatMap((s) => s.links);

export function docHref(slug: string): string {
  return slug ? `/docs/${slug}` : "/docs";
}

export function neighbors(slug: string): { prev?: DocLink; next?: DocLink } {
  const i = DOC_ORDER.findIndex((l) => l.slug === slug);
  if (i === -1) return {};
  return { prev: DOC_ORDER[i - 1], next: DOC_ORDER[i + 1] };
}
