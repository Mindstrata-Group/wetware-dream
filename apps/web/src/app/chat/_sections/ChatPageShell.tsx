"use client";

import { T } from "../theme";
import { ChatDesktopRail } from "./ChatDesktopRail";
import { ChatMobileBottomNav } from "./ChatMobileBottomNav";
import { ChatSheets } from "./ChatSheets";
import { ChatTopBar } from "./ChatTopBar";
import { ChatWorkspace } from "./ChatWorkspace";
import type { useChatPageController } from "../_hooks/useChatPageController";

export function ChatPageShell({ controller }: { controller: ReturnType<typeof useChatPageController> }) {
  return (
    <>
      <style>{`
        @keyframes ms-typ { 0%, 60%, 100% { opacity: .25; transform: translateY(0) } 30% { opacity: 1; transform: translateY(-2px) } }
        @keyframes ms-orch-in { from { opacity: 0; transform: translateY(-4px) } to { opacity: 1; transform: none } }
        .ms-chat-input { flex: 1; min-height: 44px; max-height: 130px; border-radius: 14px; border: 1px solid ${T.ink20}; background: ${T.surfaceSoft}; padding: 12px 14px; font-size: 14px; font-family: ${T.fontBody}; color: ${T.ink}; outline: none; resize: none; line-height: 1.4; scrollbar-width: none; }
        .ms-chat-input::-webkit-scrollbar { display: none; }
        .ms-chat-input::placeholder { color: ${T.ink50}; }
        .ms-chat-input:focus { border-color: ${T.green}; }
        .ms-chat-rail { scrollbar-width: none; }
        .ms-chat-rail::-webkit-scrollbar { display: none; }
        .ms-chat-feed { scrollbar-width: thin; scrollbar-color: ${T.ink10} transparent; }
        .ms-mode-pill-strip { scrollbar-width: none; }
        .ms-mode-pill-strip::-webkit-scrollbar { display: none; }
        @keyframes ms-dots-drift { 0% { background-position: 0 0, 0 0; } 50% { background-position: 180px 120px, -120px 180px; } 100% { background-position: 0 0, 0 0; } }
      `}</style>

      <div className="ms-chat-page" style={{ height: "100dvh", overflow: "hidden", overscrollBehavior: "none", display: "flex", flexDirection: "column", background: T.bg, fontFamily: T.fontBody, color: T.ink, position: "relative", fontSize: 14 }}>
        <ChatTopBar controller={controller} />
        <div style={{ flex: 1, minHeight: 0, display: "flex", flexDirection: "row" }}>
          <ChatDesktopRail controller={controller} />
          <ChatWorkspace controller={controller} />
        </div>
        <ChatMobileBottomNav controller={controller} />
      </div>

      <ChatSheets controller={controller} />
    </>
  );
}
