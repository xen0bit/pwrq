// Package system provides host lookup and PATH helpers that touch the network
// or the filesystem, so they exist in the CLI only and are flagged unavailable
// in the browser.
package system

import (
	"fmt"
	"net"
	"os/exec"

	"github.com/itchyny/gojq"
	"github.com/xen0bit/pwrq/pkg/udf/common"
)

// RegisterAll registers every system cmdlet.
func RegisterAll() []gojq.CompilerOption {
	return []gojq.CompilerOption{
		RegisterResolveHost(),
		RegisterReverseDNS(),
		RegisterWhich(),
	}
}

// hostArg resolves the input from the first argument or the pipeline.
func hostArg(v any, args []any, name string) (string, error) {
	if len(args) > 0 {
		if s, ok := common.BindValue(args[0]).(string); ok {
			return s, nil
		}
	}
	switch val := common.BindValue(v).(type) {
	case string:
		return val, nil
	default:
		return "", fmt.Errorf("%s: expected a string, got %T", name, val)
	}
}

// RegisterResolveHost registers resolve_host, the addresses a hostname
// resolves to.
func RegisterResolveHost() gojq.CompilerOption {
	return gojq.WithFunction("resolve_host", 0, 1, func(v any, args []any) any {
		host, err := hostArg(v, args, "resolve_host")
		if err != nil {
			return common.MakeUDFErrorResult(fmt.Errorf("resolve_host: %v", err), nil)
		}
		addresses, err := net.LookupHost(host)
		if err != nil {
			return common.MakeUDFErrorResult(fmt.Errorf("resolve_host: %q: %v", host, err), nil)
		}
		out := make([]any, len(addresses))
		for i, a := range addresses {
			out[i] = a
		}
		return common.MakeUDFSuccessResult(out, nil)
	})
}

// RegisterReverseDNS registers reverse_dns, the hostnames an address points
// back to.
func RegisterReverseDNS() gojq.CompilerOption {
	return gojq.WithFunction("reverse_dns", 0, 1, func(v any, args []any) any {
		ip, err := hostArg(v, args, "reverse_dns")
		if err != nil {
			return common.MakeUDFErrorResult(fmt.Errorf("reverse_dns: %v", err), nil)
		}
		names, err := net.LookupAddr(ip)
		if err != nil || len(names) == 0 {
			return common.MakeUDFErrorResult(fmt.Errorf("reverse_dns: no PTR record for %q", ip), nil)
		}
		out := make([]any, len(names))
		for i, n := range names {
			out[i] = n
		}
		return common.MakeUDFSuccessResult(out, nil)
	})
}

// RegisterWhich registers which, the path to an executable on PATH.
func RegisterWhich() gojq.CompilerOption {
	return gojq.WithFunction("which", 0, 1, func(v any, args []any) any {
		command, err := hostArg(v, args, "which")
		if err != nil {
			return common.MakeUDFErrorResult(fmt.Errorf("which: %v", err), nil)
		}
		path, err := exec.LookPath(command)
		if err != nil {
			return common.MakeUDFErrorResult(fmt.Errorf("which: %q not found on PATH", command), nil)
		}
		return common.MakeUDFSuccessResult(path, nil)
	})
}
