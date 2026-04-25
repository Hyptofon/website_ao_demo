import { useState, useEffect } from "react";
import { Toaster, toast } from "sonner";
import { AnimatePresence, motion } from "motion/react";
import {
  getToken,
  setToken,
  clearToken,
  refreshAccessToken,
  logout as apiLogout,
} from "./api";
import { LoginScreen } from "./LoginScreen";
import { Sidebar, type Tab } from "./Sidebar";
import { OverviewTab } from "./OverviewTab";
import { DocumentsTab } from "./DocumentsTab";
import { QueriesTab } from "./QueriesTab";
import { PromptsTab } from "./PromptsTab";
import { AuditTab } from "./AuditTab";
import { AdminsTab } from "./AdminsTab";
import { RefreshCw } from "lucide-react";

export default function AdminPanel() {
  const [tab, setTab] = useState<Tab>("overview");
  const [authed, setAuthed] = useState(false);
  const [ready, setReady] = useState(false);

  useEffect(() => {
    async function initAuth() {
      // Step 1: Check URL fragment/query for token (post-OAuth redirect).
      // The backend sends token in #token=... fragment (never logged server-side).
      let urlToken: string | null = null;

      if (window.location.hash) {
        const hashParams = new URLSearchParams(window.location.hash.slice(1));
        urlToken = hashParams.get("token");
      }
      if (!urlToken) {
        const params = new URLSearchParams(window.location.search);
        urlToken = params.get("token");
      }

      if (urlToken) {
        // Store in module-level variable only (NOT localStorage or sessionStorage).
        setToken(urlToken);
        window.history.replaceState({}, "", window.location.pathname);
      }

      // Step 2: If no token in URL, try silent refresh via HttpOnly cookie.
      // This handles the "page refresh" case: _memoryToken is null but the
      // refresh cookie is still valid — user gets logged in transparently.
      if (!getToken()) {
        await refreshAccessToken();
      }

      setAuthed(!!getToken());
      setReady(true);
    }

    initAuth();
  }, []);

  const handleLogout = async () => {
    try {
      // POST to backend: clears HttpOnly refresh cookie + records audit entry
      await apiLogout();
    } catch {
      // Even if server call fails, clear local token anyway
      clearToken();
    }
    setAuthed(false);
    toast.success("Ви вийшли з системи");
  };

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
        onLogout={handleLogout}
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
              {tab === "prompts" && <PromptsTab />}
              {tab === "audit" && <AuditTab />}
              {tab === "admins" && <AdminsTab />}
            </motion.div>
          </AnimatePresence>
        </div>
      </main>
    </div>
  );
}
