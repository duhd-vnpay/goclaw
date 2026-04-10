/** Ardenn types matching Go internal/store/pg/ardenn/ structs */

export type ArdennTier = "light" | "standard" | "full";
export type WorkflowStatus = "draft" | "published" | "archived";
export type RunStatus = "pending" | "running" | "completed" | "failed" | "cancelled" | "paused";
export type StepRunStatus = "pending" | "blocked" | "running" | "waiting_gate" | "completed" | "failed" | "cancelled" | "skipped";
export type GateStatus = "pending" | "approved" | "rejected" | "auto_passed" | "timed_out";
export type HandType = "agent" | "user" | "api" | "mcp";
export type TriggerType = "manual" | "webhook" | "event" | "cron";
export type Visibility = "domain" | "department" | "project" | "public";

export interface ArdennDomain {
  id: string;
  tenant_id: string;
  slug: string;
  name: string;
  description?: string;
  department_id?: string;
  default_tier: ArdennTier;
  settings: Record<string, unknown>;
}

export interface ArdennWorkflow {
  id: string;
  tenant_id: string;
  domain_id: string;
  slug: string;
  name: string;
  description?: string;
  version: number;
  tier: ArdennTier;
  trigger_config: Record<string, unknown>;
  variables: Record<string, unknown>;
  settings: Record<string, unknown>;
  visibility: Visibility;
  status: WorkflowStatus;
  created_by?: string;
  published_at?: string;
  created_at: string;
  updated_at: string;
}

export interface ArdennStep {
  id: string;
  workflow_id: string;
  slug: string;
  name: string;
  description?: string;
  position: number;
  agent_key?: string;
  task_template?: string;
  depends_on?: string[];
  condition?: string;
  timeout: string;
  constraints: Record<string, unknown>;
  continuity: Record<string, unknown>;
  evaluation: Record<string, unknown>;
  gate: Record<string, unknown>;
  dispatch_to?: string;
  dispatch_target?: string;
}

export interface ArdennRunState {
  ID: string;
  TenantID: string;
  WorkflowID: string;
  ProjectID?: string;
  TriggeredBy?: string;
  Variables: Record<string, unknown>;
  Tier: ArdennTier;
  Status: RunStatus;
  StepRuns: Record<string, ArdennStepRunState>;
  LastSequence: number;
  StartedAt?: string;
  CompletedAt?: string;
}

export interface ArdennStepRunState {
  ID: string;
  StepID: string;
  Status: StepRunStatus;
  AssignedUser?: string;
  AssignedAgent?: string;
  HandType: string;
  Result: string;
  DispatchCount: number;
  EvalRound: number;
  EvalScore: number;
  EvalPassed?: boolean;
  GateStatus: string;
  GateDecidedBy?: string;
  DependsOn: string[];
  Metadata: Record<string, unknown>;
  LastSequence: number;
}

export interface ArdennEvent {
  id: string;
  tenant_id: string;
  run_id: string;
  step_id?: string;
  sequence: number;
  event_type: string;
  actor_type: string;
  actor_id?: string;
  payload: Record<string, unknown>;
  created_at: string;
}

export interface ArdennMyTask {
  id: string;
  runId: string;
  stepId: string;
  stepName: string;
  workflowName: string;
  workflowId: string;
  status: StepRunStatus;
  gateStatus?: GateStatus;
  handType?: HandType;
  createdAt: string;
}

export interface WorkflowWithSteps {
  workflow: ArdennWorkflow;
  steps: ArdennStep[];
}

export interface RunWithEvents {
  run: ArdennRunState;
  events: ArdennEvent[];
}

// Builder types (used by Plan 5B)
export interface StepFormData {
  slug: string;
  name: string;
  description: string;
  position: number;
  agentKey: string;
  taskTemplate: string;
  dependsOn: string[];
  condition: string;
  timeout: string;
  dispatchTo: string;
  dispatchTarget: string;
  gate: Record<string, unknown>;
  constraints: Record<string, unknown>;
  continuity: Record<string, unknown>;
  evaluation: Record<string, unknown>;
}

export interface VariableFormData {
  name: string;
  type: "text" | "number" | "boolean" | "select";
  required: boolean;
  options?: string[];
  defaultValue?: string;
}

export const TIER_LABELS: Record<ArdennTier, string> = {
  light: "Light",
  standard: "Standard",
  full: "Full",
};

export const TIER_COLORS: Record<ArdennTier, string> = {
  light: "bg-muted text-muted-foreground",
  standard: "bg-blue-100 text-blue-800 dark:bg-blue-900 dark:text-blue-300",
  full: "bg-purple-100 text-purple-800 dark:bg-purple-900 dark:text-purple-300",
};

export const STATUS_COLORS: Record<RunStatus, string> = {
  pending: "text-muted-foreground",
  running: "text-blue-600",
  completed: "text-green-600",
  failed: "text-red-600",
  cancelled: "text-muted-foreground",
  paused: "text-yellow-600",
};
