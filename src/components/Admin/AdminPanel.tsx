// AdminPanel — root orchestrator.
// Uses client:only="react" in Astro for full client-side rendering.

import { useState, useEffect } from "react";
import { Toaster } from "sonner";
import { AnimatePresence, motion } from "motion/react";
import { getToken, clearToken } from "./api";
import { LoginScreen } from "./LoginScreen";
import { Sidebar, type Tab } from "./Sidebar";
import { OverviewTab } from "./OverviewTab";
import { DocumentsTab } from "./DocumentsTab";
import { QueriesTab } from "./QueriesTab";
import { RefreshCw } from "lucide-react";

export default function AdminPanel() {
  const [tab, setTab] = useState<Tab>("overview");
  const [authed, setAuthed] = useState(false);
  const [ready, setReady] = useState(false);

  useEffect(() => {
    // Check for token in URL (OAuth redirect) or localStorage
    const params = new URLSearchParams(window.location.search);
    const urlToken = params.get("token");
    if (urlToken) {
      localStorage.setItem("admin_jwt", urlToken);
      window.history.replaceState({}, "", "/admin");
    }
    setAuthed(!!getToken());
    setReady(true);
  }, []);

  if (!ready) {
    return (
      <div className="flex min-h-screen items-center justify-center bg-[#0a0c0f]">
        <motion.div
          animate={{ rotate: 360 }}
          transition={{ repeat: Infinity, duration: 1, ease: "linear" }}
        >
          <RefreshCw size={28} className="text-zinc-600" />
        </motion.div>
      </div>
    );
  }

  if (!authed) {
    return (
      <>
        <Toaster
          theme="dark"
          toastOptions={{ classNames: { toast: "!bg-zinc-900 !border-zinc-800 !text-zinc-200" } }}
        />
        <LoginScreen onAuth={() => setAuthed(true)} />
      </>
    );
  }

  return (
    <div className="flex min-h-screen bg-[#0a0c0f] font-[Roboto,system-ui,sans-serif] text-zinc-300 antialiased">
      <Toaster
        theme="dark"
        toastOptions={{ classNames: { toast: "!bg-zinc-900 !border-zinc-800 !text-zinc-200" } }}
      />

      <Sidebar
        active={tab}
        onChange={setTab}
        onLogout={() => { clearToken(); setAuthed(false); }}
      />

      {/* Main content */}
      <main className="ml-[220px] flex-1 overflow-y-auto p-6 md:ml-[240px] md:p-8">
        <div className="mx-auto max-w-[1100px]">
          <AnimatePresence mode="wait">
            <motion.div
              key={tab}
              initial={{ opacity: 0, y: 8 }}
              animate={{ opacity: 1, y: 0 }}
              exit={{ opacity: 0, y: -8 }}
              transition={{ duration: 0.25 }}
            >
              {tab === "overview" && <OverviewTab />}
              {tab === "documents" && <DocumentsTab />}
              {tab === "queries" && <QueriesTab />}
            </motion.div>
          </AnimatePresence>
        </div>
      </main>
    </div>
  );
}
