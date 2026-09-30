export type PromoStatus = {
  id: number;
  code: string;
  activeFrom?: string | null;
  activeTo?: string | null;
  maxUses?: number;
  usedCount?: number;
  summaryLimit?: number | null;
  summaryUsed?: number;
  summaryRemaining?: number | null;
  messageCount?: number;
  activationsWithoutMessages?: number;
};
export type PromptOption = { id: number; name: string; isDefault?: boolean };
export type ModeOption = { id: number; name: string };
export type StatusResponse = {
  ok: boolean;
  promo: PromoStatus;
  prompts: PromptOption[];
  modes: ModeOption[];
  history?: HistoryItem[];
};
export type SummarizeResponse = {
  ok: boolean;
  summaryId: number;
  result: string;
  messageCount: number;
  sourceBytes: number;
  approxTokens: number;
  used: number;
  remaining: number | null;
  createdAt: string;
};
export type HistoryItem = SummarizeResponse & { modeLabel: string };
