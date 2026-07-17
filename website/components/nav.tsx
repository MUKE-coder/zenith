import Link from "next/link";
import { GitHubIcon } from "./icons";
import { Logo } from "./logo";
import { ThemeToggle } from "./theme-toggle";
import { Button, Container } from "./ui";
import { GITHUB_URL, NPM_URL } from "@/lib/site";

export function Nav() {
  return (
    <header className="sticky top-0 z-50 border-b border-line bg-bg/80 backdrop-blur-xl">
      <Container className="flex h-16 items-center justify-between gap-4">
        <div className="flex items-center gap-8">
          <Logo />
          <nav className="hidden items-center gap-6 text-sm text-muted md:flex">
            <Link href="/docs" className="transition-colors hover:text-text">
              Docs
            </Link>
            <Link href="/#features" className="transition-colors hover:text-text">
              Features
            </Link>
            <Link href="/#how" className="transition-colors hover:text-text">
              How it works
            </Link>
          </nav>
        </div>

        <div className="flex items-center gap-2">
          <Link
            href={NPM_URL}
            target="_blank"
            rel="noreferrer"
            className="hidden rounded-lg px-2 py-1 font-mono text-xs text-subtle transition-colors hover:text-text sm:block"
          >
            npm i zenith-analytics
          </Link>
          <Link
            href={GITHUB_URL}
            target="_blank"
            rel="noreferrer"
            aria-label="Zenith on GitHub"
            className="grid size-9 place-items-center rounded-lg border border-line text-muted transition-colors hover:border-line-strong hover:text-text"
          >
            <GitHubIcon size={16} />
          </Link>
          <ThemeToggle />
          <Button href="/docs" className="ml-1 hidden sm:inline-flex">
            Get started
          </Button>
        </div>
      </Container>
    </header>
  );
}
