package hands

import (
	"context"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"strings"
	"time"

	"github.com/google/uuid"

	engine "github.com/nextlevelbuilder/goclaw/internal/ardenn"
)

const (
	apiMaxRetries     = 3
	apiDefaultTimeout = 30 * time.Second
	apiMaxBodyBytes   = 1 << 20 // 1 MB response limit
)

// APIHand makes HTTP POST/GET calls to external APIs with retry on 5xx.
type APIHand struct {
	client *http.Client
}

// NewAPIHand creates an APIHand with the given HTTP client (nil uses default).
func NewAPIHand(client *http.Client) *APIHand {
	if client == nil {
		client = &http.Client{}
	}
	return &APIHand{client: client}
}

func (h *APIHand) Type() engine.HandType { return engine.HandAPI }

func (h *APIHand) Execute(ctx context.Context, req engine.HandRequest) engine.HandResult {
	start := time.Now()

	timeout := req.Timeout
	if timeout == 0 {
		timeout = apiDefaultTimeout
	}
	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	method := "POST"
	if m, ok := req.Metadata["http_method"].(string); ok && m != "" {
		method = strings.ToUpper(m)
	}

	var lastErr error
	backoff := 1 * time.Second

	for attempt := 0; attempt < apiMaxRetries; attempt++ {
		if attempt > 0 {
			select {
			case <-time.After(backoff):
				backoff *= 2
			case <-ctx.Done():
				return engine.HandResult{Error: ctx.Err(), Duration: time.Since(start)}
			}
		}

		httpReq, err := http.NewRequestWithContext(ctx, method, req.Name, strings.NewReader(req.Input))
		if err != nil {
			return engine.HandResult{Error: fmt.Errorf("build request: %w", err), Duration: time.Since(start)}
		}
		httpReq.Header.Set("Content-Type", "application/json")
		applyHeaders(httpReq, req.Metadata)

		resp, err := h.client.Do(httpReq)
		if err != nil {
			lastErr = err
			continue
		}

		body, _ := io.ReadAll(io.LimitReader(resp.Body, apiMaxBodyBytes))
		resp.Body.Close()

		if resp.StatusCode >= 500 {
			lastErr = fmt.Errorf("HTTP %d: %s", resp.StatusCode, string(body))
			slog.Warn("ardenn.api_hand: retryable error",
				"attempt", attempt+1, "status", resp.StatusCode, "url", req.Name)
			continue
		}

		if resp.StatusCode >= 400 {
			return engine.HandResult{
				Error:    fmt.Errorf("HTTP %d: %s", resp.StatusCode, string(body)),
				Duration: time.Since(start),
			}
		}

		return engine.HandResult{Output: string(body), Duration: time.Since(start)}
	}

	return engine.HandResult{
		Error:    fmt.Errorf("max retries (%d) exhausted: %w", apiMaxRetries, lastErr),
		Duration: time.Since(start),
	}
}

func (h *APIHand) Cancel(_ context.Context, runID uuid.UUID) error {
	slog.Info("ardenn.api_hand: cancel requested (no-op)", "run_id", runID)
	return nil
}

func applyHeaders(req *http.Request, metadata map[string]any) {
	headers, ok := metadata["headers"].(map[string]any)
	if !ok {
		return
	}
	for k, v := range headers {
		if s, ok := v.(string); ok {
			req.Header.Set(k, s)
		}
	}
}
