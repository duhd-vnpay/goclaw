package constraints

import (
	"fmt"
	"sync"
)

type DependencyLayer struct {
	Name     string   `json:"name"`
	Order    float64  `json:"order"`
	AllowsUp []string `json:"allows_up"`
}

type DependencyConfig struct {
	Layers       []DependencyLayer    `json:"layers"`
	AgentMapping map[string]string    `json:"agent_layer"`
	Enforcement  string               `json:"enforcement"` // "block","warn","off"
}

type DependencyEngine struct {
	layers   map[string]DependencyLayer
	agents   map[string]string
	enforce  string
	warnings []string
	mu       sync.Mutex
}

func NewDependencyEngine(cfg DependencyConfig) *DependencyEngine {
	layers := make(map[string]DependencyLayer, len(cfg.Layers))
	for _, l := range cfg.Layers {
		layers[l.Name] = l
	}
	return &DependencyEngine{
		layers:  layers,
		agents:  cfg.AgentMapping,
		enforce: cfg.Enforcement,
	}
}

func (e *DependencyEngine) ValidateCall(callerAgent, targetAgent string) error {
	if e.enforce == "off" {
		return nil
	}
	callerLayer, callerOK := e.agents[callerAgent]
	targetLayer, targetOK := e.agents[targetAgent]
	if !callerOK || !targetOK {
		return nil
	}
	if callerLayer == targetLayer {
		return nil
	}
	layer, ok := e.layers[callerLayer]
	if !ok {
		return nil
	}
	for _, allowed := range layer.AllowsUp {
		if allowed == targetLayer {
			return nil
		}
	}
	msg := fmt.Sprintf("dependency layer violation: %s (%s) cannot call %s (%s) — allowed targets: %v",
		callerAgent, callerLayer, targetAgent, targetLayer, layer.AllowsUp)
	if e.enforce == "warn" {
		e.mu.Lock()
		e.warnings = append(e.warnings, msg)
		e.mu.Unlock()
		return nil
	}
	return fmt.Errorf("%s", msg)
}

func (e *DependencyEngine) Warnings() []string {
	e.mu.Lock()
	defer e.mu.Unlock()
	cp := make([]string, len(e.warnings))
	copy(cp, e.warnings)
	return cp
}
