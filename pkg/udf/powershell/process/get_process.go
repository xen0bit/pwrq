// Package process provides PowerShell-style process management cmdlets.
// This file implements Get-Process functionality.
package process

import (
	"encoding/csv"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"time"

	"github.com/itchyny/gojq"
	"github.com/xen0bit/pwrq/pkg/core/sessionstate"
	"github.com/xen0bit/pwrq/pkg/udf/common"
)

// ProcessInfo holds information about a process
type ProcessInfo struct {
	ID                 int           `json:"Id"`
	Name               string        `json:"Name"`
	Path               string        `json:"Path"`
	CPU                float64       `json:"CPU"`
	WorkingSet         int64         `json:"WorkingSet"`
	VirtualMemory      int64         `json:"VirtualMemory"`
	StartTime          time.Time     `json:"StartTime"`
	UserName           string        `json:"UserName"`
	PriorityClass      string        `json:"PriorityClass"`
	ThreadCount        int           `json:"Threads"`
	Handles            int32         `json:"Handles"`
	ResponseTime       time.Duration `json:"ResponseTime"`
	TotalProcessorTime time.Duration `json:"TotalProcessorTime"`
	UserProcessorTime  time.Duration `json:"UserProcessorTime"`
}

// GetProcessOptions holds options for the get_process function
type GetProcessOptions struct {
	Name            string // Process name filter
	ID              int    // Process ID filter
	IncludeUserName bool   // Include username in output (requires privileges)
}

// RegisterGetProcess registers the get_process function with gojq
// Supports PowerShell-style parameters: -Name, -Id, -IncludeUserName
// Usage:
//   - get_process() - get all processes
//   - get_process("notepad") - get processes by name
//   - get_process({"Name": "chrome"; "Id": 1234}) - get by options
func RegisterGetProcess() gojq.CompilerOption {
	return gojq.WithIterFunction("get_process", 0, 2, func(v any, args []any) gojq.Iter {
		opts := GetProcessOptions{}

		// Parse arguments
		if len(args) > 0 {
			firstArg := common.BindValue(args[0])

			// Check if first arg is a string (process name)
			if nameStr, ok := firstArg.(string); ok {
				opts.Name = nameStr
			} else if idInt, ok := firstArg.(int); ok {
				opts.ID = idInt
			} else if optsMap, ok := firstArg.(map[string]any); ok {
				// First arg is options map
				parseGetProcessOptions(&opts, optsMap)
			}
		}

		// Get processes
		processes, err := getProcesses(opts)
		if err != nil {
			return gojq.NewIter(err)
		}

		// Update $? automatic variable (last command success)
		ss := common.GetSessionState()
		if ss != nil {
			_ = ss.SetVariable("?", true, sessionstate.None)
		}

		// Convert to []any for iterator
		results := make([]any, len(processes))
		for i, p := range processes {
			results[i] = p
		}

		return &anySliceIter{values: results, index: 0}
	})
}

// anySliceIter is an iterator over a slice of any
type anySliceIter struct {
	values []any
	index  int
}

func (iter *anySliceIter) Next() (any, bool) {
	if iter.index >= len(iter.values) {
		return nil, false
	}
	value := iter.values[iter.index]
	iter.index++
	return value, true
}

// parseGetProcessOptions parses options from a map
func parseGetProcessOptions(opts *GetProcessOptions, optsMap map[string]any) {
	if nameVal, exists := optsMap["Name"]; exists {
		if nStr, ok := nameVal.(string); ok {
			opts.Name = nStr
		}
	}
	if idVal, exists := optsMap["Id"]; exists {
		switch v := idVal.(type) {
		case int:
			opts.ID = v
		case float64:
			opts.ID = int(v)
		}
	}
	if userNameVal, exists := optsMap["IncludeUserName"]; exists {
		if b, ok := userNameVal.(bool); ok {
			opts.IncludeUserName = b
		}
	}
}

// getProcesses retrieves process information based on options
func getProcesses(opts GetProcessOptions) ([]map[string]any, error) {
	var processes []map[string]any

	// Use OS-specific method to get process info
	var procInfos []ProcessInfo
	var err error

	if runtime.GOOS == "windows" {
		procInfos, err = getProcessesWindows(opts)
	} else {
		procInfos, err = getProcessesUnix(opts)
	}

	if err != nil {
		return nil, err
	}

	// Convert to map format for JSON output
	for _, proc := range procInfos {
		// Apply filters
		if opts.Name != "" && !strings.EqualFold(proc.Name, opts.Name) {
			continue
		}
		if opts.ID != 0 && proc.ID != opts.ID {
			continue
		}

		// Every value here must already be in gojq's value space (nil, bool,
		// int, float64, *big.Int, string, []any, map[string]any). WorkingSet
		// and VirtualMemory are int64 and Handles is int32 on the struct, and
		// gojq panics on those: not only in the encoder, but inside any
		// builtin that touches the value, so `get_process | .Handles | type`
		// would take the process down. Widen them to int at the boundary.
		processMap := map[string]any{
			"Id":                 proc.ID,
			"Name":               proc.Name,
			"Path":               proc.Path,
			"CPU":                proc.CPU,
			"WorkingSet":         int(proc.WorkingSet),
			"VirtualMemory":      int(proc.VirtualMemory),
			"StartTime":          proc.StartTime.Format(time.RFC3339),
			"PriorityClass":      proc.PriorityClass,
			"Threads":            proc.ThreadCount,
			"Handles":            int(proc.Handles),
			"ResponseTime":       proc.ResponseTime.String(),
			"TotalProcessorTime": proc.TotalProcessorTime.String(),
			"UserProcessorTime":  proc.UserProcessorTime.String(),
		}

		if opts.IncludeUserName && proc.UserName != "" {
			processMap["UserName"] = proc.UserName
		}

		processes = append(processes, processMap)
	}

	return processes, nil
}

// getProcessesUnix gets process information on Unix-like systems
func getProcessesUnix(opts GetProcessOptions) ([]ProcessInfo, error) {
	// Use ps with comprehensive output format
	// Format: pid,comm,pcpu,rss,vsz,lstart,user,nice,etime,state,thcount
	// Note: Handles not available on Unix (set to 0)
	cmd := exec.Command("ps", "-eo", "pid,comm,pcpu,rss,vsz,lstart,user,nice,etime,state,thcount", "--no-headers")
	output, err := cmd.Output()
	if err != nil {
		return nil, fmt.Errorf("get_process: failed to list processes: %w", err)
	}

	var processes []ProcessInfo
	lines := strings.Split(string(output), "\n")

	for _, line := range lines {
		if strings.TrimSpace(line) == "" {
			continue
		}

		// Parse ps output - lstart is multi-word so we need careful parsing
		// Format: PID COMM %CPU RSS VSZ LSTART USER NI ELAPSED STATE THCOUNT
		// LSTART is "Day Mon DD HH:MM:SS YYYY" (5 space-separated tokens)
		// Total: 1+1+1+1+1+5+1+1+1+1+1 = 15 fields
		fields := strings.Fields(line)
		if len(fields) < 15 {
			continue
		}

		pid, err := strconv.Atoi(fields[0])
		if err != nil {
			continue
		}

		name := fields[1]

		// Apply name filter using filepath.Match for wildcard support
		if opts.Name != "" {
			matched, matchErr := filepath.Match(opts.Name, name)
			if matchErr != nil || !matched {
				// Also try case-insensitive match
				matched, matchErr = filepath.Match(strings.ToLower(opts.Name), strings.ToLower(name))
			}
			if matchErr != nil || !matched {
				continue
			}
		}

		// Apply ID filter
		if opts.ID != 0 && pid != opts.ID {
			continue
		}

		cpu, _ := strconv.ParseFloat(fields[2], 64)
		rss, _ := strconv.ParseInt(fields[3], 10, 64)
		vsz, _ := strconv.ParseInt(fields[4], 10, 64)

		// LSTART is fields[5:10] (5 tokens): "Wed Apr 29 00:29:28 2026"
		// Format: Day Mon DD HH:MM:SS YYYY
		lstartStr := strings.Join(fields[5:10], " ")
		startTime, _ := time.Parse("Mon Jan 02 15:04:05 2006", lstartStr)

		user := fields[10]
		nice, _ := strconv.Atoi(fields[11])
		// elapsed time in fields[12] - format varies (MM:SS, HH:MM:SS, D-HH:MM:SS)
		state := fields[13]
		threadCount, _ := strconv.Atoi(fields[14])

		// Map nice value to priority class (Linux-specific)
		priorityClass := mapNiceToPriorityClass(nice)

		// Map state to response time indicator
		responseTime := time.Duration(0)
		if state == "R" {
			responseTime = time.Millisecond
		}

		processes = append(processes, ProcessInfo{
			ID:                 pid,
			Name:               name,
			Path:               getProcessPathUnix(pid),
			CPU:                cpu,
			WorkingSet:         rss * 1024, // RSS is in KB
			VirtualMemory:      vsz * 1024, // VSZ is in KB
			StartTime:          startTime,
			UserName:           user,
			PriorityClass:      priorityClass,
			ThreadCount:        threadCount,
			Handles:            0, // Not available on Unix
			ResponseTime:       responseTime,
			TotalProcessorTime: time.Duration(cpu * float64(time.Second)),
			UserProcessorTime:  time.Duration(cpu * float64(time.Second) * 0.8),
		})
	}

	return processes, nil
}

// mapNiceToPriorityClass maps Linux nice value to PowerShell priority class
func mapNiceToPriorityClass(nice int) string {
	switch {
	case nice < -10:
		return "RealTime"
	case nice < 0:
		return "High"
	case nice == 0:
		return "Normal"
	case nice < 10:
		return "BelowNormal"
	default:
		return "Idle"
	}
}

// getProcessPathUnix gets the executable path for a process
func getProcessPathUnix(pid int) string {
	// Read /proc/<pid>/exe symlink on Linux
	if runtime.GOOS == "linux" {
		link := fmt.Sprintf("/proc/%d/exe", pid)
		path, err := os.Readlink(link)
		if err == nil {
			return path
		}
	}
	return ""
}

// getProcessesWindows gets process information on Windows
func getProcessesWindows(opts GetProcessOptions) ([]ProcessInfo, error) {
	// Use tasklist with CSV output format
	// Columns: Image Name,PID,Session Name,Session#,Mem Usage,Status,User Name,CPU Time,Window Title
	cmd := exec.Command("tasklist", "/FO", "CSV", "/V")
	output, err := cmd.Output()
	if err != nil {
		return nil, fmt.Errorf("get_process: failed to list processes: %w", err)
	}

	// Use encoding/csv.Reader for proper CSV parsing
	reader := csv.NewReader(strings.NewReader(string(output)))

	var processes []ProcessInfo
	firstRow := true

	for {
		record, err := reader.Read()
		if err == io.EOF {
			break
		}
		if err != nil {
			continue
		}

		// Skip header row
		if firstRow {
			firstRow = false
			continue
		}

		// tasklist /V columns:
		// 0: Image Name
		// 1: PID
		// 2: Session Name
		// 3: Session#
		// 4: Mem Usage
		// 5: Status
		// 6: User Name
		// 7: CPU Time
		// 8: Window Title
		if len(record) < 9 {
			continue
		}

		name := strings.TrimSpace(record[0])

		// Apply name filter
		if opts.Name != "" {
			matched, matchErr := filepath.Match(opts.Name, name)
			if matchErr != nil || !matched {
				matched, matchErr = filepath.Match(strings.ToLower(opts.Name), strings.ToLower(name))
			}
			if matchErr != nil || !matched {
				continue
			}
		}

		pid, err := strconv.Atoi(strings.TrimSpace(record[1]))
		if err != nil {
			continue
		}

		// Apply ID filter
		if opts.ID != 0 && pid != opts.ID {
			continue
		}

		sessionName := strings.TrimSpace(record[2])
		// sessionNum, _ := strconv.Atoi(strings.TrimSpace(record[3]))
		memUsage := parseMemoryUsage(record[4])
		// status := strings.TrimSpace(record[5])
		userName := strings.TrimSpace(record[6])
		cpuTime := parseCPUTime(record[7])
		// windowTitle := strings.TrimSpace(record[8])

		processes = append(processes, ProcessInfo{
			ID:                 pid,
			Name:               name,
			Path:               getProcessPathWindows(pid),
			CPU:                cpuTime.Seconds(),
			WorkingSet:         memUsage,
			VirtualMemory:      memUsage,    // Not separately available from tasklist
			StartTime:          time.Time{}, // Not available from tasklist
			UserName:           userName,
			PriorityClass:      "Normal", // Not available from tasklist without additional query
			ThreadCount:        0,        // Not available from tasklist without additional query
			Handles:            0,        // Not available from tasklist without additional query
			ResponseTime:       time.Duration(0),
			TotalProcessorTime: cpuTime,
			UserProcessorTime:  cpuTime,
		})

		// Include session info in path for context
		_ = sessionName
	}

	return processes, nil
}

// parseMemoryUsage parses tasklist memory usage string (e.g., "12,345 K")
func parseMemoryUsage(memStr string) int64 {
	memStr = strings.TrimSpace(memStr)
	memStr = strings.ReplaceAll(memStr, ",", "")

	// Remove " K" or " M" suffix and convert
	if strings.HasSuffix(memStr, " K") {
		memStr = strings.TrimSuffix(memStr, " K")
		val, err := strconv.ParseInt(memStr, 10, 64)
		if err != nil {
			return 0
		}
		return val * 1024
	}
	if strings.HasSuffix(memStr, " M") {
		memStr = strings.TrimSuffix(memStr, " M")
		val, err := strconv.ParseInt(memStr, 10, 64)
		if err != nil {
			return 0
		}
		return val * 1024 * 1024
	}

	val, err := strconv.ParseInt(memStr, 10, 64)
	if err != nil {
		return 0
	}
	return val
}

// parseCPUTime parses tasklist CPU time string (e.g., "0:01:23.456")
func parseCPUTime(cpuStr string) time.Duration {
	cpuStr = strings.TrimSpace(cpuStr)

	// Format: H:MM:SS or H:MM:SS.mmm
	parts := strings.Split(cpuStr, ":")
	if len(parts) != 3 {
		return time.Duration(0)
	}

	hours, _ := strconv.Atoi(parts[0])
	minutes, _ := strconv.Atoi(parts[1])

	// Seconds may have milliseconds
	secParts := strings.Split(parts[2], ".")
	seconds, _ := strconv.Atoi(secParts[0])

	milliseconds := 0
	if len(secParts) > 1 {
		milliseconds, _ = strconv.Atoi(secParts[1])
	}

	return time.Duration(hours)*time.Hour +
		time.Duration(minutes)*time.Minute +
		time.Duration(seconds)*time.Second +
		time.Duration(milliseconds)*time.Millisecond
}

// getProcessPathWindows gets the executable path for a process
func getProcessPathWindows(pid int) string {
	// Would use Windows API via golang.org/x/sys/windows in production
	// For now, return empty string
	return ""
}

// RegisterStopProcess registers the stop_process function with gojq
// Supports PowerShell-style parameters: -Name, -Id, -Force
// Usage:
//   - stop_process("notepad") - stop processes by name
//   - stop_process({"Id": 1234}) - stop by process ID
//   - stop_process("chrome"; {"Force": true}) - force kill
func RegisterStopProcess() gojq.CompilerOption {
	return gojq.WithFunction("stop_process", 0, 2, func(v any, args []any) any {
		opts := StopProcessOptions{}

		// Parse arguments
		if len(args) > 0 {
			firstArg := common.BindValue(args[0])

			if nameStr, ok := firstArg.(string); ok {
				opts.Name = nameStr
			} else if idInt, ok := firstArg.(int); ok {
				opts.ID = idInt
			} else if optsMap, ok := firstArg.(map[string]any); ok {
				parseStopProcessOptions(&opts, optsMap)
			}
		}

		// Stop processes
		stopped, failed, err := stopProcesses(opts)
		if err != nil {
			return common.MakeUDFErrorResult(err, map[string]any{
				"stopped": stopped,
				"failed":  failed,
			})
		}

		// Update $? automatic variable
		ss := common.GetSessionState()
		if ss != nil {
			_ = ss.SetVariable("?", len(failed) == 0, sessionstate.None)
		}

		result := map[string]any{
			"Stopped": stopped,
			"Failed":  failed,
		}

		return common.MakeUDFSuccessResult(result, map[string]any{
			"operation": "stop_process",
		})
	})
}

// StopProcessOptions holds options for the stop_process function
type StopProcessOptions struct {
	Name  string
	ID    int
	Force bool
}

// parseStopProcessOptions parses options from a map
func parseStopProcessOptions(opts *StopProcessOptions, optsMap map[string]any) {
	if nameVal, exists := optsMap["Name"]; exists {
		if nStr, ok := nameVal.(string); ok {
			opts.Name = nStr
		}
	}
	if idVal, exists := optsMap["Id"]; exists {
		switch v := idVal.(type) {
		case int:
			opts.ID = v
		case float64:
			opts.ID = int(v)
		}
	}
	if forceVal, exists := optsMap["Force"]; exists {
		if b, ok := forceVal.(bool); ok {
			opts.Force = b
		}
	}
}

// toPID reads a process id out of a result map, accepting the int the cmdlets
// produce and the float64 the same map takes on after a JSON round trip.
func toPID(v any) (int, bool) {
	switch id := v.(type) {
	case int:
		return id, true
	case float64:
		return int(id), true
	}
	return 0, false
}

// stopProcesses stops processes based on options
func stopProcesses(opts StopProcessOptions) (stopped []int, failed []map[string]any, err error) {
	// First get the processes to stop
	getOpts := GetProcessOptions{Name: opts.Name, ID: opts.ID}
	processes, err := getProcesses(getOpts)
	if err != nil {
		return nil, nil, err
	}

	if len(processes) == 0 {
		return nil, nil, fmt.Errorf("stop_process: no processes found matching criteria")
	}

	for _, proc := range processes {
		name, _ := proc["Name"].(string)

		// getProcesses fills Id as int; accept the float64 form too in case a
		// map was hand-built or round-tripped through JSON. A missing or
		// unusable Id must not fall through as 0: on Unix, signalling pid 0
		// signals every process in this process group.
		pid, ok := toPID(proc["Id"])
		if !ok || pid <= 0 {
			failed = append(failed, map[string]any{
				"Id":    proc["Id"],
				"Name":  name,
				"Error": fmt.Sprintf("unusable process id %v", proc["Id"]),
			})
			continue
		}

		// Skip self
		if pid == os.Getpid() {
			failed = append(failed, map[string]any{
				"Id":    pid,
				"Name":  name,
				"Error": "cannot stop self",
			})
			continue
		}

		// Find process
		p, err := os.FindProcess(pid)
		if err != nil {
			failed = append(failed, map[string]any{
				"Id":    pid,
				"Name":  name,
				"Error": err.Error(),
			})
			continue
		}

		// Try to kill the process
		var killErr error
		if opts.Force {
			killErr = p.Kill()
		} else {
			// On Unix, Signal with SIGTERM; Windows Kill is same as force
			if runtime.GOOS == "windows" {
				killErr = p.Kill()
			} else {
				killErr = p.Signal(os.Interrupt)
			}
		}

		if killErr != nil {
			failed = append(failed, map[string]any{
				"Id":    pid,
				"Name":  name,
				"Error": killErr.Error(),
			})
		} else {
			stopped = append(stopped, pid)
		}
	}

	return stopped, failed, nil
}

// RegisterStartProcess registers the start_process function with gojq
// Supports PowerShell-style parameters: -FilePath, -ArgumentList, -WindowStyle, -PassThru
// Usage:
//   - start_process("notepad.exe")
//   - start_process("cmd"; {"/c"; "echo hello"})
//   - start_process({"FilePath": "python"; "ArgumentList": ["script.py"]; "PassThru": true})
func RegisterStartProcess() gojq.CompilerOption {
	return gojq.WithFunction("start_process", 0, 2, func(v any, args []any) any {
		opts := StartProcessOptions{}

		// Parse arguments
		if len(args) > 0 {
			firstArg := common.BindValue(args[0])

			if filePathStr, ok := firstArg.(string); ok {
				opts.FilePath = filePathStr
			} else if optsMap, ok := firstArg.(map[string]any); ok {
				parseStartProcessOptions(&opts, optsMap)
			}
		}

		// Second argument could be argument list
		if len(args) > 1 {
			if opts.ArgumentList == nil {
				if argList, ok := args[1].([]any); ok {
					opts.ArgumentList = argList
				} else if argStr, ok := args[1].(string); ok {
					opts.ArgumentList = []any{argStr}
				}
			}
		}

		// Validate file path
		if opts.FilePath == "" {
			return common.MakeUDFErrorResult(fmt.Errorf("start_process: FilePath is required"), nil)
		}

		// Start the process
		result, err := startProcess(opts)
		if err != nil {
			return common.MakeUDFErrorResult(err, nil)
		}

		// Update $? automatic variable
		ss := common.GetSessionState()
		if ss != nil {
			_ = ss.SetVariable("?", true, sessionstate.None)
		}

		if opts.PassThru {
			return common.MakeUDFSuccessResult(result, map[string]any{
				"operation": "start_process",
			})
		}

		return common.MakeUDFSuccessResult(nil, map[string]any{
			"operation": "start_process",
			"Id":        result["Id"],
		})
	})
}

// StartProcessOptions holds options for the start_process function
type StartProcessOptions struct {
	FilePath       string
	ArgumentList   []any
	WindowStyle    string // Normal, Hidden, Minimized, Maximized
	PassThru       bool
	WorkingDir     string
	Environment    map[string]any
	RedirectStdin  bool
	RedirectStdout bool
	RedirectStderr bool
}

// parseStartProcessOptions parses options from a map
func parseStartProcessOptions(opts *StartProcessOptions, optsMap map[string]any) {
	if filePathVal, exists := optsMap["FilePath"]; exists {
		if fpStr, ok := filePathVal.(string); ok {
			opts.FilePath = fpStr
		}
	}
	if argListVal, exists := optsMap["ArgumentList"]; exists {
		if al, ok := argListVal.([]any); ok {
			opts.ArgumentList = al
		}
	}
	if windowStyleVal, exists := optsMap["WindowStyle"]; exists {
		if wsStr, ok := windowStyleVal.(string); ok {
			opts.WindowStyle = wsStr
		}
	}
	if passThruVal, exists := optsMap["PassThru"]; exists {
		if b, ok := passThruVal.(bool); ok {
			opts.PassThru = b
		}
	}
	if workingDirVal, exists := optsMap["WorkingDirectory"]; exists {
		if wdStr, ok := workingDirVal.(string); ok {
			opts.WorkingDir = wdStr
		}
	}
	if envVal, exists := optsMap["Environment"]; exists {
		if envMap, ok := envVal.(map[string]any); ok {
			opts.Environment = envMap
		}
	}
}

// startProcess starts a new process
func startProcess(opts StartProcessOptions) (map[string]any, error) {
	// Build command
	args := make([]string, 0, len(opts.ArgumentList))
	for _, arg := range opts.ArgumentList {
		if argStr, ok := arg.(string); ok {
			args = append(args, argStr)
		}
	}

	cmd := exec.Command(opts.FilePath, args...)

	// Set working directory
	if opts.WorkingDir != "" {
		cmd.Dir = opts.WorkingDir
	}

	// Set environment variables
	if opts.Environment != nil {
		env := os.Environ()
		for k, v := range opts.Environment {
			env = append(env, fmt.Sprintf("%s=%v", k, v))
		}
		cmd.Env = env
	}

	// Configure I/O redirection
	if !opts.RedirectStdout {
		cmd.Stdout = nil // Inherit stdout
	}
	if !opts.RedirectStderr {
		cmd.Stderr = nil // Inherit stderr
	}

	// Start the process
	err := cmd.Start()
	if err != nil {
		return nil, fmt.Errorf("start_process: failed to start %s: %w", opts.FilePath, err)
	}

	result := map[string]any{
		"Id":        cmd.Process.Pid,
		"Name":      opts.FilePath,
		"HasExited": false,
		"StartTime": time.Now().Format(time.RFC3339),
		"Process":   cmd.Process,
	}

	if opts.WindowStyle != "" {
		result["WindowStyle"] = opts.WindowStyle
	}

	return result, nil
}
