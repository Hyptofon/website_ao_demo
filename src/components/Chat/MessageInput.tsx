import { useRef, useEffect, type KeyboardEvent } from "react";
import type { Language } from "./api";

interface MessageInputProps {
  value: string;
  onChange: (v: string) => void;
  onSubmit: () => void;
  isLoading: boolean;
  language: Language;
}

const PLACEHOLDER = {
  uk: "Запитайте про вступ, документи, спеціальності...",
  en: "Ask about admission, documents, specialties...",
};

const SEND_LABEL = { uk: "Надіслати", en: "Send" };

export function MessageInput({ value, onChange, onSubmit, isLoading, language }: MessageInputProps) {
  const textareaRef = useRef<HTMLTextAreaElement>(null);

  // Auto-resize textarea
  useEffect(() => {
    const ta = textareaRef.current;
    if (!ta) return;
    ta.style.height = "auto";
    ta.style.height = `${Math.min(ta.scrollHeight, 160)}px`;
  }, [value]);

  const handleKeyDown = (e: KeyboardEvent<HTMLTextAreaElement>) => {
    if (e.key === "Enter" && !e.shiftKey) {
      e.preventDefault();
      if (!isLoading && value.trim()) onSubmit();
    }
  };

  const remaining = 500 - value.length;
  const isOverLimit = remaining < 0;

  return (
    <div className="cb-input-area">
      <div className={`cb-input-wrapper ${isOverLimit ? "cb-input-wrapper--over-limit" : ""}`}>
        <textarea
          ref={textareaRef}
          id="cb-message-input"
          className="cb-input"
          value={value}
          onChange={(e) => onChange(e.target.value)}
          onKeyDown={handleKeyDown}
          placeholder={PLACEHOLDER[language]}
          disabled={isLoading}
          rows={1}
          maxLength={510} // soft cap; server enforces 500
          aria-label={PLACEHOLDER[language]}
          aria-describedby="cb-char-count"
        />
        <div className="cb-input-controls">
          <span
            id="cb-char-count"
            className={`cb-char-count ${remaining < 50 ? "cb-char-count--warning" : ""} ${isOverLimit ? "cb-char-count--error" : ""}`}
          >
            {remaining}
          </span>
          <button
            className="cb-send-btn"
            onClick={onSubmit}
            disabled={isLoading || !value.trim() || isOverLimit}
            aria-label={SEND_LABEL[language]}
            title={SEND_LABEL[language]}
          >
            {isLoading ? (
              <svg className="cb-spinner" width="20" height="20" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2">
                <path d="M21 12a9 9 0 1 1-6.219-8.56" />
              </svg>
            ) : (
              <svg width="20" height="20" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2">
                <line x1="22" y1="2" x2="11" y2="13" />
                <polygon points="22 2 15 22 11 13 2 9 22 2" />
              </svg>
            )}
          </button>
        </div>
      </div>
      <p className="cb-input-hint">
        {language === "uk" ? "Enter — надіслати, Shift+Enter — новий рядок" : "Enter to send, Shift+Enter for new line"}
      </p>
    </div>
  );
}
