package sh

import (
	"fmt"
	"os/exec"
	"strings"

	"github.com/itchyny/gojq"
	"github.com/xen0bit/pwrq/pkg/udf/common"
)

// RegisterSh registers the sh function with gojq
func RegisterSh() gojq.CompilerOption {
	return gojq.WithFunction("sh", 0, 1, func(v any, args []any) any {
		var command string

		// Parse argument: command string can come from pipe or as argument
		if len(args) == 0 {
			// Try to get command from pipeline
			inputVal := common.ExtractUDFValue(v)
			if cmdStr, ok := inputVal.(string); ok {
				command = cmdStr
			} else {
				return common.MakeUDFErrorResult(fmt.Errorf("sh: command must be a string, got %T", inputVal), nil)
			}
		} else {
			// Command provided as argument
			if cmd, ok := args[0].(string); ok {
				command = cmd
			} else {
				// Try to extract from UDF result
				cmdVal := common.ExtractUDFValue(args[0])
				if cmdStr, ok := cmdVal.(string); ok {
					command = cmdStr
				} else {
					return common.MakeUDFErrorResult(fmt.Errorf("sh: argument must be a string command, got %T", args[0]), nil)
				}
			}
		}

		if command == "" {
			return common.MakeUDFErrorResult(fmt.Errorf("sh: command cannot be empty"), nil)
		}

		// Execute command using sh -c
		cmd := exec.Command("sh", "-c", command)
		
		// Capture stdout and stderr
		stdout, err := cmd.Output()
		stderr := []byte{}
		if err != nil {
			// If there's an error, try to get stderr from the exec.ExitError
			if exitErr, ok := err.(*exec.ExitError); ok {
				stderr = exitErr.Stderr
			}
		}

		// Get exit code
		exitCode := 0
		if err != nil {
			if exitErr, ok := err.(*exec.ExitError); ok {
				exitCode = exitErr.ExitCode()
			} else {
				// If it's not an ExitError, it's a different kind of error (e.g., command not found)
				meta := map[string]any{
					"operation": "sh",
					"command":   command,
				}
				return common.MakeUDFErrorResult(fmt.Errorf("sh: failed to execute command: %v", err), meta)
			}
		}

		// Prepare metadata
		meta := map[string]any{
			"operation": "sh",
			"command":   command,
			"exit_code": exitCode,
		}

		// If exit code is non-zero, return error result with stderr
		if exitCode != 0 {
			stderrStr := strings.TrimSpace(string(stderr))
			if stderrStr == "" {
				stderrStr = fmt.Sprintf("command exited with code %d", exitCode)
			}
			
			// Return error result with stdout in _val and stderr in _err
			result := map[string]any{
				"_val":  strings.TrimSpace(string(stdout)),
				"_meta": meta,
				"_err":  stderrStr,
			}
			return result
		}

		// Success: return stdout
		stdoutStr := strings.TrimSpace(string(stdout))
		return common.MakeUDFSuccessResult(stdoutStr, meta)
	})
}

