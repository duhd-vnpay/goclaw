import { useEffect } from "react";
import { useNavigate, useParams } from "react-router";
import { useTranslation } from "react-i18next";
import { Save, Send, ArrowLeft } from "lucide-react";
import { useBuilderStore } from "./use-builder-store";
import { PropertiesPanel } from "./properties-panel";
import { VariablesPanel } from "./variables-panel";
import { StepList } from "./step-list";
import { AdvancedPanel } from "./advanced-panel";
import { FlowPreview } from "./flow-preview";
import {
  useArdennWorkflow,
  useCreateWorkflow,
  useUpdateWorkflow,
  usePublishWorkflow,
} from "@/hooks/use-ardenn";
import type { StepFormData, VariableFormData, ArdennTier, TriggerType, Visibility } from "@/types/ardenn";

export function WorkflowBuilderPage() {
  const { t } = useTranslation("ardenn");
  const navigate = useNavigate();
  const { id } = useParams<{ id: string }>();
  const isEdit = !!id;

  const store = useBuilderStore();
  const { data: existingWorkflow, isLoading } = useArdennWorkflow(id ?? "");
  const createWorkflow = useCreateWorkflow();
  const updateWorkflow = useUpdateWorkflow();
  const publishWorkflow = usePublishWorkflow();

  // Load existing workflow data for edit mode
  useEffect(() => {
    if (!isEdit) {
      store.reset();
      return;
    }
    if (existingWorkflow) {
      const wf = existingWorkflow.workflow;
      const triggerType =
        (wf.trigger_config as Record<string, string>)?.type ?? "manual";

      // Parse variables from JSONB
      const variables: VariableFormData[] = Object.entries(
        wf.variables ?? {},
      ).map(([name, config]) => ({
        name,
        type: ((config as Record<string, string>)?.type ?? "text") as VariableFormData["type"],
        required: (config as Record<string, boolean>)?.required ?? false,
        options: (config as Record<string, string[]>)?.options,
        defaultValue: (config as Record<string, string>)?.default,
      }));

      // Parse steps
      const steps: StepFormData[] = existingWorkflow.steps.map((s) => ({
        slug: s.slug,
        name: s.name,
        description: s.description ?? "",
        position: s.position,
        agentKey: s.agent_key ?? "",
        taskTemplate: s.task_template ?? "",
        dependsOn: s.depends_on ?? [],
        condition: s.condition ?? "",
        timeout: s.timeout,
        dispatchTo: s.dispatch_to ?? "agent",
        dispatchTarget: s.dispatch_target ?? "",
        gate: s.gate ?? {},
        constraints: s.constraints ?? {},
        continuity: s.continuity ?? {},
        evaluation: s.evaluation ?? {},
      }));

      store.loadWorkflow({
        id: wf.id,
        name: wf.name,
        slug: wf.slug,
        description: wf.description ?? "",
        domainId: wf.domain_id,
        tier: wf.tier as ArdennTier,
        triggerType: triggerType as TriggerType,
        visibility: wf.visibility as Visibility,
        variables,
        steps,
      });
    }
  }, [isEdit, existingWorkflow]); // eslint-disable-line react-hooks/exhaustive-deps

  const handleSaveDraft = async () => {
    const payload = store.toCreatePayload();
    try {
      if (isEdit && store.id) {
        await updateWorkflow.mutateAsync({ id: store.id, ...payload });
      } else {
        const result = await createWorkflow.mutateAsync(payload);
        // Navigate to edit mode after first save
        if (result?.id) {
          navigate(`/workflows/${result.id}/edit`, { replace: true });
        }
      }
    } catch (err) {
      console.error("Save failed:", err);
    }
  };

  const handlePublish = async () => {
    // Save first, then publish
    await handleSaveDraft();
    const wfId = store.id || id;
    if (wfId) {
      await publishWorkflow.mutateAsync(wfId);
      navigate(`/workflows/${wfId}`);
    }
  };

  const isSaving = createWorkflow.isPending || updateWorkflow.isPending;
  const isPublishing = publishWorkflow.isPending;

  if (isEdit && isLoading) {
    return (
      <div className="flex h-full items-center justify-center">
        <div className="h-8 w-8 animate-pulse rounded-full bg-muted" />
      </div>
    );
  }

  return (
    <div className="flex h-full flex-col">
      {/* Top bar */}
      <div className="flex items-center justify-between border-b px-4 py-3">
        <div className="flex items-center gap-3">
          <button
            onClick={() => navigate("/workflows")}
            className="rounded-md p-1 hover:bg-muted"
          >
            <ArrowLeft className="h-5 w-5" />
          </button>
          <h1 className="text-lg font-bold">
            {isEdit
              ? t("builder.editTitle", { name: store.name || t("builder.workflow") })
              : t("builder.newTitle")}
          </h1>
          {store.isDirty && (
            <span className="text-xs text-yellow-600">({t("builder.unsavedChanges")})</span>
          )}
        </div>
        <div className="flex items-center gap-2">
          <button
            onClick={handleSaveDraft}
            disabled={isSaving || !store.name || !store.domainId}
            className="inline-flex items-center gap-1.5 rounded-md border px-3 py-1.5 text-sm font-medium hover:bg-muted disabled:opacity-50"
          >
            <Save className="h-4 w-4" />
            {isSaving ? t("builder.saving") : t("builder.saveDraft")}
          </button>
          <button
            onClick={handlePublish}
            disabled={isPublishing || isSaving || !store.name || !store.domainId || store.steps.length === 0}
            className="inline-flex items-center gap-1.5 rounded-md bg-primary px-3 py-1.5 text-sm font-medium text-primary-foreground hover:bg-primary/90 disabled:opacity-50"
          >
            <Send className="h-4 w-4" />
            {isPublishing ? t("builder.publishing") : t("builder.publish")}
          </button>
        </div>
      </div>

      {/* Builder content — 2-column on desktop, stacked on mobile */}
      <div className="flex-1 overflow-y-auto">
        <div className="mx-auto max-w-7xl p-4 pb-20 lg:grid lg:grid-cols-3 lg:gap-6">
          {/* Left: Form panels */}
          <div className="lg:col-span-2 space-y-4">
            <PropertiesPanel />
            <VariablesPanel />
            <StepList />
            <AdvancedPanel />
          </div>

          {/* Right: Preview (desktop only) */}
          <div className="hidden lg:block sticky top-4 self-start">
            <FlowPreview />
          </div>

          {/* Mobile preview: below form */}
          <div className="mt-4 lg:hidden">
            <FlowPreview />
          </div>
        </div>
      </div>
    </div>
  );
}
