import {
  BarChart3, FileText, Shield, Settings,
  LogOut, Activity,
} from "lucide-react";
import { cn } from "@/lib/utils";

export type Tab = "overview" | "documents" | "audit" | "settings";

const NAV: { id: Tab; label: string; icon: React.ReactNode }[] = [
  { id: "overview", label: "Аналітика", icon: <BarChart3 size={18} /> },
  { id: "documents", label: "Документи", icon: <FileText size={18} /> },
  { id: "audit", label: "Журнал дій", icon: <Shield size={18} /> },
  { id: "settings", label: "Налаштування", icon: <Settings size={18} /> },
];

export function Sidebar({
  active,
  onChange,
  onLogout,
}: {
  active: Tab;
  onChange: (t: Tab) => void;
  onLogout: () => void;
}) {
  return (
    <aside className="fixed inset-y-0 left-0 z-50 flex w-[220px] flex-col border-r border-white/[0.06] bg-dark-panel md:w-[240px]">
      {/* Logo */}
      <div className="flex items-center gap-2.5 border-b border-white/[0.06] px-5 py-4">
        <Activity size={22} className="text-brand-blue" />
        <span className="text-base font-semibold text-white hidden md:inline">Admin Panel</span>
      </div>

      {/* Nav */}
      <nav className="flex flex-1 flex-col gap-0.5 px-2.5 py-3">
        {NAV.map((item) => (
          <button
            key={item.id}
            onClick={() => onChange(item.id)}
            className={cn(
              "flex w-full items-center gap-2.5 rounded-lg px-3 py-2.5 text-left text-[13px] font-medium transition-all",
              active === item.id
                ? "bg-brand-blue/12 text-brand-blue-soft"
                : "text-zinc-500 hover:bg-white/[0.04] hover:text-zinc-300",
            )}
          >
            <span className={cn(active === item.id && "text-brand-blue")}>{item.icon}</span>
            <span className="hidden md:inline">{item.label}</span>
          </button>
        ))}
      </nav>

      {/* Logout */}
      <div className="border-t border-white/[0.06] px-2.5 py-3">
        <button
          onClick={onLogout}
          className="flex w-full items-center gap-2.5 rounded-lg px-3 py-2.5 text-[13px] text-zinc-500 transition-colors hover:text-red-400"
        >
          <LogOut size={18} />
          <span className="hidden md:inline">Вийти</span>
        </button>
      </div>
    </aside>
  );
}
