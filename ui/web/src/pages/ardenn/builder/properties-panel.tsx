import { useTranslation } from "react-i18next";
import { useBuilderStore } from "./use-builder-store";
import { useArdennDomains } from "@/hooks/use-ardenn";
import type { ArdennTier, TriggerType } from "@/types/ardenn";

const TIERS: { value: ArdennTier; labelKey: string }[] = [
  { value: "light", labelKey: "builder.tierLight" },
  { value: "standard", labelKey: "builder.tierStandard" },
  { value: "full", labelKey: "builder.tierFull" },
];

const TRIGGERS: { value: TriggerType; labelKey: string }[] = [
  { value: "manual", labelKey: "builder.triggerManual" },
  { value: "webhook", labelKey: "builder.triggerWebhook" },
  { value: "event", labelKey: "builder.triggerEvent" },
  { value: "cron", labelKey: "builder.triggerCron" },
];

export function PropertiesPanel() {
  const { t } = useTranslation("ardenn");
  const store = useBuilderStore();
  const { data: domains } = useArdennDomains();

  return (
    <div className="rounded-lg border bg-card p-4 space-y-4">
      <h3 className="text-sm font-semibold text-muted-foreground uppercase tracking-wide">
        {t("builder.properties")}
      </h3>

      <div className="grid grid-cols-1 sm:grid-cols-2 gap-4">
        {/* Name */}
        <div>
          <label className="text-sm font-medium">{t("builder.name")}</label>
          <input
            value={store.name}
            onChange={(e) => {
              store.setName(e.target.value);
              // Auto-generate slug from name
              if (!store.id) {
                store.setSlug(
                  e.target.value
                    .toLowerCase()
                    .replace(/[^a-z0-9]+/g, "-")
                    .replace(/^-|-$/g, ""),
                );
              }
            }}
            placeholder={t("builder.namePlaceholder")}
            className="mt-1 h-9 w-full rounded-md border bg-background px-3 text-base md:text-sm"
          />
        </div>

        {/* Domain */}
        <div>
          <label className="text-sm font-medium">{t("builder.domain")}</label>
          <select
            value={store.domainId}
            onChange={(e) => store.setDomainId(e.target.value)}
            className="mt-1 h-9 w-full rounded-md border bg-background px-3 text-base md:text-sm"
          >
            <option value="">{t("builder.selectDomain")}</option>
            {(domains ?? []).map((d) => (
              <option key={d.id} value={d.id}>
                {d.name}
              </option>
            ))}
          </select>
        </div>

        {/* Tier */}
        <div>
          <label className="text-sm font-medium">{t("builder.tier")}</label>
          <div className="mt-1 flex gap-2">
            {TIERS.map((tier) => (
              <button
                key={tier.value}
                type="button"
                onClick={() => store.setTier(tier.value)}
                className={`flex-1 rounded-md border px-3 py-1.5 text-sm font-medium transition-colors ${
                  store.tier === tier.value
                    ? "border-primary bg-primary text-primary-foreground"
                    : "hover:bg-muted"
                }`}
              >
                {t(tier.labelKey)}
              </button>
            ))}
          </div>
        </div>

        {/* Trigger */}
        <div>
          <label className="text-sm font-medium">{t("builder.trigger")}</label>
          <div className="mt-1 flex gap-2">
            {TRIGGERS.map((trigger) => (
              <button
                key={trigger.value}
                type="button"
                onClick={() => store.setTriggerType(trigger.value)}
                className={`flex-1 rounded-md border px-3 py-1.5 text-sm font-medium transition-colors ${
                  store.triggerType === trigger.value
                    ? "border-primary bg-primary text-primary-foreground"
                    : "hover:bg-muted"
                }`}
              >
                {t(trigger.labelKey)}
              </button>
            ))}
          </div>
        </div>

        {/* Description */}
        <div className="sm:col-span-2">
          <label className="text-sm font-medium">{t("builder.description")}</label>
          <textarea
            value={store.description}
            onChange={(e) => store.setDescription(e.target.value)}
            placeholder={t("builder.descriptionPlaceholder")}
            rows={2}
            className="mt-1 w-full rounded-md border bg-background px-3 py-2 text-base md:text-sm resize-none"
          />
        </div>
      </div>
    </div>
  );
}
