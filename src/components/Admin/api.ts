// ─── Admin Panel API Client ────────────────────────────────────────────────
// All communication goes through this module.
//
// SECURITY: Admin endpoints are mounted at /admin-{segment}/* where {segment}
// is derived from SHA256(ADMIN_TOKEN)[:16 bytes] as 32 hex chars.
// The PUBLIC_ADMIN_PATH env var must be set at build time to match the backend.

const API_BASE = import.meta.env.PUBLIC_API_URL ?? "http://localhost:8080";

// PUBLIC_ADMIN_PATH: 32-char hex segment that forms the hidden admin URL.
// Must match adminPathFromToken(ADMIN_TOKEN) computed by the Go backend.
// Default 'panel' matches the Go backend fallback when ADMIN_TOKEN is empty.
const ADMIN_PATH = import.meta.env.PUBLIC_ADMIN_PATH ?? "panel";

/** Full admin API prefix, e.g. /admin-04643cb4e1702d075c9ff1ca95c81950 */
const ADMIN_BASE = `/admin-${ADMIN_PATH}`;

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
  query_text: string;
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

export interface QueryRow {
  query_hash: string;
  query_text: string;
  language: string;
  response_ms: number;
  sources_cnt: number;
  feedback: number;
  is_blocked: number;
  created_at: string;
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

export const getLoginUrl = async () => (await api<{ url: string }>(`${ADMIN_BASE}/auth/login`)).url;

export const fetchSummary = (days = 30) => api<AnalyticsSummary>(`${ADMIN_BASE}/analytics/summary?days=${days}`);
export const fetchDaily = (days = 30) => api<DailyStat[]>(`${ADMIN_BASE}/analytics/daily?days=${days}`);
export const fetchTopQueries = (days = 30, limit = 10) => api<TopQuery[]>(`${ADMIN_BASE}/analytics/top-queries?days=${days}&limit=${limit}`);
export const fetchFeedback = (days = 30) => api<FeedbackStat>(`${ADMIN_BASE}/analytics/feedback?days=${days}`);

export const fetchDocuments = () => api<DocumentRecord[]>(`${ADMIN_BASE}/documents`);
export const deleteDocument = (id: string) => api<unknown>(`${ADMIN_BASE}/documents/${id}`, { method: "DELETE" });

// Fix #13: uploadDocument polling now has a 5-minute timeout to prevent
// infinite polling when a job gets stuck in a pending/processing state.
const UPLOAD_POLL_TIMEOUT_MS = 5 * 60 * 1000; // 5 minutes

export const uploadDocument = async (file: File) => {
  const token = getToken();
  const fd = new FormData();
  fd.append("file", file);
  const h: Record<string, string> = {};
  if (token) h["Authorization"] = `Bearer ${token}`;
  
  const uploadRes = await fetch(`${API_BASE}${ADMIN_BASE}/documents/upload`, { method: "POST", headers: h, body: fd });
  if (!uploadRes.ok) throw new Error(await uploadRes.text());
  
  const { job_id } = await uploadRes.json() as { job_id: string };

  // Poll for job completion with a hard timeout.
  return new Promise<void>((resolve, reject) => {
    const startedAt = Date.now();

    const check = async () => {
      // Guard: abort if we've been polling for more than 5 minutes
      if (Date.now() - startedAt > UPLOAD_POLL_TIMEOUT_MS) {
        reject(new Error(
          "Час очікування закінчився (5 хв). Перевірте документи — файл міг завантажитися успішно."
        ));
        return;
      }

      try {
        const jobRes = await fetch(`${API_BASE}${ADMIN_BASE}/documents/jobs/${job_id}`, { headers: h });
        if (!jobRes.ok) throw new Error("Failed to check status");
        
        const job = await jobRes.json() as { status: string; error: string; progress: number };
        
        if (job.status === "completed") {
          resolve();
        } else if (job.status === "failed") {
          reject(new Error(job.error || "Upload failed during processing"));
        } else {
          setTimeout(check, 1500); // Check again in 1.5s
        }
      } catch (err) {
        reject(err);
      }
    };
    setTimeout(check, 1000);
  });
};

export const fetchAudit = (offset = 0, limit = 5) => api<AuditResponse>(`${ADMIN_BASE}/audit?offset=${offset}&limit=${limit}`);

export const fetchQueries = (days = 30, limit = 50) => api<QueryRow[]>(`${ADMIN_BASE}/queries?days=${days}&limit=${limit}`);

export const renameDocument = (id: string, newName: string) => api<unknown>(`${ADMIN_BASE}/documents/${id}/rename`, {
  method: "PATCH",
  body: JSON.stringify({ filename: newName }),
});

export const getDocumentDownloadUrl = (id: string) => `${API_BASE}${ADMIN_BASE}/documents/${id}/download`;

export const fetchPrompts = () => api<PromptVariant[]>(`${ADMIN_BASE}/prompts`);
export const createPrompt = (prompt: Partial<PromptVariant>) => api<unknown>(`${ADMIN_BASE}/prompts`, {
  method: "POST",
  body: JSON.stringify(prompt),
});
export const togglePromptActive = (id: number, isActive: boolean) => api<unknown>(`${ADMIN_BASE}/prompts/${id}/active`, {
  method: "PATCH",
  body: JSON.stringify({ is_active: isActive }),
});
export const updatePrompt = (id: number, promptText: string) => api<unknown>(`/admin/prompts/${id}`, {
  method: "PATCH",
  body: JSON.stringify({ prompt_text: promptText }),
});
export const deletePrompt = (id: number) => api<unknown>(`/admin/prompts/${id}`, {
  method: "DELETE",
});
