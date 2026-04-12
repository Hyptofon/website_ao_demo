import { useEffect, useState } from "react";
import { Settings, MessageSquare } from "lucide-react";
import { fetchPrompts, fetchSuggestions, type PromptVariant, type SuggestedQuestion } from "./api";
import { AnimatedSection, GlassCard, Badge, TabLoader } from "./ui";

export function SettingsTab() {
  const [prompts, setPrompts] = useState<PromptVariant[]>([]);
  const [suggestions, setSuggestions] = useState<SuggestedQuestion[]>([]);
  const [loading, setLoading] = useState(true);

  useEffect(() => {
    (async () => {
      try {
        const [p, s] = await Promise.all([fetchPrompts(), fetchSuggestions()]);
        setPrompts(p); setSuggestions(s);
      } catch (e) { console.error(e); }
      finally { setLoading(false); }
    })();
  }, []);

  if (loading) return <TabLoader />;

  return (
    <div className="space-y-6">
      <AnimatedSection>
        <h2 className="text-xl font-bold text-white">Налаштування</h2>
      </AnimatedSection>

      {/* Prompt Variants */}
      <AnimatedSection delay={0.1}>
        <GlassCard title="A/B тестування промптів" icon={Settings}>
          {prompts.length === 0 ? (
            <p className="py-4 text-xs text-zinc-600">Варіантів ще немає. Створіть через POST /admin/prompts</p>
          ) : (
            <div className="overflow-x-auto">
              <table className="w-full text-sm">
                <thead>
                  <tr className="border-b border-white/[0.06] text-left text-[11px] font-medium uppercase tracking-wider text-zinc-600">
                    <th className="px-3 py-2">Назва</th>
                    <th className="px-3 py-2">Мова</th>
                    <th className="px-3 py-2">Активний</th>
                    <th className="px-3 py-2">Використань</th>
                    <th className="px-3 py-2">Оцінка</th>
                  </tr>
                </thead>
                <tbody>
                  {prompts.map((p) => (
                    <tr key={p.id} className="border-b border-white/[0.03] hover:bg-white/[0.02]">
                      <td className="px-3 py-2.5 font-medium text-zinc-300">{p.name}</td>
                      <td className="px-3 py-2.5">{p.language === "uk" ? "🇺🇦" : "🇬🇧"}</td>
                      <td className="px-3 py-2.5">{p.is_active
                        ? <Badge color="green">Active</Badge>
                        : <Badge color="gray">Off</Badge>}
                      </td>
                      <td className="px-3 py-2.5 text-zinc-400">{p.usage_count}</td>
                      <td className="px-3 py-2.5 font-mono text-xs text-zinc-400">{p.avg_score.toFixed(2)}</td>
                    </tr>
                  ))}
                </tbody>
              </table>
            </div>
          )}
        </GlassCard>
      </AnimatedSection>

      {/* Suggestions */}
      <AnimatedSection delay={0.2}>
        <GlassCard title="Запропоновані питання" icon={MessageSquare}>
          {suggestions.length === 0 ? (
            <p className="py-4 text-xs text-zinc-600">Питань ще немає. Додайте через POST /admin/suggestions</p>
          ) : (
            <ul className="divide-y divide-white/[0.04]">
              {suggestions.map((s) => (
                <li key={s.id} className="flex items-center justify-between px-1 py-3">
                  <span className="text-sm text-zinc-300">{s.question}</span>
                  <Badge color={s.is_auto ? "purple" : "blue"}>{s.is_auto ? "auto" : "manual"}</Badge>
                </li>
              ))}
            </ul>
          )}
        </GlassCard>
      </AnimatedSection>
    </div>
  );
}
