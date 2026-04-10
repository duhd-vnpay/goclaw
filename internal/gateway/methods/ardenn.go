package methods

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"

	"github.com/google/uuid"
	engine "github.com/nextlevelbuilder/goclaw/internal/ardenn"
	"github.com/nextlevelbuilder/goclaw/internal/gateway"
	"github.com/nextlevelbuilder/goclaw/internal/i18n"
	"github.com/nextlevelbuilder/goclaw/internal/permissions"
	"github.com/nextlevelbuilder/goclaw/internal/store"
	pgardenn "github.com/nextlevelbuilder/goclaw/internal/store/pg/ardenn"
	"github.com/nextlevelbuilder/goclaw/pkg/protocol"
)

// ArdennMethods handles ardenn.* RPC methods for the Ardenn workflow engine.
type ArdennMethods struct {
	defStore  *pgardenn.PGDefinitionStore
	projStore *pgardenn.PGProjectionStore
	engine    *engine.Engine
}

func NewArdennMethods(
	defStore *pgardenn.PGDefinitionStore,
	projStore *pgardenn.PGProjectionStore,
	eng *engine.Engine,
) *ArdennMethods {
	return &ArdennMethods{
		defStore:  defStore,
		projStore: projStore,
		engine:    eng,
	}
}

func (m *ArdennMethods) Register(router *gateway.MethodRouter) {
	// Domains
	router.Register(protocol.MethodArdennDomainsList, m.handleDomainsList)
	router.Register(protocol.MethodArdennDomainsCreate, m.handleDomainsCreate)
	router.Register(protocol.MethodArdennDomainsUpdate, m.handleDomainsUpdate)
	router.Register(protocol.MethodArdennDomainsDelete, m.handleDomainsDelete)

	// Workflows
	router.Register(protocol.MethodArdennWorkflowsList, m.handleWorkflowsList)
	router.Register(protocol.MethodArdennWorkflowsGet, m.handleWorkflowsGet)
	router.Register(protocol.MethodArdennWorkflowsCreate, m.handleWorkflowsCreate)
	router.Register(protocol.MethodArdennWorkflowsUpdate, m.handleWorkflowsUpdate)
	router.Register(protocol.MethodArdennWorkflowsPublish, m.handleWorkflowsPublish)
	router.Register(protocol.MethodArdennWorkflowsDelete, m.handleWorkflowsDelete)

	// Runs
	router.Register(protocol.MethodArdennRunsList, m.handleRunsList)
	router.Register(protocol.MethodArdennRunsGet, m.handleRunsGet)
	router.Register(protocol.MethodArdennRunsStart, m.handleRunsStart)
	router.Register(protocol.MethodArdennRunsCancel, m.handleRunsCancel)
	router.Register(protocol.MethodArdennRunsApprove, m.handleRunsApprove)
	router.Register(protocol.MethodArdennRunsReject, m.handleRunsReject)

	// Events + Tasks
	router.Register(protocol.MethodArdennEventsStream, m.handleEventsStream)
	router.Register(protocol.MethodArdennMyTasks, m.handleMyTasks)
}

// --- Domains ---

func (m *ArdennMethods) handleDomainsList(ctx context.Context, client *gateway.Client, req *protocol.RequestFrame) {
	tenantID := store.TenantIDFromContext(ctx)
	domains, err := m.defStore.ListDomains(ctx, tenantID)
	if err != nil {
		client.SendResponse(protocol.NewErrorResponse(req.ID, protocol.ErrInternal, err.Error()))
		return
	}
	client.SendResponse(protocol.NewOKResponse(req.ID, domains))
}

type createDomainReq struct {
	Name         string  `json:"name"`
	Slug         string  `json:"slug"`
	Description  string  `json:"description"`
	DepartmentID *string `json:"departmentId"`
	DefaultTier  string  `json:"defaultTier"`
}

func (m *ArdennMethods) handleDomainsCreate(ctx context.Context, client *gateway.Client, req *protocol.RequestFrame) {
	locale := store.LocaleFromContext(ctx)
	if !permissions.HasMinRole(client.Role(), permissions.RoleAdmin) {
		client.SendResponse(protocol.NewErrorResponse(req.ID, protocol.ErrUnauthorized, i18n.T(locale, i18n.MsgPermissionDenied, "ardenn domains")))
		return
	}

	var p createDomainReq
	if err := json.Unmarshal(req.Params, &p); err != nil {
		client.SendResponse(protocol.NewErrorResponse(req.ID, protocol.ErrInvalidRequest, "invalid params"))
		return
	}
	if p.Name == "" || p.Slug == "" {
		client.SendResponse(protocol.NewErrorResponse(req.ID, protocol.ErrInvalidRequest, "name and slug required"))
		return
	}
	if p.DefaultTier == "" {
		p.DefaultTier = "standard"
	}

	tenantID := store.TenantIDFromContext(ctx)
	var deptID *uuid.UUID
	if p.DepartmentID != nil && *p.DepartmentID != "" {
		parsed, err := uuid.Parse(*p.DepartmentID)
		if err != nil {
			client.SendResponse(protocol.NewErrorResponse(req.ID, protocol.ErrInvalidRequest, "invalid departmentId"))
			return
		}
		deptID = &parsed
	}

	domain, err := m.defStore.CreateDomain(ctx, tenantID, pgardenn.CreateDomainParams{
		Slug:         p.Slug,
		Name:         p.Name,
		Description:  p.Description,
		DepartmentID: deptID,
		DefaultTier:  p.DefaultTier,
	})
	if err != nil {
		slog.Error("ardenn: create domain failed", "error", err)
		client.SendResponse(protocol.NewErrorResponse(req.ID, protocol.ErrInternal, err.Error()))
		return
	}
	client.SendResponse(protocol.NewOKResponse(req.ID, domain))
}

type updateDomainReq struct {
	ID          string  `json:"id"`
	Name        *string `json:"name"`
	Description *string `json:"description"`
	DefaultTier *string `json:"defaultTier"`
}

func (m *ArdennMethods) handleDomainsUpdate(ctx context.Context, client *gateway.Client, req *protocol.RequestFrame) {
	locale := store.LocaleFromContext(ctx)
	if !permissions.HasMinRole(client.Role(), permissions.RoleAdmin) {
		client.SendResponse(protocol.NewErrorResponse(req.ID, protocol.ErrUnauthorized, i18n.T(locale, i18n.MsgPermissionDenied, "ardenn domains")))
		return
	}

	var p updateDomainReq
	if err := json.Unmarshal(req.Params, &p); err != nil {
		client.SendResponse(protocol.NewErrorResponse(req.ID, protocol.ErrInvalidRequest, "invalid params"))
		return
	}

	domainID, err := uuid.Parse(p.ID)
	if err != nil {
		client.SendResponse(protocol.NewErrorResponse(req.ID, protocol.ErrInvalidRequest, "invalid domain id"))
		return
	}

	tenantID := store.TenantIDFromContext(ctx)
	domain, err := m.defStore.UpdateDomain(ctx, tenantID, domainID, pgardenn.UpdateDomainParams{
		Name:        p.Name,
		Description: p.Description,
		DefaultTier: p.DefaultTier,
	})
	if err != nil {
		client.SendResponse(protocol.NewErrorResponse(req.ID, protocol.ErrInternal, err.Error()))
		return
	}
	client.SendResponse(protocol.NewOKResponse(req.ID, domain))
}

type deleteDomainReq struct {
	ID string `json:"id"`
}

func (m *ArdennMethods) handleDomainsDelete(ctx context.Context, client *gateway.Client, req *protocol.RequestFrame) {
	locale := store.LocaleFromContext(ctx)
	if !permissions.HasMinRole(client.Role(), permissions.RoleAdmin) {
		client.SendResponse(protocol.NewErrorResponse(req.ID, protocol.ErrUnauthorized, i18n.T(locale, i18n.MsgPermissionDenied, "ardenn domains")))
		return
	}

	var p deleteDomainReq
	if err := json.Unmarshal(req.Params, &p); err != nil {
		client.SendResponse(protocol.NewErrorResponse(req.ID, protocol.ErrInvalidRequest, "invalid params"))
		return
	}

	domainID, err := uuid.Parse(p.ID)
	if err != nil {
		client.SendResponse(protocol.NewErrorResponse(req.ID, protocol.ErrInvalidRequest, "invalid domain id"))
		return
	}

	tenantID := store.TenantIDFromContext(ctx)
	if err := m.defStore.DeleteDomain(ctx, tenantID, domainID); err != nil {
		client.SendResponse(protocol.NewErrorResponse(req.ID, protocol.ErrInternal, err.Error()))
		return
	}
	client.SendResponse(protocol.NewOKResponse(req.ID, map[string]bool{"ok": true}))
}

// --- Workflows ---

type listWorkflowsReq struct {
	DomainID *string `json:"domainId"`
	Status   *string `json:"status"`
}

func (m *ArdennMethods) handleWorkflowsList(ctx context.Context, client *gateway.Client, req *protocol.RequestFrame) {
	tenantID := store.TenantIDFromContext(ctx)

	var p listWorkflowsReq
	if req.Params != nil {
		json.Unmarshal(req.Params, &p)
	}

	workflows, err := m.defStore.ListWorkflows(ctx, tenantID, pgardenn.ListWorkflowsFilter{
		DomainID: parseOptionalUUID(p.DomainID),
		Status:   p.Status,
	})
	if err != nil {
		client.SendResponse(protocol.NewErrorResponse(req.ID, protocol.ErrInternal, err.Error()))
		return
	}
	client.SendResponse(protocol.NewOKResponse(req.ID, workflows))
}

type getWorkflowReq struct {
	ID string `json:"id"`
}

func (m *ArdennMethods) handleWorkflowsGet(ctx context.Context, client *gateway.Client, req *protocol.RequestFrame) {
	var p getWorkflowReq
	if err := json.Unmarshal(req.Params, &p); err != nil {
		client.SendResponse(protocol.NewErrorResponse(req.ID, protocol.ErrInvalidRequest, "invalid params"))
		return
	}

	wfID, err := uuid.Parse(p.ID)
	if err != nil {
		client.SendResponse(protocol.NewErrorResponse(req.ID, protocol.ErrInvalidRequest, "invalid workflow id"))
		return
	}

	workflow, err := m.defStore.GetWorkflowByID(ctx, wfID)
	if err != nil {
		client.SendResponse(protocol.NewErrorResponse(req.ID, protocol.ErrInternal, err.Error()))
		return
	}

	steps, err := m.defStore.GetSteps(ctx, wfID)
	if err != nil {
		client.SendResponse(protocol.NewErrorResponse(req.ID, protocol.ErrInternal, err.Error()))
		return
	}

	client.SendResponse(protocol.NewOKResponse(req.ID, map[string]any{
		"workflow": workflow,
		"steps":    steps,
	}))
}

type createWorkflowReq struct {
	DomainID      string          `json:"domainId"`
	Name          string          `json:"name"`
	Slug          string          `json:"slug"`
	Description   string          `json:"description"`
	Tier          string          `json:"tier"`
	TriggerConfig any             `json:"triggerConfig"`
	Variables     any             `json:"variables"`
	Settings      any             `json:"settings"`
	Visibility    string          `json:"visibility"`
	Steps         []createStepReq `json:"steps"`
}

type createStepReq struct {
	Slug           string   `json:"slug"`
	Name           string   `json:"name"`
	Description    string   `json:"description"`
	Position       int      `json:"position"`
	AgentKey       string   `json:"agentKey"`
	TaskTemplate   string   `json:"taskTemplate"`
	DependsOn      []string `json:"dependsOn"`
	Condition      string   `json:"condition"`
	Timeout        string   `json:"timeout"`
	DispatchTo     string   `json:"dispatchTo"`
	DispatchTarget string   `json:"dispatchTarget"`
	Gate           any      `json:"gate"`
	Constraints    any      `json:"constraints"`
	Continuity     any      `json:"continuity"`
	Evaluation     any      `json:"evaluation"`
}

func (m *ArdennMethods) handleWorkflowsCreate(ctx context.Context, client *gateway.Client, req *protocol.RequestFrame) {
	var p createWorkflowReq
	if err := json.Unmarshal(req.Params, &p); err != nil {
		client.SendResponse(protocol.NewErrorResponse(req.ID, protocol.ErrInvalidRequest, "invalid params"))
		return
	}
	if p.Name == "" || p.DomainID == "" || p.Tier == "" {
		client.SendResponse(protocol.NewErrorResponse(req.ID, protocol.ErrInvalidRequest, "name, domainId, and tier required"))
		return
	}

	tenantID := store.TenantIDFromContext(ctx)
	domainID, err := uuid.Parse(p.DomainID)
	if err != nil {
		client.SendResponse(protocol.NewErrorResponse(req.ID, protocol.ErrInvalidRequest, "invalid domainId"))
		return
	}

	// Auto-generate slug if not provided
	if p.Slug == "" {
		p.Slug = slugify(p.Name)
	}
	if p.Visibility == "" {
		p.Visibility = "domain"
	}

	triggerJSON, _ := json.Marshal(p.TriggerConfig)
	varsJSON, _ := json.Marshal(p.Variables)
	settingsJSON, _ := json.Marshal(p.Settings)

	userID := client.UserID()

	workflow, err := m.defStore.CreateWorkflow(ctx, pgardenn.CreateWorkflowParams{
		TenantID:      tenantID,
		DomainID:      domainID,
		Slug:          p.Slug,
		Name:          p.Name,
		Description:   p.Description,
		Tier:          p.Tier,
		TriggerConfig: triggerJSON,
		Variables:     varsJSON,
		Settings:      settingsJSON,
		Visibility:    p.Visibility,
		CreatedBy:     userID,
	})
	if err != nil {
		slog.Error("ardenn: create workflow failed", "error", err)
		client.SendResponse(protocol.NewErrorResponse(req.ID, protocol.ErrInternal, err.Error()))
		return
	}

	// Create steps
	for _, s := range p.Steps {
		gateJSON, _ := json.Marshal(s.Gate)
		constraintsJSON, _ := json.Marshal(s.Constraints)
		continuityJSON, _ := json.Marshal(s.Continuity)
		evaluationJSON, _ := json.Marshal(s.Evaluation)

		var depIDs []uuid.UUID
		for _, d := range s.DependsOn {
			if id, err := uuid.Parse(d); err == nil {
				depIDs = append(depIDs, id)
			}
		}

		timeout := s.Timeout
		if timeout == "" {
			timeout = "30 minutes"
		}

		err := m.defStore.CreateStep(ctx, pgardenn.CreateStepParams{
			WorkflowID:     workflow.ID,
			Slug:           s.Slug,
			Name:           s.Name,
			Description:    s.Description,
			Position:       s.Position,
			AgentKey:       s.AgentKey,
			TaskTemplate:   s.TaskTemplate,
			DependsOn:      depIDs,
			Condition:      s.Condition,
			Timeout:        timeout,
			DispatchTo:     s.DispatchTo,
			DispatchTarget: s.DispatchTarget,
			Gate:           gateJSON,
			Constraints:    constraintsJSON,
			Continuity:     continuityJSON,
			Evaluation:     evaluationJSON,
		})
		if err != nil {
			slog.Error("ardenn: create step failed", "error", err, "step", s.Slug)
		}
	}

	client.SendResponse(protocol.NewOKResponse(req.ID, workflow))
}

func (m *ArdennMethods) handleWorkflowsUpdate(ctx context.Context, client *gateway.Client, req *protocol.RequestFrame) {
	var p struct {
		ID            string           `json:"id"`
		Name          *string          `json:"name"`
		Description   *string          `json:"description"`
		Tier          *string          `json:"tier"`
		TriggerConfig *json.RawMessage `json:"triggerConfig"`
		Variables     *json.RawMessage `json:"variables"`
		Settings      *json.RawMessage `json:"settings"`
		Visibility    *string          `json:"visibility"`
		Steps         *[]createStepReq `json:"steps"`
	}
	if err := json.Unmarshal(req.Params, &p); err != nil {
		client.SendResponse(protocol.NewErrorResponse(req.ID, protocol.ErrInvalidRequest, "invalid params"))
		return
	}

	wfID, err := uuid.Parse(p.ID)
	if err != nil {
		client.SendResponse(protocol.NewErrorResponse(req.ID, protocol.ErrInvalidRequest, "invalid workflow id"))
		return
	}

	workflow, err := m.defStore.UpdateWorkflow(ctx, wfID, pgardenn.UpdateWorkflowParams{
		Name:          p.Name,
		Description:   p.Description,
		Tier:          p.Tier,
		TriggerConfig: p.TriggerConfig,
		Variables:     p.Variables,
		Settings:      p.Settings,
		Visibility:    p.Visibility,
	})
	if err != nil {
		client.SendResponse(protocol.NewErrorResponse(req.ID, protocol.ErrInternal, err.Error()))
		return
	}

	// If steps provided, replace all steps
	if p.Steps != nil {
		m.defStore.DeleteSteps(ctx, wfID)
		for _, s := range *p.Steps {
			gateJSON, _ := json.Marshal(s.Gate)
			constraintsJSON, _ := json.Marshal(s.Constraints)
			continuityJSON, _ := json.Marshal(s.Continuity)
			evaluationJSON, _ := json.Marshal(s.Evaluation)
			var depIDs []uuid.UUID
			for _, d := range s.DependsOn {
				if id, err := uuid.Parse(d); err == nil {
					depIDs = append(depIDs, id)
				}
			}
			timeout := s.Timeout
			if timeout == "" {
				timeout = "30 minutes"
			}
			m.defStore.CreateStep(ctx, pgardenn.CreateStepParams{
				WorkflowID: wfID, Slug: s.Slug, Name: s.Name,
				Description: s.Description, Position: s.Position,
				AgentKey: s.AgentKey, TaskTemplate: s.TaskTemplate,
				DependsOn: depIDs, Condition: s.Condition, Timeout: timeout,
				DispatchTo: s.DispatchTo, DispatchTarget: s.DispatchTarget,
				Gate: gateJSON, Constraints: constraintsJSON,
				Continuity: continuityJSON, Evaluation: evaluationJSON,
			})
		}
	}

	client.SendResponse(protocol.NewOKResponse(req.ID, workflow))
}

func (m *ArdennMethods) handleWorkflowsPublish(ctx context.Context, client *gateway.Client, req *protocol.RequestFrame) {
	var p struct {
		ID string `json:"id"`
	}
	if err := json.Unmarshal(req.Params, &p); err != nil {
		client.SendResponse(protocol.NewErrorResponse(req.ID, protocol.ErrInvalidRequest, "invalid params"))
		return
	}

	wfID, err := uuid.Parse(p.ID)
	if err != nil {
		client.SendResponse(protocol.NewErrorResponse(req.ID, protocol.ErrInvalidRequest, "invalid workflow id"))
		return
	}

	workflow, err := m.defStore.PublishWorkflow(ctx, wfID)
	if err != nil {
		client.SendResponse(protocol.NewErrorResponse(req.ID, protocol.ErrInternal, err.Error()))
		return
	}
	client.SendResponse(protocol.NewOKResponse(req.ID, workflow))
}

func (m *ArdennMethods) handleWorkflowsDelete(ctx context.Context, client *gateway.Client, req *protocol.RequestFrame) {
	locale := store.LocaleFromContext(ctx)
	if !permissions.HasMinRole(client.Role(), permissions.RoleAdmin) {
		client.SendResponse(protocol.NewErrorResponse(req.ID, protocol.ErrUnauthorized, i18n.T(locale, i18n.MsgPermissionDenied, "ardenn workflows")))
		return
	}

	var p struct {
		ID string `json:"id"`
	}
	if err := json.Unmarshal(req.Params, &p); err != nil {
		client.SendResponse(protocol.NewErrorResponse(req.ID, protocol.ErrInvalidRequest, "invalid params"))
		return
	}

	wfID, err := uuid.Parse(p.ID)
	if err != nil {
		client.SendResponse(protocol.NewErrorResponse(req.ID, protocol.ErrInvalidRequest, "invalid workflow id"))
		return
	}

	if err := m.defStore.DeleteWorkflow(ctx, wfID); err != nil {
		client.SendResponse(protocol.NewErrorResponse(req.ID, protocol.ErrInternal, err.Error()))
		return
	}
	client.SendResponse(protocol.NewOKResponse(req.ID, map[string]bool{"ok": true}))
}

// --- Runs ---

type listRunsReq struct {
	WorkflowID *string `json:"workflowId"`
	Status     *string `json:"status"`
	Limit      int     `json:"limit"`
	Offset     int     `json:"offset"`
}

func (m *ArdennMethods) handleRunsList(ctx context.Context, client *gateway.Client, req *protocol.RequestFrame) {
	tenantID := store.TenantIDFromContext(ctx)

	var p listRunsReq
	if req.Params != nil {
		json.Unmarshal(req.Params, &p)
	}
	if p.Limit == 0 {
		p.Limit = 50
	}

	runs, err := m.projStore.ListRuns(ctx, tenantID, pgardenn.ListRunsFilter{
		WorkflowID: parseOptionalUUID(p.WorkflowID),
		Status:     p.Status,
		Limit:      p.Limit,
		Offset:     p.Offset,
	})
	if err != nil {
		client.SendResponse(protocol.NewErrorResponse(req.ID, protocol.ErrInternal, err.Error()))
		return
	}
	client.SendResponse(protocol.NewOKResponse(req.ID, runs))
}

func (m *ArdennMethods) handleRunsGet(ctx context.Context, client *gateway.Client, req *protocol.RequestFrame) {
	var p struct {
		RunID string `json:"runId"`
	}
	if err := json.Unmarshal(req.Params, &p); err != nil {
		client.SendResponse(protocol.NewErrorResponse(req.ID, protocol.ErrInvalidRequest, "invalid params"))
		return
	}

	runID, err := uuid.Parse(p.RunID)
	if err != nil {
		client.SendResponse(protocol.NewErrorResponse(req.ID, protocol.ErrInvalidRequest, "invalid runId"))
		return
	}

	state, err := m.engine.GetRunState(ctx, runID)
	if err != nil {
		client.SendResponse(protocol.NewErrorResponse(req.ID, protocol.ErrInternal, err.Error()))
		return
	}

	events, err := m.engine.GetEvents(ctx, engine.EventQuery{RunID: runID, Limit: 200})
	if err != nil {
		slog.Warn("ardenn: failed to get events", "runId", runID, "error", err)
	}

	client.SendResponse(protocol.NewOKResponse(req.ID, map[string]any{
		"run":    state,
		"events": events,
	}))
}

func (m *ArdennMethods) handleRunsStart(ctx context.Context, client *gateway.Client, req *protocol.RequestFrame) {
	var p struct {
		WorkflowID string         `json:"workflowId"`
		ProjectID  *string        `json:"projectId"`
		Variables  map[string]any `json:"variables"`
	}
	if err := json.Unmarshal(req.Params, &p); err != nil {
		client.SendResponse(protocol.NewErrorResponse(req.ID, protocol.ErrInvalidRequest, "invalid params"))
		return
	}

	tenantID := store.TenantIDFromContext(ctx)
	wfID, err := uuid.Parse(p.WorkflowID)
	if err != nil {
		client.SendResponse(protocol.NewErrorResponse(req.ID, protocol.ErrInvalidRequest, "invalid workflowId"))
		return
	}

	// Load workflow definition + steps
	workflow, err := m.defStore.GetWorkflowByID(ctx, wfID)
	if err != nil {
		client.SendResponse(protocol.NewErrorResponse(req.ID, protocol.ErrInternal, fmt.Sprintf("workflow not found: %v", err)))
		return
	}
	if workflow.Status != "published" {
		client.SendResponse(protocol.NewErrorResponse(req.ID, protocol.ErrInvalidRequest, "workflow must be published to start a run"))
		return
	}

	steps, err := m.defStore.GetSteps(ctx, wfID)
	if err != nil {
		client.SendResponse(protocol.NewErrorResponse(req.ID, protocol.ErrInternal, err.Error()))
		return
	}
	stepDefs := pgardenn.ToStepDefs(steps)

	userID := client.UserID()
	var triggeredBy *uuid.UUID
	if userID != "" {
		if uid, err := uuid.Parse(userID); err == nil {
			triggeredBy = &uid
		}
	}

	var projectID *uuid.UUID
	if p.ProjectID != nil && *p.ProjectID != "" {
		pid, err := uuid.Parse(*p.ProjectID)
		if err != nil {
			client.SendResponse(protocol.NewErrorResponse(req.ID, protocol.ErrInvalidRequest, "invalid projectId"))
			return
		}
		projectID = &pid
	}

	runID, err := m.engine.StartRun(ctx, engine.StartRunRequest{
		TenantID:    tenantID,
		WorkflowID:  wfID,
		ProjectID:   projectID,
		TriggeredBy: triggeredBy,
		Tier:        workflow.Tier,
		Variables:   p.Variables,
		StepDefs:    stepDefs,
	})
	if err != nil {
		client.SendResponse(protocol.NewErrorResponse(req.ID, protocol.ErrInternal, err.Error()))
		return
	}

	client.SendResponse(protocol.NewOKResponse(req.ID, map[string]any{
		"runId": runID.String(),
	}))
}

func (m *ArdennMethods) handleRunsCancel(ctx context.Context, client *gateway.Client, req *protocol.RequestFrame) {
	var p struct {
		RunID  string `json:"runId"`
		Reason string `json:"reason"`
	}
	if err := json.Unmarshal(req.Params, &p); err != nil {
		client.SendResponse(protocol.NewErrorResponse(req.ID, protocol.ErrInvalidRequest, "invalid params"))
		return
	}

	runID, err := uuid.Parse(p.RunID)
	if err != nil {
		client.SendResponse(protocol.NewErrorResponse(req.ID, protocol.ErrInvalidRequest, "invalid runId"))
		return
	}

	// Verify run exists
	_, err = m.engine.GetEvents(ctx, engine.EventQuery{RunID: runID, Limit: 1})
	if err != nil {
		client.SendResponse(protocol.NewErrorResponse(req.ID, protocol.ErrInternal, "run not found"))
		return
	}

	// TODO: Engine needs a Cancel method — use event store directly for now
	client.SendResponse(protocol.NewOKResponse(req.ID, map[string]bool{"ok": true}))
}

func (m *ArdennMethods) handleRunsApprove(ctx context.Context, client *gateway.Client, req *protocol.RequestFrame) {
	m.handleGateDecision(ctx, client, req, true)
}

func (m *ArdennMethods) handleRunsReject(ctx context.Context, client *gateway.Client, req *protocol.RequestFrame) {
	m.handleGateDecision(ctx, client, req, false)
}

func (m *ArdennMethods) handleGateDecision(ctx context.Context, client *gateway.Client, req *protocol.RequestFrame, approved bool) {
	var p struct {
		RunID    string `json:"runId"`
		StepID   string `json:"stepId"`
		Feedback string `json:"feedback"`
	}
	if err := json.Unmarshal(req.Params, &p); err != nil {
		client.SendResponse(protocol.NewErrorResponse(req.ID, protocol.ErrInvalidRequest, "invalid params"))
		return
	}

	runID, err := uuid.Parse(p.RunID)
	if err != nil {
		client.SendResponse(protocol.NewErrorResponse(req.ID, protocol.ErrInvalidRequest, "invalid runId"))
		return
	}
	stepID, err := uuid.Parse(p.StepID)
	if err != nil {
		client.SendResponse(protocol.NewErrorResponse(req.ID, protocol.ErrInvalidRequest, "invalid stepId"))
		return
	}

	userID := client.UserID()
	var actorID *uuid.UUID
	if userID != "" {
		if uid, err := uuid.Parse(userID); err == nil {
			actorID = &uid
		}
	}

	if err := m.engine.GateDecide(ctx, runID, stepID, approved, actorID, p.Feedback); err != nil {
		client.SendResponse(protocol.NewErrorResponse(req.ID, protocol.ErrInternal, err.Error()))
		return
	}
	client.SendResponse(protocol.NewOKResponse(req.ID, map[string]bool{"ok": true}))
}

// --- Events + Tasks ---

func (m *ArdennMethods) handleEventsStream(ctx context.Context, client *gateway.Client, req *protocol.RequestFrame) {
	var p struct {
		RunID        string `json:"runId"`
		FromSequence int64  `json:"fromSequence"`
	}
	if err := json.Unmarshal(req.Params, &p); err != nil {
		client.SendResponse(protocol.NewErrorResponse(req.ID, protocol.ErrInvalidRequest, "invalid params"))
		return
	}

	runID, err := uuid.Parse(p.RunID)
	if err != nil {
		client.SendResponse(protocol.NewErrorResponse(req.ID, protocol.ErrInvalidRequest, "invalid runId"))
		return
	}

	events, err := m.engine.GetEvents(ctx, engine.EventQuery{
		RunID:        runID,
		FromSequence: p.FromSequence,
		Limit:        500,
	})
	if err != nil {
		client.SendResponse(protocol.NewErrorResponse(req.ID, protocol.ErrInternal, err.Error()))
		return
	}
	client.SendResponse(protocol.NewOKResponse(req.ID, events))
}

func (m *ArdennMethods) handleMyTasks(ctx context.Context, client *gateway.Client, req *protocol.RequestFrame) {
	tenantID := store.TenantIDFromContext(ctx)
	userID := client.UserID()
	if userID == "" {
		client.SendResponse(protocol.NewErrorResponse(req.ID, protocol.ErrInvalidRequest, "user id required"))
		return
	}

	tasks, err := m.projStore.GetMyTasks(ctx, tenantID, userID)
	if err != nil {
		client.SendResponse(protocol.NewErrorResponse(req.ID, protocol.ErrInternal, err.Error()))
		return
	}
	client.SendResponse(protocol.NewOKResponse(req.ID, tasks))
}

// --- Helpers ---

func parseOptionalUUID(s *string) *uuid.UUID {
	if s == nil || *s == "" {
		return nil
	}
	id, err := uuid.Parse(*s)
	if err != nil {
		return nil
	}
	return &id
}

func slugify(name string) string {
	slug := ""
	for _, r := range name {
		if r >= 'a' && r <= 'z' || r >= '0' && r <= '9' || r == '-' {
			slug += string(r)
		} else if r >= 'A' && r <= 'Z' {
			slug += string(r + 32)
		} else if r == ' ' {
			slug += "-"
		}
	}
	return slug
}
