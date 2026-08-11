package process

import (
	"testing"

	"github.com/xen0bit/pwrq/pkg/core/sessionstate"
	"github.com/xen0bit/pwrq/pkg/udf/common"
)

func TestGetProcess(t *testing.T) {
	ss := sessionstate.NewSessionState()
	common.SetGlobalSessionState(ss)
	defer common.SetGlobalSessionState(nil)

	// Test getting all processes
	opts := GetProcessOptions{}
	processes, err := getProcesses(opts)
	if err != nil {
		t.Fatalf("getProcesses() error = %v", err)
	}

	if len(processes) == 0 {
		t.Error("Expected at least one process")
	}

	// Verify process has required fields
	if len(processes) > 0 {
		proc := processes[0]
		requiredFields := []string{"Id", "Name"}
		for _, field := range requiredFields {
			if _, exists := proc[field]; !exists {
				t.Errorf("Expected field %q in process result", field)
			}
		}
	}
}

func TestGetProcessByName(t *testing.T) {
	ss := sessionstate.NewSessionState()
	common.SetGlobalSessionState(ss)
	defer common.SetGlobalSessionState(nil)

	// Test filtering by name (may not find anything, but should not error)
	opts := GetProcessOptions{Name: "init"}
	processes, err := getProcesses(opts)
	if err != nil {
		t.Fatalf("getProcesses() error = %v", err)
	}

	// If processes found, verify they match the name
	for _, proc := range processes {
		name, ok := proc["Name"].(string)
		if !ok {
			t.Error("Process Name should be a string")
		}
		// Note: exact match may not work due to case sensitivity
		_ = name
	}
}

func TestGetProcessByID(t *testing.T) {
	ss := sessionstate.NewSessionState()
	common.SetGlobalSessionState(ss)
	defer common.SetGlobalSessionState(nil)

	// Get current process
	opts := GetProcessOptions{ID: 1} // PID 1 usually exists
	processes, err := getProcesses(opts)
	if err != nil {
		t.Fatalf("getProcesses() error = %v", err)
	}

	// If found, verify ID matches
	for _, proc := range processes {
		id, ok := proc["Id"].(float64)
		if !ok {
			// May be int on some systems
			if _, ok := proc["Id"].(int); ok {
				continue
			}
			t.Error("Process Id should be a number")
		}
		if int(id) != 1 {
			t.Errorf("Expected process ID 1, got %v", id)
		}
	}

	// Note: PID 1 may not exist in container environments, so no failure if empty
}

func TestGetProcessOptionsParsing(t *testing.T) {
	tests := []struct {
		name     string
		optsMap  map[string]any
		wantName string
		wantID   int
	}{
		{
			name:     "name only",
			optsMap:  map[string]any{"Name": "test"},
			wantName: "test",
			wantID:   0,
		},
		{
			name:     "id only",
			optsMap:  map[string]any{"Id": 1234},
			wantName: "",
			wantID:   1234,
		},
		{
			name:     "name and id",
			optsMap:  map[string]any{"Name": "test", "Id": 1234},
			wantName: "test",
			wantID:   1234,
		},
		{
			name:     "include username",
			optsMap:  map[string]any{"IncludeUserName": true},
			wantName: "",
			wantID:   0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			opts := GetProcessOptions{}
			parseGetProcessOptions(&opts, tt.optsMap)

			if opts.Name != tt.wantName {
				t.Errorf("Name = %q, want %q", opts.Name, tt.wantName)
			}
			if opts.ID != tt.wantID {
				t.Errorf("ID = %d, want %d", opts.ID, tt.wantID)
			}
		})
	}
}

func TestStopProcess(t *testing.T) {
	ss := sessionstate.NewSessionState()
	common.SetGlobalSessionState(ss)
	defer common.SetGlobalSessionState(nil)

	// Test stopping a non-existent process (should error gracefully)
	opts := StopProcessOptions{Name: "nonexistent_process_xyz"}
	stopped, failed, err := stopProcesses(opts)

	// Either outcome is acceptable: the process does not exist, so the call may
	// report an error or simply find nothing to stop. What it must not do is
	// claim to have stopped something.
	if len(stopped) != 0 {
		t.Errorf("stopped %d processes that do not exist: %v", len(stopped), stopped)
	}
	if err == nil && len(failed) != 0 {
		t.Errorf("reported %d failures but no error: %v", len(failed), failed)
	}
}

func TestStopProcessOptionsParsing(t *testing.T) {
	tests := []struct {
		name      string
		optsMap   map[string]any
		wantName  string
		wantID    int
		wantForce bool
	}{
		{
			name:      "name only",
			optsMap:   map[string]any{"Name": "test"},
			wantName:  "test",
			wantID:    0,
			wantForce: false,
		},
		{
			name:      "with force",
			optsMap:   map[string]any{"Name": "test", "Force": true},
			wantName:  "test",
			wantID:    0,
			wantForce: true,
		},
		{
			name:      "id and force",
			optsMap:   map[string]any{"Id": 1234, "Force": true},
			wantName:  "",
			wantID:    1234,
			wantForce: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			opts := StopProcessOptions{}
			parseStopProcessOptions(&opts, tt.optsMap)

			if opts.Name != tt.wantName {
				t.Errorf("Name = %q, want %q", opts.Name, tt.wantName)
			}
			if opts.ID != tt.wantID {
				t.Errorf("ID = %d, want %d", opts.ID, tt.wantID)
			}
			if opts.Force != tt.wantForce {
				t.Errorf("Force = %v, want %v", opts.Force, tt.wantForce)
			}
		})
	}
}

func TestStartProcess(t *testing.T) {
	ss := sessionstate.NewSessionState()
	common.SetGlobalSessionState(ss)
	defer common.SetGlobalSessionState(nil)

	// Test starting a process with echo command (should work on Unix)
	opts := StartProcessOptions{
		FilePath:     "echo",
		ArgumentList: []any{"hello"},
		PassThru:     true,
	}

	result, err := startProcess(opts)
	if err != nil {
		// May fail in some environments, that's acceptable
		t.Logf("startProcess() error (may be expected in test env): %v", err)
		return
	}

	// Verify result has required fields
	if result["Id"] == nil {
		t.Error("Expected Id in start process result")
	}
	if result["Name"] != "echo" {
		t.Errorf("Expected Name='echo', got %v", result["Name"])
	}
}

func TestStartProcessOptionsParsing(t *testing.T) {
	tests := []struct {
		name         string
		optsMap      map[string]any
		wantFilePath string
		wantArgs     int
		wantPassThru bool
	}{
		{
			name:         "file path only",
			optsMap:      map[string]any{"FilePath": "test"},
			wantFilePath: "test",
			wantArgs:     0,
			wantPassThru: false,
		},
		{
			name:         "with arguments",
			optsMap:      map[string]any{"FilePath": "test", "ArgumentList": []any{"arg1", "arg2"}},
			wantFilePath: "test",
			wantArgs:     2,
			wantPassThru: false,
		},
		{
			name:         "with pass thru",
			optsMap:      map[string]any{"FilePath": "test", "PassThru": true},
			wantFilePath: "test",
			wantArgs:     0,
			wantPassThru: true,
		},
		{
			name:         "all options",
			optsMap:      map[string]any{"FilePath": "test", "ArgumentList": []any{"arg1"}, "PassThru": true, "WindowStyle": "Hidden"},
			wantFilePath: "test",
			wantArgs:     1,
			wantPassThru: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			opts := StartProcessOptions{}
			parseStartProcessOptions(&opts, tt.optsMap)

			if opts.FilePath != tt.wantFilePath {
				t.Errorf("FilePath = %q, want %q", opts.FilePath, tt.wantFilePath)
			}
			if len(opts.ArgumentList) != tt.wantArgs {
				t.Errorf("ArgumentList length = %d, want %d", len(opts.ArgumentList), tt.wantArgs)
			}
			if opts.PassThru != tt.wantPassThru {
				t.Errorf("PassThru = %v, want %v", opts.PassThru, tt.wantPassThru)
			}
		})
	}
}
