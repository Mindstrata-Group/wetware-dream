import type React from "react";
import type { ChatAttachment, ChatHistoryItem, DailyQuota, Message, Mode } from "../types";

export type StateSetter<T> = React.Dispatch<React.SetStateAction<T>>;
export type ModeIdState = number | "";

export type ChatApiActionsContext = {
  modes: Mode[];
  quota: DailyQuota | null;
  modeId: ModeIdState;
  modeName: string;
  input: string;
  chatMessageMaxChars: number;
  attachments: ChatAttachment[];
  dialogId: number | null;
  selectedModeIds: number[];
  beforeId: number | null;
  responseMode: string;
  dialogCompleted: boolean;
  dialogAccessRequired: boolean;
  dailyLimitExhausted: boolean;
  sending: boolean;
  completing: boolean;
  startingAfterSummary: boolean;
  modeSelectionLoadedRef: React.MutableRefObject<boolean>;
  loadingMoreRef: React.MutableRefObject<boolean>;
  setLoading: StateSetter<boolean>;
  setError: StateSetter<string | null>;
  setErrorDebug: StateSetter<unknown>;
  setUserRole: StateSetter<string>;
  setUserEmail: StateSetter<string>;
  setMaxLinked: StateSetter<boolean>;
  setMaxBotLink: StateSetter<string>;
  setResponseMode: StateSetter<string>;
  setModes: StateSetter<Mode[]>;
  setSelectedModeIds: StateSetter<number[]>;
  setPromoModeIds: StateSetter<number[]>;
  setQuota: StateSetter<DailyQuota | null>;
  setChatMessageMaxChars: StateSetter<number>;
  setModeId: StateSetter<ModeIdState>;
  setModeName: StateSetter<string>;
  setDialogId: StateSetter<number | null>;
  setMessages: StateSetter<Message[]>;
  setBeforeId: StateSetter<number | null>;
  setSummaryHistoryVisible: StateSetter<boolean>;
  setDialogAccessRequired: StateSetter<boolean>;
  setDialogCompleted: StateSetter<boolean>;
  setInput: StateSetter<string>;
  setAttachments: StateSetter<ChatAttachment[]>;
  setOrchestrationNotice: StateSetter<string>;
  setLockedModeNotice: StateSetter<string>;
  setSending: StateSetter<boolean>;
  setCompleting: StateSetter<boolean>;
  setStartingAfterSummary: StateSetter<boolean>;
  scrollChatToBottom: (behavior?: ScrollBehavior) => void;
};

export type ChatApiActions = {
  start: () => Promise<void>;
  loadHistory: (targetDialogId: number, more?: boolean, knownModes?: Mode[], defaultSelectedModeIds?: number[]) => Promise<boolean>;
  openMode: (nextModeId: number, preservedInput?: string, newDialog?: boolean) => Promise<void>;
  openBaseMode: (preservedInput?: string, newDialog?: boolean) => Promise<void>;
  sendMessage: (text?: string) => Promise<void>;
  completeDialog: () => Promise<void>;
  startNewDialogAfterSummary: (preservedInput: string) => Promise<void>;
};

export type ChatHistoryActionsContext = Pick<ChatApiActionsContext,
  "modes" | "modeId" | "input" | "dialogId" | "dialogCompleted" | "dialogAccessRequired" |
  "selectedModeIds" | "setInput" | "setSummaryHistoryVisible" |
  "setDialogId" | "setModeId" | "setModeName" | "setBeforeId" | "setDialogCompleted" |
  "setDialogAccessRequired" | "setLockedModeNotice" | "setSelectedModeIds" | "setOrchestrationNotice" |
  "setError" | "setErrorDebug" | "setMessages"
> & {
  editingHistoryId: number | null;
  editingHistoryTitle: string;
  historyItems: ChatHistoryItem[];
  startingAfterSummary: boolean;
  openMode: ChatApiActions["openMode"];
  openBaseMode: ChatApiActions["openBaseMode"];
  loadHistory: ChatApiActions["loadHistory"];
  startNewDialogAfterSummary: ChatApiActions["startNewDialogAfterSummary"];
  setHistoryItems: StateSetter<ChatHistoryItem[]>;
  setEditingHistoryId: StateSetter<number | null>;
  setEditingHistoryTitle: StateSetter<string>;
  setHistorySearch: StateSetter<string>;
};
