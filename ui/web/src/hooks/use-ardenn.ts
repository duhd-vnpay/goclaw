import { useQuery, useMutation, useQueryClient } from "@tanstack/react-query";
import { useWs } from "./use-ws";
import { queryKeys } from "@/lib/query-keys";
import type {
  ArdennDomain,
  ArdennWorkflow,
  ArdennMyTask,
  WorkflowWithSteps,
  RunWithEvents,
} from "@/types/ardenn";

// --- Domains ---

export function useArdennDomains() {
  const ws = useWs();
  return useQuery({
    queryKey: queryKeys.ardenn.domains,
    queryFn: () => ws.call<ArdennDomain[]>("ardenn.domains.list"),
    staleTime: 60_000,
  });
}

export function useCreateDomain() {
  const ws = useWs();
  const qc = useQueryClient();
  return useMutation({
    mutationFn: (params: {
      name: string;
      slug: string;
      description?: string;
      departmentId?: string;
      defaultTier?: string;
    }) => ws.call<ArdennDomain>("ardenn.domains.create", params),
    onSuccess: () => qc.invalidateQueries({ queryKey: queryKeys.ardenn.domains }),
  });
}

export function useDeleteDomain() {
  const ws = useWs();
  const qc = useQueryClient();
  return useMutation({
    mutationFn: (id: string) => ws.call("ardenn.domains.delete", { id }),
    onSuccess: () => qc.invalidateQueries({ queryKey: queryKeys.ardenn.domains }),
  });
}

// --- Workflows ---

export function useArdennWorkflows(params?: { domainId?: string; status?: string }) {
  const ws = useWs();
  return useQuery({
    queryKey: queryKeys.ardenn.workflows.list(params ?? {}),
    queryFn: () => ws.call<ArdennWorkflow[]>("ardenn.workflows.list", params),
    staleTime: 30_000,
  });
}

export function useArdennWorkflow(id: string) {
  const ws = useWs();
  return useQuery({
    queryKey: queryKeys.ardenn.workflows.detail(id),
    queryFn: () => ws.call<WorkflowWithSteps>("ardenn.workflows.get", { id }),
    enabled: !!id,
    staleTime: 30_000,
  });
}

export function useCreateWorkflow() {
  const ws = useWs();
  const qc = useQueryClient();
  return useMutation({
    mutationFn: (params: Record<string, unknown>) =>
      ws.call<ArdennWorkflow>("ardenn.workflows.create", params),
    onSuccess: () => qc.invalidateQueries({ queryKey: queryKeys.ardenn.workflows.all }),
  });
}

export function useUpdateWorkflow() {
  const ws = useWs();
  const qc = useQueryClient();
  return useMutation({
    mutationFn: (params: Record<string, unknown>) =>
      ws.call<ArdennWorkflow>("ardenn.workflows.update", params),
    onSuccess: (_data, params) => {
      qc.invalidateQueries({ queryKey: queryKeys.ardenn.workflows.all });
      if (params.id) {
        qc.invalidateQueries({ queryKey: queryKeys.ardenn.workflows.detail(params.id as string) });
      }
    },
  });
}

export function usePublishWorkflow() {
  const ws = useWs();
  const qc = useQueryClient();
  return useMutation({
    mutationFn: (id: string) => ws.call<ArdennWorkflow>("ardenn.workflows.publish", { id }),
    onSuccess: () => qc.invalidateQueries({ queryKey: queryKeys.ardenn.workflows.all }),
  });
}

export function useDeleteWorkflow() {
  const ws = useWs();
  const qc = useQueryClient();
  return useMutation({
    mutationFn: (id: string) => ws.call("ardenn.workflows.delete", { id }),
    onSuccess: () => qc.invalidateQueries({ queryKey: queryKeys.ardenn.workflows.all }),
  });
}

// --- Runs ---

export function useArdennRuns(workflowId: string) {
  const ws = useWs();
  return useQuery({
    queryKey: queryKeys.ardenn.runs.list(workflowId),
    queryFn: () => ws.call<RunWithEvents[]>("ardenn.runs.list", { workflowId }),
    enabled: !!workflowId,
    staleTime: 10_000,
  });
}

export function useArdennRunState(runId: string) {
  const ws = useWs();
  return useQuery({
    queryKey: queryKeys.ardenn.runs.detail(runId),
    queryFn: () => ws.call<RunWithEvents>("ardenn.runs.get", { runId }),
    enabled: !!runId,
    refetchInterval: 5_000, // Poll every 5s for active runs
  });
}

export function useStartRun() {
  const ws = useWs();
  const qc = useQueryClient();
  return useMutation({
    mutationFn: (params: { workflowId: string; projectId?: string; variables?: Record<string, unknown> }) =>
      ws.call<{ runId: string }>("ardenn.runs.start", params),
    onSuccess: () => qc.invalidateQueries({ queryKey: queryKeys.ardenn.runs.all }),
  });
}

export function useApproveGate() {
  const ws = useWs();
  const qc = useQueryClient();
  return useMutation({
    mutationFn: (params: { runId: string; stepId: string; feedback?: string }) =>
      ws.call("ardenn.runs.approve", params),
    onSuccess: () => {
      qc.invalidateQueries({ queryKey: queryKeys.ardenn.myTasks });
    },
  });
}

export function useRejectGate() {
  const ws = useWs();
  const qc = useQueryClient();
  return useMutation({
    mutationFn: (params: { runId: string; stepId: string; feedback?: string }) =>
      ws.call("ardenn.runs.reject", params),
    onSuccess: () => {
      qc.invalidateQueries({ queryKey: queryKeys.ardenn.myTasks });
    },
  });
}

// --- My Tasks ---

export function useArdennMyTasks() {
  const ws = useWs();
  return useQuery({
    queryKey: queryKeys.ardenn.myTasks,
    queryFn: () => ws.call<ArdennMyTask[]>("ardenn.tasks.my"),
    staleTime: 10_000,
    refetchInterval: 15_000, // Poll for new tasks
  });
}
