"use client";

import Link from "next/link";
import { usePathname } from "next/navigation";
import { DOC_NAV, docHref } from "@/lib/docs";

export function DocsSidebar() {
  const pathname = usePathname();

  return (
    <nav aria-label="Documentation" className="space-y-7 text-sm">
      {DOC_NAV.map((section) => (
        <div key={section.title}>
          <p className="mb-2 px-3 text-xs font-medium uppercase tracking-[0.1em] text-subtle">
            {section.title}
          </p>
          <ul className="space-y-0.5">
            {section.links.map((link) => {
              const href = docHref(link.slug);
              const active = pathname === href;
              return (
                <li key={link.slug}>
                  <Link
                    href={href}
                    aria-current={active ? "page" : undefined}
                    className={`block rounded-lg px-3 py-1.5 transition-colors ${
                      active
                        ? "bg-surface-2 font-medium text-accent-bright"
                        : "text-muted hover:bg-surface hover:text-text"
                    }`}
                  >
                    {link.title}
                  </Link>
                </li>
              );
            })}
          </ul>
        </div>
      ))}
    </nav>
  );
}
