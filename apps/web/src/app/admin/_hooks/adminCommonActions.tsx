import type { SortState, Tab } from "../types";
import type { AdminPageContextValue, MetricValue } from "../components/AdminPageContext";
import React from "react";
import { ApiError } from "@/lib/api";

type AdminCommonActionsContext = Pick<AdminPageContextValue, "router" | "setError" | "setErrorDebug" | "setNotice" | "setExportSort" | "setStatsSort" | "exportPromocodes" | "exportFilters"> & { tabs: Array<{ id: Tab; label: string }> };

export function createAdminCommonActions(ctx: AdminCommonActionsContext) {
  const { router, setError, setErrorDebug, setNotice, setExportSort, setStatsSort, exportPromocodes, exportFilters, tabs } = ctx;
  function nextSort(current: SortState, key: string): SortState {
    if (!current || current.key !== key) return { key, direction: "desc" };
    if (current.direction === "desc") return { key, direction: "asc" };
    return null;
  }
  function sortValue(row: Record<string, unknown>, key: string): MetricValue {
    const value = key.split(".").reduce<unknown>((acc, part) => (acc && typeof acc === "object" ? (acc as Record<string, unknown>)[part] : undefined), row);
    if (typeof value === "number") return value;
    if (typeof value === "boolean") return value ? 1 : 0;
    if (typeof value === "string") {
      const ts = Date.parse(value);
      if (Number.isFinite(ts) && /\d{4}-\d{2}-\d{2}/.test(value)) return ts;
      return value.toLowerCase();
    }
    if (value === null || value === undefined) return "";
    return String(value);
  }
  function sortedRows<T extends Record<string, unknown>>(rows: T[], sort: SortState) {
    if (!sort) return rows;
    return [...rows].sort((a, b) => {
      const av = sortValue(a, sort.key);
      const bv = sortValue(b, sort.key);
      if (typeof av === "number" && typeof bv === "number") {
        if (av < bv) return sort.direction === "asc" ? -1 : 1;
        if (av > bv) return sort.direction === "asc" ? 1 : -1;
        return 0;
      }
      const as = String(av);
      const bs = String(bv);
      if (as < bs) return sort.direction === "asc" ? -1 : 1;
      if (as > bs) return sort.direction === "asc" ? 1 : -1;
      return 0;
    });
  }
  function sortButton(label: string, key: string, current: SortState, onClick: () => void) {
    const mark = current?.key === key ? (current.direction === "desc" ? " ↓" : " ↑") : "";
    return (
      <button type="button" className="ms-table-sort" onClick={onClick}>{label}{mark}</button>
    );
  }
  function toggleExportSort(scope: string, key: string) {
    setExportSort((value) => ({ ...value, [scope]: nextSort(value[scope] || null, key) }));
  }
  function toggleStatsSort(title: string, key: string) {
    setStatsSort((value) => ({ ...value, [title]: nextSort(value[title] || null, key) }));
  }
  function selectedPromoModeSet() {
    const selected = exportPromocodes.filter((promo) => exportFilters.promocodeIds.includes(promo.id));
    if (selected.length === 0) return null;
    const union = new Set<number>();
    for (const promo of selected) {
      for (const modeId of promo.modeIds || []) { union.add(modeId); }
    }
    return union;
  }

  function showNotice(message: string) {
    setError(""); setErrorDebug(null); setNotice(message);
  }
  function handleError(e: unknown, fallback: string) {
    const message = e instanceof Error ? e.message : fallback;
    if (message === "auth required") {
      router.replace("/login");
      return false;
    }
    if (message === "forbidden") {
      router.replace("/profile");
      return false;
    }
    setNotice("");
    setErrorDebug(e instanceof ApiError ? e.debug : null);
    setError(message);
    return true;
  }

  function adminTabsForRole(role?: string): Tab[] {
    switch (role) {
      case "owner":
      case "admin":
        return tabs.map((item) => item.id);
      case "support":
        return ["profile", "system", "stats", "users", "access"];
      case "content_admin":
        return ["profile", "modes", "exports", "orchestration", "content", "blog"];
      case "billing_admin":
        return ["profile", "system", "stats", "tariffs", "payments"];
      default:
        return [];
    }
  }

  function canUseAdminPage(role?: string) {
    return adminTabsForRole(role).length > 0;
  }


  return { nextSort, sortValue, sortedRows, sortButton, toggleExportSort, toggleStatsSort, selectedPromoModeSet, showNotice, handleError, adminTabsForRole, canUseAdminPage };
}
