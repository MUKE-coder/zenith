"use client";

import { useEffect, useState } from "react";

type Heading = { id: string; text: string; level: number };

/**
 * The "On this page" list.
 *
 * It reads the rendered article's h2/h3 headings from the DOM rather than
 * being hand-maintained per page, and highlights the section currently in
 * view with an IntersectionObserver.
 */
export function Toc() {
  const [headings, setHeadings] = useState<Heading[]>([]);
  const [active, setActive] = useState<string>("");

  useEffect(() => {
    const nodes = Array.from(
      document.querySelectorAll<HTMLElement>(".prose h2, .prose h3"),
    ).filter((n) => n.id);

    setHeadings(
      nodes.map((n) => ({
        id: n.id,
        text: n.textContent ?? "",
        level: n.tagName === "H3" ? 3 : 2,
      })),
    );

    const observer = new IntersectionObserver(
      (entries) => {
        for (const entry of entries) {
          if (entry.isIntersecting) setActive(entry.target.id);
        }
      },
      { rootMargin: "-80px 0px -70% 0px", threshold: 0 },
    );

    nodes.forEach((n) => observer.observe(n));
    return () => observer.disconnect();
  }, []);

  if (headings.length === 0) return null;

  return (
    <div className="text-sm">
      <p className="mb-3 text-xs font-medium uppercase tracking-[0.1em] text-subtle">
        On this page
      </p>
      <ul className="space-y-2 border-l border-line">
        {headings.map((h) => (
          <li key={h.id} style={{ paddingLeft: h.level === 3 ? 24 : 12 }}>
            <a
              href={`#${h.id}`}
              className={`-ml-px block border-l-2 pl-3 transition-colors ${
                active === h.id
                  ? "border-accent text-accent-bright"
                  : "border-transparent text-muted hover:text-text"
              }`}
            >
              {h.text}
            </a>
          </li>
        ))}
      </ul>
    </div>
  );
}
