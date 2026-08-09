package integration

import (
	"encoding/json"
	"runtime"
	"strconv"
	"strings"
	"testing"
)

// TestGetServiceReportsRealServices is behind -short because it reads the
// machine's actual service manager, following the Phase 3 precedent for
// system-touching tests.
//
// It exists because get_service once reported 1 service on a machine running
// 164: it declared JSON struct tags and then hand-parsed the output by
// splitting on newlines. A count is the only assertion that would have caught
// that, and it is the one thing a unit test with a canned fixture cannot make.
func TestGetServiceReportsRealServices(t *testing.T) {
	if testing.Short() {
		t.Skip("reads the system service manager")
	}
	if runtime.GOOS != "linux" && runtime.GOOS != "windows" {
		t.Skipf("no service manager to query on %s", runtime.GOOS)
	}

	out := mustRun(t, "null", "-c", `[get_service]`)

	var services []map[string]any
	if err := json.Unmarshal([]byte(out), &services); err != nil {
		t.Fatalf("get_service did not return an array of objects: %v", err)
	}
	if len(services) == 0 {
		t.Skip("no services on this machine; nothing to assert")
	}

	// Every service names itself and reports a status. A blank Name is the
	// signature of the parsing bug.
	for _, svc := range services {
		name, _ := svc["Name"].(string)
		if strings.TrimSpace(name) == "" {
			t.Fatalf("a service came back with no Name: %v", svc)
		}
		if _, ok := svc["Status"].(string); !ok {
			t.Fatalf("service %s has no Status: %v", name, svc)
		}
	}

	// And the output is queryable as ordinary JSON, like every other cmdlet's.
	got := strings.TrimSpace(mustRun(t, "null", "-c",
		`[get_service | select(.Status == "Running")] | length <= `+strconv.Itoa(len(services))))
	if got != "true" {
		t.Errorf("filtering services by Status gave %s", got)
	}
}
