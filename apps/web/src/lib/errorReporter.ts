// Minimal error tracker: window.onerror + window.onunhandledrejection
// -> POST /api/_error. The API increments the prometheus counter frontend_errors_total.
// Uptime-Kuma polls /metrics and sends an alert when the counter grows.
//
// + breadcrumbs (last 30 events: clicks, route changes, fetch, console.error)
// are sent together with the error: shows what the user did before the crash.

import { getBreadcrumbs, installBreadcrumbs } from "./breadcrumbs";

const REPORT_URL = "/api/_error";
const RATE_LIMIT_MS = 5000; // no more than 1 report per 5 s for the same type
const recentReports = new Map<string, number>();
let installed = false;
let errorHandler: ((event: ErrorEvent) => void) | null = null;
let rejectionHandler: ((event: PromiseRejectionEvent) => void) | null = null;

function shouldReport(key: string): boolean {
  const now = Date.now();
  const last = recentReports.get(key);
  if (last && now - last < RATE_LIMIT_MS) {
    return false;
  }
  recentReports.set(key, now);
  // Clean up old entries so the Map does not grow forever.
  if (recentReports.size > 100) {
    const cutoff = now - RATE_LIMIT_MS * 10;
    for (const [k, t] of recentReports) {
      if (t < cutoff) recentReports.delete(k);
    }
  }
  return true;
}

function send(payload: { type: string; message: string; page: string; stack: string; breadcrumbs?: unknown[] }) {
  // sendBeacon delivers even when the user closes the tab; this is critical
  // for the "error in beforeunload" case.
  try {
    const blob = new Blob([JSON.stringify(payload)], { type: "application/json" });
    if (navigator.sendBeacon && navigator.sendBeacon(REPORT_URL, blob)) {
      return;
    }
  } catch {
    /* fallthrough to fetch */
  }
  fetch(REPORT_URL, {
    method: "POST",
    headers: { "Content-Type": "application/json" },
    body: JSON.stringify(payload),
    keepalive: true,
  }).catch(() => {
    /* our own error reporter must not fail itself */
  });
}

export function installErrorReporter() {
  if (typeof window === "undefined") return; // SSR safe
  if (installed) return;
  installed = true;

  installBreadcrumbs();

  errorHandler = (event: ErrorEvent) => {
    const message = String(event.message || "Unknown error").slice(0, 500);
    const key = `error:${message}`;
    if (!shouldReport(key)) return;
    send({
      type: "error",
      message,
      page: window.location.pathname,
      stack: String(event.error?.stack || "").slice(0, 400),
      breadcrumbs: getBreadcrumbs(),
    });
  };

  rejectionHandler = (event: PromiseRejectionEvent) => {
    const reason = event.reason;
    const message = String(
      reason instanceof Error ? reason.message : reason,
    ).slice(0, 500);
    const key = `rejection:${message}`;
    if (!shouldReport(key)) return;
    send({
      type: "unhandledrejection",
      message,
      page: window.location.pathname,
      stack: String(reason?.stack || "").slice(0, 400),
      breadcrumbs: getBreadcrumbs(),
    });
  };

  window.addEventListener("error", errorHandler);
  window.addEventListener("unhandledrejection", rejectionHandler);
}

export function resetErrorReporterForTests() {
  if (typeof window !== "undefined") {
    if (errorHandler) window.removeEventListener("error", errorHandler);
    if (rejectionHandler) window.removeEventListener("unhandledrejection", rejectionHandler);
  }
  errorHandler = null;
  rejectionHandler = null;
  installed = false;
  recentReports.clear();
}
