import { useCallback, useEffect, useState } from "react";
import { FileText, Upload, Trash2 } from "lucide-react";
import { toast } from "sonner";
import { fetchDocuments, uploadDocument, deleteDocument, type DocumentRecord } from "./api";
import { AnimatedSection, GlassCard, Badge, TabLoader, EmptyState } from "./ui";

export function DocumentsTab() {
  const [docs, setDocs] = useState<DocumentRecord[]>([]);
  const [loading, setLoading] = useState(true);
  const [uploading, setUploading] = useState(false);

  const load = useCallback(async () => {
    setLoading(true);
    try { setDocs(await fetchDocuments()); }
    catch { toast.error("Не вдалось завантажити документи"); }
    finally { setLoading(false); }
  }, []);

  useEffect(() => { load(); }, [load]);

  const handleUpload = async (e: React.ChangeEvent<HTMLInputElement>) => {
    const file = e.target.files?.[0];
    if (!file) return;
    setUploading(true);
    try {
      await uploadDocument(file);
      toast.success(`${file.name} завантажено`);
      await load();
    } catch { toast.error("Помилка завантаження"); }
    finally { setUploading(false); e.target.value = ""; }
  };

  const handleDelete = async (id: string, name: string) => {
    if (!confirm(`Видалити "${name}"?`)) return;
    try {
      await deleteDocument(id);
      setDocs((prev) => prev.filter((d) => d.id !== id));
      toast.success(`${name} видалено`);
    } catch { toast.error("Помилка видалення"); }
  };

  if (loading) return <TabLoader />;

  return (
    <div className="space-y-6">
      <AnimatedSection className="flex flex-wrap items-center justify-between gap-3">
        <h2 className="text-xl font-bold text-white">Документи бази знань</h2>
        <label className="inline-flex cursor-pointer items-center gap-2 rounded-lg bg-brand-blue px-4 py-2 text-sm font-medium text-white transition-colors hover:bg-brand-blue-deep">
          <Upload size={16} />
          {uploading ? "Завантаження..." : "Завантажити"}
          <input type="file" accept=".pdf,.docx,.xlsx,.txt" onChange={handleUpload} hidden disabled={uploading} />
        </label>
      </AnimatedSection>

      {docs.length === 0 ? (
        <AnimatedSection delay={0.1}>
          <EmptyState icon={FileText} title="Документів ще немає" description="Завантажте PDF, DOCX або TXT файл" />
        </AnimatedSection>
      ) : (
        <AnimatedSection delay={0.1}>
          <GlassCard>
            <div className="overflow-x-auto">
              <table className="w-full text-sm">
                <thead>
                  <tr className="border-b border-white/[0.06] text-left text-[11px] font-medium uppercase tracking-wider text-zinc-600">
                    <th className="px-3 py-2">Файл</th>
                    <th className="px-3 py-2">Тип</th>
                    <th className="px-3 py-2">Мова</th>
                    <th className="px-3 py-2">Чанків</th>
                    <th className="px-3 py-2">Дата</th>
                    <th className="px-3 py-2 w-10" />
                  </tr>
                </thead>
                <tbody>
                  {docs.map((d) => (
                    <tr key={d.id} className="border-b border-white/[0.03] transition-colors hover:bg-white/[0.02]">
                      <td className="flex items-center gap-2 px-3 py-2.5 font-medium text-zinc-300">
                        <FileText size={15} className="shrink-0 text-brand-blue-soft" />
                        {d.filename}
                      </td>
                      <td className="px-3 py-2.5"><Badge color="cyan">{d.doc_type}</Badge></td>
                      <td className="px-3 py-2.5">{d.language === "uk" ? "🇺🇦" : "🇬🇧"}</td>
                      <td className="px-3 py-2.5 text-zinc-400">{d.chunk_count}</td>
                      <td className="px-3 py-2.5 text-xs text-zinc-600">{d.uploaded_at?.slice(0, 10) ?? "—"}</td>
                      <td className="px-3 py-2.5">
                        <button
                          onClick={() => handleDelete(d.id, d.filename)}
                          className="rounded-md p-1.5 text-zinc-600 transition-colors hover:bg-red-500/10 hover:text-red-400"
                          title="Видалити"
                        >
                          <Trash2 size={15} />
                        </button>
                      </td>
                    </tr>
                  ))}
                </tbody>
              </table>
            </div>
          </GlassCard>
        </AnimatedSection>
      )}
    </div>
  );
}
