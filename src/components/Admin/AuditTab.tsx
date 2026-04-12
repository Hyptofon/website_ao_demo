import { useCallback, useEffect, useState } from "react";
import { Eye } from "lucide-react";
import { Button } from "@/components/ui/button";
import { fetchAudit, type AuditResponse } from "./api";
import { AnimatedSection, GlassCard, Badge, TabLoader, EmptyState } from "./ui";

const ACTION_MAP: Record<string, { label: string; color: string }> = {
  login: { label: "Логін", color: "blue" },
  upload_document: { label: "Завантаження", color: "green" },
  delete_document: { label: "Видалення", color: "red" },
  view_analytics: { label: "Перегляд", color: "cyan" },
  view_audit_log: { label: "Аудит", color: "purple" },
};

export function AuditTab() {
  const [data, setData] = useState<AuditResponse | null>(null);
  const [loading, setLoading] = useState(true);
  const [page, setPage] = useState(0);
  const limit = 20;

  const load = useCallback(async () => {
    setLoading(true);
    try { setData(await fetchAudit(page * limit, limit)); }
    catch (e) { console.error(e); }
    finally { setLoading(false); }
  }, [page]);

  useEffect(() => { load(); }, [load]);

  if (loading) return <TabLoader />;

  return (
    <div className="space-y-6">
      <AnimatedSection className="flex flex-wrap items-center justify-between gap-3">
        <h2 className="text-xl font-bold text-white">Журнал адміністратора</h2>
        <span className="text-xs text-zinc-600">{data?.total ?? 0} записів</span>
      </AnimatedSection>

      {(!data || data.entries.length === 0) ? (
        <AnimatedSection delay={0.1}>
          <EmptyState icon={Eye} title="Записів ще немає" />
        </AnimatedSection>
      ) : (
        <AnimatedSection delay={0.1}>
          <GlassCard>
            <div className="overflow-x-auto">
              <table className="w-full text-sm">
                <thead>
                  <tr className="border-b border-white/[0.06] text-left text-[11px] font-medium uppercase tracking-wider text-zinc-600">
                    <th className="px-3 py-2">Дія</th>
                    <th className="px-3 py-2">Адмін</th>
                    <th className="px-3 py-2">Ціль</th>
                    <th className="px-3 py-2">IP</th>
                    <th className="px-3 py-2">Дата</th>
                  </tr>
                </thead>
                <tbody>
                  {data.entries.map((e) => {
                    const a = ACTION_MAP[e.action] ?? { label: e.action, color: "gray" };
                    return (
                      <tr key={e.id} className="border-b border-white/[0.03] transition-colors hover:bg-white/[0.02]">
                        <td className="px-3 py-2.5"><Badge color={a.color}>{a.label}</Badge></td>
                        <td className="px-3 py-2.5 font-mono text-xs text-zinc-400">{e.admin_email}</td>
                        <td className="px-3 py-2.5 text-xs text-zinc-500 max-w-[200px] truncate">{e.target || "—"}</td>
                        <td className="px-3 py-2.5 font-mono text-[11px] text-zinc-600">{e.ip}</td>
                        <td className="px-3 py-2.5 text-xs text-zinc-600">{new Date(e.created_at).toLocaleString("uk-UA")}</td>
                      </tr>
                    );
                  })}
                </tbody>
              </table>
            </div>
          </GlassCard>

          {/* Pagination */}
          <div className="flex items-center justify-center gap-4 pt-2">
            <Button variant="outline" size="sm" disabled={page === 0} onClick={() => setPage((p) => p - 1)}>
              ← Назад
            </Button>
            <span className="text-xs text-zinc-600">Стор. {page + 1}</span>
            <Button variant="outline" size="sm" disabled={(page + 1) * limit >= (data?.total ?? 0)} onClick={() => setPage((p) => p + 1)}>
              Далі →
            </Button>
          </div>
        </AnimatedSection>
      )}
    </div>
  );
}
