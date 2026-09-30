"use client";

import { useEffect } from "react";
import { installErrorReporter } from "@/lib/errorReporter";

/**
 * Mounted once in RootLayout; installs the global
 * window.onerror and window.onunhandledrejection listeners.
 * SSR-safe (installErrorReporter checks window itself).
 */
export function ErrorReporterMount() {
  useEffect(() => {
    installErrorReporter();
  }, []);
  return null;
}
