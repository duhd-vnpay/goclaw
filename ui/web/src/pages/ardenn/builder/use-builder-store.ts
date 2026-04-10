import { create } from "zustand";
import type { ArdennTier, TriggerType, Visibility, StepFormData, VariableFormData } from "@/types/ardenn";

interface BuilderState {
  // Workflow properties
  id: string | null; // null for new, set for edit
  name: string;
  slug: string;
  description: string;
  domainId: string;
  tier: ArdennTier;
  triggerType: TriggerType;
  visibility: Visibility;

  // Variables
  variables: VariableFormData[];

  // Steps
  steps: StepFormData[];

  // Dirty tracking
  isDirty: boolean;

  // Actions — properties
  setName: (name: string) => void;
  setSlug: (slug: string) => void;
  setDescription: (description: string) => void;
  setDomainId: (domainId: string) => void;
  setTier: (tier: ArdennTier) => void;
  setTriggerType: (triggerType: TriggerType) => void;
  setVisibility: (visibility: Visibility) => void;

  // Actions — variables
  addVariable: () => void;
  removeVariable: (index: number) => void;
  updateVariable: (index: number, field: keyof VariableFormData, value: unknown) => void;

  // Actions — steps
  addStep: () => void;
  removeStep: (index: number) => void;
  updateStep: (index: number, field: keyof StepFormData, value: unknown) => void;
  moveStep: (from: number, to: number) => void;

  // Actions — lifecycle
  loadWorkflow: (data: {
    id: string;
    name: string;
    slug: string;
    description: string;
    domainId: string;
    tier: ArdennTier;
    triggerType: TriggerType;
    visibility: Visibility;
    variables: VariableFormData[];
    steps: StepFormData[];
  }) => void;
  reset: () => void;
  toCreatePayload: () => Record<string, unknown>;
}

const defaultStep: StepFormData = {
  slug: "",
  name: "",
  description: "",
  position: 0,
  agentKey: "",
  taskTemplate: "",
  dependsOn: [],
  condition: "",
  timeout: "30 minutes",
  dispatchTo: "agent",
  dispatchTarget: "",
  gate: {},
  constraints: {},
  continuity: {},
  evaluation: {},
};

const defaultVariable: VariableFormData = {
  name: "",
  type: "text",
  required: false,
};

export const useBuilderStore = create<BuilderState>((set, get) => ({
  // Initial state
  id: null,
  name: "",
  slug: "",
  description: "",
  domainId: "",
  tier: "standard",
  triggerType: "manual",
  visibility: "domain",
  variables: [],
  steps: [],
  isDirty: false,

  // Property setters
  setName: (name) => set({ name, isDirty: true }),
  setSlug: (slug) => set({ slug, isDirty: true }),
  setDescription: (description) => set({ description, isDirty: true }),
  setDomainId: (domainId) => set({ domainId, isDirty: true }),
  setTier: (tier) => set({ tier, isDirty: true }),
  setTriggerType: (triggerType) => set({ triggerType, isDirty: true }),
  setVisibility: (visibility) => set({ visibility, isDirty: true }),

  // Variable actions
  addVariable: () =>
    set((s) => ({
      variables: [...s.variables, { ...defaultVariable }],
      isDirty: true,
    })),
  removeVariable: (index) =>
    set((s) => ({
      variables: s.variables.filter((_, i) => i !== index),
      isDirty: true,
    })),
  updateVariable: (index, field, value) =>
    set((s) => ({
      variables: s.variables.map((v, i) =>
        i === index ? { ...v, [field]: value } : v,
      ),
      isDirty: true,
    })),

  // Step actions
  addStep: () =>
    set((s) => ({
      steps: [
        ...s.steps,
        {
          ...defaultStep,
          slug: `step-${s.steps.length + 1}`,
          name: `Step ${s.steps.length + 1}`,
          position: s.steps.length,
        },
      ],
      isDirty: true,
    })),
  removeStep: (index) =>
    set((s) => ({
      steps: s.steps
        .filter((_, i) => i !== index)
        .map((step, i) => ({ ...step, position: i })),
      isDirty: true,
    })),
  updateStep: (index, field, value) =>
    set((s) => ({
      steps: s.steps.map((step, i) =>
        i === index ? { ...step, [field]: value } : step,
      ),
      isDirty: true,
    })),
  moveStep: (from, to) =>
    set((s) => {
      if (to < 0 || to >= s.steps.length) return s;
      const steps = [...s.steps];
      const removed = steps.splice(from, 1);
      if (!removed[0]) return s;
      steps.splice(to, 0, removed[0]);
      return {
        steps: steps.map((step, i) => ({ ...step, position: i })),
        isDirty: true,
      };
    }),

  // Lifecycle
  loadWorkflow: (data) =>
    set({
      id: data.id,
      name: data.name,
      slug: data.slug,
      description: data.description,
      domainId: data.domainId,
      tier: data.tier,
      triggerType: data.triggerType,
      visibility: data.visibility,
      variables: data.variables,
      steps: data.steps,
      isDirty: false,
    }),
  reset: () =>
    set({
      id: null,
      name: "",
      slug: "",
      description: "",
      domainId: "",
      tier: "standard",
      triggerType: "manual",
      visibility: "domain",
      variables: [],
      steps: [],
      isDirty: false,
    }),
  toCreatePayload: () => {
    const s = get();
    return {
      domainId: s.domainId,
      name: s.name,
      slug: s.slug,
      description: s.description,
      tier: s.tier,
      triggerConfig: { type: s.triggerType },
      variables: Object.fromEntries(
        s.variables.map((v) => [
          v.name,
          { type: v.type, required: v.required, options: v.options, default: v.defaultValue },
        ]),
      ),
      visibility: s.visibility,
      steps: s.steps.map((step) => ({
        slug: step.slug,
        name: step.name,
        description: step.description,
        position: step.position,
        agentKey: step.agentKey,
        taskTemplate: step.taskTemplate,
        dependsOn: step.dependsOn,
        condition: step.condition,
        timeout: step.timeout,
        dispatchTo: step.dispatchTo,
        dispatchTarget: step.dispatchTarget,
        gate: step.gate,
        constraints: step.constraints,
        continuity: step.continuity,
        evaluation: step.evaluation,
      })),
    };
  },
}));
