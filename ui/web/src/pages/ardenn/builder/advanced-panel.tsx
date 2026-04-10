import { useState } from "react";
import { ChevronRight, Shield, Brain, BarChart3, AlertTriangle } from "lucide-react";
import { useTranslation } from "react-i18next";
import { useBuilderStore } from "./use-builder-store";

interface CollapsibleSectionProps {
  title: string;
  icon: typeof Shield;
  children: React.ReactNode;
  defaultOpen?: boolean;
}

function CollapsibleSection({
  title,
  icon: Icon,
  children,
  defaultOpen = false,
}: CollapsibleSectionProps) {
  const [open, setOpen] = useState(defaultOpen);

  return (
    <div className="border rounded-md overflow-hidden">
      <button
        type="button"
        onClick={() => setOpen(!open)}
        className="flex w-full items-center gap-2 px-3 py-2 text-sm font-medium hover:bg-muted/50 transition-colors"
      >
        <ChevronRight
          className={`h-4 w-4 text-muted-foreground transition-transform ${
            open ? "rotate-90" : ""
          }`}
        />
        <Icon className="h-4 w-4 text-muted-foreground" />
        {title}
      </button>
      {open && <div className="border-t px-3 py-3 space-y-3">{children}</div>}
    </div>
  );
}

export function AdvancedPanel() {
  const { t } = useTranslation("ardenn");
  const tier = useBuilderStore((s) => s.tier);

  if (tier === "light") {
    return null; // No advanced config for light tier
  }

  return (
    <div className="space-y-3">
      <h3 className="text-sm font-semibold text-muted-foreground uppercase tracking-wide">
        {t("builder.advanced")}
        <span className="ml-1 text-xs font-normal capitalize">
          ({t("builder.tier")}: {tier})
        </span>
      </h3>

      {/* L1: Constraints — standard + full */}
      <CollapsibleSection title={t("builder.constraints")} icon={Shield}>
        <p className="text-xs text-muted-foreground">
          {t("builder.constraintsHelp")}
        </p>
        <textarea
          placeholder={`{\n  "guards": [\n    {"name": "require_lead_role", "kind": "permission", "action": "blocked"}\n  ]\n}`}
          rows={4}
          className="w-full rounded-md border bg-background px-2 py-1.5 text-base md:text-sm font-mono resize-none"
        />
      </CollapsibleSection>

      {/* L3: Evaluation — full only */}
      {tier === "full" && (
        <CollapsibleSection title={t("builder.evaluation")} icon={BarChart3}>
          <p className="text-xs text-muted-foreground">
            {t("builder.evaluationHelp")}
          </p>
          <textarea
            placeholder={`{\n  "sensors": [\n    {"name": "code_quality", "kind": "rubric", "config": {}}\n  ]\n}`}
            rows={4}
            className="w-full rounded-md border bg-background px-2 py-1.5 text-base md:text-sm font-mono resize-none"
          />
        </CollapsibleSection>
      )}

      {/* L2: Continuity — full only */}
      {tier === "full" && (
        <CollapsibleSection title={t("builder.continuity")} icon={Brain}>
          <p className="text-xs text-muted-foreground">
            {t("builder.continuityHelp")}
          </p>
          <textarea
            placeholder={`{\n  "strategy": "full",\n  "maxEvents": 100\n}`}
            rows={4}
            className="w-full rounded-md border bg-background px-2 py-1.5 text-base md:text-sm font-mono resize-none"
          />
        </CollapsibleSection>
      )}

      {/* Failure Policy — standard + full */}
      <CollapsibleSection title={t("builder.failurePolicy")} icon={AlertTriangle}>
        <p className="text-xs text-muted-foreground">
          {t("builder.failurePolicyHelp")}
        </p>
        <textarea
          placeholder={`{\n  "maxRetries": 3,\n  "escalation": "notify"\n}`}
          rows={4}
          className="w-full rounded-md border bg-background px-2 py-1.5 text-base md:text-sm font-mono resize-none"
        />
      </CollapsibleSection>
    </div>
  );
}
