// SOLID-split of admin/page.tsx 2026-05-31: types moved out.

export type Tab =
  | "profile"
  | "system"
  | "stats"
  | "users"
  | "access"
  | "modes"
  | "tariffs"
  | "promocodes"
  | "exports"
  | "orchestration"
  | "broadcast"
  | "payments"
  | "notifications"
  | "content"
  | "blog";

export type UserRow = {
  id: number;
  email: string;
  role: string;
  status: string;
  telegramUsername: string;
  createdAt?: string;
  lastLoginAt?: string | null;
  messageCount?: number;
};

export type UserAccessMode = {
  id: number;
  modeId: number;
  modeName: string;
  activeFrom?: string | null;
  activeTo?: string | null;
  status: string;
  dailyMessageLimit?: number | null;
  priority: number;
  accessType: string;
  sourceId?: number | null;
  sourceLabel: string;
  quota?: { limit?: number; used: number; remaining?: number };
};

export type UserDetail = UserRow & {
  activeModes?: UserAccessMode[];
  stats?: { messages?: number; dialogs?: number };
};

export type ModeRow = {
  id: number;
  name: string;
  hidden: boolean;
  paid?: boolean;
  paidChains?: string;
  aiModel: string;
  modelTemperature?: number;
};

export type ModeDetail = ModeRow & {
  prompt: string;
  welcomeMessage?: string;
  demoChat: string;
  audioEnabled?: boolean;
  criteria?: string;
  orchestratorCheckInterval?: number;
  reminderCount?: number | null;
  aiProvider?: "vsegpt" | "gemini" | "anthropic";
  thinkingMode?: "default" | "off" | "low" | "high";
  // Response token ceiling. 0 = global default (from the AI settings).
  aiMaxTokens?: number;
  // Lead notifications: trigger "user replied N times" -> message to Max.
  leadNotifyEnabled?: boolean;
  leadNotifyChatIds?: string;
  leadNotifyTelegramIds?: string;
  leadNotifyThreshold?: number;
};

export type Tariff = {
  id: number;
  name: string;
  description?: string;
  tariffType: string;
  monthlyPrice: number;
  dailyMessageLimit?: number | null;
  limitType?: string;
  groupId?: number | null;
  groupName?: string;
  availableForSubscription: boolean;
  createdAt?: string;
  archivedAt?: string | null;
  modeIds?: number[];
  firstModeId?: number | null;
};

export type Promo = {
  id: number;
  code: string;
  grantsType: string;
  targetId: number;
  targetIds?: number[];
  modeIds?: number[];
  firstModeId?: number | null;
  maxUses: number;
  usedCount: number;
  activeFrom?: string | null;
  activeTo?: string | null;
  active?: boolean;
  applyUrl: string;
  temporaryAdminUrl?: string;
  comment?: string;
  purpose?: string;
  summaryLimit?: number | null;
  lastUsedAt?: string | null;
};

export type CreatedPromo = {
  id: number;
  code: string;
  targetId?: number;
  targetIds?: number[];
  applyUrl?: string;
  temporaryAdminUrl?: string;
};

export type SummaryPrompt = {
  id: number;
  name: string;
  prompt: string;
  isDefault: boolean;
};

export type PageMeta = { total: number; limit: number; offset: number };

export type ModelStat = {
  model: string;
  total: number;
  visible: number;
  hidden: number;
};

export type SortState = { key: string; direction: "asc" | "desc" } | null;
