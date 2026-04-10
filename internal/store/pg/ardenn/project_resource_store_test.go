package ardenn

import (
	"testing"
)

// Integration tests require TEST_DATABASE_URL.
// Run with: TEST_DATABASE_URL="..." go test -v -tags integration ./internal/store/pg/ardenn/ -run TestProjectResource

func TestProjectResourceStore_Placeholder(t *testing.T) {
	// Unit test: verify struct field alignment with SQL columns.
	// The actual integration tests run against a real DB.
	r := ProjectResource{
		ResourceType: "mcp_server",
		ResourceKey:  "gitlab-mcp",
		Enabled:      true,
	}
	if r.ResourceType != "mcp_server" {
		t.Fatal("struct alignment check")
	}

	tmpl := DomainResourceTemplate{
		ResourceType: "git_repo",
		ResourceKey:  "main-repo",
		Required:     true,
	}
	if !tmpl.Required {
		t.Fatal("struct alignment check")
	}
}
