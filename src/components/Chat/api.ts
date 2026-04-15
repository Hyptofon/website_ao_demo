// Typed API client for the Go backend
// All communication goes through this module

const API_BASE = import.meta.env.PUBLIC_API_URL ?? "http://localhost:8080";

export type Language = "uk" | "en";

export interface Source {
  document_name: string;
  score: number;
  page_number: number;
}

export interface FeedbackPayload {
  query_hash: string;
  feedback: 1 | -1;
}

export interface StreamCallbacks {
  onToken: (token: string) => void;
  onSources: (sources: Source[]) => void;
  onMeta: (queryHash: string) => void;
  onError: (code: string, message: string) => void;
  onDone: () => void;
}

/**
 * Opens an SSE stream to POST /api/v1/chat/stream.
 * Uses fetch + ReadableStream (not EventSource) because EventSource doesn't
 * support POST bodies.
 * Returns an AbortController so the caller can cancel the stream.
 */
export function streamChat(
  message: string,
  language: Language,
  sessionId: string,
  callbacks: StreamCallbacks
): AbortController {
  const controller = new AbortController();

  (async () => {
    try {
      const res = await fetch(`${API_BASE}/api/v1/chat/stream`, {
        method: "POST",
        headers: { "Content-Type": "application/json" },
        body: JSON.stringify({ message, language, session_id: sessionId }),
        signal: controller.signal,
      });

      if (!res.ok || !res.body) {
        const body = await res.text();
        let errMsg = body;
        let errCode = "http_error";
        
        // The backend sends errors as SSE events format: event: error\ndata: {"error":"...", "message":"..."}
        if (body.includes("data: {")) {
          const match = body.match(/data: (\{.*?\})/);
          if (match) {
            try {
              const parsed = JSON.parse(match[1]);
              if (parsed.error) errCode = parsed.error;
              if (parsed.message) errMsg = parsed.message;
            } catch {}
          }
        }
        
        callbacks.onError(errCode, errMsg);
        return;
      }

      const reader = res.body.getReader();
      const decoder = new TextDecoder();
      let buffer = "";

      while (true) {
        const { done, value } = await reader.read();
        if (done) break;

        buffer += decoder.decode(value, { stream: true });
        const lines = buffer.split("\n");
        buffer = lines.pop() ?? "";

        for (const line of lines) {
          if (line.startsWith("event: error")) continue;
          if (line.startsWith("event: sources")) continue;
          if (line.startsWith("event: meta")) continue;

          if (line.startsWith("data: ")) {
            const data = line.slice(6);

            if (data === "[DONE]") {
              callbacks.onDone();
              continue;
            }

            // Check if this is a JSON payload (sources event)
            if (data.startsWith("[") || data.startsWith("{")) {
              try {
                const parsed = JSON.parse(data);
                if (Array.isArray(parsed)) {
                  callbacks.onSources(parsed as Source[]);
                } else if (parsed.query_hash) {
                  callbacks.onMeta(parsed.query_hash);
                } else if (parsed.error) {
                  callbacks.onError(parsed.error, parsed.message ?? "Unknown error");
                }
              } catch {
                // not JSON — treat as text token
                callbacks.onToken(data.replace(/\\n/g, "\n"));
              }
              continue;
            }

            // Regular text token
            callbacks.onToken(data.replace(/\\n/g, "\n"));
          }
        }
      }
    } catch (err) {
      if ((err as Error).name !== "AbortError") {
        callbacks.onError("network_error", (err as Error).message);
      }
    }
  })();

  return controller;
}

/** POST /api/v1/feedback */
export async function submitFeedback(payload: FeedbackPayload): Promise<void> {
  await fetch(`${API_BASE}/api/v1/feedback`, {
    method: "POST",
    headers: { "Content-Type": "application/json" },
    body: JSON.stringify(payload),
  });
}

export interface SuggestedQuestion {
  id: number;
  question: string;
  language: Language;
  is_auto: boolean;
  priority: number;
}

export async function fetchSuggestions(language: Language): Promise<SuggestedQuestion[]> {
  try {
    const res = await fetch(`${API_BASE}/api/v1/suggestions?lang=${language}&limit=3`);
    if (!res.ok) return [];
    return await res.json() as SuggestedQuestion[];
  } catch (err) {
    console.error("Failed to fetch suggestions", err);
    return [];
  }
}
