import Link from "next/link";
import type { ReactNode } from "react";

/** Max-width page container. */
export function Container({
  children,
  className = "",
}: {
  children: ReactNode;
  className?: string;
}) {
  return <div className={`mx-auto w-full max-w-6xl px-5 sm:px-6 ${className}`}>{children}</div>;
}

type ButtonProps = {
  href: string;
  children: ReactNode;
  variant?: "primary" | "secondary" | "ghost";
  external?: boolean;
  className?: string;
};

/** A link styled as a button. One primary per view. */
export function Button({
  href,
  children,
  variant = "primary",
  external,
  className = "",
}: ButtonProps) {
  const base =
    "inline-flex items-center justify-center gap-2 rounded-lg px-4 py-2.5 text-sm font-medium transition-colors duration-200 cursor-pointer whitespace-nowrap";
  const styles = {
    primary: "bg-accent text-white hover:bg-accent-hover",
    secondary: "border border-line-strong bg-surface text-text hover:bg-surface-2",
    ghost: "text-muted hover:text-text",
  }[variant];

  const props = external ? { target: "_blank", rel: "noreferrer" } : {};
  return (
    <Link href={href} className={`${base} ${styles} ${className}`} {...props}>
      {children}
    </Link>
  );
}

/** A small pill, e.g. the hero eyebrow badge. */
export function Pill({ children }: { children: ReactNode }) {
  return (
    <span className="inline-flex items-center gap-2 rounded-full border border-line bg-surface/70 px-3 py-1 text-xs font-medium text-muted backdrop-blur">
      {children}
    </span>
  );
}

/** The small uppercase eyebrow label. */
export function Eyebrow({ children }: { children: ReactNode }) {
  return (
    <p className="text-xs font-medium uppercase tracking-[0.14em] text-subtle">{children}</p>
  );
}
