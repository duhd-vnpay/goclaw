import { useState, useMemo } from "react";
import { useParams, useNavigate } from "react-router";
import { useTranslation } from "react-i18next";
import { ArrowLeft, Pause, Play, Ban, Loader2 } from "lucide-react";
import { useRunEvents } from "./use-run-events";
import { PipelineView } from "./pipeline-view";
import { StepDetail } from "./step-detail";
import { EventLog } from "./event-log";

export function RunPage() {
  const { t } = useTranslation("ardenn");
  const { workflowId, runId } = useParams<{
    workflowId: string;
    runId: string;
  }>();
  const navigate = useNavigate();
  const [selectedStepId, setSelectedStepId] = useState<string | null>(null);

  const { run, events, isLoading, error } = useRunEvents(runId ?? "");

  // Build step name map (from events — step names are in step.ready/dispatched payloads)
  const stepNames = useMemo(() => {
    const names: Record<string, string> = {};
    for (const e of events) {
      if (e.step_id && e.payload?.step_name) {
        names[e.step_id] = e.payload.step_name as string;
      }
    }
    // Fallback: use step ID truncated
    if (run?.StepRuns) {
      for (const sr of Object.values(run.StepRuns)) {
        if (!names[sr.StepID]) {
          names[sr.StepID] = `Step ${sr.StepID.substring(0, 8)}`;
        }
      }
    }
    return names;
  }, [events, run]);

  const stepRuns = useMemo(
    () => (run?.StepRuns ? Object.values(run.StepRuns) : []),
    [run],
  );

  const selectedStep = useMemo(
    () =>
      selectedStepId
        ? stepRuns.find((sr) => sr.StepID === selectedStepId)
        : null,
    [selectedStepId, stepRuns],
  );

  // Progress calculation
  const completedSteps = stepRuns.filter(
    (sr) => sr.Status === "completed" || sr.Status === "skipped",
  ).length;
  const totalSteps = stepRuns.length;
  const progressPct = totalSteps > 0 ? (completedSteps / totalSteps) * 100 : 0;

  if (isLoading) {
    return (
      <div className="flex h-full items-center justify-center">
        <Loader2 className="h-8 w-8 animate-spin text-muted-foreground" />
      </div>
    );
  }

  if (error) {
    return (
      <div className="flex h-full items-center justify-center">
        <p className="text-destructive">{error}</p>
      </div>
    );
  }

  if (!run) {
    return (
      <div className="flex h-full items-center justify-center">
        <p className="text-muted-foreground">Run not found</p>
      </div>
    );
  }

  return (
    <div className="flex h-full flex-col">
      {/* Header */}
      <div className="flex items-center justify-between border-b px-4 py-3">
        <div className="flex items-center gap-3">
          <button
            onClick={() =>
              navigate(workflowId ? `/workflows/${workflowId}` : "/workflows")
            }
            className="rounded-md p-1 hover:bg-muted"
          >
            <ArrowLeft className="h-5 w-5" />
          </button>
          <div>
            <h1 className="text-lg font-bold">
              {t("runs.title")} {runId?.substring(0, 8)}
            </h1>
            <div className="flex items-center gap-2 text-xs text-muted-foreground">
              <span className="capitalize font-medium">
                {t(`runs.status.${run.Status}`)}
              </span>
              <span>|</span>
              <span>Tier: {run.Tier}</span>
              <span>|</span>
              <span>
                {completedSteps}/{totalSteps} steps
              </span>
            </div>
          </div>
        </div>

        {/* Controls */}
        <div className="flex items-center gap-2">
          {run.Status === "running" && (
            <button className="inline-flex items-center gap-1.5 rounded-md border px-3 py-1.5 text-sm hover:bg-muted">
              <Pause className="h-4 w-4" />
              {t("runs.controls.pause")}
            </button>
          )}
          {run.Status === "paused" && (
            <button className="inline-flex items-center gap-1.5 rounded-md border px-3 py-1.5 text-sm hover:bg-muted">
              <Play className="h-4 w-4" />
              {t("runs.controls.resume")}
            </button>
          )}
          {(run.Status === "running" || run.Status === "paused") && (
            <button className="inline-flex items-center gap-1.5 rounded-md border border-destructive px-3 py-1.5 text-sm text-destructive hover:bg-destructive/10">
              <Ban className="h-4 w-4" />
              {t("runs.controls.cancel")}
            </button>
          )}
        </div>
      </div>

      {/* Progress bar */}
      <div className="h-1 bg-muted">
        <div
          className="h-full bg-primary transition-all duration-500"
          style={{ width: `${progressPct}%` }}
        />
      </div>

      {/* Content */}
      <div className="flex-1 overflow-y-auto">
        <div className="mx-auto max-w-6xl p-4 space-y-4">
          {/* Pipeline */}
          <PipelineView
            stepRuns={stepRuns}
            stepNames={stepNames}
            selectedStepId={selectedStepId}
            onSelectStep={setSelectedStepId}
          />

          {/* Two-column: step detail + event log */}
          <div className="grid grid-cols-1 lg:grid-cols-2 gap-4">
            {/* Step detail */}
            <div>
              {selectedStep ? (
                <StepDetail
                  stepRun={selectedStep}
                  stepName={stepNames[selectedStep.StepID] ?? "Step"}
                  runId={runId ?? ""}
                  events={events}
                  onClose={() => setSelectedStepId(null)}
                />
              ) : (
                <div className="flex items-center justify-center rounded-lg border border-dashed p-12 text-sm text-muted-foreground">
                  Click a step in the pipeline to view details
                </div>
              )}
            </div>

            {/* Event log */}
            <div className="h-[400px] lg:h-auto">
              <EventLog events={events} />
            </div>
          </div>
        </div>
      </div>
    </div>
  );
}
