import { useState, useMemo } from "react";
import { useNavigate } from "react-router";
import { useTranslation } from "react-i18next";
import {
  CheckSquare,
  CheckCircle2,
  XCircle,
  ExternalLink,
  AlertTriangle,
  Clock,
  Inbox,
} from "lucide-react";
import { useArdennMyTasks, useApproveGate, useRejectGate } from "@/hooks/use-ardenn";
import { useWsEvent } from "@/hooks/use-ws-event";
import { useQueryClient } from "@tanstack/react-query";
import { queryKeys } from "@/lib/query-keys";
import type { ArdennMyTask } from "@/types/ardenn";

export function MyTasksPage() {
  const { t } = useTranslation("ardenn");
  const navigate = useNavigate();
  const { data: tasks, isLoading } = useArdennMyTasks();
  const approveGate = useApproveGate();
  const rejectGate = useRejectGate();
  const qc = useQueryClient();

  // Feedback state per task
  const [feedbackMap, setFeedbackMap] = useState<Record<string, string>>({});

  // Real-time refresh when new tasks arrive
  useWsEvent("ardenn.run.event", () => {
    qc.invalidateQueries({ queryKey: queryKeys.ardenn.myTasks });
  });

  // Split tasks into sections
  const approvals = useMemo(
    () =>
      (tasks ?? []).filter(
        (task) =>
          task.gateStatus === "pending" && task.status === "waiting_gate",
      ),
    [tasks],
  );

  const assigned = useMemo(
    () =>
      (tasks ?? []).filter(
        (task) => task.status === "running" && task.gateStatus !== "pending",
      ),
    [tasks],
  );

  const handleApprove = async (task: ArdennMyTask) => {
    await approveGate.mutateAsync({
      runId: task.runId,
      stepId: task.stepId,
      feedback: feedbackMap[task.id] || undefined,
    });
    setFeedbackMap((m) => {
      const next = { ...m };
      delete next[task.id];
      return next;
    });
  };

  const handleReject = async (task: ArdennMyTask) => {
    await rejectGate.mutateAsync({
      runId: task.runId,
      stepId: task.stepId,
      feedback: feedbackMap[task.id] || undefined,
    });
    setFeedbackMap((m) => {
      const next = { ...m };
      delete next[task.id];
      return next;
    });
  };

  const openRun = (task: ArdennMyTask) => {
    navigate(`/workflows/${task.workflowId}/runs/${task.runId}`);
  };

  return (
    <div className="flex h-full flex-col gap-6 p-6">
      {/* Header */}
      <div>
        <h1 className="text-2xl font-bold">{t("myTasks.title")}</h1>
        <p className="text-sm text-muted-foreground">
          {t("myTasks.description")}
        </p>
      </div>

      {isLoading ? (
        <div className="space-y-3">
          {Array.from({ length: 3 }).map((_, i) => (
            <div key={i} className="h-20 animate-pulse rounded-md bg-muted" />
          ))}
        </div>
      ) : (tasks ?? []).length === 0 ? (
        <div className="flex flex-1 flex-col items-center justify-center gap-3 text-center">
          <Inbox className="h-12 w-12 text-muted-foreground/50" />
          <h3 className="text-lg font-medium">{t("myTasks.empty")}</h3>
          <p className="text-sm text-muted-foreground">
            {t("myTasks.emptyDescription")}
          </p>
        </div>
      ) : (
        <div className="space-y-6">
          {/* Approvals section */}
          {approvals.length > 0 && (
            <div>
              <h2 className="flex items-center gap-2 text-sm font-semibold uppercase text-yellow-600 mb-3">
                <AlertTriangle className="h-4 w-4" />
                {t("myTasks.sections.approvals")} ({approvals.length})
              </h2>
              <div className="space-y-3">
                {approvals.map((task) => (
                  <div
                    key={task.id}
                    className="rounded-lg border-2 border-yellow-300 dark:border-yellow-700 bg-card p-4 space-y-3"
                  >
                    <div className="flex items-start justify-between">
                      <div>
                        <p className="font-medium">{task.stepName}</p>
                        <p className="text-xs text-muted-foreground">
                          {task.workflowName} | Step in {task.runId.substring(0, 8)}
                        </p>
                      </div>
                      <div className="flex items-center gap-1 text-xs text-muted-foreground">
                        <Clock className="h-3 w-3" />
                        {new Date(task.createdAt).toLocaleString()}
                      </div>
                    </div>

                    {/* Feedback input */}
                    <textarea
                      value={feedbackMap[task.id] ?? ""}
                      onChange={(e) =>
                        setFeedbackMap((m) => ({
                          ...m,
                          [task.id]: e.target.value,
                        }))
                      }
                      placeholder="Feedback (optional)..."
                      rows={2}
                      className="w-full rounded-md border bg-background px-3 py-2 text-base md:text-sm resize-none"
                    />

                    {/* Action buttons */}
                    <div className="flex items-center gap-2">
                      <button
                        onClick={() => handleApprove(task)}
                        disabled={approveGate.isPending}
                        className="inline-flex items-center gap-1.5 rounded-md bg-green-600 px-3 py-1.5 text-sm font-medium text-white hover:bg-green-700 disabled:opacity-50"
                      >
                        <CheckCircle2 className="h-4 w-4" />
                        {t("myTasks.actions.approve")}
                      </button>
                      <button
                        onClick={() => handleReject(task)}
                        disabled={rejectGate.isPending}
                        className="inline-flex items-center gap-1.5 rounded-md bg-red-600 px-3 py-1.5 text-sm font-medium text-white hover:bg-red-700 disabled:opacity-50"
                      >
                        <XCircle className="h-4 w-4" />
                        {t("myTasks.actions.reject")}
                      </button>
                      <button
                        onClick={() => openRun(task)}
                        className="inline-flex items-center gap-1.5 rounded-md border px-3 py-1.5 text-sm hover:bg-muted"
                      >
                        <ExternalLink className="h-4 w-4" />
                        {t("myTasks.actions.open")}
                      </button>
                    </div>
                  </div>
                ))}
              </div>
            </div>
          )}

          {/* Assigned section */}
          {assigned.length > 0 && (
            <div>
              <h2 className="flex items-center gap-2 text-sm font-semibold uppercase text-blue-600 mb-3">
                <CheckSquare className="h-4 w-4" />
                {t("myTasks.sections.assigned")} ({assigned.length})
              </h2>
              <div className="space-y-2">
                {assigned.map((task) => (
                  <div
                    key={task.id}
                    className="flex items-center justify-between rounded-lg border bg-card p-3 hover:bg-muted/50 transition-colors"
                  >
                    <div>
                      <p className="font-medium text-sm">{task.stepName}</p>
                      <p className="text-xs text-muted-foreground">
                        {task.workflowName} | {task.handType ?? "agent"} hand
                      </p>
                    </div>
                    <div className="flex items-center gap-2">
                      <span className="text-xs text-muted-foreground">
                        {new Date(task.createdAt).toLocaleString()}
                      </span>
                      <button
                        onClick={() => openRun(task)}
                        className="inline-flex items-center gap-1 rounded-md border px-2.5 py-1 text-xs hover:bg-muted"
                      >
                        <ExternalLink className="h-3 w-3" />
                        {t("myTasks.actions.open")}
                      </button>
                    </div>
                  </div>
                ))}
              </div>
            </div>
          )}
        </div>
      )}
    </div>
  );
}
