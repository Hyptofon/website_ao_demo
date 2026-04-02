import type { Source } from "./api";
import { submitFeedback } from "./api";
import type { ChatMessage } from "./types";
import { SourcesList } from "./SourcesList";

interface MessageBubbleProps {
  message: ChatMessage;
  onFeedback: (messageId: string, feedback: 1 | -1) => void;
}

function ThumbsUp({ filled }: { filled: boolean }) {
  return (
    <svg width="16" height="16" viewBox="0 0 24 24" fill={filled ? "currentColor" : "none"} stroke="currentColor" strokeWidth="2">
      <path d="M14 9V5a3 3 0 0 0-3-3l-4 9v11h11.28a2 2 0 0 0 2-1.7l1.38-9a2 2 0 0 0-2-2.3H14z" />
      <path d="M7 22H4a2 2 0 0 1-2-2v-7a2 2 0 0 1 2-2h3" />
    </svg>
  );
}

function ThumbsDown({ filled }: { filled: boolean }) {
  return (
    <svg width="16" height="16" viewBox="0 0 24 24" fill={filled ? "currentColor" : "none"} stroke="currentColor" strokeWidth="2">
      <path d="M10 15v4a3 3 0 0 0 3 3l4-9V2H5.72a2 2 0 0 0-2 1.7l-1.38 9a2 2 0 0 0 2 2.3H10z" />
      <path d="M17 2h2.67A2.31 2.31 0 0 1 22 4v7a2.31 2.31 0 0 1-2.33 2H17" />
    </svg>
  );
}

export function MessageBubble({ message, onFeedback }: MessageBubbleProps) {
  const isUser = message.role === "user";

  const handleFeedback = (value: 1 | -1) => {
    if (message.feedback !== null && message.feedback !== undefined) return;
    if (message.queryHash) {
      submitFeedback({ query_hash: message.queryHash, feedback: value });
    }
    onFeedback(message.id, value);
  };

  return (
    <div className={`cb-message cb-message--${message.role}`}>
      {!isUser && (
        <div className="cb-message__avatar">
          <svg width="20" height="20" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2">
            <circle cx="12" cy="12" r="3" />
            <path d="M12 1v4M12 19v4M4.22 4.22l2.83 2.83M16.95 16.95l2.83 2.83M1 12h4M19 12h4M4.22 19.78l2.83-2.83M16.95 7.05l2.83-2.83" />
          </svg>
        </div>
      )}

      <div className="cb-message__content-wrapper">
        <div className={`cb-message__bubble ${message.isStreaming ? "cb-message__bubble--streaming" : ""}`}>
          <div className="cb-message__text">
            {message.content}
            {message.isStreaming && <span className="cb-cursor" aria-hidden="true" />}
          </div>
        </div>

        {!isUser && !message.isStreaming && message.content && (
          <div className="cb-message__footer">
            {message.sources && message.sources.length > 0 && (
              <SourcesList sources={message.sources} />
            )}
            <div className="cb-feedback">
              <span className="cb-feedback__label">Корисна відповідь?</span>
              <button
                className={`cb-feedback__btn ${message.feedback === 1 ? "cb-feedback__btn--active-positive" : ""}`}
                onClick={() => handleFeedback(1)}
                disabled={message.feedback !== null && message.feedback !== undefined}
                title="Корисно"
                aria-label="Позначити відповідь як корисну"
              >
                <ThumbsUp filled={message.feedback === 1} />
              </button>
              <button
                className={`cb-feedback__btn ${message.feedback === -1 ? "cb-feedback__btn--active-negative" : ""}`}
                onClick={() => handleFeedback(-1)}
                disabled={message.feedback !== null && message.feedback !== undefined}
                title="Некорисно"
                aria-label="Позначити відповідь як некорисну"
              >
                <ThumbsDown filled={message.feedback === -1} />
              </button>
            </div>
          </div>
        )}
      </div>

      {isUser && (
        <div className="cb-message__avatar cb-message__avatar--user">
          <svg width="20" height="20" viewBox="0 0 24 24" fill="currentColor">
            <path d="M12 12c2.7 0 4.8-2.1 4.8-4.8S14.7 2.4 12 2.4 7.2 4.5 7.2 7.2 9.3 12 12 12zm0 2.4c-3.2 0-9.6 1.6-9.6 4.8v2.4h19.2v-2.4c0-3.2-6.4-4.8-9.6-4.8z"/>
          </svg>
        </div>
      )}
    </div>
  );
}
