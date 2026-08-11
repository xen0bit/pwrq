// Package service provides PowerShell-style service management cmdlets.
// This file implements Get-Service, Start-Service, and Stop-Service functionality.
package service

import (
	"encoding/json"
	"fmt"
	"os/exec"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"

	"github.com/itchyny/gojq"
	"github.com/xen0bit/pwrq/pkg/core/sessionstate"
	"github.com/xen0bit/pwrq/pkg/udf/common"
)

// ServiceInfo holds information about a service
type ServiceInfo struct {
	Name               string   `json:"Name"`
	DisplayName        string   `json:"DisplayName"`
	Status             string   `json:"Status"`
	StartType          string   `json:"StartType"`
	CanStop            bool     `json:"CanStop"`
	CanPause           bool     `json:"CanPause"`
	CanShutdown        bool     `json:"CanShutdown"`
	DependentServices  []string `json:"DependentServices"`
	ServicesDependedOn []string `json:"ServicesDependedOn"`
	MachineName        string   `json:"MachineName"`
	ProcessId          int      `json:"ProcessId"`
	ServiceType        string   `json:"ServiceType"`
	Site               string   `json:"Site"`
	Container          string   `json:"Container"`
}

// GetServiceOptions holds options for the get_service function
type GetServiceOptions struct {
	Name            string // Service name filter
	DisplayName     string // Display name filter
	Exclude         string // Exclude pattern
	IncludeUserName bool   // Include username in output
}

// RegisterGetService registers the get_service function with gojq
// Supports PowerShell-style parameters: -Name, -DisplayName, -Exclude
// Usage:
//   - get_service() - get all services
//   - get_service("ssh") - get services by name (wildcard supported)
//   - get_service({"Name": "*service*"; "Exclude": "disabled"})
func RegisterGetService() gojq.CompilerOption {
	return gojq.WithIterFunction("get_service", 0, 2, func(v any, args []any) gojq.Iter {
		opts := GetServiceOptions{}

		// Parse arguments
		if len(args) > 0 {
			firstArg := common.BindValue(args[0])

			// Check if first arg is a string (service name)
			if nameStr, ok := firstArg.(string); ok {
				opts.Name = nameStr
			} else if optsMap, ok := firstArg.(map[string]any); ok {
				// First arg is options map
				parseGetServiceOptions(&opts, optsMap)
			}
		}

		// Get services
		services, err := getServices(opts)
		if err != nil {
			return gojq.NewIter(err)
		}

		// Update $? automatic variable (last command success)
		ss := common.GetSessionState()
		if ss != nil {
			ss.SetVariable("?", true, sessionstate.None)
		}

		// Convert to []any for iterator
		results := make([]any, len(services))
		for i, s := range services {
			results[i] = s
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

// parseGetServiceOptions parses options from a map
func parseGetServiceOptions(opts *GetServiceOptions, optsMap map[string]any) {
	if nameVal, exists := optsMap["Name"]; exists {
		if nStr, ok := nameVal.(string); ok {
			opts.Name = nStr
		}
	}
	if displayNameVal, exists := optsMap["DisplayName"]; exists {
		if dnStr, ok := displayNameVal.(string); ok {
			opts.DisplayName = dnStr
		}
	}
	if excludeVal, exists := optsMap["Exclude"]; exists {
		if eStr, ok := excludeVal.(string); ok {
			opts.Exclude = eStr
		}
	}
}

// getServices retrieves service information based on options
func getServices(opts GetServiceOptions) ([]map[string]any, error) {
	var services []ServiceInfo
	var err error

	if runtime.GOOS == "windows" {
		services, err = getServicesWindows(opts)
	} else {
		services, err = getServicesUnix(opts)
	}

	if err != nil {
		return nil, err
	}

	// Convert to map format for JSON output
	var result []map[string]any
	for _, svc := range services {
		// Apply name filter
		if opts.Name != "" && !matchPattern(svc.Name, opts.Name) {
			continue
		}

		// Apply display name filter
		if opts.DisplayName != "" && !matchPattern(svc.DisplayName, opts.DisplayName) {
			continue
		}

		// Apply exclude filter
		if opts.Exclude != "" && matchPattern(svc.Name, opts.Exclude) {
			continue
		}

		serviceMap := map[string]any{
			"Name":        svc.Name,
			"DisplayName": svc.DisplayName,
			"Status":      svc.Status,
			"StartType":   svc.StartType,
			"CanStop":     svc.CanStop,
			"CanPause":    svc.CanPause,
			"CanShutdown": svc.CanShutdown,
			"MachineName": svc.MachineName,
			"ServiceType": svc.ServiceType,
		}

		if len(svc.DependentServices) > 0 {
			serviceMap["DependentServices"] = svc.DependentServices
		}
		if len(svc.ServicesDependedOn) > 0 {
			serviceMap["ServicesDependedOn"] = svc.ServicesDependedOn
		}
		if svc.ProcessId > 0 {
			serviceMap["ProcessId"] = svc.ProcessId
		}

		result = append(result, serviceMap)
	}

	return result, nil
}

// matchPattern checks if a string matches a wildcard pattern
// Supports case-insensitive matching like PowerShell
func matchPattern(s, pattern string) bool {
	// First try exact case match
	matched, err := filepath.Match(pattern, s)
	if err != nil {
		// If pattern is invalid, fall back to exact match
		return s == pattern
	}
	if matched {
		return true
	}

	// If no match, try case-insensitive
	matched, err = filepath.Match(strings.ToLower(pattern), strings.ToLower(s))
	if err != nil {
		return false
	}
	return matched
}

// getServicesUnix gets service information on Unix-like systems using systemctl
func getServicesUnix(opts GetServiceOptions) ([]ServiceInfo, error) {
	// Get all services with a single systemctl call using JSON output
	cmd := exec.Command("systemctl", "list-units", "--type=service", "--all", "--no-pager", "--output=json")
	output, err := cmd.Output()
	if err != nil {
		// Fallback to text format if JSON output not supported
		return getServicesUnixText(opts)
	}

	// Parse with encoding/json. The previous version declared these struct
	// tags and then hand-parsed the output by splitting on newlines, but
	// systemctl emits the whole array on one line, so it only ever found the
	// first service - one, on a machine with a hundred and sixty.
	var units []struct {
		Unit        string `json:"unit"`
		Load        string `json:"load"`
		Active      string `json:"active"`
		Sub         string `json:"sub"`
		Description string `json:"description"`
	}
	if err := json.Unmarshal(output, &units); err != nil {
		return getServicesUnixText(opts)
	}

	// Get detailed info for all services in batch
	var services []ServiceInfo
	for _, unit := range units {
		serviceName := strings.TrimSuffix(unit.Unit, ".service")

		// Map active state to PowerShell Status
		status := "Stopped"
		switch unit.Active {
		case "active":
			status = "Running"
		case "failed":
			status = "Failed"
		}

		// Get start type from unit file state
		startType := getStartTypeUnix(serviceName)

		services = append(services, ServiceInfo{
			Name:        serviceName,
			DisplayName: unit.Description,
			Status:      status,
			StartType:   startType,
			CanStop:     status == "Running",
			CanPause:    status == "Running",
			CanShutdown: status == "Running",
			MachineName: "localhost",
			ServiceType: "Win32OwnProcess",
		})
	}

	return services, nil
}

// getStartTypeUnix gets the start type for a service
func getStartTypeUnix(name string) string {
	cmd := exec.Command("systemctl", "is-enabled", name+".service", "--quiet")
	err := cmd.Run()
	if err == nil {
		return "Automatic"
	}

	// Check if it's disabled
	cmd = exec.Command("systemctl", "is-enabled", name+".service")
	output, _ := cmd.Output()
	if strings.TrimSpace(string(output)) == "disabled" {
		return "Disabled"
	}

	return "Manual"
}

// getServicesUnixText is a fallback when JSON output is not available
func getServicesUnixText(opts GetServiceOptions) ([]ServiceInfo, error) {
	cmd := exec.Command("systemctl", "list-units", "--type=service", "--all", "--no-pager")
	output, err := cmd.Output()
	if err != nil {
		return nil, fmt.Errorf("get_service: failed to list services: %w", err)
	}

	var services []ServiceInfo
	lines := strings.Split(string(output), "\n")

	for _, line := range lines {
		fields := strings.Fields(line)
		if len(fields) < 5 {
			continue
		}

		// Skip header and footer lines
		if !strings.Contains(fields[0], ".service") {
			continue
		}

		unitName := strings.TrimSuffix(fields[0], ".service")
		load := fields[1]
		active := fields[2]
		sub := fields[3]

		// Description is everything after the first 4 fields
		description := strings.Join(fields[4:], " ")

		// Map active state to PowerShell Status
		status := "Stopped"
		if active == "active" && sub == "running" {
			status = "Running"
		} else if active == "failed" {
			status = "Failed"
		} else if active == "activating" {
			status = "StartPending"
		} else if active == "deactivating" {
			status = "StopPending"
		}

		// Skip unloaded services
		if load == "not-found" || load == "masked" {
			continue
		}

		services = append(services, ServiceInfo{
			Name:        unitName,
			DisplayName: description,
			Status:      status,
			StartType:   getStartTypeUnix(unitName),
			CanStop:     status == "Running",
			CanPause:    status == "Running",
			CanShutdown: status == "Running",
			MachineName: "localhost",
			ServiceType: "Win32OwnProcess",
		})
	}

	return services, nil
}

// getServicesWindows gets service information on Windows
// Uses sc query with a state machine parser to extract all fields
func getServicesWindows(opts GetServiceOptions) ([]ServiceInfo, error) {
	// Get all services
	cmd := exec.Command("sc", "query", "type=", "service", "state=", "all")
	output, err := cmd.Output()
	if err != nil {
		return nil, fmt.Errorf("get_service: failed to query services: %w", err)
	}

	var services []ServiceInfo
	lines := strings.Split(string(output), "\n")

	var currentService *ServiceInfo

	for _, line := range lines {
		trimmed := strings.TrimSpace(line)
		if trimmed == "" {
			continue
		}

		// Check for service header line (SERVICE_NAME:)
		if strings.HasPrefix(trimmed, "SERVICE_NAME:") {
			// Save previous service if exists
			if currentService != nil {
				services = append(services, *currentService)
			}

			// Start new service entry
			serviceName := strings.TrimSpace(strings.TrimPrefix(trimmed, "SERVICE_NAME:"))
			currentService = &ServiceInfo{
				Name:        serviceName,
				MachineName: "localhost",
				ServiceType: "Win32OwnProcess",
			}
			continue
		}

		// Parse service details
		if currentService == nil {
			continue
		}

		// Parse key-value pairs (KEY : VALUE format)
		if idx := strings.Index(trimmed, " : "); idx != -1 {
			key := strings.TrimSpace(trimmed[:idx])
			value := strings.TrimSpace(trimmed[idx+3:])

			switch key {
			case "DISPLAY_NAME":
				currentService.DisplayName = value
			case "TYPE":
				// Parse TYPE: e.g., "10  WIN32_OWN_PROCESS  (interactive)"
				currentService.ServiceType = parseServiceType(value)
			case "STATE":
				// Parse STATE: e.g., "4  RUNNING"
				currentService.Status = parseServiceState(value)
				currentService.CanStop = currentService.Status == "Running"
				currentService.CanPause = currentService.Status == "Running"
				currentService.CanShutdown = currentService.Status == "Running"
			case "WIN32_EXIT_CODE":
				// Not surfaced yet. A value other than "0"/"NO_ERROR" would
				// say the service exited with an error.
			case "WAIT_HINT":
				// Time estimate for operation completion
			case "PID":
				if pid, err := strconv.Atoi(value); err == nil && pid > 0 {
					currentService.ProcessId = pid
				}
			case "FLAGS":
				// Additional flags
			}
		}
	}

	// Don't forget the last service
	if currentService != nil {
		services = append(services, *currentService)
	}

	return services, nil
}

// parseServiceType parses the TYPE field from sc query output
func parseServiceType(typeStr string) string {
	// Common service type values:
	// 10 - WIN32_OWN_PROCESS
	// 20 - WIN32_SHARE_PROCESS
	// 100 - INTERACTIVE_PROCESS
	// 110 - WIN32_OWN_PROCESS + INTERACTIVE_PROCESS
	if strings.Contains(typeStr, "WIN32_OWN_PROCESS") {
		if strings.Contains(typeStr, "INTERACTIVE") {
			return "Win32OwnProcess + Interactive"
		}
		return "Win32OwnProcess"
	}
	if strings.Contains(typeStr, "WIN32_SHARE_PROCESS") {
		return "Win32ShareProcess"
	}
	if strings.Contains(typeStr, "INTERACTIVE") {
		return "InteractiveProcess"
	}
	return "Unknown"
}

// parseServiceState parses the STATE field from sc query output
func parseServiceState(stateStr string) string {
	// Common state values:
	// 1 - STOPPED
	// 2 - START_PENDING
	// 3 - STOP_PENDING
	// 4 - RUNNING
	// 5 - CONTINUE_PENDING
	// 6 - PAUSE_PENDING
	// 7 - PAUSED

	if strings.Contains(stateStr, "RUNNING") {
		return "Running"
	}
	if strings.Contains(stateStr, "STOPPED") {
		return "Stopped"
	}
	if strings.Contains(stateStr, "START_PENDING") {
		return "StartPending"
	}
	if strings.Contains(stateStr, "STOP_PENDING") {
		return "StopPending"
	}
	if strings.Contains(stateStr, "PAUSED") {
		return "Paused"
	}
	if strings.Contains(stateStr, "PAUSE_PENDING") {
		return "PausePending"
	}
	if strings.Contains(stateStr, "CONTINUE_PENDING") {
		return "ContinuePending"
	}

	return "Unknown"
}

// RegisterStartService registers the start_service function with gojq
// Supports PowerShell-style parameters: -Name, -InputObject, -PassThru
// Usage:
//   - start_service("ssh")
//   - start_service({"Name": "nginx"})
func RegisterStartService() gojq.CompilerOption {
	return gojq.WithFunction("start_service", 0, 2, func(v any, args []any) any {
		opts := StartServiceOptions{}

		// Parse arguments
		if len(args) > 0 {
			firstArg := common.BindValue(args[0])

			if nameStr, ok := firstArg.(string); ok {
				opts.Name = nameStr
			} else if optsMap, ok := firstArg.(map[string]any); ok {
				parseStartServiceOptions(&opts, optsMap)
			}
		}

		// Validate name
		if opts.Name == "" {
			return common.MakeUDFErrorResult(fmt.Errorf("start_service: Name is required"), nil)
		}

		// Start the service
		result, err := startService(opts)
		if err != nil {
			return common.MakeUDFErrorResult(err, nil)
		}

		// Update $? automatic variable
		ss := common.GetSessionState()
		if ss != nil {
			ss.SetVariable("?", result["Status"] == "Running", sessionstate.None)
		}

		if opts.PassThru {
			return common.MakeUDFSuccessResult(result, map[string]any{
				"operation": "start_service",
			})
		}

		return common.MakeUDFSuccessResult(nil, map[string]any{
			"operation": "start_service",
			"name":      opts.Name,
		})
	})
}

// StartServiceOptions holds options for the start_service function
type StartServiceOptions struct {
	Name     string
	PassThru bool
}

// parseStartServiceOptions parses options from a map
func parseStartServiceOptions(opts *StartServiceOptions, optsMap map[string]any) {
	if nameVal, exists := optsMap["Name"]; exists {
		if nStr, ok := nameVal.(string); ok {
			opts.Name = nStr
		}
	}
	if passThruVal, exists := optsMap["PassThru"]; exists {
		if b, ok := passThruVal.(bool); ok {
			opts.PassThru = b
		}
	}
}

// startService starts a service
func startService(opts StartServiceOptions) (map[string]any, error) {
	var err error

	if runtime.GOOS == "windows" {
		err = startServiceWindows(opts.Name)
	} else {
		err = startServiceUnix(opts.Name)
	}

	if err != nil {
		return nil, err
	}

	// Get updated service info
	services, getErr := getServices(GetServiceOptions{Name: opts.Name})
	if getErr != nil || len(services) == 0 {
		return map[string]any{
			"Name":   opts.Name,
			"Status": "Unknown",
		}, nil
	}

	return services[0], nil
}

// startServiceUnix starts a service on Unix using systemctl
func startServiceUnix(name string) error {
	cmd := exec.Command("systemctl", "start", name)
	output, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("start_service: failed to start %s: %w - %s", name, err, string(output))
	}
	return nil
}

// startServiceWindows starts a service on Windows
func startServiceWindows(name string) error {
	cmd := exec.Command("sc", "start", name)
	output, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("start_service: failed to start %s: %w - %s", name, err, string(output))
	}
	return nil
}

// RegisterStopService registers the stop_service function with gojq
// Supports PowerShell-style parameters: -Name, -InputObject, -Force, -PassThru
// Usage:
//   - stop_service("ssh")
//   - stop_service({"Name": "nginx"; "Force": true})
func RegisterStopService() gojq.CompilerOption {
	return gojq.WithFunction("stop_service", 0, 2, func(v any, args []any) any {
		opts := StopServiceOptions{}

		// Parse arguments
		if len(args) > 0 {
			firstArg := common.BindValue(args[0])

			if nameStr, ok := firstArg.(string); ok {
				opts.Name = nameStr
			} else if optsMap, ok := firstArg.(map[string]any); ok {
				parseStopServiceOptions(&opts, optsMap)
			}
		}

		// Validate name
		if opts.Name == "" {
			return common.MakeUDFErrorResult(fmt.Errorf("stop_service: Name is required"), nil)
		}

		// Stop the service
		result, err := stopService(opts)
		if err != nil {
			return common.MakeUDFErrorResult(err, nil)
		}

		// Update $? automatic variable
		ss := common.GetSessionState()
		if ss != nil {
			ss.SetVariable("?", result["Status"] == "Stopped", sessionstate.None)
		}

		if opts.PassThru {
			return common.MakeUDFSuccessResult(result, map[string]any{
				"operation": "stop_service",
			})
		}

		return common.MakeUDFSuccessResult(nil, map[string]any{
			"operation": "stop_service",
			"name":      opts.Name,
		})
	})
}

// StopServiceOptions holds options for the stop_service function
type StopServiceOptions struct {
	Name     string
	Force    bool
	PassThru bool
}

// parseStopServiceOptions parses options from a map
func parseStopServiceOptions(opts *StopServiceOptions, optsMap map[string]any) {
	if nameVal, exists := optsMap["Name"]; exists {
		if nStr, ok := nameVal.(string); ok {
			opts.Name = nStr
		}
	}
	if forceVal, exists := optsMap["Force"]; exists {
		if b, ok := forceVal.(bool); ok {
			opts.Force = b
		}
	}
	if passThruVal, exists := optsMap["PassThru"]; exists {
		if b, ok := passThruVal.(bool); ok {
			opts.PassThru = b
		}
	}
}

// stopService stops a service
func stopService(opts StopServiceOptions) (map[string]any, error) {
	var err error

	if runtime.GOOS == "windows" {
		err = stopServiceWindows(opts.Name, opts.Force)
	} else {
		err = stopServiceUnix(opts.Name, opts.Force)
	}

	if err != nil {
		return nil, err
	}

	// Get updated service info
	services, getErr := getServices(GetServiceOptions{Name: opts.Name})
	if getErr != nil || len(services) == 0 {
		return map[string]any{
			"Name":   opts.Name,
			"Status": "Unknown",
		}, nil
	}

	return services[0], nil
}

// stopServiceUnix stops a service on Unix using systemctl
func stopServiceUnix(name string, force bool) error {
	var cmd *exec.Cmd

	if force {
		// Force stop uses systemctl kill which sends SIGKILL
		// This is more aggressive than stop and cannot be caught by the service
		cmd = exec.Command("systemctl", "kill", name, "--signal=SIGKILL")
	} else {
		// Normal stop sends SIGTERM, allowing graceful shutdown
		cmd = exec.Command("systemctl", "stop", name)
	}

	output, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("stop_service: failed to stop %s: %w - %s", name, err, string(output))
	}
	return nil
}

// stopServiceWindows stops a service on Windows
func stopServiceWindows(name string, force bool) error {
	var cmd *exec.Cmd
	if force {
		cmd = exec.Command("sc", "stop", name)
	} else {
		cmd = exec.Command("sc", "stop", name)
	}

	output, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("stop_service: failed to stop %s: %w - %s", name, err, string(output))
	}
	return nil
}
