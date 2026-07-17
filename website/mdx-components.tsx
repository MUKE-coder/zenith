import type { MDXComponents } from "mdx/types";
import Link from "next/link";
import type { ReactNode } from "react";
import { AlertTriangle, Info, Lightbulb } from "lucide-react";

/**
 * Required by @next/mdx in the App Router. Maps MDX elements to components and
 * exposes reusable doc building blocks (Callout, CardGrid, Card, Steps) that
 * pages can use directly in .mdx.
 */
export function useMDXComponents(components: MDXComponents): MDXComponents {
  return {
    // Internal links use next/link; external open in a new tab.
    a: ({ href = "", children }) => {
      const external = /^https?:\/\//.test(href);
      return external ? (
        <a href={href} target="_blank" rel="noreferrer">
          {children}
        </a>
      ) : (
        <Link href={href}>{children}</Link>
      );
    },
    Callout,
    CardGrid,
    Card,
    Steps,
    Step,
    ...components,
  };
}

type CalloutType = "note" | "tip" | "warning";

const CALLOUT: Record<CalloutType, { icon: typeof Info; color: string }> = {
  note: { icon: Info, color: "var(--accent-bright)" },
  tip: { icon: Lightbulb, color: "var(--positive)" },
  warning: { icon: AlertTriangle, color: "var(--warning)" },
};

export function Callout({
  type = "note",
  children,
}: {
  type?: CalloutType;
  children: ReactNode;
}) {
  const { icon: Icon, color } = CALLOUT[type];
  return (
    <div
      className="my-6 flex gap-3 rounded-xl border border-line bg-surface p-4 text-sm"
      style={{ borderLeft: `2px solid ${color}` }}
    >
      <Icon size={17} style={{ color, flexShrink: 0, marginTop: 2 }} />
      <div className="[&>*]:my-0 [&>*+*]:mt-2">{children}</div>
    </div>
  );
}

export function CardGrid({ children }: { children: ReactNode }) {
  return <div className="my-6 grid gap-4 sm:grid-cols-2">{children}</div>;
}

export function Card({
  title,
  href,
  children,
}: {
  title: string;
  href?: string;
  children: ReactNode;
}) {
  const inner = (
    <>
      <p className="font-semibold text-text">{title}</p>
      <p className="mt-1.5 text-sm text-muted">{children}</p>
    </>
  );
  const cls =
    "block rounded-xl border border-line bg-surface p-5 no-underline transition-colors hover:border-line-strong hover:bg-surface-2";
  return href ? (
    <Link href={href} className={cls}>
      {inner}
    </Link>
  ) : (
    <div className={cls}>{inner}</div>
  );
}

export function Steps({ children }: { children: ReactNode }) {
  return (
    <div className="my-6 [counter-reset:step] [&>*]:relative">
      <div className="space-y-6 border-l border-line pl-8">{children}</div>
    </div>
  );
}

export function Step({ title, children }: { title: string; children: ReactNode }) {
  return (
    <div className="relative [counter-increment:step]">
      <span className="absolute -left-[41px] grid size-6 place-items-center rounded-full border border-line bg-surface font-mono text-xs text-accent-bright before:content-[counter(step)]" />
      <p className="mt-0.5 font-semibold text-text">{title}</p>
      <div className="mt-1.5 text-sm text-muted [&>*]:my-0 [&>*+*]:mt-3">{children}</div>
    </div>
  );
}
