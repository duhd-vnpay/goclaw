import { useState } from "react";
import { useNavigate } from "react-router";
import { useTranslation } from "react-i18next";
import { Plus, Search, Route } from "lucide-react";
import { useArdennWorkflows, useArdennDomains } from "@/hooks/use-ardenn";
import { ROUTES } from "@/lib/constants";
import { TIER_COLORS, TIER_LABELS } from "@/types/ardenn";
import type { ArdennTier } from "@/types/ardenn";

export function WorkflowsPage() {
  const { t } = useTranslation("ardenn");
  const navigate = useNavigate();
  const [search, setSearch] = useState("");
  const [domainFilter, setDomainFilter] = useState<string>("");
  const [tierFilter, setTierFilter] = useState<string>("");
  const [statusFilter, setStatusFilter] = useState<string>("");

  const { data: workflows, isLoading } = useArdennWorkflows({
    domainId: domainFilter || undefined,
    status: statusFilter || undefined,
  });
  const { data: domains } = useArdennDomains();

  const filtered = (workflows ?? []).filter((wf) => {
    if (search && !wf.name.toLowerCase().includes(search.toLowerCase())) return false;
    if (tierFilter && wf.tier !== tierFilter) return false;
    return true;
  });

  const domainMap = new Map((domains ?? []).map((d) => [d.id, d.name]));

  return (
    <div className="flex h-full flex-col gap-6 p-6">
      {/* Header */}
      <div className="flex items-center justify-between">
        <div>
          <h1 className="text-2xl font-bold">{t("workflows.title")}</h1>
          <p className="text-sm text-muted-foreground">{t("workflows.description")}</p>
        </div>
        <button
          onClick={() => navigate(ROUTES.WORKFLOW_NEW)}
          className="inline-flex items-center gap-2 rounded-md bg-primary px-4 py-2 text-sm font-medium text-primary-foreground hover:bg-primary/90"
        >
          <Plus className="h-4 w-4" />
          {t("workflows.createNew")}
        </button>
      </div>

      {/* Filters */}
      <div className="flex flex-wrap items-center gap-3">
        <div className="relative flex-1 min-w-[200px] max-w-sm">
          <Search className="absolute left-3 top-1/2 h-4 w-4 -translate-y-1/2 text-muted-foreground" />
          <input
            type="text"
            value={search}
            onChange={(e) => setSearch(e.target.value)}
            placeholder={t("workflows.search")}
            className="h-9 w-full rounded-md border bg-background pl-9 pr-3 text-base md:text-sm"
          />
        </div>
        <select
          value={domainFilter}
          onChange={(e) => setDomainFilter(e.target.value)}
          className="h-9 rounded-md border bg-background px-3 text-base md:text-sm"
        >
          <option value="">{t("workflows.allDomains")}</option>
          {(domains ?? []).map((d) => (
            <option key={d.id} value={d.id}>{d.name}</option>
          ))}
        </select>
        <select
          value={tierFilter}
          onChange={(e) => setTierFilter(e.target.value)}
          className="h-9 rounded-md border bg-background px-3 text-base md:text-sm"
        >
          <option value="">{t("workflows.allTiers")}</option>
          <option value="light">{t("workflows.tierLight")}</option>
          <option value="standard">{t("workflows.tierStandard")}</option>
          <option value="full">{t("workflows.tierFull")}</option>
        </select>
        <select
          value={statusFilter}
          onChange={(e) => setStatusFilter(e.target.value)}
          className="h-9 rounded-md border bg-background px-3 text-base md:text-sm"
        >
          <option value="">{t("workflows.allStatuses")}</option>
          <option value="draft">Draft</option>
          <option value="published">Published</option>
          <option value="archived">Archived</option>
        </select>
      </div>

      {/* Table */}
      {isLoading ? (
        <div className="space-y-3">
          {Array.from({ length: 5 }).map((_, i) => (
            <div key={i} className="h-14 animate-pulse rounded-md bg-muted" />
          ))}
        </div>
      ) : filtered.length === 0 ? (
        <div className="flex flex-1 flex-col items-center justify-center gap-3 text-center">
          <Route className="h-12 w-12 text-muted-foreground/50" />
          <h3 className="text-lg font-medium">{t("workflows.empty")}</h3>
          <p className="text-sm text-muted-foreground">{t("workflows.emptyDescription")}</p>
          <button
            onClick={() => navigate(ROUTES.WORKFLOW_NEW)}
            className="mt-2 inline-flex items-center gap-2 rounded-md bg-primary px-4 py-2 text-sm font-medium text-primary-foreground hover:bg-primary/90"
          >
            <Plus className="h-4 w-4" />
            {t("workflows.createNew")}
          </button>
        </div>
      ) : (
        <div className="overflow-x-auto">
          <table className="w-full min-w-[600px]">
            <thead>
              <tr className="border-b text-left text-sm font-medium text-muted-foreground">
                <th className="pb-3 pr-4">{t("workflows.columns.name")}</th>
                <th className="pb-3 pr-4">{t("workflows.columns.tier")}</th>
                <th className="pb-3 pr-4">{t("workflows.columns.domain")}</th>
                <th className="pb-3 pr-4">{t("workflows.columns.status")}</th>
                <th className="pb-3 pr-4">{t("workflows.columns.lastRun")}</th>
              </tr>
            </thead>
            <tbody>
              {filtered.map((wf) => (
                <tr
                  key={wf.id}
                  onClick={() => navigate(`/workflows/${wf.id}`)}
                  className="cursor-pointer border-b hover:bg-muted/50 transition-colors"
                >
                  <td className="py-3 pr-4">
                    <div className="font-medium">{wf.name}</div>
                    {wf.description && (
                      <div className="text-xs text-muted-foreground truncate max-w-xs">
                        {wf.description}
                      </div>
                    )}
                  </td>
                  <td className="py-3 pr-4">
                    <span className={`inline-flex items-center rounded-full px-2.5 py-0.5 text-xs font-medium ${TIER_COLORS[wf.tier as ArdennTier]}`}>
                      {TIER_LABELS[wf.tier as ArdennTier]}
                    </span>
                  </td>
                  <td className="py-3 pr-4 text-sm">
                    {domainMap.get(wf.domain_id) ?? "-"}
                  </td>
                  <td className="py-3 pr-4">
                    <span className={`text-xs capitalize ${wf.status === "published" ? "text-green-600" : wf.status === "draft" ? "text-yellow-600" : "text-muted-foreground"}`}>
                      {wf.status}
                    </span>
                  </td>
                  <td className="py-3 pr-4 text-xs text-muted-foreground">
                    {wf.updated_at ? new Date(wf.updated_at).toLocaleDateString() : "-"}
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
