"use client";

import { ChatPageShell } from "./_sections/ChatPageShell";
import { useChatPageController } from "./_hooks/useChatPageController";

export default function ChatPage() {
  const controller = useChatPageController();
  return <ChatPageShell controller={controller} />;
}
