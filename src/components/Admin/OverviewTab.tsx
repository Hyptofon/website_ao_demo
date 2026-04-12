import { useCallback, useEffect, useState } from "react";
import {
  MessageSquare, Clock, ThumbsUp, ThumbsDown,
  TrendingUp, Users, RefreshCw, Shield,
} from "lucide-react";
import { motion } from "motion/react";
import {
  fetchSummary, fetchDaily, fetchTopQueries, fetchFeedback, fetchAudit,
  type AnalyticsSummary, type DailyStat, type TopQuery, type FeedbackStat, type AuditResponse,
} from "./api";
import { AnimatedSection, StatCard, GlassCard, Badge, TabLoader } from "./ui";
import { cn } from "@/lib/utils";

// Animated counter for stat values
function AnimCount({ target }: { target: number }) {
  const [val, setVal] = useState(0);
  useEffect(() => {
    const duration = 1200;
    const start = performance.now();
    const tick = (now: number) => {
      const p = Math.min((now - start) / duration, 1);
      const eased = 1 - Math.pow(1 - p, 3);
      setVal(Math.floor(eased * target));
      if (p < 1) requestAnimationFrame(tick);
    };
    requestAnimationFrame(tick);
  }, [target]);
  return <>{val}</>;
}

export function OverviewTab() {
  const [summary, setSummary] = useState<AnalyticsSummary | null>(null);
  const [daily, setDaily] = useState<DailyStat[]>([]);
  const [topQ, setTopQ] = useState<TopQuery[]>([]);
  const [fb, setFb] = useState<FeedbackStat | null>(null);
  const [audit, setAudit] = useState<AuditResponse | null>(null);
  const [days, setDays] = useState(30);
  const [loading, setLoading] = useState(true);

  const load = useCallback(async () => {
    setLoading(true);
    try {
      const [s, d, q, f, a] = await Promise.all([
        fetchSummary(days), fetchDaily(days), fetchTopQueries(days, 10), fetchFeedback(days), fetchAudit(0, 5),
      ]);
      setSummary(s); setDaily(d); setTopQ(q); setFb(f); setAudit(a);
    } catch (e) { console.error(e); }
    finally { setLoading(false); }
  }, [days]);

  useEffect(() => { load(); }, [load]);

  if (loading) return <TabLoader />;

  return (
    <div className="space-y-6">
      {/* Header + period filter */}
      <AnimatedSection i={0} className="flex flex-wrap items-center justify-between gap-4">
        <div>
          <h2 className="text-2xl font-bold tracking-tight text-white">Аналітика</h2>
          <p className="mt-0.5 text-xs text-zinc-600">Статистика використання чат-бота</p>
        </div>
        <div className="flex items-center gap-1 rounded-xl bg-white/[0.03] p-1 ring-1 ring-white/[0.06]">
          {[7, 14, 30].map((d) => (
            <button
              key={d}
              onClick={() => setDays(d)}
              className={cn(
                "relative rounded-lg px-3.5 py-1.5 text-xs font-medium transition-all",
                days === d ? "text-white" : "text-zinc-500 hover:text-zinc-300",
              )}
            >
              {days === d && (
                <motion.div
                  layoutId="period-active"
                  className="absolute inset-0 rounded-lg bg-blue-500/20 ring-1 ring-blue-500/30"
                  transition={{ type: "spring", stiffness: 400, damping: 30 }}
                />
              )}
              <span className="relative z-10">{d}д</span>
            </button>
          ))}
          <button onClick={load} className="ml-1.5 rounded-lg p-1.5 text-zinc-600 transition-colors hover:bg-white/[0.04] hover:text-zinc-300" title="Оновити">
            <RefreshCw size={14} />
          </button>
        </div>
      </AnimatedSection>

      {/* Info Block */}
      <AnimatedSection i={0.5}>
        <div className="rounded-xl border border-blue-500/20 bg-blue-500/5 p-4 text-[13px] leading-relaxed text-blue-100/80">
          <strong className="text-blue-300">Як користуватися цією сторінкою:</strong> Тут зібрана загальна статистика роботи чат-бота. 
          Ви можете перевірити популярність бота (кількість запитів), його швидкість та оцінки користувачів (лайки 👍 / дизлайки 👎). 
          Гістограма відображає активність по днях, а блок «Топ запити» допоможе зрозуміти, що найчастіше цікавить ваших абітурієнтів чи студентів.
        </div>
      </AnimatedSection>

      {/* Stat Cards */}
      {summary && (
        <div className="grid grid-cols-1 gap-4 sm:grid-cols-2 lg:grid-cols-4">
          <AnimatedSection i={1}>
            <StatCard icon={MessageSquare} label="Всього запитів" value={<AnimCount target={summary.total_queries} />} accent="blue" />
          </AnimatedSection>
          <AnimatedSection i={2}>
            <StatCard icon={Clock} label="Сер. відповідь" value={`${(summary.avg_response_ms / 1000).toFixed(1)}с`} accent="cyan" />
          </AnimatedSection>
          <AnimatedSection i={3}>
            <StatCard icon={ThumbsUp} label="Позитивних" value={<AnimCount target={fb?.positive ?? 0} />} accent="green" />
          </AnimatedSection>
          <AnimatedSection i={4}>
            <StatCard icon={ThumbsDown} label="Негативних" value={<AnimCount target={fb?.negative ?? 0} />} accent="red" />
          </AnimatedSection>
        </div>
      )}

      {/* Bar Chart */}
      {daily.length > 0 && (
        <AnimatedSection i={5}>
          <GlassCard title="Запити по днях" icon={TrendingUp}>
            <div className="flex h-48 items-end gap-0.5 pt-2">
              {daily.map((d, i) => {
                const max = Math.max(...daily.map((s) => s.total_queries), 1);
                const pct = (d.total_queries / max) * 100;
                return (
                  <motion.div
                    key={d.date}
                    className="group relative flex flex-1 flex-col items-center"
                    initial={{ scaleY: 0 }}
                    animate={{ scaleY: 1 }}
                    transition={{ delay: 0.4 + i * 0.025, duration: 0.5, ease: [0.25, 0.46, 0.45, 0.94] }}
                    style={{ transformOrigin: "bottom" }}
                  >
                    {/* Tooltip */}
                    <div className="pointer-events-none absolute -top-11 z-20 hidden rounded-lg bg-zinc-800/95 px-2.5 py-1.5 text-[10px] text-zinc-200 shadow-xl ring-1 ring-white/5 group-hover:block whitespace-nowrap backdrop-blur-sm">
                      <div className="font-medium">{d.date}</div>
                      <div className="text-zinc-400">{d.total_queries} запитів</div>
                    </div>
                    <div
                      className="w-full max-w-[24px] rounded-t-md bg-gradient-to-t from-blue-600/80 to-blue-400/90 transition-all duration-200 group-hover:from-blue-500 group-hover:to-blue-300 group-hover:shadow-lg group-hover:shadow-blue-500/20"
                      style={{ height: `${Math.max(pct, 3)}%` }}
                    />
                    {i % Math.max(1, Math.floor(daily.length / 6)) === 0 && (
                      <span className="mt-2 text-[9px] text-zinc-700 truncate max-w-[36px]">{d.date.slice(5)}</span>
                    )}
                  </motion.div>
                );
              })}
            </div>
          </GlassCard>
        </AnimatedSection>
      )}

      <div className="grid grid-cols-1 gap-4 lg:grid-cols-2">
        {/* Top Queries */}
        {topQ.length > 0 && (
          <AnimatedSection i={6}>
            <GlassCard title="Топ запити" icon={Users}>
              <div className="space-y-0">
                {topQ.slice(0, 6).map((q, i) => (
                  <div key={q.query_hash} className="flex items-center gap-3 border-b border-white/[0.04] py-2.5 last:border-0">
                    <span className="flex h-6 w-6 shrink-0 items-center justify-center rounded-md bg-zinc-800/80 text-[10px] font-bold text-zinc-500">{i + 1}</span>
                    <span className="flex-1 truncate font-medium text-[13px] text-zinc-300" title={q.query_hash}>{q.query_hash}</span>
                    <Badge>{q.count}×</Badge>
                  </div>
                ))}
              </div>
            </GlassCard>
          </AnimatedSection>
        )}

        {/* Feedback + Audit mini */}
        <div className="space-y-4">
          {/* Feedback Ratio */}
          {fb && fb.total > 0 && (
            <AnimatedSection i={7}>
              <GlassCard title="Задоволеність" icon={ThumbsUp}>
                <div className="space-y-3">
                  <div className="flex h-8 overflow-hidden rounded-xl text-xs font-medium">
                    <motion.div
                      initial={{ width: 0 }}
                      animate={{ width: `${fb.ratio * 100}%` }}
                      transition={{ duration: 1, ease: "easeOut" }}
                      className="flex items-center justify-center bg-gradient-to-r from-emerald-600 to-emerald-400 text-white"
                      style={{ minWidth: 40 }}
                    >
                      {(fb.ratio * 100).toFixed(0)}% 👍
                    </motion.div>
                    <motion.div
                      initial={{ width: 0 }}
                      animate={{ width: `${(1 - fb.ratio) * 100}%` }}
                      transition={{ duration: 1, ease: "easeOut", delay: 0.1 }}
                      className="flex items-center justify-center bg-gradient-to-r from-red-600 to-red-400 text-white"
                      style={{ minWidth: 40 }}
                    >
                      {((1 - fb.ratio) * 100).toFixed(0)}% 👎
                    </motion.div>
                  </div>
                  <div className="flex justify-between text-[11px] text-zinc-600">
                    <span>Позитивних: {fb.positive}</span>
                    <span>Негативних: {fb.negative}</span>
                  </div>
                </div>
              </GlassCard>
            </AnimatedSection>
          )}

          {/* Mini audit log */}
          {audit && audit.entries.length > 0 && (
            <AnimatedSection i={8}>
              <GlassCard title="Лог адміністратора" icon={Shield}>
                <div className="space-y-0">
                  {audit.entries.slice(0, 5).map((e) => (
                    <div key={e.id} className="flex items-center gap-2 border-b border-white/[0.03] py-2 last:border-0 text-xs">
                      <Badge color="purple">{e.action}</Badge>
                      <span className="flex-1 truncate text-zinc-500">{e.admin_email}</span>
                      <span className="text-[10px] text-zinc-700">{new Date(e.created_at).toLocaleDateString("uk-UA")}</span>
                    </div>
                  ))}
                </div>
              </GlassCard>
            </AnimatedSection>
          )}
        </div>
      </div>
    </div>
  );
}
