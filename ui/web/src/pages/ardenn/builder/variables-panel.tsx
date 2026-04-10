import { Plus, X } from "lucide-react";
import { useTranslation } from "react-i18next";
import { useBuilderStore } from "./use-builder-store";

const VARIABLE_TYPES = [
  { value: "text", labelKey: "builder.varTypeText" },
  { value: "number", labelKey: "builder.varTypeNumber" },
  { value: "select", labelKey: "builder.varTypeSelect" },
  { value: "date", labelKey: "builder.varTypeDate" },
] as const;

export function VariablesPanel() {
  const { t } = useTranslation("ardenn");
  const { variables, addVariable, removeVariable, updateVariable } =
    useBuilderStore();

  return (
    <div className="rounded-lg border bg-card p-4 space-y-3">
      <div className="flex items-center justify-between">
        <h3 className="text-sm font-semibold text-muted-foreground uppercase tracking-wide">
          {t("builder.variables")}
        </h3>
        <button
          type="button"
          onClick={addVariable}
          className="inline-flex items-center gap-1 rounded-md px-2 py-1 text-xs font-medium text-primary hover:bg-primary/10"
        >
          <Plus className="h-3 w-3" />
          {t("builder.addVariable")}
        </button>
      </div>

      {variables.length === 0 ? (
        <p className="text-xs text-muted-foreground">
          {t("builder.noVariables")}
        </p>
      ) : (
        <div className="space-y-2">
          {variables.map((v, i) => (
            <div
              key={i}
              className="flex items-center gap-2 rounded-md border bg-background p-2"
            >
              <input
                value={v.name}
                onChange={(e) => updateVariable(i, "name", e.target.value)}
                placeholder={t("builder.variableName")}
                className="h-7 flex-1 rounded border-0 bg-transparent px-2 text-base md:text-sm focus:ring-1 focus:ring-primary"
              />
              <select
                value={v.type}
                onChange={(e) => updateVariable(i, "type", e.target.value)}
                className="h-7 rounded border-0 bg-transparent px-1 text-base md:text-sm"
              >
                {VARIABLE_TYPES.map((vt) => (
                  <option key={vt.value} value={vt.value}>
                    {t(vt.labelKey)}
                  </option>
                ))}
              </select>
              <label className="flex items-center gap-1 text-xs whitespace-nowrap">
                <input
                  type="checkbox"
                  checked={v.required}
                  onChange={(e) =>
                    updateVariable(i, "required", e.target.checked)
                  }
                  className="rounded"
                />
                {t("builder.required")}
              </label>
              <button
                type="button"
                onClick={() => removeVariable(i)}
                className="rounded p-0.5 text-muted-foreground hover:text-destructive"
              >
                <X className="h-3.5 w-3.5" />
              </button>
            </div>
          ))}
        </div>
      )}
    </div>
  );
}
