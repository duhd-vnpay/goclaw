import { Plus } from "lucide-react";
import { useTranslation } from "react-i18next";
import { useBuilderStore } from "./use-builder-store";
import { StepEditor } from "./step-editor";

export function StepList() {
  const { t } = useTranslation("ardenn");
  const { steps, addStep } = useBuilderStore();

  return (
    <div className="space-y-3">
      <div className="flex items-center justify-between">
        <h3 className="text-sm font-semibold text-muted-foreground uppercase tracking-wide">
          {t("builder.steps")}
          {steps.length > 0 && (
            <span className="ml-1 text-xs font-normal">
              ({steps.length})
            </span>
          )}
        </h3>
        <button
          type="button"
          onClick={addStep}
          className="inline-flex items-center gap-1 rounded-md px-2 py-1 text-xs font-medium text-primary hover:bg-primary/10"
        >
          <Plus className="h-3 w-3" />
          {t("builder.addStep")}
        </button>
      </div>

      {steps.length === 0 ? (
        <div className="flex flex-col items-center gap-2 rounded-lg border border-dashed py-8 text-center">
          <p className="text-sm text-muted-foreground">
            {t("builder.noSteps")}
          </p>
          <button
            type="button"
            onClick={addStep}
            className="inline-flex items-center gap-1.5 rounded-md bg-primary px-3 py-1.5 text-xs font-medium text-primary-foreground hover:bg-primary/90"
          >
            <Plus className="h-3 w-3" />
            {t("builder.addStep")}
          </button>
        </div>
      ) : (
        <div className="space-y-3">
          {steps.map((step, i) => (
            <StepEditor key={`step-${i}`} index={i} step={step} />
          ))}
        </div>
      )}
    </div>
  );
}
