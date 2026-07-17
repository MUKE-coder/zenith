"use client";

import { useEffect, useState } from "react";
import { Moon, Sun } from "lucide-react";

/** Toggles the site between dark (default) and light, persisted in localStorage. */
export function ThemeToggle() {
  const [light, setLight] = useState(false);
  const [mounted, setMounted] = useState(false);

  useEffect(() => {
    setMounted(true);
    setLight(document.documentElement.getAttribute("data-theme") === "light");
  }, []);

  function toggle() {
    const next = !light;
    setLight(next);
    const root = document.documentElement;
    if (next) {
      root.setAttribute("data-theme", "light");
      localStorage.setItem("zenith-theme", "light");
    } else {
      root.removeAttribute("data-theme");
      localStorage.setItem("zenith-theme", "dark");
    }
  }

  return (
    <button
      type="button"
      onClick={toggle}
      aria-label={light ? "Switch to dark theme" : "Switch to light theme"}
      className="grid size-9 cursor-pointer place-items-center rounded-lg border border-line text-muted transition-colors duration-200 hover:border-line-strong hover:text-text"
    >
      {/* Avoid a hydration mismatch: render nothing decisive until mounted. */}
      {mounted && (light ? <Moon size={16} /> : <Sun size={16} />)}
    </button>
  );
}
