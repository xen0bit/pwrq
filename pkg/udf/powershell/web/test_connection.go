// Package web provides PowerShell-style web and network cmdlets.
// This file implements Test-Connection functionality (ping).
package web

import (
	"context"
	"crypto/tls"
	"fmt"
	"net"
	"net/http"
	"runtime"
	"strconv"
	"time"

	"github.com/itchyny/gojq"
	"github.com/xen0bit/pwrq/pkg/core/sessionstate"
	"github.com/xen0bit/pwrq/pkg/udf/common"
)

// PingResult holds the result of a ping operation
type PingResult struct {
	Target      string        `json:"Target"`
	IPv4Address string        `json:"IPv4Address"`
	IPv6Address string        `json:"IPv6Address"`
	Bytes       int           `json:"Bytes"`
	Time        time.Duration `json:"Time"`
	TTL         int           `json:"TTL"`
	Hops        int           `json:"Hops"`
	StatusCode  string        `json:"StatusCode"`
	Success     bool          `json:"Success"`
}

// TestConnectionOptions holds options for test_connection
type TestConnectionOptions struct {
	Target           string
	Count            int           // Number of pings
	Timeout          int           // Timeout in seconds
	TTL              int           // Time to live
	BufferSize       int           // Buffer size in bytes
	ResolveDNS       bool          // Resolve DNS names
	Quiet            bool          // Return only success/failure
	TcpPort          int           // Test TCP connection to port instead of ping
	HttpProbe        bool          // Test HTTP connectivity
	HttpsProbe       bool          // Test HTTPS connectivity
	SkipSSLVerify    bool          // Skip SSL verification for HTTP probes
}

// RegisterTestConnection registers the test_connection function with gojq
// PowerShell compatibility: Test-Connection
// Usage:
//   - test_connection("google.com") - ping a host
//   - test_connection("google.com"; {"Count": 4; "Timeout": 5})
//   - test_connection({"Target": "8.8.8.8"; "Count": 2; "TcpPort": 443})
func RegisterTestConnection() gojq.CompilerOption {
	return gojq.WithFunction("test_connection", 0, 2, func(v any, args []any) any {
		opts := TestConnectionOptions{
			Count:      4,
			Timeout:    5,
			TTL:        64,
			BufferSize: 32,
			ResolveDNS: true,
		}

		// Parse arguments
		if len(args) > 0 {
			firstArg := common.ExtractUDFValue(args[0])

			if targetStr, ok := firstArg.(string); ok {
				opts.Target = targetStr
			} else if optsMap, ok := firstArg.(map[string]any); ok {
				parseTestConnectionOptions(&opts, optsMap)
			}
		}

		// Second argument could be options map
		if len(args) > 1 {
			if secondArg := common.ExtractUDFValue(args[1]); secondArg != nil {
				if optsMap, ok := secondArg.(map[string]any); ok {
					parseTestConnectionOptions(&opts, optsMap)
				}
			}
		}

		// If target still empty, try to get from pipeline input
		if opts.Target == "" {
			if pipelineVal := common.ExtractUDFValue(v); pipelineVal != nil {
				if targetStr, ok := pipelineVal.(string); ok {
					opts.Target = targetStr
				}
			}
		}

		// Validate target
		if opts.Target == "" {
			return common.MakeUDFErrorResult(fmt.Errorf("test_connection: Target is required"), nil)
		}

		// Perform the connection test
		results, err := testConnection(opts)
		if err != nil {
			ss := common.GetSessionState()
			if ss != nil {
				ss.SetVariable("?", false, sessionstate.None)
			}

			return common.MakeUDFErrorResult(err, map[string]any{
				"operation": "test_connection",
				"target":    opts.Target,
			})
		}

		ss := common.GetSessionState()
		if ss != nil {
			// Set $? based on whether any ping succeeded
			anySuccess := false
			for _, r := range results {
				if r.Success {
					anySuccess = true
					break
				}
			}
			ss.SetVariable("?", anySuccess, sessionstate.None)
		}

		// If Quiet mode, return just success/failure
		if opts.Quiet {
			anySuccess := false
			for _, r := range results {
				if r.Success {
					anySuccess = true
					break
				}
			}
			return common.MakeUDFSuccessResult(anySuccess, map[string]any{
				"operation": "test_connection",
				"quiet":     true,
			})
		}

		// Convert results to maps for JSON encoding
		resultsArray := make([]any, len(results))
		for i, r := range results {
			resultsArray[i] = pingResultToMap(r)
		}

		return common.MakeUDFSuccessResult(resultsArray, map[string]any{
			"operation": "test_connection",
		})
	})
}

// parseTestConnectionOptions parses options from a map
func parseTestConnectionOptions(opts *TestConnectionOptions, optsMap map[string]any) {
	if targetVal, exists := optsMap["Target"]; exists {
		if tStr, ok := targetVal.(string); ok {
			opts.Target = tStr
		}
	}
	if countVal, exists := optsMap["Count"]; exists {
		switch c := countVal.(type) {
		case int:
			opts.Count = c
		case float64:
			opts.Count = int(c)
		}
	}
	if timeoutVal, exists := optsMap["TimeoutSeconds"]; exists {
		switch t := timeoutVal.(type) {
		case int:
			opts.Timeout = t
		case float64:
			opts.Timeout = int(t)
		}
	}
	if ttlVal, exists := optsMap["TTL"]; exists {
		switch t := ttlVal.(type) {
		case int:
			opts.TTL = t
		case float64:
			opts.TTL = int(t)
		}
	}
	if bufferSizeVal, exists := optsMap["BufferSize"]; exists {
		switch b := bufferSizeVal.(type) {
		case int:
			opts.BufferSize = b
		case float64:
			opts.BufferSize = int(b)
		}
	}
	if resolveVal, exists := optsMap["ResolveDNS"]; exists {
		if b, ok := resolveVal.(bool); ok {
			opts.ResolveDNS = b
		}
	}
	if quietVal, exists := optsMap["Quiet"]; exists {
		if b, ok := quietVal.(bool); ok {
			opts.Quiet = b
		}
	}
	if tcpPortVal, exists := optsMap["TcpPort"]; exists {
		switch p := tcpPortVal.(type) {
		case int:
			opts.TcpPort = p
		case float64:
			opts.TcpPort = int(p)
		}
	}
	if httpProbeVal, exists := optsMap["HttpProbe"]; exists {
		if b, ok := httpProbeVal.(bool); ok {
			opts.HttpProbe = b
		}
	}
	if httpsProbeVal, exists := optsMap["HttpsProbe"]; exists {
		if b, ok := httpsProbeVal.(bool); ok {
			opts.HttpsProbe = b
		}
	}
	if skipSSLVal, exists := optsMap["SkipSSLVerify"]; exists {
		if b, ok := skipSSLVal.(bool); ok {
			opts.SkipSSLVerify = b
		}
	}
}

// testConnection performs the connection test
func testConnection(opts TestConnectionOptions) ([]PingResult, error) {
	// If TcpPort is specified, test TCP connection instead of ping
	if opts.TcpPort > 0 {
		return testTCPConnection(opts)
	}

	// If HTTP/HTTPS probe is specified
	if opts.HttpProbe || opts.HttpsProbe {
		return testHTTPConnection(opts)
	}

	// Standard ping test
	return testPing(opts)
}

// testPing performs ICMP ping (or fallback to TCP on restricted systems)
func testPing(opts TestConnectionOptions) ([]PingResult, error) {
	// Resolve the target address
	ipAddr, err := resolveTarget(opts.Target, opts.ResolveDNS)
	if err != nil {
		return []PingResult{{
			Target:     opts.Target,
			StatusCode: "UnknownHost",
			Success:    false,
		}}, nil
	}

	// On most Unix systems, raw ICMP requires root privileges
	// Fall back to TCP connection test on port 80/443 or DNS port 53
	if runtime.GOOS != "windows" {
		// Try TCP connection as fallback
		return testTCPConnectToHost(opts, ipAddr)
	}

	// Windows: could use ICMP, but for simplicity use TCP
	return testTCPConnectToHost(opts, ipAddr)
}

// resolveTarget resolves a hostname to IP addresses
func resolveTarget(target string, resolveDNS bool) (string, error) {
	// Check if target is already an IP address
	if net.ParseIP(target) != nil {
		return target, nil
	}

	if !resolveDNS {
		return target, nil
	}

	// Resolve hostname
	ips, err := net.LookupIP(target)
	if err != nil {
		return "", fmt.Errorf("failed to resolve %q: %w", target, err)
	}

	if len(ips) == 0 {
		return "", fmt.Errorf("no IP addresses found for %q", target)
	}

	// Prefer IPv4
	for _, ip := range ips {
		if ip.To4() != nil {
			return ip.String(), nil
		}
	}

	return ips[0].String(), nil
}

// testTCPConnectToHost tests TCP connectivity to a host
func testTCPConnectToHost(opts TestConnectionOptions, ipAddr string) ([]PingResult, error) {
	var results []PingResult

	// Determine ports to test
	ports := []int{80, 443, 53} // HTTP, HTTPS, DNS
	if opts.TcpPort > 0 {
		ports = []int{opts.TcpPort}
	}

	dialer := net.Dialer{} // Reuse dialer across iterations

	for i := 0; i < opts.Count; i++ {
		result := PingResult{
			Target:      opts.Target,
			IPv4Address: ipAddr,
			Bytes:       opts.BufferSize,
			TTL:         opts.TTL,
			Hops:        1,
		}

		// Try each port until one succeeds
		var success bool
		var responseTime time.Duration

		for _, port := range ports {
			startTime := time.Now()

			ctx, cancel := context.WithTimeout(context.Background(), time.Duration(opts.Timeout)*time.Second)

			conn, err := dialer.DialContext(ctx, "tcp", net.JoinHostPort(ipAddr, strconv.Itoa(port)))
			responseTime = time.Since(startTime)
			cancel() // Explicitly cancel after each dial attempt

			if err == nil {
				conn.Close()
				success = true
				result.StatusCode = "Success"
				result.Time = responseTime
				break
			}
		}

		result.Success = success
		if !success {
			result.StatusCode = "TimedOut"
			result.Time = time.Duration(opts.Timeout) * time.Second
		}

		results = append(results, result)

		// Small delay between pings
		if i < opts.Count-1 {
			time.Sleep(100 * time.Millisecond)
		}
	}

	return results, nil
}

// testTCPConnection tests TCP connection to a specific port
func testTCPConnection(opts TestConnectionOptions) ([]PingResult, error) {
	var results []PingResult

	// Resolve target
	ipAddr, err := resolveTarget(opts.Target, opts.ResolveDNS)
	if err != nil {
		return []PingResult{{
			Target:     opts.Target,
			StatusCode: "UnknownHost",
			Success:    false,
		}}, nil
	}

	dialer := net.Dialer{} // Reuse dialer across iterations

	for i := 0; i < opts.Count; i++ {
		result := PingResult{
			Target:      opts.Target,
			IPv4Address: ipAddr,
			Bytes:       opts.BufferSize,
			TTL:         opts.TTL,
			Hops:        1,
		}

		startTime := time.Now()

		ctx, cancel := context.WithTimeout(context.Background(), time.Duration(opts.Timeout)*time.Second)

		conn, err := dialer.DialContext(ctx, "tcp", net.JoinHostPort(ipAddr, strconv.Itoa(opts.TcpPort)))
		responseTime := time.Since(startTime)
		cancel() // Explicitly cancel after dial attempt

		if err == nil {
			conn.Close()
			result.Success = true
			result.StatusCode = "Success"
			result.Time = responseTime
		} else {
			result.Success = false
			result.StatusCode = "ConnectionFailed"
			result.Time = time.Duration(opts.Timeout) * time.Second
		}

		results = append(results, result)

		if i < opts.Count-1 {
			time.Sleep(100 * time.Millisecond)
		}
	}

	return results, nil
}

// testHTTPConnection tests HTTP/HTTPS connectivity
func testHTTPConnection(opts TestConnectionOptions) ([]PingResult, error) {
	var results []PingResult

	for i := 0; i < opts.Count; i++ {
		result := PingResult{
			Target: opts.Target,
			Bytes:  opts.BufferSize,
			TTL:    opts.TTL,
			Hops:   1,
		}

		// Determine URL
		scheme := "http"
		if opts.HttpsProbe {
			scheme = "https"
		} else if !opts.HttpProbe {
			// Try both
			scheme = "https"
		}

		url := fmt.Sprintf("%s://%s", scheme, opts.Target)

		startTime := time.Now()

		ctx, cancel := context.WithTimeout(context.Background(), time.Duration(opts.Timeout)*time.Second)

		client := &http.Client{
			Timeout: time.Duration(opts.Timeout) * time.Second,
		}

		if opts.SkipSSLVerify {
			client.Transport = &http.Transport{
				TLSClientConfig: &tls.Config{InsecureSkipVerify: true},
			}
		}

		req, err := http.NewRequestWithContext(ctx, "HEAD", url, nil)
		cancel() // Explicitly cancel after request creation

		if err != nil {
			result.Success = false
			result.StatusCode = "RequestFailed"
			results = append(results, result)
			continue
		}

		resp, err := client.Do(req)
		responseTime := time.Since(startTime)

		if err == nil {
			resp.Body.Close()
			result.Success = resp.StatusCode >= 200 && resp.StatusCode < 400
			if result.Success {
				result.StatusCode = "Success"
			} else {
				result.StatusCode = fmt.Sprintf("HTTP_%d", resp.StatusCode)
			}
			result.Time = responseTime
		} else {
			result.Success = false
			result.StatusCode = "ConnectionFailed"
			result.Time = time.Duration(opts.Timeout) * time.Second
		}

		results = append(results, result)

		if i < opts.Count-1 {
			time.Sleep(100 * time.Millisecond)
		}
	}

	return results, nil
}

// pingResultToMap converts a PingResult to a map for JSON encoding
func pingResultToMap(r PingResult) map[string]any {
	return map[string]any{
		"Target":      r.Target,
		"IPv4Address": r.IPv4Address,
		"IPv6Address": r.IPv6Address,
		"Bytes":       r.Bytes,
		"Time":        r.Time.String(),
		"TTL":         r.TTL,
		"Hops":        r.Hops,
		"StatusCode":  r.StatusCode,
		"Success":     r.Success,
	}
}
