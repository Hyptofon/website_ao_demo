import type { Source } from "./api";

export type MessageRole = "user" | "assistant";

export interface ChatMessage {
  id: string;
  role: MessageRole;
  content: string;
  sources?: Source[];
  queryHash?: string;
  feedback?: 1 | -1 | null;
  isStreaming?: boolean;
  isOffTopic?: boolean;
  timestamp: Date;
}

export function generateId(): string {
  return Math.random().toString(36).slice(2, 11);
}
