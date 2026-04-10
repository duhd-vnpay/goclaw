import { Trash2, ChevronUp, ChevronDown, Bot, User, Globe, Plug } from "lucide-react";
import { useTranslation } from "react-i18next";
import { useBuilderStore } from "./use-builder-store";
import type { StepFormData, HandType } from "@/types/ardenn";

interface StepEditorProps {
  index: number;
  step: StepFormData;
}

const HAND_TYPES: { value: HandType; labelKey: string; icon: typeof Bot }[] = [
  { value: "agent", labelKey: "builder.handAgent", icon: Bot },
  { value: "user", labelKey: "builder.handUser", icon: User },
  { value: "api", labelKey: "builder.handApi", icon: Globe },
  { value: "mcp", labelKey: "builder.handMcp", icon: Plug },
];

const GATE_TYPES = [
  { value: "", labelKey: "builder.gateNone" },
  { value: "auto", labelKey: "builder.gateAuto" },
  { value: "human", labelKey: "builder.gateHuman" },
  { value: "conditional", labelKey: "builder.gateConditional" },
];

export function StepEditor({ index, step }: StepEditorProps) {
  const { t } = useTranslation("ardenn");
  const { updateStep, removeStep, moveStep, steps, tier } = useBuilderStore();

  const update = (field: keyof StepFormData, value: unknown) => {
    updateStep(index, field, value);
  };

  // Determine dispatch fields from hand type
  const handType = step.dispatchTo || "agent";
  const showGate = tier !== "light";
  const isFirst = index === 0;
  const isLast = index === steps.length - 1;

  return (
    <div className="rounded-lg border bg-card overflow-hidden">
      {/* Header */}
      <div className="flex items-center gap-2 border-b bg-muted/30 px-3 py-2">
        <span className="text-xs font-medium text-muted-foreground">
          {index + 1}.
        </span>
        <input
          value={step.name}
          onChange={(e) => update("name", e.target.value)}
          placeholder={t("builder.stepName")}
          className="flex-1 bg-transparent text-sm font-medium border-0 p-0 focus:ring-0 text-base md:text-sm"
        />
        <div className="flex items-center gap-0.5">
          <button
            type="button"
            onClick={() => moveStep(index, index - 1)}
            disabled={isFirst}
            className="rounded p-1 text-muted-foreground hover:bg-muted disabled:opacity-30"
            title={t("builder.moveUp")}
          >
            <ChevronUp className="h-3.5 w-3.5" />
          </button>
          <button
            type="button"
            onClick={() => moveStep(index, index + 1)}
            disabled={isLast}
            className="rounded p-1 text-muted-foreground hover:bg-muted disabled:opacity-30"
            title={t("builder.moveDown")}
          >
            <ChevronDown className="h-3.5 w-3.5" />
          </button>
          <button
            type="button"
            onClick={() => removeStep(index)}
            className="rounded p-1 text-muted-foreground hover:bg-destructive/10 hover:text-destructive"
            title={t("builder.deleteStep")}
          >
            <Trash2 className="h-3.5 w-3.5" />
          </button>
        </div>
      </div>

      {/* Body */}
      <div className="p-3 space-y-3">
        {/* Hand type selector */}
        <div>
          <label className="text-xs font-medium text-muted-foreground">
            {t("builder.handType")}
          </label>
          <div className="mt-1 flex gap-1.5">
            {HAND_TYPES.map((ht) => {
              const Icon = ht.icon;
              return (
                <button
                  key={ht.value}
                  type="button"
                  onClick={() => {
                    update("dispatchTo", ht.value);
                    update("dispatchTarget", "");
                  }}
                  className={`inline-flex items-center gap-1.5 rounded-md border px-2.5 py-1 text-xs font-medium transition-colors ${
                    handType === ht.value
                      ? "border-primary bg-primary/10 text-primary"
                      : "hover:bg-muted"
                  }`}
                >
                  <Icon className="h-3 w-3" />
                  {t(ht.labelKey)}
                </button>
              );
            })}
          </div>
        </div>

        {/* Target picker — changes based on hand type */}
        <div className="grid grid-cols-1 sm:grid-cols-2 gap-3">
          <div>
            <label className="text-xs font-medium text-muted-foreground">
              {handType === "agent"
                ? t("builder.targetAgent")
                : handType === "user"
                  ? t("builder.targetUser")
                  : handType === "api"
                    ? t("builder.targetApi")
                    : t("builder.targetMcp")}
            </label>
            <input
              value={step.dispatchTarget}
              onChange={(e) => update("dispatchTarget", e.target.value)}
              placeholder={
                handType === "agent"
                  ? "e.g., code-review-lead"
                  : handType === "user"
                    ? "e.g., lead@company.com"
                    : "endpoint or tool"
              }
              className="mt-1 h-8 w-full rounded-md border bg-background px-2 text-base md:text-sm"
            />
          </div>

          {/* Timeout */}
          <div>
            <label className="text-xs font-medium text-muted-foreground">
              {t("builder.timeout")}
            </label>
            <input
              value={step.timeout}
              onChange={(e) => update("timeout", e.target.value)}
              placeholder="30 minutes"
              className="mt-1 h-8 w-full rounded-md border bg-background px-2 text-base md:text-sm"
            />
          </div>
        </div>

        {/* Task template */}
        <div>
          <label className="text-xs font-medium text-muted-foreground">
            {t("builder.taskTemplate")}
          </label>
          <textarea
            value={step.taskTemplate}
            onChange={(e) => update("taskTemplate", e.target.value)}
            placeholder={t("builder.taskTemplatePlaceholder")}
            rows={3}
            className="mt-1 w-full rounded-md border bg-background px-2 py-1.5 text-base md:text-sm resize-none font-mono"
          />
        </div>

        {/* Dependencies */}
        {index > 0 && (
          <div>
            <label className="text-xs font-medium text-muted-foreground">
              {t("builder.dependsOn")}
            </label>
            <select
              multiple
              value={step.dependsOn}
              onChange={(e) => {
                const selected = Array.from(
                  e.target.selectedOptions,
                  (opt) => opt.value,
                );
                update("dependsOn", selected);
              }}
              className="mt-1 h-16 w-full rounded-md border bg-background px-2 text-base md:text-sm"
            >
              {steps.slice(0, index).map((prev, pi) => (
                <option key={pi} value={prev.slug}>
                  {pi + 1}. {prev.name || prev.slug}
                </option>
              ))}
            </select>
          </div>
        )}

        {/* Gate config (hidden for light tier) */}
        {showGate && (
          <div className="grid grid-cols-1 sm:grid-cols-2 gap-3">
            <div>
              <label className="text-xs font-medium text-muted-foreground">
                {t("builder.gate")}
              </label>
              <select
                value={(step.gate as Record<string, string>)?.type ?? ""}
                onChange={(e) =>
                  update("gate", e.target.value ? { type: e.target.value } : {})
                }
                className="mt-1 h-8 w-full rounded-md border bg-background px-2 text-base md:text-sm"
              >
                {GATE_TYPES.map((g) => (
                  <option key={g.value} value={g.value}>
                    {t(g.labelKey)}
                  </option>
                ))}
              </select>
            </div>

            {/* Condition (optional) */}
            <div>
              <label className="text-xs font-medium text-muted-foreground">
                {t("builder.condition")}
              </label>
              <input
                value={step.condition}
                onChange={(e) => update("condition", e.target.value)}
                placeholder="e.g., variables.priority == 'high'"
                className="mt-1 h-8 w-full rounded-md border bg-background px-2 text-base md:text-sm"
              />
            </div>
          </div>
        )}
      </div>
    </div>
  );
}
