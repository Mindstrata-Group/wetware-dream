export type SubTab = "landing" | "about" | "privacy" | "access" | "chat" | "features";

export type ContentItem = {
  key: string;
  value: unknown;
  updatedAt: string;
};

export type Tile = { id: number; title: string; body: string };
export type FeatureItem = { icon: string; title: string; body: string };
export type ComparisonItem = { aspect: string; regular: string; stratum: string };
export type EditableListField = { key: string; label: string; multiline?: boolean; placeholder?: string };
