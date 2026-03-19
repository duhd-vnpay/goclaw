// internal/http/audit_middleware.go
package http

import (
	"log/slog"
	"net/http"
	"time"

	"github.com/nextlevelbuilder/goclaw/internal/access"
)

// responseCapture wraps http.ResponseWriter to capture status code.
type responseCapture struct {
	http.ResponseWriter
	statusCode int
}

func (r *responseCapture) WriteHeader(code int) {
	r.statusCode = code
	r.ResponseWriter.WriteHeader(code)
}

// FileAuditMiddleware wraps file-serving handlers with audit logging and security headers.
// Logs all requests (allow + deny) to the AccessChecker.
func FileAuditMiddleware(checker access.AccessChecker, resourceType string) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			start := time.Now()

			// Security headers
			w.Header().Set("Referrer-Policy", "no-referrer")
			w.Header().Set("X-Content-Type-Options", "nosniff")

			// Capture response status
			capture := &responseCapture{ResponseWriter: w, statusCode: 200}
			next.ServeHTTP(capture, r)

			// Audit log after response
			if checker != nil {
				action := access.ActionRead
				switch {
				case capture.statusCode == 403:
					action = access.ActionDeny
				case r.Method == "DELETE":
					action = access.ActionDelete
				case r.Method == "POST" || r.Method == "PUT":
					action = access.ActionWrite
				}

				if err := checker.RecordAccess(r.Context(), access.AccessRequest{
					SubjectID:    extractUserID(r),
					Resource:     r.URL.Path,
					ResourceType: resourceType,
					Action:       action,
					Source:       "http",
					IPAddress:    r.RemoteAddr,
				}, capture.statusCode < 400); err != nil {
					slog.Error("audit middleware: record failed", "error", err)
				}
			}

			slog.Debug("file.access.http",
				"method", r.Method,
				"path", r.URL.Path,
				"status", capture.statusCode,
				"duration", time.Since(start),
			)
		})
	}
}
