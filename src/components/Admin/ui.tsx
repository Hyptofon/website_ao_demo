// Shared UI primitives for admin panel — all Tailwind, no custom CSS.

import { cn } from "@/lib/utils";
import { RefreshCw, type LucideIcon } from "lucide-react";
import { motion } from "motion/react";
import type { ReactNode } from "react";

// ─── Animated Container ─────────────────────────────────────────────────────

export function AnimatedSection({
  children,
  className,
  delay = 0,
}: {
  children: ReactNode;
  className?: string;
  delay?: number;
}) {
  return (
    <motion.div
      initial={{ opacity: 0, y: 12 }}
      animate={{ opacity: 1, y: 0 }}
      transition={{ duration: 0.4, delay, ease: "easeOut" }}
      className={className}
    >
      {children}
    </motion.div>
  );
}

// ─── Stat Card ──────────────────────────────────────────────────────────────

const accentMap: Record<string, string> = {
  blue: "bg-brand-blue/15 text-brand-blue",
  cyan: "bg-brand-cyan/15 text-brand-cyan",
  green: "bg-emerald-500/15 text-emerald-400",
  red: "bg-red-500/15 text-red-400",
  purple: "bg-purple-500/15 text-purple-400",
};

export function StatCard({
  icon: Icon,
  label,
  value,
  accent = "blue",
  subtitle,
}: {
  icon: LucideIcon;
  label: string;
  value: string | number;
  accent?: string;
  subtitle?: string;
}) {
  return (
    <motion.div
      whileHover={{ scale: 1.02, y: -2 }}
      transition={{ type: "spring", stiffness: 400, damping: 20 }}
      className="relative overflow-hidden rounded-xl border border-white/[0.06] bg-dark-panel p-5 backdrop-blur-sm"
    >
      {/* Subtle glow */}
      <div className="absolute -top-12 -right-12 h-24 w-24 rounded-full bg-brand-blue/5 blur-2xl" />
      <div className="flex items-center gap-4">
        <div className={cn("flex h-11 w-11 shrink-0 items-center justify-center rounded-lg", accentMap[accent])}>
          <Icon size={20} />
        </div>
        <div className="min-w-0">
          <div className="text-2xl font-bold text-white tracking-tight">{value}</div>
          <div className="text-xs text-zinc-500">{label}</div>
          {subtitle && <div className="text-[10px] text-zinc-600 mt-0.5">{subtitle}</div>}
        </div>
      </div>
    </motion.div>
  );
}

// ─── Glass Card ─────────────────────────────────────────────────────────────

export function GlassCard({
  children,
  className,
  title,
  icon: Icon,
}: {
  children: ReactNode;
  className?: string;
  title?: string;
  icon?: LucideIcon;
}) {
  return (
    <div className={cn("rounded-xl border border-white/[0.06] bg-dark-panel p-5", className)}>
      {title && (
        <h3 className="mb-4 flex items-center gap-2 text-sm font-semibold text-zinc-200">
          {Icon && <Icon size={16} className="text-brand-blue-soft" />}
          {title}
        </h3>
      )}
      {children}
    </div>
  );
}

// ─── Badge ──────────────────────────────────────────────────────────────────

const badgeColors: Record<string, string> = {
  blue: "bg-brand-blue/12 text-blue-400",
  green: "bg-emerald-500/12 text-emerald-400",
  red: "bg-red-500/12 text-red-400",
  cyan: "bg-brand-cyan/12 text-brand-cyan",
  purple: "bg-purple-500/12 text-purple-400",
  gray: "bg-zinc-500/12 text-zinc-400",
};

export function Badge({ children, color = "blue" }: { children: ReactNode; color?: string }) {
  return (
    <span className={cn("inline-flex items-center rounded-md px-2 py-0.5 text-xs font-medium", badgeColors[color])}>
      {children}
    </span>
  );
}

// ─── Tab Loader ─────────────────────────────────────────────────────────────

export function TabLoader() {
  return (
    <div className="flex flex-col items-center justify-center gap-3 py-20 text-zinc-500">
      <RefreshCw className="animate-spin" size={24} />
      <p className="text-sm">Завантаження...</p>
    </div>
  );
}

// ─── Empty State ────────────────────────────────────────────────────────────

export function EmptyState({
  icon: Icon,
  title,
  description,
}: {
  icon: LucideIcon;
  title: string;
  description?: string;
}) {
  return (
    <div className="flex flex-col items-center justify-center gap-3 py-16 text-center text-zinc-500">
      <Icon size={48} className="text-zinc-700" />
      <p className="text-sm font-medium">{title}</p>
      {description && <p className="text-xs text-zinc-600">{description}</p>}
    </div>
  );
}
