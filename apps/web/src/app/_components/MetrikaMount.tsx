"use client";

import { useEffect, useRef } from "react";
import { usePathname } from "next/navigation";

declare global {
  interface Window {
    ym?: (...args: unknown[]) => void;
  }
}

// Service sections are not counted: owner/admin visits distort the statistics.
const EXCLUDED_PREFIXES = ["/admin", "/promo-admin", "/tester"];

function isExcludedPath(pathname: string) {
  return EXCLUDED_PREFIXES.some((p) => pathname === p || pathname.startsWith(p + "/"));
}

// Equivalent of the official tag.js loader: a call queue until the script loads.
function installMetrikaTag() {
  if (window.ym) return;
  const queue: unknown[][] = [];
  const ym = (...args: unknown[]) => {
    queue.push(args);
  };
  (ym as unknown as { a: unknown[][] }).a = queue;
  (ym as unknown as { l: number }).l = Date.now();
  window.ym = ym;
  const script = document.createElement("script");
  script.async = true;
  script.src = "https://mc.yandex.ru/metrika/tag.js";
  document.head.appendChild(script);
}

/**
 * Mounted once in RootLayout. The Yandex.Metrica counter number and
 * init parameters come from the admin panel (the "System" tab) via
 * /api/public/analytics; an empty number = analytics disabled.
 * App Router SPA navigations are sent as ym('hit', ...).
 */
export function MetrikaMount() {
  const pathname = usePathname();
  const counterIdRef = useRef<number | null>(null);
  const initStartedRef = useRef(false);

  useEffect(() => {
    if (!pathname || isExcludedPath(pathname)) return;

    // Lazy initialisation: if the first page is /admin, the counter is not loaded,
    // but on navigation to a public page it starts here.
    if (!initStartedRef.current) {
      initStartedRef.current = true;
      let cancelled = false;
      (async () => {
        try {
          const res = await fetch("/api/public/analytics");
          const json = await res.json().catch(() => null);
          const id = Number(json?.metrikaCounterId);
          if (cancelled || !Number.isFinite(id) || id <= 0) return;
          installMetrikaTag();
          const params =
            json.metrikaParams && typeof json.metrikaParams === "object"
              ? json.metrikaParams
              : {};
          window.ym?.(id, "init", params); // init sends the first page view itself
          counterIdRef.current = id;
        } catch {
          // Analytics must not break the site; skip silently.
        }
      })();
      return () => {
        cancelled = true;
      };
    }

    // Subsequent SPA navigations.
    if (counterIdRef.current && window.ym) {
      window.ym(counterIdRef.current, "hit", window.location.href);
    }
  }, [pathname]);

  return null;
}
