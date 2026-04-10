import { useState } from "react";
import { useTranslation } from "react-i18next";
import { CheckCircle2, XCircle, User, Bot, Clock } from "lucide-react";
import { useApproveGate, useRejectGate } from "@/hooks/use-ardenn";
import type { ArdennStepRunState, ArdennEvent } from "@/types/ardenn";

interface StepDetailProps {
  stepRun: ArdennStepRunState;
  stepName: string;
  runId: string;
  events: ArdennEvent[];
  onClose: () => void;
}

export function StepDetail({
  stepRun,
  stepName,
  runId,
  events,
  onClose,
}: StepDetailProps) {
  const { t } = useTranslation("ardenn");
  const [feedback, setFeedback] = useState("");
  const approveGate = useApproveGate();
  const rejectGate = useRejectGate();

  const stepEvents = events.filter((e) => e.step_id === stepRun.StepID);
  const isWaitingGate = stepRun.Status === "waiting_gate" && stepRun.GateStatus === "pending";

  const handleApprove = async () => {
    await approveGate.mutateAsync({
      runId,
      stepId: stepRun.StepID,
      feedback: feedback || undefined,
    });
    setFeedback("");
  };

  const handleReject = async () => {
    await rejectGate.mutateAsync({
      runId,
      stepId: stepRun.StepID,
      feedback: feedback || undefined,
    });
    setFeedback("");
  };

  return (
    <div className="rounded-lg border bg-card overflow-hidden">
      {/* Header */}
      <div className="flex items-center justify-between border-b bg-muted/30 px-4 py-3">
        <div>
          <h3 className="font-semibold">{stepName}</h3>
          <p className="text-xs text-muted-foreground">
            {t(`runs.status.${stepRun.Status}`)} | {stepRun.HandType ?? "unknown"} hand
          </p>
        </div>
        <button
          onClick={onClose}
          className="rounded-md p-1 hover:bg-muted text-muted-foreground"
        >
          <XCircle className="h-4 w-4" />
        </button>
      </div>

      <div className="p-4 space-y-4">
        {/* Status overview */}
        <div className="grid grid-cols-2 sm:grid-cols-4 gap-3">
          <div>
            <label className="text-[10px] font-medium text-muted-foreground uppercase">
              Status
            </label>
            <p className="text-sm font-medium capitalize">{stepRun.Status}</p>
          </div>
          <div>
            <label className="text-[10px] font-medium text-muted-foreground uppercase">
              Hand Type
            </label>
            <p className="text-sm">{stepRun.HandType ?? "-"}</p>
          </div>
          <div>
            <label className="text-[10px] font-medium text-muted-foreground uppercase">
              Dispatches
            </label>
            <p className="text-sm">{stepRun.DispatchCount}</p>
          </div>
          {stepRun.EvalScore > 0 && (
            <div>
              <label className="text-[10px] font-medium text-muted-foreground uppercase">
                Eval Score
              </label>
              <p className="text-sm">
                {(stepRun.EvalScore * 100).toFixed(0)}% (round {stepRun.EvalRound})
              </p>
            </div>
          )}
        </div>

        {/* Assignee */}
        <div>
          <label className="text-[10px] font-medium text-muted-foreground uppercase">
            Assignee
          </label>
          <div className="flex items-center gap-1.5 mt-1">
            {stepRun.AssignedAgent ? (
              <>
                <Bot className="h-4 w-4 text-muted-foreground" />
                <span className="text-sm font-mono">{stepRun.AssignedAgent}</span>
              </>
            ) : stepRun.AssignedUser ? (
              <>
                <User className="h-4 w-4 text-muted-foreground" />
                <span className="text-sm">{stepRun.AssignedUser}</span>
              </>
            ) : (
              <span className="text-sm text-muted-foreground">Unassigned</span>
            )}
          </div>
        </div>

        {/* Result preview */}
        {stepRun.Result && (
          <div>
            <label className="text-[10px] font-medium text-muted-foreground uppercase">
              Result
            </label>
            <pre className="mt-1 max-h-40 overflow-y-auto rounded-md bg-muted p-2 text-xs font-mono whitespace-pre-wrap">
              {stepRun.Result}
            </pre>
          </div>
        )}

        {/* Gate section */}
        {isWaitingGate && (
          <div className="rounded-md border-2 border-yellow-500 bg-yellow-50 dark:bg-yellow-950 p-4 space-y-3">
            <div className="flex items-center gap-2">
              <Clock className="h-4 w-4 text-yellow-600" />
              <span className="text-sm font-semibold text-yellow-800 dark:text-yellow-200">
                Approval Required
              </span>
            </div>
            <textarea
              value={feedback}
              onChange={(e) => setFeedback(e.target.value)}
              placeholder={t("runs.gate.feedbackPlaceholder")}
              rows={2}
              className="w-full rounded-md border bg-background px-3 py-2 text-base md:text-sm resize-none"
            />
            <div className="flex gap-2">
              <button
                onClick={handleApprove}
                disabled={approveGate.isPending}
                className="inline-flex items-center gap-1.5 rounded-md bg-green-600 px-4 py-2 text-sm font-medium text-white hover:bg-green-700 disabled:opacity-50"
              >
                <CheckCircle2 className="h-4 w-4" />
                {approveGate.isPending ? "Approving..." : t("runs.gate.approve")}
              </button>
              <button
                onClick={handleReject}
                disabled={rejectGate.isPending}
                className="inline-flex items-center gap-1.5 rounded-md bg-red-600 px-4 py-2 text-sm font-medium text-white hover:bg-red-700 disabled:opacity-50"
              >
                <XCircle className="h-4 w-4" />
                {rejectGate.isPending ? "Rejecting..." : t("runs.gate.reject")}
              </button>
            </div>
          </div>
        )}

        {/* Gate decided */}
        {stepRun.GateStatus && stepRun.GateStatus !== "pending" && (
          <div
            className={`rounded-md border p-3 text-sm ${
              stepRun.GateStatus === "approved"
                ? "border-green-300 bg-green-50 dark:bg-green-950"
                : "border-red-300 bg-red-50 dark:bg-red-950"
            }`}
          >
            Gate: <strong className="capitalize">{stepRun.GateStatus}</strong>
            {stepRun.GateDecidedBy && (
              <span className="ml-1 text-muted-foreground">
                by {stepRun.GateDecidedBy}
              </span>
            )}
          </div>
        )}

        {/* Step events timeline */}
        {stepEvents.length > 0 && (
          <div>
            <label className="text-[10px] font-medium text-muted-foreground uppercase">
              Step Events ({stepEvents.length})
            </label>
            <div className="mt-1 max-h-40 overflow-y-auto space-y-1">
              {stepEvents.map((e) => (
                <div
                  key={e.id}
                  className="flex items-start gap-2 text-xs py-1"
                >
                  <span className="shrink-0 text-muted-foreground font-mono w-6 text-right">
                    #{e.sequence}
                  </span>
                  <span className="font-medium font-mono">{e.event_type}</span>
                  <span className="text-muted-foreground truncate">
                    {new Date(e.created_at).toLocaleTimeString()}
                  </span>
                </div>
              ))}
            </div>
          </div>
        )}
      </div>
    </div>
  );
}
