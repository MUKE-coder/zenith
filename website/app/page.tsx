import Link from "next/link";
import {
  ArrowRight,
  Globe,
  LayoutGrid,
  Mail,
  SearchCheck,
  Server,
  ShieldCheck,
  Sparkles,
} from "lucide-react";
import { Nav } from "@/components/nav";
import { Footer } from "@/components/footer";
import { DashboardMock } from "@/components/dashboard-mock";
import { Button, Container, Eyebrow, Pill } from "@/components/ui";
import { FEATURES, STATS, TECH, GITHUB_URL } from "@/lib/site";

const ICONS: Record<string, typeof Globe> = {
  "shield-check": ShieldCheck,
  "layout-grid": LayoutGrid,
  globe: Globe,
  "search-check": SearchCheck,
  mail: Mail,
  server: Server,
};

export default function Home() {
  return (
    <>
      <Nav />
      <main>
        <Hero />
        <TechStrip />
        <Features />
        <HowItWorks />
        <Stats />
        <CtaBand />
      </main>
      <Footer />
    </>
  );
}

function Hero() {
  return (
    <section className="relative overflow-hidden border-b border-line">
      <div className="pointer-events-none absolute inset-0 grid-bg grid-fade opacity-70" />
      <div className="pointer-events-none absolute inset-x-0 top-0 h-[520px] accent-glow" />

      <Container className="relative grid items-center gap-12 py-20 lg:grid-cols-2 lg:py-28">
        <div>
          <Pill>
            <Sparkles size={13} className="text-accent-bright" />
            Cookieless · No consent banner
          </Pill>

          <h1 className="mt-6 text-balance text-4xl font-semibold leading-[1.05] tracking-tight sm:text-5xl lg:text-[3.4rem]">
            <span className="text-accent-bright">Privacy-first analytics</span>
            <br />
            for every site you build.
          </h1>

          <p className="mt-5 max-w-lg text-pretty text-base leading-relaxed text-muted sm:text-lg">
            Self-host Zenith once. Track unlimited client sites with a tiny cookieless script, run
            on-demand SEO audits, and give each client a dashboard that lives on{" "}
            <span className="text-text">their own domain</span>.
          </p>

          <div className="mt-8 flex flex-wrap items-center gap-3">
            <Button href="/docs" className="px-5 py-3 text-[15px]">
              Get started
              <ArrowRight size={16} />
            </Button>
            <Button
              href={GITHUB_URL}
              variant="secondary"
              external
              className="px-5 py-3 text-[15px]"
            >
              Star on GitHub
            </Button>
          </div>

          <p className="mt-5 font-mono text-xs text-subtle">$ npm install zenith-analytics</p>
        </div>

        <div className="relative lg:pl-6">
          <DashboardMock />
        </div>
      </Container>
    </section>
  );
}

function TechStrip() {
  return (
    <section className="border-b border-line">
      <Container className="flex flex-col items-center gap-5 py-10 sm:flex-row sm:justify-between">
        <p className="text-sm text-subtle">Built with a boring, dependable stack</p>
        <div className="flex flex-wrap items-center justify-center gap-2.5">
          {TECH.map((t) => (
            <span
              key={t}
              className="rounded-full border border-line px-3.5 py-1.5 font-mono text-sm text-muted"
            >
              {t}
            </span>
          ))}
        </div>
      </Container>
    </section>
  );
}

function Features() {
  return (
    <section id="features" className="border-b border-line py-20 lg:py-28">
      <Container>
        <div className="max-w-2xl">
          <Eyebrow>Everything you need</Eyebrow>
          <h2 className="mt-3 text-3xl font-semibold tracking-tight sm:text-4xl">
            One deploy. Every site. Zero cookies.
          </h2>
          <p className="mt-4 text-pretty text-muted">
            The four things the product exists to do — analytics, domain-native dashboards, SEO
            auditing, and privacy — with nothing you have to bolt on.
          </p>
        </div>

        <div className="mt-12 grid gap-px overflow-hidden rounded-2xl border border-line bg-line sm:grid-cols-2 lg:grid-cols-3">
          {FEATURES.map((f) => {
            const Icon = ICONS[f.icon] ?? Globe;
            return (
              <div
                key={f.title}
                className="group bg-bg p-6 transition-colors duration-200 hover:bg-surface"
              >
                <div className="flex size-10 items-center justify-center rounded-lg border border-line bg-surface text-accent-bright transition-colors group-hover:border-line-strong">
                  <Icon size={18} />
                </div>
                <h3 className="mt-4 font-semibold">{f.title}</h3>
                <p className="mt-2 text-sm leading-relaxed text-muted">{f.description}</p>
              </div>
            );
          })}
        </div>
      </Container>
    </section>
  );
}

function HowItWorks() {
  const steps = [
    {
      n: "01",
      title: "Deploy once",
      body: "One docker compose up brings up the core service and its data volume. Add a site in the console and get its keys.",
    },
    {
      n: "02",
      title: "Install the package",
      body: "npm install zenith-analytics in your client's Next.js app, then npx zenith init scaffolds the tracker and the dashboard route.",
    },
    {
      n: "03",
      title: "Hand off the dashboard",
      body: "Your client visits theirsite.com/analytics-dashboard, enters a password, and reads their own analytics — on their own domain.",
    },
  ];

  return (
    <section id="how" className="border-b border-line py-20 lg:py-28">
      <Container>
        <div className="max-w-2xl">
          <Eyebrow>How it works</Eyebrow>
          <h2 className="mt-3 text-3xl font-semibold tracking-tight sm:text-4xl">
            From zero to live analytics in three steps.
          </h2>
        </div>

        <div className="mt-12 grid gap-6 md:grid-cols-3">
          {steps.map((s) => (
            <div key={s.n} className="rounded-2xl border border-line bg-surface p-6">
              <p className="font-mono text-sm text-accent-bright">{s.n}</p>
              <h3 className="mt-3 text-lg font-semibold">{s.title}</h3>
              <p className="mt-2 text-sm leading-relaxed text-muted">{s.body}</p>
            </div>
          ))}
        </div>

        <div className="mt-8">
          <Button href="/docs/quickstart" variant="ghost">
            Read the quickstart
            <ArrowRight size={16} />
          </Button>
        </div>
      </Container>
    </section>
  );
}

function Stats() {
  return (
    <section className="relative overflow-hidden border-b border-line py-16">
      <div className="pointer-events-none absolute inset-0 grid-bg opacity-40" />
      <Container className="relative grid grid-cols-2 gap-px overflow-hidden rounded-2xl border border-line bg-line lg:grid-cols-4">
        {STATS.map((s) => (
          <div key={s.label} className="bg-bg px-6 py-10 text-center">
            <p className="font-mono text-4xl font-semibold tracking-tight text-text sm:text-5xl">
              {s.value}
            </p>
            <p className="mt-2 text-sm text-muted">
              {s.unit ? <span className="text-subtle">{s.unit} · </span> : null}
              {s.label}
            </p>
          </div>
        ))}
      </Container>
    </section>
  );
}

function CtaBand() {
  return (
    <section className="relative overflow-hidden py-24">
      <div className="pointer-events-none absolute inset-x-0 top-0 h-full accent-glow opacity-60" />
      <Container className="relative text-center">
        <h2 className="mx-auto max-w-2xl text-balance text-3xl font-semibold tracking-tight sm:text-4xl">
          Own your clients&rsquo; analytics.
        </h2>
        <p className="mx-auto mt-4 max-w-xl text-pretty text-muted">
          No third-party subdomain, no cookie banner, no per-site setup. Deploy once and every site
          you build reports into it.
        </p>
        <div className="mt-8 flex flex-wrap items-center justify-center gap-3">
          <Button href="/docs" className="px-5 py-3 text-[15px]">
            Get started
            <ArrowRight size={16} />
          </Button>
          <Link
            href="/docs/nextjs"
            className="rounded-lg px-5 py-3 text-[15px] font-medium text-muted transition-colors hover:text-text"
          >
            Next.js setup →
          </Link>
        </div>
      </Container>
    </section>
  );
}
