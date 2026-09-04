import { Activity, Archive, Database, HardDrive, ShieldCheck } from "lucide-react";

const statusRows = [
  { label: "Recording Core", value: "Bootstrap", icon: Activity },
  { label: "SQLite", value: "Ready endpoint pending migration", icon: Database },
  { label: "Local Storage", value: "Always enabled", icon: HardDrive },
  { label: "Optional Modules", value: "Disabled until configured", icon: Archive },
  { label: "Deployment", value: "main-only production", icon: ShieldCheck }
];

export function AdminDashboard() {
  return (
    <main className="min-h-screen bg-[#f7f8f5] text-ink">
      <div className="mx-auto flex max-w-6xl flex-col gap-6 px-4 py-6 sm:px-6 lg:px-8">
        <header className="flex flex-col gap-2 border-b border-border pb-5">
          <p className="text-sm font-medium text-accent">7GRecorder Admin</p>
          <h1 className="text-3xl font-semibold tracking-normal">录播平台控制台</h1>
          <p className="max-w-3xl text-sm leading-6 text-muted">
            Phase 0 工程骨架已预留 Admin、Public、Internal API 边界。后续实现会先完成可靠录制与本地滚动存储，再逐个接入可选模块。
          </p>
        </header>

        <section className="grid gap-3 md:grid-cols-2 lg:grid-cols-3">
          {statusRows.map(({ label, value, icon: Icon }) => (
            <article key={label} className="rounded-md border border-border bg-panel p-4 shadow-sm">
              <div className="flex items-start justify-between gap-3">
                <div>
                  <h2 className="text-sm font-semibold">{label}</h2>
                  <p className="mt-2 text-sm leading-5 text-muted">{value}</p>
                </div>
                <Icon className="h-5 w-5 shrink-0 text-accent" aria-hidden="true" />
              </div>
            </article>
          ))}
        </section>
      </div>
    </main>
  );
}
