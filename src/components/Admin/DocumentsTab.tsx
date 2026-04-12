import { useCallback, useEffect, useState, useRef } from "react";
import { FileText, Upload, Trash2, FileUp, Eye, PencilLine, MoreVertical } from "lucide-react";
import { toast } from "sonner";
import { motion, AnimatePresence } from "motion/react";
import { fetchDocuments, uploadDocument, deleteDocument, renameDocument, getDocumentDownloadUrl, type DocumentRecord } from "./api";
import { AnimatedSection, GlassCard, Badge, TabLoader, EmptyState } from "./ui";

function DocumentRow({ d, onDelete, onRename, onPreview }: { d: DocumentRecord, onDelete: (id: string, n: string) => void, onRename: (id: string, oldName: string, newName: string) => void, onPreview: (id: string) => void }) {
  const [isRenaming, setIsRenaming] = useState(false);
  const [editName, setEditName] = useState(d.filename);
  const [menuOpen, setMenuOpen] = useState(false);
  const menuRef = useRef<HTMLDivElement>(null);

  useEffect(() => {
    const handleClickOutside = (e: MouseEvent) => {
      if (menuRef.current && !menuRef.current.contains(e.target as Node)) {
        setMenuOpen(false);
      }
    };
    if (menuOpen) document.addEventListener("mousedown", handleClickOutside);
    return () => document.removeEventListener("mousedown", handleClickOutside);
  }, [menuOpen]);

  const handleSave = () => {
    setIsRenaming(false);
    if (editName.trim() && editName !== d.filename) {
      onRename(d.id, d.filename, editName.trim());
    } else {
      setEditName(d.filename);
    }
  };

  const handleKeyDown = (e: React.KeyboardEvent) => {
    if (e.key === "Enter") handleSave();
    if (e.key === "Escape") {
      setIsRenaming(false);
      setEditName(d.filename);
    }
  };

  return (
    <motion.tr
      key={d.id}
      initial={{ opacity: 0, x: -8 }}
      animate={{ opacity: 1, x: 0 }}
      exit={{ opacity: 0, x: 8 }}
      className="border-b border-white/[0.03] transition-colors hover:bg-white/[0.02]"
    >
      <td className="flex items-center gap-2.5 px-3 py-3 font-medium text-zinc-300">
        <div className="flex h-8 w-8 shrink-0 items-center justify-center rounded-lg bg-blue-500/10">
          <FileText size={14} className="text-blue-400" />
        </div>
        {isRenaming ? (
          <input
            autoFocus
            type="text"
            className="h-7 w-[180px] rounded bg-white/5 border border-white/10 px-2 text-sm text-zinc-200 outline-none focus:border-blue-500/50 focus:ring-1 focus:ring-blue-500/50"
            value={editName}
            onChange={(e) => setEditName(e.target.value)}
            onBlur={handleSave}
            onKeyDown={handleKeyDown}
          />
        ) : (
          <span className="truncate max-w-[180px] cursor-pointer hover:text-blue-400 transition-colors" onClick={() => setIsRenaming(true)} title="Клікніть для перейменування">
            {d.filename}
          </span>
        )}
      </td>
      <td className="px-3 py-3"><Badge color="cyan">{d.doc_type || "pdf"}</Badge></td>
      <td className="px-3 py-3">{d.language === "uk" ? "🇺🇦" : "🇬🇧"}</td>
      <td className="px-3 py-3 text-zinc-400 tabular-nums">{d.chunk_count}</td>
      <td className="px-3 py-3 text-xs text-zinc-600">{d.uploaded_at?.slice(0, 10) ?? "—"}</td>
      <td className="px-3 py-3 text-right">
        <div className="relative inline-block text-left" ref={menuRef}>
          <button
            onClick={() => setMenuOpen(!menuOpen)}
            className="rounded-lg p-1.5 text-zinc-500 transition-all hover:bg-white/5 hover:text-zinc-300 focus:outline-none"
          >
            <MoreVertical size={16} />
          </button>
          
          <AnimatePresence>
            {menuOpen && (
              <motion.div
                initial={{ opacity: 0, scale: 0.95, y: -5 }}
                animate={{ opacity: 1, scale: 1, y: 0 }}
                exit={{ opacity: 0, scale: 0.95, y: -5 }}
                transition={{ duration: 0.15 }}
                className="absolute right-0 top-full z-50 mt-1 w-48 overflow-hidden rounded-xl border border-white/10 bg-zinc-900/95 shadow-xl backdrop-blur-xl"
              >
                <div className="flex flex-col p-1">
                  <button
                    onClick={() => { setMenuOpen(false); onPreview(d.id); }}
                    className="flex w-full items-center gap-2 rounded-lg px-3 py-2 text-sm text-zinc-300 transition-colors hover:bg-white/5"
                  >
                    <Eye size={15} className="text-blue-400" />
                    Переглянути
                  </button>
                  <button
                    onClick={() => { setMenuOpen(false); setIsRenaming(true); }}
                    className="flex w-full items-center gap-2 rounded-lg px-3 py-2 text-sm text-zinc-300 transition-colors hover:bg-white/5"
                  >
                    <PencilLine size={15} className="text-yellow-500" />
                    Перейменувати
                  </button>
                  <div className="my-1 h-px w-full bg-white/5" />
                  <button
                    onClick={() => { setMenuOpen(false); onDelete(d.id, d.filename); }}
                    className="flex w-full items-center gap-2 rounded-lg px-3 py-2 text-sm text-red-400 transition-colors hover:bg-red-500/10"
                  >
                    <Trash2 size={15} />
                    Видалити
                  </button>
                </div>
              </motion.div>
            )}
          </AnimatePresence>
        </div>
      </td>
    </motion.tr>
  );
}

export function DocumentsTab() {
  const [docs, setDocs] = useState<DocumentRecord[]>([]);
  const [loading, setLoading] = useState(true);
  const [uploading, setUploading] = useState(false);
  const [dragging, setDragging] = useState(false);

  const load = useCallback(async () => {
    setLoading(true);
    try { setDocs(await fetchDocuments()); }
    catch { toast.error("Не вдалось завантажити документи"); }
    finally { setLoading(false); }
  }, []);

  useEffect(() => { load(); }, [load]);

  const doUpload = async (file: File) => {
    setUploading(true);
    try {
      await uploadDocument(file);
      toast.success(`${file.name} завантажено`);
      await load();
    } catch (err: any) { 
      toast.error(err?.message || "Помилка завантаження"); 
    }
    finally { setUploading(false); }
  };

  const handleUpload = async (e: React.ChangeEvent<HTMLInputElement>) => {
    const file = e.target.files?.[0];
    if (file) await doUpload(file);
    e.target.value = "";
  };

  const handleDrop = async (e: React.DragEvent) => {
    e.preventDefault();
    setDragging(false);
    const file = e.dataTransfer.files?.[0];
    if (file) await doUpload(file);
  };

  const handleDelete = async (id: string, name: string) => {
    if (!confirm(`Видалити "${name}"?`)) return;
    try {
      await deleteDocument(id);
      setDocs((prev) => prev.filter((d) => d.id !== id));
      toast.success(`${name} видалено`);
    } catch { toast.error("Помилка видалення"); }
  };

  const handleRename = async (id: string, currentName: string) => {
    const newName = prompt("Введіть нову назву для документу:", currentName);
    if (!newName || newName === currentName) return;
    try {
      await renameDocument(id, newName);
      setDocs((prev) => prev.map((d) => d.id === id ? { ...d, filename: newName } : d));
      toast.success("Документ розпізнано і перейменовано!");
    } catch {
      toast.error("Помилка при перейменуванні");
    }
  };

  const handlePreview = (id: string) => {
    const token = localStorage.getItem("admin_jwt");
    // Append token as query param so we can view in a new tab without modifying fetch headers
    // Actually simpler to just open it, the download endpoint isn't strictly gated by JWT if we allow CORS, but wait!
    // Since we open a new window, headers are lost. We might get 401 if it's protected.
    // We should fetch it as BLOB and create object URL.
    fetch(getDocumentDownloadUrl(id), {
      headers: { "Authorization": `Bearer ${token}` }
    })
      .then(r => {
        if (!r.ok) {
           if (r.status === 404) throw new Error("Файл не знайдено (можливо, це старий документ)");
           throw new Error("Помилка доступу");
        }
        return r.blob();
      })
      .then(blob => {
        const url = URL.createObjectURL(blob);
        window.open(url, "_blank");
        setTimeout(() => URL.revokeObjectURL(url), 10000);
      })
      .catch((e) => toast.error(e.message));
  };

  if (loading) return <TabLoader />;

  return (
    <div className="space-y-6">
      <AnimatedSection i={0} className="flex flex-wrap items-center justify-between gap-4">
        <div>
          <h2 className="text-2xl font-bold tracking-tight text-white">Документи бази знань</h2>
          <p className="mt-0.5 text-xs text-zinc-600">{docs.length} документів у базі</p>
        </div>
        <label className="inline-flex cursor-pointer items-center gap-2 rounded-xl bg-gradient-to-r from-blue-600 to-blue-500 px-4 py-2.5 text-sm font-medium text-white shadow-lg shadow-blue-600/15 transition-all hover:shadow-blue-600/25 hover:from-blue-500 hover:to-blue-400">
          <Upload size={16} />
          {uploading ? "Завантаження..." : "Завантажити"}
          <input type="file" accept=".pdf,.docx,.xlsx,.txt" onChange={handleUpload} hidden disabled={uploading} />
        </label>
      </AnimatedSection>

      {/* Info Block */}
      <AnimatedSection i={0.5}>
        <div className="rounded-xl border border-blue-500/20 bg-blue-500/5 p-4 text-[13px] leading-relaxed text-blue-100/80">
          <strong className="text-blue-300">Як користуватися цією сторінкою:</strong> Ця сторінка — це «мозок» вашого чат-бота. 
          Завантажуйте сюди офіційні документи, загальні положення, накази та довідники (підтримуються PDF, DOCX, XLSX, TXT). 
          Бот зможе читати ці файли та надавати точні відповіді студентам на їх основі. Чим актуальніші документи, тим якісніша допомога бота.
        </div>
      </AnimatedSection>

      {/* Drop zone */}
      <AnimatedSection i={1}>
        <div
          onDragOver={(e) => { e.preventDefault(); setDragging(true); }}
          onDragLeave={() => setDragging(false)}
          onDrop={handleDrop}
          className={`relative flex flex-col items-center justify-center gap-3 rounded-2xl border-2 border-dashed py-12 transition-all duration-300 ${
            dragging
              ? "border-blue-400/50 bg-blue-500/5"
              : "border-white/[0.06] bg-transparent hover:border-white/[0.12]"
          }`}
        >
          <FileUp size={32} className={`transition-colors ${dragging ? "text-blue-400" : "text-zinc-700"}`} />
          <p className="text-sm text-zinc-500">
            Перетягніть файл або{" "}
            <label className="cursor-pointer text-blue-400 hover:underline">
              оберіть
              <input type="file" accept=".pdf,.docx,.xlsx,.txt" onChange={handleUpload} hidden disabled={uploading} />
            </label>
          </p>
          <p className="text-[10px] text-zinc-700">PDF, DOCX, XLSX, TXT</p>
        </div>
      </AnimatedSection>

      {docs.length === 0 ? (
        <AnimatedSection i={2}>
          <EmptyState icon={FileText} title="Документів ще немає" description="Завантажте файл, щоб додати до бази знань" />
        </AnimatedSection>
      ) : (
        <AnimatedSection i={2}>
          <GlassCard>
            <div className="overflow-x-auto">
              <table className="w-full text-sm">
                <thead>
                  <tr className="border-b border-white/[0.06] text-left text-[11px] font-semibold uppercase tracking-wider text-zinc-600">
                    <th className="px-3 py-2.5">Файл</th>
                    <th className="px-3 py-2.5">Тип</th>
                    <th className="px-3 py-2.5">Мова</th>
                    <th className="px-3 py-2.5">Чанків</th>
                    <th className="px-3 py-2.5">Дата</th>
                    <th className="px-3 py-2.5 w-10" />
                  </tr>
                </thead>
                <tbody>
                  <AnimatePresence>
                    {docs.map((d) => (
                      <DocumentRow
                        key={d.id}
                        d={d}
                        onRename={handleRename}
                        onDelete={handleDelete}
                        onPreview={handlePreview}
                      />
                    ))}
                  </AnimatePresence>
                </tbody>
              </table>
            </div>
          </GlassCard>
        </AnimatedSection>
      )}
    </div>
  );
}
