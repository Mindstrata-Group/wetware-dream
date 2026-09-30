import { useCallback, useEffect, useState } from "react";
import { apiFetch } from "@/lib/api";
import type { HistoryItem, ModeOption, PromoStatus, PromptOption } from "../_types";

type StatusResponse = {
  promo: PromoStatus;
  prompts?: PromptOption[];
  modes?: ModeOption[];
  history?: HistoryItem[];
};

// usePromoStatusLoader: state and loader for /api/promo-admin/status.
//
// - The initial load does the first fetch and initialises promptId (default).
// - Silent auto-refresh every 60 seconds (no spinner) while the tab is active.
//   With document.hidden we skip the tick: do not call the API when the user has left.
// - On a silent error, stay quiet (network loss, retry in a minute).
//   On an initial load error, show the user a clear text.
export function usePromoStatusLoader(
  promo: string,
  promoKey: string,
  setError: (msg: string) => void,
) {
  const [status, setStatus] = useState<PromoStatus | null>(null);
  const [prompts, setPrompts] = useState<PromptOption[]>([]);
  const [modes, setModes] = useState<ModeOption[]>([]);
  const [history, setHistory] = useState<HistoryItem[]>([]);
  const [promptId, setPromptId] = useState<number | null>(null);
  const [loadingStatus, setLoadingStatus] = useState(true);

  const loadStatus = useCallback(
    async (silent = false) => {
      if (!promo || !promoKey) return;
      if (!silent) setLoadingStatus(true);
      try {
        const json = await apiFetch<StatusResponse>(
          `/api/promo-admin/status?promo=${encodeURIComponent(promo)}&key=${encodeURIComponent(promoKey)}`,
        );
        setStatus(json.promo);
        setPrompts(json.prompts || []);
        setModes(json.modes || []);
        setHistory((json.history || []).slice(0, 20));
        if (!silent) {
          const def =
            (json.prompts || []).find((p) => p.isDefault) || (json.prompts || [])[0];
          if (def) setPromptId(def.id);
        }
      } catch {
        if (!silent) {
          setError("Промокод не активна или не найдена. Проверьте ссылку.");
        }
      } finally {
        if (!silent) setLoadingStatus(false);
      }
    },
    [promo, promoKey, setError],
  );

  useEffect(() => {
    if (!promo) {
      setError("В URL отсутствует параметр promo. Откройте панель по ссылке из QR-кода.");
      setLoadingStatus(false);
      return;
    }
    if (!promoKey) {
      setError("В URL отсутствует параметр key. Откройте панель по подписанной ссылке.");
      setLoadingStatus(false);
      return;
    }
    setError("");
    loadStatus();
  }, [promo, promoKey, loadStatus, setError]);

  // Auto-refresh the statistics once a minute while the tab is active.
  useEffect(() => {
    if (!promo || !promoKey) return;
    const id = window.setInterval(() => {
      if (typeof document !== "undefined" && document.hidden) return;
      loadStatus(true);
    }, 60_000);
    return () => window.clearInterval(id);
  }, [promo, promoKey, loadStatus]);

  return {
    status,
    setStatus,
    prompts,
    modes,
    history,
    setHistory,
    promptId,
    setPromptId,
    loadingStatus,
  };
}
