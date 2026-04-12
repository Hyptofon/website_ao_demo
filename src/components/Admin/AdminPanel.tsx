// AdminPanel — root orchestrator.
// Uses client:only="react" in Astro, so no SSR concerns.

import { useState, useEffect } from "react";
import { Toaster } from "sonner";
import { getToken, clearToken } from "./api";
import { LoginScreen } from "./LoginScreen";
import { Sidebar, type Tab } from "./Sidebar";
import { OverviewTab } from "./OverviewTab";
import { DocumentsTab } from "./DocumentsTab";
import { AuditTab } from "./AuditTab";
import { SettingsTab } from "./SettingsTab";
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
      <div className="flex min-h-screen items-center justify-center bg-layout-bg text-zinc-500">
        <RefreshCw className="animate-spin" size={28} />
      </div>
    );
  }

  if (!authed) {
    return (
      <>
        <Toaster
          theme="dark"
          toastOptions={{ classNames: { toast: "bg-zinc-900 border-zinc-800 text-zinc-200" } }}
        />
        <LoginScreen onAuth={() => setAuthed(true)} />
      </>
    );
  }

  return (
    <div className="flex min-h-screen bg-layout-bg font-[Roboto,system-ui,sans-serif] text-zinc-300">
      <Toaster
        theme="dark"
        toastOptions={{ classNames: { toast: "bg-zinc-900 border-zinc-800 text-zinc-200" } }}
      />

      <Sidebar
        active={tab}
        onChange={setTab}
        onLogout={() => { clearToken(); setAuthed(false); }}
      />

      {/* Main content area — offset by sidebar width */}
      <main className="ml-[220px] flex-1 overflow-y-auto p-6 md:ml-[240px] md:p-8">
        <div className="mx-auto max-w-[1100px]">
          {tab === "overview" && <OverviewTab />}
          {tab === "documents" && <DocumentsTab />}
          {tab === "audit" && <AuditTab />}
          {tab === "settings" && <SettingsTab />}
        </div>
      </main>
    </div>
  );
}
