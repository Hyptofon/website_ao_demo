"use client";
import { useState, useCallback, useRef, useEffect } from "react";
import type { Language, Source } from "./api";
import { streamChat } from "./api";
import type { ChatMessage } from "./types";
import { generateId } from "./types";
import { MessageList } from "./MessageList";
import { MessageInput } from "./MessageInput";
import "./chatbot.css";

const STORAGE_KEY_LANG = "cb_language";

interface RateLimitState {
  blocked: boolean;
  retryAfterSec: number;
}

const WELCOME_UA =
  "Вітаю! 👋 Я офіційний асистент кафедри. Я можу відповісти на ваші запитання про вступ, спеціальності, вартість навчання, необхідні документи та багато іншого. Чим можу допомогти?";
const WELCOME_EN =
  "Welcome! 👋 I'm the official department assistant. I can answer your questions about admission, specialties, tuition costs, required documents, and more. How can I help you?";

export default function ChatWidget() {
  const [isOpen, setIsOpen] = useState(false);
  const [messages, setMessages] = useState<ChatMessage[]>([]);
  const [inputValue, setInputValue] = useState("");
  const [isLoading, setIsLoading] = useState(false);
  const [language, setLanguage] = useState<Language>(() => {
    if (typeof window !== "undefined") {
      return (localStorage.getItem(STORAGE_KEY_LANG) as Language) ?? "uk";
    }
    return "uk";
  });
  const [rateLimit, setRateLimit] = useState<RateLimitState>({ blocked: false, retryAfterSec: 0 });
  const [errorMsg, setErrorMsg] = useState<string | null>(null);

  const sessionId = useRef(generateId());
  const abortRef = useRef<AbortController | null>(null);
  const rateLimitTimer = useRef<ReturnType<typeof setInterval> | null>(null);

  // Inject welcome message when chat opens for the first time
  useEffect(() => {
    if (isOpen && messages.length === 0) {
      setMessages([
        {
          id: generateId(),
          role: "assistant",
          content: language === "uk" ? WELCOME_UA : WELCOME_EN,
          timestamp: new Date(),
        },
      ]);
    }
  }, [isOpen]);

  // Persist language choice
  const switchLanguage = (lang: Language) => {
    setLanguage(lang);
    localStorage.setItem(STORAGE_KEY_LANG, lang);
  };

  // Rate limit countdown
  useEffect(() => {
    if (!rateLimit.blocked) return;
    rateLimitTimer.current = setInterval(() => {
      setRateLimit((prev) => {
        const next = prev.retryAfterSec - 1;
        if (next <= 0) {
          clearInterval(rateLimitTimer.current!);
          return { blocked: false, retryAfterSec: 0 };
        }
        return { ...prev, retryAfterSec: next };
      });
    }, 1000);
    return () => clearInterval(rateLimitTimer.current!);
  }, [rateLimit.blocked]);

  const handleSubmit = useCallback(() => {
    const text = inputValue.trim();
    if (!text || isLoading || rateLimit.blocked) return;

    setErrorMsg(null);

    // Add user message
    const userMsg: ChatMessage = {
      id: generateId(),
      role: "user",
      content: text,
      timestamp: new Date(),
    };

    const assistantId = generateId();
    const assistantMsg: ChatMessage = {
      id: assistantId,
      role: "assistant",
      content: "",
      isStreaming: true,
      timestamp: new Date(),
    };

    setMessages((prev) => [...prev, userMsg, assistantMsg]);
    setInputValue("");
    setIsLoading(true);

    const ctrl = streamChat(text, language, sessionId.current, {
      onToken: (token) => {
        setMessages((prev) =>
          prev.map((m) =>
            m.id === assistantId ? { ...m, content: m.content + token } : m
          )
        );
      },
      onSources: (sources: Source[]) => {
        setMessages((prev) =>
          prev.map((m) =>
            m.id === assistantId ? { ...m, sources } : m
          )
        );
      },
      onError: (code, message) => {
        if (code === "rate_limit_exceeded") {
          // Extract seconds from message
          const match = message.match(/(\d+)/);
          const secs = match ? parseInt(match[1]) : 60;
          setRateLimit({ blocked: true, retryAfterSec: secs });
          setMessages((prev) => prev.filter((m) => m.id !== assistantId));
          setErrorMsg(message);
        } else {
          setMessages((prev) =>
            prev.map((m) =>
              m.id === assistantId
                ? { ...m, content: message, isStreaming: false }
                : m
            )
          );
        }
        setIsLoading(false);
      },
      onDone: () => {
        setMessages((prev) =>
          prev.map((m) =>
            m.id === assistantId
              ? { ...m, isStreaming: false, queryHash: userMsg.id }
              : m
          )
        );
        setIsLoading(false);
      },
    });

    abortRef.current = ctrl;
  }, [inputValue, isLoading, language, rateLimit.blocked]);

  const handleFeedback = useCallback((messageId: string, feedback: 1 | -1) => {
    setMessages((prev) =>
      prev.map((m) => (m.id === messageId ? { ...m, feedback } : m))
    );
  }, []);

  const handleNewConversation = () => {
    if (abortRef.current) abortRef.current.abort();
    sessionId.current = generateId();
    setMessages([
      {
        id: generateId(),
        role: "assistant",
        content: language === "uk" ? WELCOME_UA : WELCOME_EN,
        timestamp: new Date(),
      },
    ]);
    setIsLoading(false);
    setErrorMsg(null);
    setRateLimit({ blocked: false, retryAfterSec: 0 });
  };

  return (
    <>
      {/* ── Floating button ── */}
      <button
        id="cb-toggle-btn"
        className={`cb-fab ${isOpen ? "cb-fab--open" : ""}`}
        onClick={() => setIsOpen((o) => !o)}
        aria-label={isOpen ? "Закрити чат" : "Відкрити чат з асистентом"}
        aria-expanded={isOpen}
      >
        {isOpen ? (
          <svg width="24" height="24" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2.5">
            <line x1="18" y1="6" x2="6" y2="18" /><line x1="6" y1="6" x2="18" y2="18" />
          </svg>
        ) : (
          <svg width="24" height="24" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2">
            <path d="M21 15a2 2 0 0 1-2 2H7l-4 4V5a2 2 0 0 1 2-2h14a2 2 0 0 1 2 2z" />
          </svg>
        )}
        {!isOpen && <span className="cb-fab__badge" aria-hidden="true">AI</span>}
      </button>

      {/* ── Chat panel ── */}
      <div
        id="cb-panel"
        className={`cb-panel ${isOpen ? "cb-panel--open" : ""}`}
        role="dialog"
        aria-label="Чат-асистент кафедри"
        aria-modal="false"
      >
        {/* Header */}
        <div className="cb-header">
          <div className="cb-header__info">
            <div className="cb-header__avatar" aria-hidden="true">
              <svg width="20" height="20" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2">
                <circle cx="12" cy="12" r="3" />
                <path d="M12 1v4M12 19v4M4.22 4.22l2.83 2.83M16.95 16.95l2.83 2.83M1 12h4M19 12h4M4.22 19.78l2.83-2.83M16.95 7.05l2.83-2.83" />
              </svg>
            </div>
            <div>
              <div className="cb-header__title">
                {language === "uk" ? "Асистент кафедри" : "Department Assistant"}
              </div>
              <div className="cb-header__subtitle">
                <span className="cb-status-dot" aria-hidden="true" />
                {language === "uk" ? "Онлайн" : "Online"}
              </div>
            </div>
          </div>

          <div className="cb-header__actions">
            {/* Language switcher */}
            <div className="cb-lang-switcher" role="group" aria-label="Вибір мови">
              <button
                className={`cb-lang-btn ${language === "uk" ? "cb-lang-btn--active" : ""}`}
                onClick={() => switchLanguage("uk")}
                aria-pressed={language === "uk"}
                title="Українська"
              >
                🇺🇦 UA
              </button>
              <button
                className={`cb-lang-btn ${language === "en" ? "cb-lang-btn--active" : ""}`}
                onClick={() => switchLanguage("en")}
                aria-pressed={language === "en"}
                title="English"
              >
                🇬🇧 EN
              </button>
            </div>

            {/* New conversation */}
            <button
              className="cb-new-chat-btn"
              onClick={handleNewConversation}
              title={language === "uk" ? "Нова розмова" : "New conversation"}
              aria-label={language === "uk" ? "Нова розмова" : "New conversation"}
            >
              <svg width="16" height="16" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2">
                <polyline points="1 4 1 10 7 10" />
                <path d="M3.51 15a9 9 0 1 0 .49-4.96" />
              </svg>
            </button>
          </div>
        </div>

        {/* Rate limit banner */}
        {rateLimit.blocked && (
          <div className="cb-rate-limit-banner" role="alert">
            <svg width="16" height="16" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2">
              <circle cx="12" cy="12" r="10" /><line x1="12" y1="8" x2="12" y2="12" /><line x1="12" y1="16" x2="12.01" y2="16" />
            </svg>
            {language === "uk"
              ? `Ліміт запитів. Зачекайте ${rateLimit.retryAfterSec} сек.`
              : `Rate limit reached. Wait ${rateLimit.retryAfterSec} sec.`}
          </div>
        )}

        {/* Error banner (non-rate-limit) */}
        {errorMsg && !rateLimit.blocked && (
          <div className="cb-error-banner" role="alert">
            {errorMsg}
            <button className="cb-error-dismiss" onClick={() => setErrorMsg(null)} aria-label="Закрити">✕</button>
          </div>
        )}

        {/* Messages */}
        <MessageList
          messages={messages}
          isLoading={isLoading}
          onFeedback={handleFeedback}
        />

        {/* Input */}
        <MessageInput
          value={inputValue}
          onChange={setInputValue}
          onSubmit={handleSubmit}
          isLoading={isLoading}
          language={language}
        />
      </div>
    </>
  );
}
