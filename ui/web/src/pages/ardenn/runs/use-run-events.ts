import { useState, useCallback, useEffect, useRef } from "react";
import { useWs } from "@/hooks/use-ws";
import { useWsEvent } from "@/hooks/use-ws-event";
import type { ArdennRunState, ArdennEvent } from "@/types/ardenn";

interface RunEventsState {
  run: ArdennRunState | null;
  events: ArdennEvent[];
  isLoading: boolean;
  error: string | null;
}

/**
 * useRunEvents — combines initial state fetch with live event streaming.
 *
 * 1. Fetches `ardenn.runs.get` for initial RunState + events
 * 2. Subscribes to `ardenn.run.event` WS events for the given runId
 * 3. Merges new events into local state in real-time
 * 4. Updates step run states based on incoming events
 */
export function useRunEvents(runId: string) {
  const ws = useWs();
  const [state, setState] = useState<RunEventsState>({
    run: null,
    events: [],
    isLoading: true,
    error: null,
  });
  const lastSequenceRef = useRef(0);

  // Initial fetch
  useEffect(() => {
    if (!runId) return;

    setState((s) => ({ ...s, isLoading: true, error: null }));

    ws.call<{ run: ArdennRunState; events: ArdennEvent[] }>(
      "ardenn.runs.get",
      { runId },
    )
      .then((result) => {
        setState({
          run: result.run,
          events: result.events ?? [],
          isLoading: false,
          error: null,
        });
        // Track last sequence for incremental fetch
        const maxSeq = Math.max(
          0,
          ...(result.events ?? []).map((e) => e.sequence),
        );
        lastSequenceRef.current = maxSeq;
      })
      .catch((err) => {
        setState((s) => ({
          ...s,
          isLoading: false,
          error: err?.message ?? "Failed to load run",
        }));
      });
  }, [ws, runId]);

  // Live event stream subscription
  useWsEvent("ardenn.run.event", (payload: unknown) => {
    const event = payload as ArdennEvent;
    if (event.run_id !== runId) return;

    // Deduplicate by sequence
    if (event.sequence <= lastSequenceRef.current) return;
    lastSequenceRef.current = event.sequence;

    setState((s) => {
      const newEvents = [...s.events, event];
      const updatedRun = s.run ? applyEventToRunState(s.run, event) : s.run;
      return { ...s, run: updatedRun, events: newEvents };
    });
  });

  // Manual refetch (for recovery)
  const refetch = useCallback(() => {
    if (!runId) return;
    ws.call<ArdennEvent[]>("ardenn.events.stream", {
      runId,
      fromSequence: lastSequenceRef.current,
    }).then((newEvents) => {
      if (!newEvents?.length) return;
      setState((s) => ({
        ...s,
        events: [...s.events, ...newEvents],
      }));
      const maxSeq = Math.max(0, ...newEvents.map((e) => e.sequence));
      lastSequenceRef.current = maxSeq;
    });
  }, [ws, runId]);

  // Poll for new events every 5s as fallback
  useEffect(() => {
    if (!runId || state.isLoading) return;
    const interval = setInterval(refetch, 5_000);
    return () => clearInterval(interval);
  }, [runId, state.isLoading, refetch]);

  return {
    run: state.run,
    events: state.events,
    isLoading: state.isLoading,
    error: state.error,
    refetch,
  };
}

/**
 * Apply a single event to the run state (client-side projection).
 * This provides optimistic UI updates before the next full fetch.
 */
function applyEventToRunState(
  run: ArdennRunState,
  event: ArdennEvent,
): ArdennRunState {
  const updated = { ...run, LastSequence: event.sequence };

  switch (event.event_type) {
    case "run.started":
      updated.Status = "running";
      break;
    case "run.completed":
      updated.Status = "completed";
      break;
    case "run.failed":
      updated.Status = "failed";
      break;
    case "run.cancelled":
      updated.Status = "cancelled";
      break;
    case "run.paused":
      updated.Status = "paused";
      break;
    case "run.resumed":
      updated.Status = "running";
      break;
  }

  // Step-level events
  if (event.step_id && updated.StepRuns) {
    const stepRun = Object.values(updated.StepRuns).find(
      (sr) => sr.StepID === event.step_id,
    );
    if (stepRun) {
      const updatedStep = { ...stepRun };
      switch (event.event_type) {
        case "step.dispatched":
          updatedStep.Status = "running";
          updatedStep.HandType =
            (event.payload.hand_type as string) ?? updatedStep.HandType;
          updatedStep.DispatchCount =
            (event.payload.dispatch_count as number) ??
            updatedStep.DispatchCount;
          break;
        case "step.result":
          updatedStep.Result = (event.payload.output as string) ?? "";
          break;
        case "step.completed":
          updatedStep.Status = "completed";
          break;
        case "step.failed":
          updatedStep.Status = "failed";
          break;
        case "step.cancelled":
          updatedStep.Status = "cancelled";
          break;
        case "step.skipped":
          updatedStep.Status = "skipped";
          break;
        case "gate.pending":
          updatedStep.Status = "waiting_gate";
          updatedStep.GateStatus = "pending";
          break;
        case "gate.approved":
          updatedStep.GateStatus = "approved";
          break;
        case "gate.rejected":
          updatedStep.GateStatus = "rejected";
          updatedStep.Status = "running"; // will be retried
          break;
        case "eval.round_passed":
          updatedStep.EvalScore =
            (event.payload.score as number) ?? updatedStep.EvalScore;
          updatedStep.EvalPassed = true;
          break;
        case "eval.round_failed":
          updatedStep.EvalScore =
            (event.payload.score as number) ?? updatedStep.EvalScore;
          updatedStep.EvalPassed = false;
          updatedStep.EvalRound = updatedStep.EvalRound + 1;
          break;
      }
      updated.StepRuns = {
        ...updated.StepRuns,
        [stepRun.ID]: updatedStep,
      };
    }
  }

  return updated;
}
