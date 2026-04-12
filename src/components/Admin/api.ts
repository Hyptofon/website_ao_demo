// ─── Admin Panel API Client ────────────────────────────────────────────────
// Typed API layer mirroring Chat/api.ts pattern.

const API_BASE = import.meta.env.PUBLIC_API_URL ?? "http://localhost:8080";

// ─── Types ──────────────────────────────────────────────────────────────────

export interface AnalyticsSummary {
  total_queries: number;
  blocked_queries: number;
  positive_feedback: number;
  negative_feedback: number;
  avg_response_ms: number;
}

export interface DailyStat {
  date: string;
  total_queries: number;
  blocked_queries: number;
  avg_response_ms: number;
  positive_feedback: number;
  negative_feedback: number;
}

export interface TopQuery {
  query_hash: string;
  count: number;
  language: string;
  last_seen: string;
}

export interface FeedbackStat {
  total: number;
  positive: number;
  negative: number;
  ratio: number;
}

export interface DocumentRecord {
  id: string;
  filename: string;
  doc_type: string;
  language: string;
  chunk_count: number;
  summary: string;
  uploaded_by: string;
  uploaded_at: string;
}

export interface AuditEntry {
  id: number;
  admin_email: string;
  action: string;
  target: string;
  ip: string;
  created_at: string;
}

export interface AuditResponse {
  entries: AuditEntry[];
  total: number;
  offset: number;
  limit: number;
}

export interface PromptVariant {
  id: number;
  name: string;
  language: string;
  prompt_text: string;
  is_active: boolean;
  usage_count: number;
  avg_score: number;
}

export interface SuggestedQuestion {
  id: number;
  question: string;
  language: string;
  is_auto: boolean;
  priority: number;
}

// ─── Auth ───────────────────────────────────────────────────────────────────

const TOKEN_KEY = "admin_jwt";
export const getToken = () => typeof window !== "undefined" ? localStorage.getItem(TOKEN_KEY) : null;
export const setToken = (t: string) => localStorage.setItem(TOKEN_KEY, t);
export const clearToken = () => localStorage.removeItem(TOKEN_KEY);

// ─── Fetch ──────────────────────────────────────────────────────────────────

async function api<T>(path: string, opts: RequestInit = {}): Promise<T> {
  const token = getToken();
  const headers: Record<string, string> = {
    "Content-Type": "application/json",
    ...((opts.headers as Record<string, string>) ?? {}),
  };
  if (token) headers["Authorization"] = `Bearer ${token}`;

  const res = await fetch(`${API_BASE}${path}`, { ...opts, headers });

  if (res.status === 401) {
    clearToken();
    throw new Error("unauthorized");
  }
  if (!res.ok) throw new Error(await res.text());
  return res.json() as Promise<T>;
}

// ─── Endpoints ──────────────────────────────────────────────────────────────

export const getLoginUrl = async () => (await api<{ url: string }>("/admin/auth/login")).url;

export const fetchSummary = (days = 30) => api<AnalyticsSummary>(`/admin/analytics/summary?days=${days}`);
export const fetchDaily = (days = 30) => api<DailyStat[]>(`/admin/analytics/daily?days=${days}`);
export const fetchTopQueries = (days = 30, limit = 10) => api<TopQuery[]>(`/admin/analytics/top-queries?days=${days}&limit=${limit}`);
export const fetchFeedback = (days = 30) => api<FeedbackStat>(`/admin/analytics/feedback?days=${days}`);

export const fetchDocuments = () => api<DocumentRecord[]>("/admin/documents");
export const deleteDocument = (id: string) => api<unknown>(`/admin/documents/${id}`, { method: "DELETE" });

export const uploadDocument = async (file: File) => {
  const token = getToken();
  const fd = new FormData();
  fd.append("file", file);
  const h: Record<string, string> = {};
  if (token) h["Authorization"] = `Bearer ${token}`;
  const res = await fetch(`${API_BASE}/admin/documents/upload`, { method: "POST", headers: h, body: fd });
  if (!res.ok) throw new Error(await res.text());
  return res.json() as Promise<{ job_id: string }>;
};

export const fetchAudit = (offset = 0, limit = 20) => api<AuditResponse>(`/admin/audit?offset=${offset}&limit=${limit}`);

export const fetchPrompts = () => api<PromptVariant[]>("/admin/prompts");
export const createPrompt = (d: Partial<PromptVariant>) => api<PromptVariant>("/admin/prompts", { method: "POST", body: JSON.stringify(d) });

export const fetchSuggestions = (lang = "uk") => api<SuggestedQuestion[]>(`/admin/suggestions?lang=${lang}&limit=20`);
export const createSuggestion = (d: Partial<SuggestedQuestion>) => api<SuggestedQuestion>("/admin/suggestions", { method: "POST", body: JSON.stringify(d) });
