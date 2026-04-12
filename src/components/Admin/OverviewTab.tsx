import { useCallback, useEffect, useState } from "react";
import {
  MessageSquare, Clock, ThumbsUp, ThumbsDown,
  TrendingUp, Users, RefreshCw,
} from "lucide-react";
import { motion } from "motion/react";
import {
  fetchSummary, fetchDaily, fetchTopQueries, fetchFeedback,
  type AnalyticsSummary, type DailyStat, type TopQuery, type FeedbackStat,
} from "./api";
import { AnimatedSection, StatCard, GlassCard, Badge, TabLoader } from "./ui";
import { cn } from "@/lib/utils";

export function OverviewTab() {
  const [summary, setSummary] = useState<AnalyticsSummary | null>(null);
  const [daily, setDaily] = useState<DailyStat[]>([]);
  const [topQ, setTopQ] = useState<TopQuery[]>([]);
  const [fb, setFb] = useState<FeedbackStat | null>(null);
  const [days, setDays] = useState(30);
  const [loading, setLoading] = useState(true);

  const load = useCallback(async () => {
    setLoading(true);
    try {
      const [s, d, q, f] = await Promise.all([
        fetchSummary(days), fetchDaily(days), fetchTopQueries(days, 10), fetchFeedback(days),
      ]);
      setSummary(s); setDaily(d); setTopQ(q); setFb(f);
    } catch (e) { console.error(e); }
    finally { setLoading(false); }
  }, [days]);

  useEffect(() => { load(); }, [load]);

  if (loading) return <TabLoader />;

  return (
    <div className="space-y-6">
      {/* Header */}
      <AnimatedSection className="flex flex-wrap items-center justify-between gap-3">
        <h2 className="text-xl font-bold text-white">Аналітика</h2>
        <div className="flex items-center gap-1 rounded-lg bg-white/[0.04] p-1">
          {[7, 14, 30].map((d) => (
            <button
              key={d}
              onClick={() => setDays(d)}
              className={cn(
                "rounded-md px-3 py-1.5 text-xs font-medium transition-all",
                days === d ? "bg-brand-blue text-white shadow-sm" : "text-zinc-500 hover:text-zinc-300",
              )}
            >
              {d}д
            </button>
          ))}
          <button onClick={load} className="ml-1 p-1.5 text-zinc-600 hover:text-zinc-300 transition-colors" title="Оновити">
            <RefreshCw size={14} />
          </button>
        </div>
      </AnimatedSection>

      {/* Stat Cards */}
      {summary && (
        <div className="grid grid-cols-1 gap-4 sm:grid-cols-2 lg:grid-cols-4">
          <AnimatedSection delay={0.05}>
            <StatCard icon={MessageSquare} label="Всього запитів" value={summary.total_queries} accent="blue" />
          </AnimatedSection>
          <AnimatedSection delay={0.1}>
            <StatCard icon={Clock} label="Сер. відповідь" value={`${(summary.avg_response_ms / 1000).toFixed(1)}с`} accent="cyan" />
          </AnimatedSection>
          <AnimatedSection delay={0.15}>
            <StatCard icon={ThumbsUp} label="Позитивних" value={fb?.positive ?? 0} accent="green" />
          </AnimatedSection>
          <AnimatedSection delay={0.2}>
            <StatCard icon={ThumbsDown} label="Негативних" value={fb?.negative ?? 0} accent="red" />
          </AnimatedSection>
        </div>
      )}

      {/* Bar Chart */}
      {daily.length > 0 && (
        <AnimatedSection delay={0.25}>
          <GlassCard title="Запити по днях" icon={TrendingUp}>
            <div className="flex h-44 items-end gap-[3px]">
              {daily.map((d, i) => {
                const max = Math.max(...daily.map((s) => s.total_queries), 1);
                const pct = (d.total_queries / max) * 100;
                return (
                  <motion.div
                    key={d.date}
                    className="group relative flex flex-1 flex-col items-center"
                    initial={{ scaleY: 0 }}
                    animate={{ scaleY: 1 }}
                    transition={{ delay: 0.3 + i * 0.03, duration: 0.4 }}
                    style={{ transformOrigin: "bottom" }}
                  >
                    {/* Tooltip */}
                    <div className="pointer-events-none absolute -top-10 z-10 hidden rounded-md bg-zinc-800 px-2 py-1 text-[10px] text-zinc-200 shadow-lg group-hover:block whitespace-nowrap">
                      {d.date}: {d.total_queries}
                    </div>
                    <div
                      className="w-full max-w-[28px] rounded-t bg-gradient-to-t from-brand-blue to-brand-blue-soft transition-all group-hover:from-brand-blue-soft group-hover:to-blue-300"
                      style={{ height: `${Math.max(pct, 4)}%` }}
                    />
                    <span className="mt-1 text-[9px] text-zinc-600 truncate max-w-[32px]">{d.date.slice(5)}</span>
                  </motion.div>
                );
              })}
            </div>
          </GlassCard>
        </AnimatedSection>
      )}

      {/* Top Queries */}
      {topQ.length > 0 && (
        <AnimatedSection delay={0.35}>
          <GlassCard title="Топ запити" icon={Users}>
            <div className="overflow-x-auto">
              <table className="w-full text-sm">
                <thead>
                  <tr className="border-b border-white/[0.06] text-left text-[11px] font-medium uppercase tracking-wider text-zinc-600">
                    <th className="px-3 py-2">Hash</th>
                    <th className="px-3 py-2">Кількість</th>
                    <th className="px-3 py-2">Мова</th>
                    <th className="px-3 py-2">Останній</th>
                  </tr>
                </thead>
                <tbody>
                  {topQ.map((q) => (
                    <tr key={q.query_hash} className="border-b border-white/[0.03] transition-colors hover:bg-white/[0.02]">
                      <td className="px-3 py-2.5 font-mono text-xs text-zinc-400">{q.query_hash.slice(0, 12)}…</td>
                      <td className="px-3 py-2.5"><Badge>{q.count}</Badge></td>
                      <td className="px-3 py-2.5">{q.language === "uk" ? "🇺🇦" : "🇬🇧"}</td>
                      <td className="px-3 py-2.5 text-xs text-zinc-600">{q.last_seen.slice(0, 10)}</td>
                    </tr>
                  ))}
                </tbody>
              </table>
            </div>
          </GlassCard>
        </AnimatedSection>
      )}

      {/* Feedback Ratio */}
      {fb && fb.total > 0 && (
        <AnimatedSection delay={0.4}>
          <GlassCard title="Задоволеність" icon={ThumbsUp}>
            <div className="flex h-9 overflow-hidden rounded-lg text-xs font-medium">
              <motion.div
                initial={{ width: 0 }}
                animate={{ width: `${fb.ratio * 100}%` }}
                transition={{ duration: 0.8, ease: "easeOut" }}
                className="flex items-center justify-center bg-gradient-to-r from-emerald-600 to-emerald-400 text-white"
                style={{ minWidth: 50 }}
              >
                {(fb.ratio * 100).toFixed(0)}% 👍
              </motion.div>
              <motion.div
                initial={{ width: 0 }}
                animate={{ width: `${(1 - fb.ratio) * 100}%` }}
                transition={{ duration: 0.8, ease: "easeOut", delay: 0.1 }}
                className="flex items-center justify-center bg-gradient-to-r from-red-600 to-red-400 text-white"
                style={{ minWidth: 50 }}
              >
                {((1 - fb.ratio) * 100).toFixed(0)}% 👎
              </motion.div>
            </div>
          </GlassCard>
        </AnimatedSection>
      )}
    </div>
  );
}
