import {
  CheckCircle2,
  XCircle,
  Loader2,
  Clock,
  PauseCircle,
  SkipForward,
  Ban,
  ArrowRight,
  Bot,
  User,
  Globe,
  Plug,
} from "lucide-react";
import type { ArdennStepRunState, StepRunStatus } from "@/types/ardenn";

interface PipelineViewProps {
  stepRuns: ArdennStepRunState[];
  stepNames: Record<string, string>; // stepId -> name
  selectedStepId: string | null;
  onSelectStep: (stepId: string) => void;
}

const STATUS_ICONS: Record<StepRunStatus, typeof CheckCircle2> = {
  completed: CheckCircle2,
  failed: XCircle,
  running: Loader2,
  pending: Clock,
  waiting_gate: PauseCircle,
  blocked: Ban,
  skipped: SkipForward,
  cancelled: Ban,
};

const STATUS_COLORS: Record<StepRunStatus, string> = {
  completed: "text-green-500 border-green-500",
  failed: "text-red-500 border-red-500",
  running: "text-blue-500 border-blue-500",
  pending: "text-muted-foreground border-muted",
  waiting_gate: "text-yellow-500 border-yellow-500",
  blocked: "text-muted-foreground border-muted",
  skipped: "text-muted-foreground border-muted",
  cancelled: "text-muted-foreground border-muted",
};

const HAND_ICONS: Record<string, typeof Bot> = {
  agent: Bot,
  user: User,
  api: Globe,
  mcp: Plug,
};

export function PipelineView({
  stepRuns,
  stepNames,
  selectedStepId,
  onSelectStep,
}: PipelineViewProps) {
  // Sort by position (via StepID order in the map)
  const sorted = [...stepRuns].sort((a, b) => {
    // Use step names as proxy for ordering when position isn't available
    return (stepNames[a.StepID] ?? "").localeCompare(stepNames[b.StepID] ?? "");
  });

  return (
    <div className="rounded-lg border bg-card p-4">
      <div className="flex items-center gap-1 overflow-x-auto pb-2">
        {sorted.map((sr, i) => {
          const status = sr.Status as StepRunStatus;
          const StatusIcon = STATUS_ICONS[status] ?? Clock;
          const HandIcon = HAND_ICONS[sr.HandType] ?? Bot;
          const colorClass = STATUS_COLORS[status] ?? "";
          const isSelected = selectedStepId === sr.StepID;
          const isRunning = status === "running";

          return (
            <div key={sr.ID} className="flex items-center gap-1.5 shrink-0">
              {i > 0 && (
                <ArrowRight className="h-4 w-4 text-muted-foreground shrink-0" />
              )}
              <button
                type="button"
                onClick={() => onSelectStep(sr.StepID)}
                className={`flex flex-col items-center gap-1.5 rounded-lg border-2 px-3 py-2.5 min-w-[90px] transition-all ${colorClass} ${
                  isSelected
                    ? "ring-2 ring-primary ring-offset-1 bg-accent"
                    : "bg-background hover:bg-muted/50"
                }`}
              >
                {/* Status icon */}
                <StatusIcon
                  className={`h-5 w-5 ${isRunning ? "animate-spin" : ""}`}
                />

                {/* Step name */}
                <span className="text-[11px] font-medium text-center leading-tight max-w-[80px] truncate">
                  {stepNames[sr.StepID] ?? `Step ${i + 1}`}
                </span>

                {/* Meta row */}
                <div className="flex items-center gap-1.5 text-[9px] text-muted-foreground">
                  <HandIcon className="h-3 w-3" />
                  {sr.EvalScore > 0 && (
                    <span className="font-mono">
                      {(sr.EvalScore * 100).toFixed(0)}%
                    </span>
                  )}
                </div>

                {/* Gate indicator */}
                {sr.GateStatus === "pending" && (
                  <span className="text-[9px] font-medium text-yellow-600 bg-yellow-100 dark:bg-yellow-900 px-1.5 rounded-full">
                    GATE
                  </span>
                )}
              </button>
            </div>
          );
        })}
      </div>
    </div>
  );
}
