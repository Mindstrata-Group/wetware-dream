// SOLID-split of page.tsx (landing) 2026-05-31: types.

export type HomeProfile = { user: { role: string }; hasAccess?: boolean };

export type DemoMode = {
  id: number;
  name: string;
  demoChat?: string;
  demo_chat?: string;
};

export type DemoMessage = { role: 'user' | 'assistant'; content: string };

export type ApiModeStats = { paid: number; total: number };

export type Conversation = { name: string; messages: DemoMessage[] };
