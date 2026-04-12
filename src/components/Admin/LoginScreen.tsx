import { Shield } from "lucide-react";
import { Button } from "@/components/ui/button";
import { motion } from "motion/react";
import { getLoginUrl } from "./api";

export function LoginScreen({ onAuth }: { onAuth: () => void }) {
  const handleLogin = async () => {
    try {
      const url = await getLoginUrl();
      window.location.href = url;
    } catch {
      const token = window.prompt("Enter Admin Token:");
      if (token) {
        localStorage.setItem("admin_jwt", token);
        onAuth();
      }
    }
  };

  return (
    <div className="flex min-h-screen items-center justify-center bg-layout-bg px-4">
      {/* Background glow */}
      <div className="pointer-events-none fixed inset-0">
        <div className="absolute left-1/2 top-1/3 h-[600px] w-[600px] -translate-x-1/2 -translate-y-1/2 rounded-full bg-brand-blue/[0.06] blur-[120px]" />
      </div>

      <motion.div
        initial={{ opacity: 0, y: 30, scale: 0.95 }}
        animate={{ opacity: 1, y: 0, scale: 1 }}
        transition={{ duration: 0.6, ease: "easeOut" }}
        className="relative w-full max-w-sm rounded-2xl border border-white/[0.08] bg-dark-panel p-8 text-center backdrop-blur-xl"
      >
        {/* Glow ring */}
        <div className="absolute -inset-px -z-10 rounded-2xl bg-gradient-to-b from-brand-blue/20 to-transparent opacity-50 blur-sm" />

        <motion.div
          initial={{ scale: 0 }}
          animate={{ scale: 1 }}
          transition={{ delay: 0.2, type: "spring", stiffness: 300, damping: 15 }}
          className="mx-auto mb-6 flex h-20 w-20 items-center justify-center rounded-2xl bg-brand-blue/12 text-brand-blue"
        >
          <Shield size={40} />
        </motion.div>

        <motion.h1
          initial={{ opacity: 0, y: 10 }}
          animate={{ opacity: 1, y: 0 }}
          transition={{ delay: 0.3 }}
          className="mb-2 text-2xl font-bold text-white"
        >
          Адмін-панель
        </motion.h1>

        <motion.p
          initial={{ opacity: 0 }}
          animate={{ opacity: 1 }}
          transition={{ delay: 0.4 }}
          className="mb-8 text-sm leading-relaxed text-zinc-500"
        >
          Увійдіть через Google для доступу до управління чат-ботом кафедри.
        </motion.p>

        <motion.div
          initial={{ opacity: 0, y: 10 }}
          animate={{ opacity: 1, y: 0 }}
          transition={{ delay: 0.5 }}
        >
          <Button
            onClick={handleLogin}
            size="lg"
            className="w-full bg-brand-blue hover:bg-brand-blue-deep text-white font-medium"
          >
            <svg className="mr-2 h-5 w-5" viewBox="0 0 24 24">
              <path fill="currentColor" d="M22.56 12.25c0-.78-.07-1.53-.2-2.25H12v4.26h5.92a5.06 5.06 0 0 1-2.2 3.32v2.77h3.57c2.08-1.92 3.28-4.74 3.28-8.1z" />
              <path fill="currentColor" d="M12 23c2.97 0 5.46-.98 7.28-2.66l-3.57-2.77c-.98.66-2.23 1.06-3.71 1.06-2.86 0-5.29-1.93-6.16-4.53H2.18v2.84C3.99 20.53 7.7 23 12 23z" />
              <path fill="currentColor" d="M5.84 14.09c-.22-.66-.35-1.36-.35-2.09s.13-1.43.35-2.09V7.07H2.18C1.43 8.55 1 10.22 1 12s.43 3.45 1.18 4.93l2.85-2.22.81-.62z" />
              <path fill="currentColor" d="M12 5.38c1.62 0 3.06.56 4.21 1.64l3.15-3.15C17.45 2.09 14.97 1 12 1 7.7 1 3.99 3.47 2.18 7.07l3.66 2.84c.87-2.6 3.3-4.53 6.16-4.53z" />
            </svg>
            Увійти з Google
          </Button>
        </motion.div>
      </motion.div>
    </div>
  );
}
