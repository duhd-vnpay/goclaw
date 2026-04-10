import { useTranslation } from "react-i18next";
import { useBuilderStore } from "./use-builder-store";
import { Bot, User, Globe, Plug, ArrowRight } from "lucide-react";

const HAND_ICONS: Record<string, typeof Bot> = {
  agent: Bot,
  user: User,
  api: Globe,
  mcp: Plug,
};

const HAND_COLORS: Record<string, string> = {
  agent: "border-blue-300 bg-blue-50 dark:border-blue-700 dark:bg-blue-950",
  user: "border-green-300 bg-green-50 dark:border-green-700 dark:bg-green-950",
  api: "border-orange-300 bg-orange-50 dark:border-orange-700 dark:bg-orange-950",
  mcp: "border-purple-300 bg-purple-50 dark:border-purple-700 dark:bg-purple-950",
};

export function FlowPreview() {
  const { t } = useTranslation("ardenn");
  const steps = useBuilderStore((s) => s.steps);

  if (steps.length === 0) {
    return (
      <div className="flex items-center justify-center rounded-lg border border-dashed p-6 text-sm text-muted-foreground">
        {t("builder.previewEmpty")}
      </div>
    );
  }

  return (
    <div className="rounded-lg border bg-card p-4">
      <h3 className="mb-3 text-sm font-semibold text-muted-foreground uppercase tracking-wide">
        {t("builder.preview")}
      </h3>
      <div className="flex items-center gap-1 overflow-x-auto pb-2">
        {steps.map((step, i) => {
          const handType = step.dispatchTo || "agent";
          const Icon = HAND_ICONS[handType] ?? Bot;
          const colorClass = HAND_COLORS[handType] ?? "border-muted bg-background";
          const gateType =
            step.gate &&
            typeof step.gate === "object" &&
            "type" in step.gate
              ? String(step.gate.type)
              : null;

          return (
            <div key={i} className="flex items-center gap-1 shrink-0">
              {i > 0 && (
                <ArrowRight className="h-4 w-4 text-muted-foreground shrink-0" />
              )}
              <div className={`flex flex-col items-center gap-1 rounded-md border px-3 py-2 min-w-[80px] ${colorClass}`}>
                <Icon className="h-4 w-4 text-muted-foreground" />
                <span className="text-[10px] font-medium text-center leading-tight max-w-[80px] truncate">
                  {step.name || `Step ${i + 1}`}
                </span>
                <span className="text-[9px] text-muted-foreground">
                  {handType}
                </span>
                {gateType && (
                  <span className="text-[9px] text-yellow-600">
                    gate: {gateType}
                  </span>
                )}
              </div>
            </div>
          );
        })}
      </div>
    </div>
  );
}
