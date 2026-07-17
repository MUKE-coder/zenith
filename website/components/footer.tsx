import Link from "next/link";
import { GitHubIcon } from "./icons";
import { Logo } from "./logo";
import { Container } from "./ui";
import { GITHUB_URL, NPM_URL } from "@/lib/site";

const COLS: { title: string; links: { label: string; href: string; external?: boolean }[] }[] = [
  {
    title: "Product",
    links: [
      { label: "Features", href: "/#features" },
      { label: "How it works", href: "/#how" },
      { label: "Docs", href: "/docs" },
    ],
  },
  {
    title: "Docs",
    links: [
      { label: "Quickstart", href: "/docs/quickstart" },
      { label: "Next.js setup", href: "/docs/nextjs" },
      { label: "Self-hosting", href: "/docs/self-hosting" },
    ],
  },
  {
    title: "Open source",
    links: [
      { label: "GitHub", href: GITHUB_URL, external: true },
      { label: "npm package", href: NPM_URL, external: true },
      { label: "MIT License", href: `${GITHUB_URL}/blob/main/LICENSE`, external: true },
    ],
  },
];

export function Footer() {
  return (
    <footer className="border-t border-line">
      <Container className="grid gap-10 py-14 sm:grid-cols-2 lg:grid-cols-4">
        <div className="space-y-3">
          <Logo />
          <p className="max-w-xs text-sm text-subtle">
            The vantage point over all your sites. Privacy-first analytics and SEO, self-hosted.
          </p>
        </div>

        {COLS.map((col) => (
          <div key={col.title} className="space-y-3">
            <p className="text-xs font-medium uppercase tracking-[0.12em] text-subtle">
              {col.title}
            </p>
            <ul className="space-y-2 text-sm">
              {col.links.map((l) => (
                <li key={l.label}>
                  <Link
                    href={l.href}
                    {...(l.external ? { target: "_blank", rel: "noreferrer" } : {})}
                    className="text-muted transition-colors hover:text-text"
                  >
                    {l.label}
                  </Link>
                </li>
              ))}
            </ul>
          </div>
        ))}
      </Container>

      <Container className="flex flex-col items-center justify-between gap-4 border-t border-line py-6 text-sm text-subtle sm:flex-row">
        <div className="flex items-center gap-2">
          <span className="size-2 rounded-full bg-positive" />
          Privacy-first · No cookies · No consent banner
        </div>
        <div className="flex items-center gap-4">
          <span>© {new Date().getFullYear()} Zenith</span>
          <Link
            href={GITHUB_URL}
            target="_blank"
            rel="noreferrer"
            aria-label="GitHub"
            className="transition-colors hover:text-text"
          >
            <GitHubIcon size={16} />
          </Link>
        </div>
      </Container>
    </footer>
  );
}
