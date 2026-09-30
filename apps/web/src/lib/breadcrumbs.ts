// Breadcrumbs: a ring buffer of the user's last N actions for incident
// investigation. Sent together with the window.onerror report -> shows what the user did
// before the crash. A free alternative to Sentry Session Replay (~85 KB gzip),
// weighs ~1 KB.
//
// What we track:
//   - clicks (CSS selector of the target, button text)
//   - route changes (history.pushState/replaceState/popstate)
//   - fetch calls (URL + status + response time)
//   - console.error (last 5)
//
// Limit: 30 events, old ones are evicted. This covers ~30 seconds of activity.

export type Breadcrumb = {
  type: "click" | "route" | "fetch" | "console";
  at: number; // ms epoch
  data: Record<string, string | number>;
};

const MAX_BREADCRUMBS = 30;
const breadcrumbs: Breadcrumb[] = [];

function push(crumb: Breadcrumb): void {
  breadcrumbs.push(crumb);
  if (breadcrumbs.length > MAX_BREADCRUMBS) {
    breadcrumbs.shift();
  }
}

export function getBreadcrumbs(): Breadcrumb[] {
  return breadcrumbs.slice();
}

export function clearBreadcrumbs(): void {
  breadcrumbs.length = 0;
}

// CSS selector up to 3 levels for the click target. No deeper: a long selector
// is useless for debugging.
function shortSelector(el: Element | null): string {
  if (!el) return "?";
  const parts: string[] = [];
  let cur: Element | null = el;
  for (let i = 0; i < 3 && cur; i++) {
    let part = cur.tagName.toLowerCase();
    if (cur.id) {
      part += `#${cur.id}`;
      parts.unshift(part);
      break;
    }
    const cls = (cur.className && typeof cur.className === "string")
      ? cur.className.split(/\s+/).filter(Boolean).slice(0, 2).join(".")
      : "";
    if (cls) part += `.${cls}`;
    parts.unshift(part);
    cur = cur.parentElement;
  }
  return parts.join(">");
}

function clickText(el: Element): string {
  const text = (el.textContent || "").trim().replace(/\s+/g, " ").slice(0, 40);
  return text;
}

function installClickTracker(): void {
  document.addEventListener("click", (e: MouseEvent) => {
    const target = e.target as Element | null;
    if (!target) return;
    // Walk up to an interactive element: button, a, [role=button], input.
    let interactive: Element | null = target;
    for (let i = 0; i < 5 && interactive; i++) {
      const tag = interactive.tagName.toLowerCase();
      if (tag === "button" || tag === "a" || tag === "input" || tag === "select" ||
          interactive.getAttribute("role") === "button" ||
          interactive.getAttribute("role") === "checkbox") {
        break;
      }
      interactive = interactive.parentElement;
    }
    if (!interactive) interactive = target;
    push({
      type: "click",
      at: Date.now(),
      data: {
        selector: shortSelector(interactive),
        text: clickText(interactive),
      },
    });
  }, { capture: true, passive: true });
}

function installRouteTracker(): void {
  // history.pushState and replaceState are custom methods and do not emit an event.
  // We wrap them.
  const origPush = history.pushState;
  const origReplace = history.replaceState;
  history.pushState = function (...args: Parameters<typeof history.pushState>) {
    const result = origPush.apply(this, args);
    push({ type: "route", at: Date.now(), data: { to: window.location.pathname + window.location.search } });
    return result;
  };
  history.replaceState = function (...args: Parameters<typeof history.replaceState>) {
    const result = origReplace.apply(this, args);
    push({ type: "route", at: Date.now(), data: { to: window.location.pathname + window.location.search, replace: 1 } });
    return result;
  };
  window.addEventListener("popstate", () => {
    push({ type: "route", at: Date.now(), data: { to: window.location.pathname + window.location.search, back: 1 } });
  });
}

function installFetchTracker(): void {
  const origFetch = window.fetch;
  window.fetch = async function (input: RequestInfo | URL, init?: RequestInit): Promise<Response> {
    const url = typeof input === "string" ? input : input instanceof URL ? input.toString() : input.url;
    const method = init?.method || (typeof input === "object" && "method" in input ? (input as Request).method : "GET");
    const start = Date.now();
    // Do not track /api/_error itself, otherwise an infinite loop on a send error.
    const skip = url.includes("/api/_error");
    try {
      const res = await origFetch.call(this, input, init);
      if (!skip) {
        push({
          type: "fetch",
          at: start,
          data: {
            method,
            url: url.length > 80 ? url.slice(0, 80) + "…" : url,
            status: res.status,
            ms: Date.now() - start,
          },
        });
      }
      return res;
    } catch (err) {
      if (!skip) {
        push({
          type: "fetch",
          at: start,
          data: {
            method,
            url: url.length > 80 ? url.slice(0, 80) + "…" : url,
            status: 0,
            ms: Date.now() - start,
            error: String(err).slice(0, 80),
          },
        });
      }
      throw err;
    }
  };
}

function installConsoleTracker(): void {
  const origError = console.error;
  console.error = function (...args: unknown[]) {
    const msg = args.map(a => String(a)).join(" ").slice(0, 200);
    push({ type: "console", at: Date.now(), data: { level: "error", msg } });
    return origError.apply(this, args);
  };
}

export function installBreadcrumbs(): void {
  if (typeof window === "undefined") return;
  installClickTracker();
  installRouteTracker();
  installFetchTracker();
  installConsoleTracker();
}
