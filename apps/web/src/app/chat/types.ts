// SOLID-split of chat/page.tsx 2026-05-31: types moved out separately.

export type DailyQuota = { limit?: number; used: number; remaining?: number };

export type Mode = {
  id: number;
  name: string;
  welcomeMessage?: string;
  quota?: DailyQuota;
};

export type Message = {
  id: number;
  role: string;
  content: string;
  createdAt: string;
  modeId?: number;
  modeName?: string;
};

export type ChatAttachment = {
  id: number;
  fileName: string;
  extension: string;
  sizeBytes: number;
  sha256: string;
  annotationStatus: string;
  duplicate?: boolean;
};

export type StartPayload = {
  ok: boolean;
  userId: number;
  role?: string;
  email?: string;
  currentModeId?: number;
  currentDialogId?: number;
  chatMessageMaxChars?: number;
  modes: Mode[];
  quota?: DailyQuota;
  messages?: Message[];
  error?: string;
  maxLinked?: boolean;
  maxBotLink?: string;
};

export type ChatHistoryItem = {
  dialogId: number;
  modeId: number | null;
  modeName: string;
  title: string;
  snippet: string;
  searchText: string;
  pinned: boolean;
  updatedAt: string;
};

export type HistoryPayload = {
  ok: boolean;
  dialogId?: number;
  modeId?: number;
  modeName?: string;
  messages?: Message[];
  quota?: DailyQuota;
  accessActive?: boolean;
  accessRequired?: boolean;
  error?: string;
};

export type StoredModeSelection = { ids: number[]; exists: boolean };
