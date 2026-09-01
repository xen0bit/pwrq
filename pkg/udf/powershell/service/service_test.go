package service

import (
	"runtime"
	"testing"

	"github.com/xen0bit/pwrq/pkg/core/sessionstate"
	"github.com/xen0bit/pwrq/pkg/udf/common"
)

// reasonDisabled is why every test here that reaches the service manager is
// skipped outright rather than only under -short.
//
// On a desktop these do not fail quietly the way they do in a container. Two
// of them ask systemd to change state, and the three that only read are worse:
// getServices calls `systemctl is-enabled` twice for every unit on the machine,
// which on a box with a hundred and sixty units is some three hundred calls
// into an action polkit guards, and it answers each one with an authentication
// dialog that takes the keyboard focus. `go test ./...` becomes unusable.
//
// What is left covers the parsing and the option handling, which is where the
// bugs in this package have actually been - the newline split that reported one
// service out of a hundred and sixty was found by reading output, not by
// talking to systemd. Point these at a unit you own, by hand, if the calling
// convention changes.
const reasonDisabled = "talks to the system service manager, which prompts for authentication once per unit"

func TestGetService(t *testing.T) {
	t.Skip(reasonDisabled)

	if testing.Short() {
		t.Skip("shells out to the system service manager")
	}
	ss := sessionstate.NewSessionState()
	common.SetGlobalSessionState(ss)
	defer common.SetGlobalSessionState(nil)

	// Test getting all services
	opts := GetServiceOptions{}
	services, err := getServices(opts)
	if err != nil {
		// May fail if systemctl is not available (e.g., in container)
		t.Logf("getServices() error (may be expected in test env): %v", err)
		return
	}

	// Services may be empty in test environment, that's acceptable
	_ = services
}

func TestGetServiceByName(t *testing.T) {
	t.Skip(reasonDisabled)

	if testing.Short() {
		t.Skip("shells out to the system service manager")
	}
	ss := sessionstate.NewSessionState()
	common.SetGlobalSessionState(ss)
	defer common.SetGlobalSessionState(nil)

	// Test filtering by name
	opts := GetServiceOptions{Name: "ssh"}
	services, err := getServices(opts)
	if err != nil {
		t.Logf("getServices() error (may be expected in test env): %v", err)
		return
	}

	// If services found, verify they match the name pattern
	for _, svc := range services {
		name, ok := svc["Name"].(string)
		if !ok {
			t.Error("Service Name should be a string")
		}
		_ = name
	}
}

func TestGetServiceOptionsParsing(t *testing.T) {
	tests := []struct {
		name        string
		optsMap     map[string]any
		wantName    string
		wantDisplay string
		wantExclude string
	}{
		{
			name:        "name only",
			optsMap:     map[string]any{"Name": "test"},
			wantName:    "test",
			wantDisplay: "",
			wantExclude: "",
		},
		{
			name:        "display name",
			optsMap:     map[string]any{"DisplayName": "Test Service"},
			wantName:    "",
			wantDisplay: "Test Service",
			wantExclude: "",
		},
		{
			name:        "exclude pattern",
			optsMap:     map[string]any{"Exclude": "disabled*"},
			wantName:    "",
			wantDisplay: "",
			wantExclude: "disabled*",
		},
		{
			name:        "all options",
			optsMap:     map[string]any{"Name": "*service*", "DisplayName": "Test", "Exclude": "disabled"},
			wantName:    "*service*",
			wantDisplay: "Test",
			wantExclude: "disabled",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			opts := GetServiceOptions{}
			parseGetServiceOptions(&opts, tt.optsMap)

			if opts.Name != tt.wantName {
				t.Errorf("Name = %q, want %q", opts.Name, tt.wantName)
			}
			if opts.DisplayName != tt.wantDisplay {
				t.Errorf("DisplayName = %q, want %q", opts.DisplayName, tt.wantDisplay)
			}
			if opts.Exclude != tt.wantExclude {
				t.Errorf("Exclude = %q, want %q", opts.Exclude, tt.wantExclude)
			}
		})
	}
}

func TestMatchPattern(t *testing.T) {
	tests := []struct {
		name    string
		s       string
		pattern string
		want    bool
	}{
		{"wildcard all", "anything", "*", true},
		{"starts with", "test123", "test*", true},
		{"starts with no match", "other123", "test*", false},
		{"ends with", "mytest", "*test", true},
		{"ends with no match", "myother", "*test", false},
		{"contains", "abc123def", "*123*", true},
		{"contains no match", "abcdef", "*123*", false},
		{"exact match", "test", "test", true},
		{"exact no match", "test", "other", false},
		{"case insensitive", "TEST", "test", true},
		{"case insensitive wildcard", "MyService", "myservice*", true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := matchPattern(tt.s, tt.pattern)
			if got != tt.want {
				t.Errorf("matchPattern(%q, %q) = %v, want %v", tt.s, tt.pattern, got, tt.want)
			}
		})
	}
}

func TestStartService(t *testing.T) {
	t.Skip(reasonDisabled)

	if testing.Short() {
		t.Skip("shells out to the system service manager")
	}
	ss := sessionstate.NewSessionState()
	common.SetGlobalSessionState(ss)
	defer common.SetGlobalSessionState(nil)

	// Test starting a service (will likely fail in test environment)
	opts := StartServiceOptions{Name: "nonexistent_test_service"}
	result, err := startService(opts)

	if err != nil {
		// Expected to fail for non-existent service
		t.Logf("startService() error (expected for non-existent service): %v", err)
		return
	}

	// If it succeeds, verify result has Status field
	if result["Status"] == nil {
		t.Error("Expected Status in start service result")
	}
}

func TestStartServiceOptionsParsing(t *testing.T) {
	tests := []struct {
		name         string
		optsMap      map[string]any
		wantName     string
		wantPassThru bool
	}{
		{
			name:         "name only",
			optsMap:      map[string]any{"Name": "test"},
			wantName:     "test",
			wantPassThru: false,
		},
		{
			name:         "with pass thru",
			optsMap:      map[string]any{"Name": "test", "PassThru": true},
			wantName:     "test",
			wantPassThru: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			opts := StartServiceOptions{}
			parseStartServiceOptions(&opts, tt.optsMap)

			if opts.Name != tt.wantName {
				t.Errorf("Name = %q, want %q", opts.Name, tt.wantName)
			}
			if opts.PassThru != tt.wantPassThru {
				t.Errorf("PassThru = %v, want %v", opts.PassThru, tt.wantPassThru)
			}
		})
	}
}

func TestStopService(t *testing.T) {
	t.Skip(reasonDisabled)

	if testing.Short() {
		t.Skip("shells out to the system service manager")
	}
	ss := sessionstate.NewSessionState()
	common.SetGlobalSessionState(ss)
	defer common.SetGlobalSessionState(nil)

	// Test stopping a service (will likely fail in test environment)
	opts := StopServiceOptions{Name: "nonexistent_test_service"}
	result, err := stopService(opts)

	if err != nil {
		// Expected to fail for non-existent service
		t.Logf("stopService() error (expected for non-existent service): %v", err)
		return
	}

	// If it succeeds, verify result has Status field
	if result["Status"] == nil {
		t.Error("Expected Status in stop service result")
	}
}

func TestStopServiceOptionsParsing(t *testing.T) {
	tests := []struct {
		name         string
		optsMap      map[string]any
		wantName     string
		wantForce    bool
		wantPassThru bool
	}{
		{
			name:         "name only",
			optsMap:      map[string]any{"Name": "test"},
			wantName:     "test",
			wantForce:    false,
			wantPassThru: false,
		},
		{
			name:         "with force",
			optsMap:      map[string]any{"Name": "test", "Force": true},
			wantName:     "test",
			wantForce:    true,
			wantPassThru: false,
		},
		{
			name:         "all options",
			optsMap:      map[string]any{"Name": "test", "Force": true, "PassThru": true},
			wantName:     "test",
			wantForce:    true,
			wantPassThru: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			opts := StopServiceOptions{}
			parseStopServiceOptions(&opts, tt.optsMap)

			if opts.Name != tt.wantName {
				t.Errorf("Name = %q, want %q", opts.Name, tt.wantName)
			}
			if opts.Force != tt.wantForce {
				t.Errorf("Force = %v, want %v", opts.Force, tt.wantForce)
			}
			if opts.PassThru != tt.wantPassThru {
				t.Errorf("PassThru = %v, want %v", opts.PassThru, tt.wantPassThru)
			}
		})
	}
}

func TestServiceInfoStructure(t *testing.T) {
	// Test that ServiceInfo has all required fields
	svc := ServiceInfo{
		Name:        "test",
		DisplayName: "Test Service",
		Status:      "Running",
		StartType:   "Automatic",
		CanStop:     true,
		CanPause:    false,
		CanShutdown: false,
		MachineName: "localhost",
		ServiceType: "Win32OwnProcess",
	}

	// Verify all fields are accessible
	if svc.Name != "test" {
		t.Error("Failed to set Name")
	}
	if svc.DisplayName != "Test Service" {
		t.Error("Failed to set DisplayName")
	}
	if svc.Status != "Running" {
		t.Error("Failed to set Status")
	}
	if svc.StartType != "Automatic" {
		t.Error("Failed to set StartType")
	}
	if svc.CanStop != true {
		t.Error("Failed to set CanStop")
	}
	if svc.MachineName != "localhost" {
		t.Error("Failed to set MachineName")
	}
}

// TestGetServicesUnixParsesAllUnits guards a bug that made get_service report
// exactly one service on a machine with a hundred and sixty: systemctl's JSON
// output is a single line, and the parser split it on newlines.
func TestGetServicesUnixParsesAllUnits(t *testing.T) {
	t.Skip(reasonDisabled)

	if testing.Short() {
		t.Skip("shells out to the system service manager")
	}
	if runtime.GOOS != "linux" {
		t.Skip("systemd is linux-only")
	}

	services, err := getServices(GetServiceOptions{})
	if err != nil {
		t.Skipf("no service manager available: %v", err)
	}
	if len(services) <= 1 {
		t.Errorf("got %d services; a single result means the listing was not parsed", len(services))
	}

	seen := make(map[string]bool)
	for _, svc := range services {
		name, _ := svc["Name"].(string)
		if name == "" {
			t.Error("a service came back with no name")
		}
		seen[name] = true
	}
	if len(seen) != len(services) {
		t.Errorf("%d services but only %d distinct names", len(services), len(seen))
	}
}
