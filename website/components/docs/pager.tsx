"use client";

import Link from "next/link";
import { usePathname } from "next/navigation";
import { ArrowLeft, ArrowRight } from "lucide-react";
import { DOC_ORDER, docHref } from "@/lib/docs";

/** Prev / next links, derived from the doc order and the current path. */
export function DocPager() {
  const pathname = usePathname();
  const slug = pathname === "/docs" ? "" : pathname.replace(/^\/docs\//, "");
  const i = DOC_ORDER.findIndex((l) => l.slug === slug);
  if (i === -1) return null;

  const prev = DOC_ORDER[i - 1];
  const next = DOC_ORDER[i + 1];

  return (
    <div className="mt-16 grid gap-4 border-t border-line pt-8 sm:grid-cols-2">
      {prev ? (
        <Link
          href={docHref(prev.slug)}
          className="group rounded-xl border border-line p-4 no-underline transition-colors hover:border-line-strong"
        >
          <span className="flex items-center gap-1.5 text-xs text-subtle">
            <ArrowLeft size={13} /> Previous
          </span>
          <span className="mt-1 block font-medium text-text group-hover:text-accent-bright">
            {prev.title}
          </span>
        </Link>
      ) : (
        <span />
      )}
      {next ? (
        <Link
          href={docHref(next.slug)}
          className="group rounded-xl border border-line p-4 text-right no-underline transition-colors hover:border-line-strong"
        >
          <span className="flex items-center justify-end gap-1.5 text-xs text-subtle">
            Next <ArrowRight size={13} />
          </span>
          <span className="mt-1 block font-medium text-text group-hover:text-accent-bright">
            {next.title}
          </span>
        </Link>
      ) : (
        <span />
      )}
    </div>
  );
}
