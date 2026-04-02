import type { Source } from "./api";

interface SourcesListProps {
  sources: Source[];
}

export function SourcesList({ sources }: SourcesListProps) {
  if (!sources || sources.length === 0) return null;

  return (
    <div className="cb-sources">
      <span className="cb-sources__label">
        <svg width="12" height="12" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2">
          <path d="M14 2H6a2 2 0 0 0-2 2v16a2 2 0 0 0 2 2h12a2 2 0 0 0 2-2V8z" />
          <polyline points="14 2 14 8 20 8" />
          <line x1="16" y1="13" x2="8" y2="13" />
          <line x1="16" y1="17" x2="8" y2="17" />
          <polyline points="10 9 9 9 8 9" />
        </svg>
        Джерела:
      </span>
      <div className="cb-sources__list">
        {sources.map((src, i) => (
          <span key={i} className="cb-sources__item" title={`Релевантність: ${(src.score * 100).toFixed(0)}%`}>
            {src.document_name}
          </span>
        ))}
      </div>
    </div>
  );
}
