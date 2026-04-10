import { useState } from "react";
import { useTranslation } from "react-i18next";
import { Plus, Layers, Trash2 } from "lucide-react";
import { useArdennDomains, useCreateDomain, useDeleteDomain } from "@/hooks/use-ardenn";
import { TIER_COLORS, TIER_LABELS } from "@/types/ardenn";
import type { ArdennTier } from "@/types/ardenn";

export function DomainsPage() {
  const { t } = useTranslation("ardenn");
  const { data: domains, isLoading } = useArdennDomains();
  const createDomain = useCreateDomain();
  const deleteDomain = useDeleteDomain();

  const [showCreate, setShowCreate] = useState(false);
  const [form, setForm] = useState({
    name: "",
    slug: "",
    description: "",
    defaultTier: "standard" as ArdennTier,
  });

  const handleCreate = async () => {
    if (!form.name || !form.slug) return;
    await createDomain.mutateAsync({
      name: form.name,
      slug: form.slug,
      description: form.description || undefined,
      defaultTier: form.defaultTier,
    });
    setForm({ name: "", slug: "", description: "", defaultTier: "standard" });
    setShowCreate(false);
  };

  const handleDelete = async (id: string) => {
    if (!confirm("Delete this domain? Workflows under it will be orphaned.")) return;
    await deleteDomain.mutateAsync(id);
  };

  return (
    <div className="flex h-full flex-col gap-6 p-6">
      {/* Header */}
      <div className="flex items-center justify-between">
        <div>
          <h1 className="text-2xl font-bold">{t("domains.title")}</h1>
          <p className="text-sm text-muted-foreground">{t("domains.description")}</p>
        </div>
        <button
          onClick={() => setShowCreate(true)}
          className="inline-flex items-center gap-2 rounded-md bg-primary px-4 py-2 text-sm font-medium text-primary-foreground hover:bg-primary/90"
        >
          <Plus className="h-4 w-4" />
          {t("domains.createNew")}
        </button>
      </div>

      {/* Create dialog */}
      {showCreate && (
        <div className="rounded-lg border bg-card p-6 space-y-4">
          <h3 className="text-lg font-semibold">{t("domains.dialog.createTitle")}</h3>
          <div className="grid grid-cols-1 sm:grid-cols-2 gap-4">
            <div>
              <label className="text-sm font-medium">{t("domains.dialog.name")}</label>
              <input
                value={form.name}
                onChange={(e) => setForm((f) => ({ ...f, name: e.target.value }))}
                placeholder={t("domains.dialog.namePlaceholder")}
                className="mt-1 h-9 w-full rounded-md border bg-background px-3 text-base md:text-sm"
              />
            </div>
            <div>
              <label className="text-sm font-medium">{t("domains.dialog.slug")}</label>
              <input
                value={form.slug}
                onChange={(e) => setForm((f) => ({ ...f, slug: e.target.value }))}
                placeholder={t("domains.dialog.slugPlaceholder")}
                className="mt-1 h-9 w-full rounded-md border bg-background px-3 text-base md:text-sm"
              />
            </div>
            <div className="sm:col-span-2">
              <label className="text-sm font-medium">{t("domains.dialog.description")}</label>
              <input
                value={form.description}
                onChange={(e) => setForm((f) => ({ ...f, description: e.target.value }))}
                className="mt-1 h-9 w-full rounded-md border bg-background px-3 text-base md:text-sm"
              />
            </div>
            <div>
              <label className="text-sm font-medium">{t("domains.dialog.defaultTier")}</label>
              <select
                value={form.defaultTier}
                onChange={(e) => setForm((f) => ({ ...f, defaultTier: e.target.value as ArdennTier }))}
                className="mt-1 h-9 w-full rounded-md border bg-background px-3 text-base md:text-sm"
              >
                <option value="light">Light</option>
                <option value="standard">Standard</option>
                <option value="full">Full</option>
              </select>
            </div>
          </div>
          <div className="flex justify-end gap-2">
            <button
              onClick={() => setShowCreate(false)}
              className="rounded-md border px-4 py-2 text-sm hover:bg-muted"
            >
              {t("domains.dialog.cancel")}
            </button>
            <button
              onClick={handleCreate}
              disabled={createDomain.isPending}
              className="rounded-md bg-primary px-4 py-2 text-sm font-medium text-primary-foreground hover:bg-primary/90 disabled:opacity-50"
            >
              {t("domains.dialog.create")}
            </button>
          </div>
        </div>
      )}

      {/* Table */}
      {isLoading ? (
        <div className="space-y-3">
          {Array.from({ length: 3 }).map((_, i) => (
            <div key={i} className="h-14 animate-pulse rounded-md bg-muted" />
          ))}
        </div>
      ) : (domains ?? []).length === 0 ? (
        <div className="flex flex-1 flex-col items-center justify-center gap-3 text-center">
          <Layers className="h-12 w-12 text-muted-foreground/50" />
          <h3 className="text-lg font-medium">{t("domains.empty")}</h3>
          <p className="text-sm text-muted-foreground">{t("domains.emptyDescription")}</p>
        </div>
      ) : (
        <div className="overflow-x-auto">
          <table className="w-full min-w-[600px]">
            <thead>
              <tr className="border-b text-left text-sm font-medium text-muted-foreground">
                <th className="pb-3 pr-4">{t("domains.columns.name")}</th>
                <th className="pb-3 pr-4">{t("domains.columns.slug")}</th>
                <th className="pb-3 pr-4">{t("domains.columns.tier")}</th>
                <th className="pb-3 pr-4">{t("domains.columns.department")}</th>
                <th className="pb-3 pr-4 w-10"></th>
              </tr>
            </thead>
            <tbody>
              {(domains ?? []).map((d) => (
                <tr key={d.id} className="border-b hover:bg-muted/50 transition-colors">
                  <td className="py-3 pr-4">
                    <div className="font-medium">{d.name}</div>
                    {d.description && (
                      <div className="text-xs text-muted-foreground">{d.description}</div>
                    )}
                  </td>
                  <td className="py-3 pr-4 text-sm font-mono text-muted-foreground">
                    {d.slug}
                  </td>
                  <td className="py-3 pr-4">
                    <span className={`inline-flex items-center rounded-full px-2.5 py-0.5 text-xs font-medium ${TIER_COLORS[d.default_tier as ArdennTier]}`}>
                      {TIER_LABELS[d.default_tier as ArdennTier]}
                    </span>
                  </td>
                  <td className="py-3 pr-4 text-sm text-muted-foreground">
                    {d.department_id ?? "-"}
                  </td>
                  <td className="py-3">
                    <button
                      onClick={() => handleDelete(d.id)}
                      className="rounded-md p-1.5 text-muted-foreground hover:bg-destructive/10 hover:text-destructive"
                    >
                      <Trash2 className="h-4 w-4" />
                    </button>
                  </td>
                </tr>
              ))}
            </tbody>
          </table>
        </div>
      )}
    </div>
  );
}
