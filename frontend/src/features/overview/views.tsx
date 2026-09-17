import { type AdminCopy } from "../../shared/console/types";

export function OverviewPanel(props: { statusRows: AdminCopy["statusRows"] }) {
  return (
    <section className="grid gap-3 md:grid-cols-2 xl:grid-cols-3">
      {props.statusRows.map(({ label, value, icon: Icon }) => (
        <article
          key={label}
          className="rounded-md border border-border bg-panel p-4 shadow-sm"
        >
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
  );
}
