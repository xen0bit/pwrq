package web

import (
	"testing"
)

func TestInvokeWebRequest_OptionsParsing(t *testing.T) {
	opts := InvokeWebRequestOptions{
		Method:               "GET",
		Timeout:              30,
		AllowAutoRedirect:    true,
		MaximumRedirection:   5,
		SkipSSLVerify:        false,
	}

	optsMap := map[string]any{
		"Uri":                "https://example.com",
		"Method":             "POST",
		"TimeoutSec":         60,
		"SkipSSLVerify":      true,
		"AllowAutoRedirect":  false,
		"MaximumRedirection": 10,
		"Headers": map[string]any{
			"Authorization": "Bearer token123",
			"Content-Type":  "application/json",
		},
		"Body": map[string]any{
			"key": "value",
		},
	}

	parseInvokeWebRequestOptions(&opts, optsMap)

	if opts.Uri != "https://example.com" {
		t.Errorf("Expected Uri to be 'https://example.com', got %q", opts.Uri)
	}
	if opts.Method != "POST" {
		t.Errorf("Expected Method to be 'POST', got %q", opts.Method)
	}
	if opts.Timeout != 60 {
		t.Errorf("Expected Timeout to be 60, got %d", opts.Timeout)
	}
	if !opts.SkipSSLVerify {
		t.Error("Expected SkipSSLVerify to be true")
	}
	if opts.AllowAutoRedirect {
		t.Error("Expected AllowAutoRedirect to be false")
	}
	if opts.MaximumRedirection != 10 {
		t.Errorf("Expected MaximumRedirection to be 10, got %d", opts.MaximumRedirection)
	}
	if opts.Headers["Authorization"] != "Bearer token123" {
		t.Errorf("Expected Authorization header to be 'Bearer token123', got %q", opts.Headers["Authorization"])
	}
}

func TestInvokeWebRequest_OptionsParsingBodyTypes(t *testing.T) {
	// Test string body
	opts1 := InvokeWebRequestOptions{}
	optsMap1 := map[string]any{
		"Body": "test body string",
	}
	parseInvokeWebRequestOptions(&opts1, optsMap1)
	if opts1.Body != "test body string" {
		t.Errorf("Expected Body to be 'test body string', got %v", opts1.Body)
	}

	// Test map body
	opts2 := InvokeWebRequestOptions{}
	optsMap2 := map[string]any{
		"Body": map[string]any{"key": "value"},
	}
	parseInvokeWebRequestOptions(&opts2, optsMap2)
	bodyMap, ok := opts2.Body.(map[string]any)
	if !ok {
		t.Error("Expected Body to be map[string]any")
	}
	if bodyMap["key"] != "value" {
		t.Errorf("Expected Body.key to be 'value', got %v", bodyMap["key"])
	}

	// Test array body
	opts3 := InvokeWebRequestOptions{}
	optsMap3 := map[string]any{
		"Body": []any{"item1", "item2"},
	}
	parseInvokeWebRequestOptions(&opts3, optsMap3)
	bodyArray, ok := opts3.Body.([]any)
	if !ok {
		t.Error("Expected Body to be []any")
	}
	if len(bodyArray) != 2 {
		t.Errorf("Expected Body array length to be 2, got %d", len(bodyArray))
	}
}

func TestTestConnection_OptionsParsing(t *testing.T) {
	opts := TestConnectionOptions{
		Count:      4,
		Timeout:    5,
		TTL:        64,
		BufferSize: 32,
		ResolveDNS: true,
	}

	optsMap := map[string]any{
		"Target":           "google.com",
		"Count":            2,
		"TimeoutSeconds":   10,
		"TTL":              128,
		"BufferSize":       64,
		"ResolveDNS":       false,
		"Quiet":            true,
		"TcpPort":          443,
		"HttpProbe":        true,
		"SkipSSLVerify":    true,
	}

	parseTestConnectionOptions(&opts, optsMap)

	if opts.Target != "google.com" {
		t.Errorf("Expected Target to be 'google.com', got %q", opts.Target)
	}
	if opts.Count != 2 {
		t.Errorf("Expected Count to be 2, got %d", opts.Count)
	}
	if opts.Timeout != 10 {
		t.Errorf("Expected Timeout to be 10, got %d", opts.Timeout)
	}
	if opts.TTL != 128 {
		t.Errorf("Expected TTL to be 128, got %d", opts.TTL)
	}
	if opts.BufferSize != 64 {
		t.Errorf("Expected BufferSize to be 64, got %d", opts.BufferSize)
	}
	if opts.ResolveDNS {
		t.Error("Expected ResolveDNS to be false")
	}
	if !opts.Quiet {
		t.Error("Expected Quiet to be true")
	}
	if opts.TcpPort != 443 {
		t.Errorf("Expected TcpPort to be 443, got %d", opts.TcpPort)
	}
	if !opts.HttpProbe {
		t.Error("Expected HttpProbe to be true")
	}
	if !opts.SkipSSLVerify {
		t.Error("Expected SkipSSLVerify to be true")
	}
}

func TestTestConnection_OptionsParsingFloatTypes(t *testing.T) {
	opts := TestConnectionOptions{}
	optsMap := map[string]any{
		"Count":          float64(3),
		"TimeoutSeconds": float64(7.5),
		"TTL":            float64(100),
		"BufferSize":     float64(128),
		"TcpPort":        float64(8080),
	}

	parseTestConnectionOptions(&opts, optsMap)

	if opts.Count != 3 {
		t.Errorf("Expected Count to be 3, got %d", opts.Count)
	}
	if opts.Timeout != 7 {
		t.Errorf("Expected Timeout to be 7, got %d", opts.Timeout)
	}
	if opts.TTL != 100 {
		t.Errorf("Expected TTL to be 100, got %d", opts.TTL)
	}
	if opts.BufferSize != 128 {
		t.Errorf("Expected BufferSize to be 128, got %d", opts.BufferSize)
	}
	if opts.TcpPort != 8080 {
		t.Errorf("Expected TcpPort to be 8080, got %d", opts.TcpPort)
	}
}

func TestResolveTarget(t *testing.T) {
	// Test IP address (should return as-is)
	ip := "8.8.8.8"
	result, err := resolveTarget(ip, true)
	if err != nil {
		t.Errorf("resolveTarget(%q) returned error: %v", ip, err)
	}
	if result != "8.8.8.8" {
		t.Errorf("Expected result to be '8.8.8.8', got %q", result)
	}

	// Test IPv6
	ipv6 := "::1"
	result, err = resolveTarget(ipv6, true)
	if err != nil {
		t.Errorf("resolveTarget(%q) returned error: %v", ipv6, err)
	}
	// IPv6 might be normalized, just check it's not empty
	if result == "" {
		t.Error("Expected non-empty result for IPv6")
	}

	// Test hostname resolution (should work in most environments)
	hostname := "localhost"
	result, err = resolveTarget(hostname, true)
	if err != nil {
		t.Errorf("resolveTarget(%q) returned error: %v", hostname, err)
	}
	if result == "" {
		t.Error("Expected non-empty result for localhost")
	}
}

func TestResolveTarget_NoDNS(t *testing.T) {
	// When ResolveDNS is false, hostname should be returned as-is
	hostname := "example.com"
	result, err := resolveTarget(hostname, false)
	if err != nil {
		t.Errorf("resolveTarget(%q, false) returned error: %v", hostname, err)
	}
	if result != hostname {
		t.Errorf("Expected result to be %q, got %q", hostname, result)
	}
}

func TestPingResult_StructFields(t *testing.T) {
	result := PingResult{
		Target:      "example.com",
		IPv4Address: "93.184.216.34",
		IPv6Address: "",
		Bytes:       32,
		Time:        50000000, // 50ms
		TTL:         64,
		Hops:        1,
		StatusCode:  "Success",
		Success:     true,
	}

	if result.Target != "example.com" {
		t.Errorf("Expected Target to be 'example.com', got %q", result.Target)
	}
	if result.IPv4Address != "93.184.216.34" {
		t.Errorf("Expected IPv4Address to be '93.184.216.34', got %q", result.IPv4Address)
	}
	if result.Bytes != 32 {
		t.Errorf("Expected Bytes to be 32, got %d", result.Bytes)
	}
	if !result.Success {
		t.Error("Expected Success to be true")
	}
	if result.StatusCode != "Success" {
		t.Errorf("Expected StatusCode to be 'Success', got %q", result.StatusCode)
	}
}

func TestWebResponse_StructFields(t *testing.T) {
	response := WebResponse{
		Content:       "test content",
		StatusCode:    200,
		Status:        "200 OK",
		Headers:       map[string][]string{"Content-Type": {"application/json"}},
		RequestMethod: "GET",
		ContentLength: 12,
		ContentType:   "application/json",
		BaseResponse:  "JSON",
	}

	if response.Content != "test content" {
		t.Errorf("Expected Content to be 'test content', got %q", response.Content)
	}
	if response.StatusCode != 200 {
		t.Errorf("Expected StatusCode to be 200, got %d", response.StatusCode)
	}
	if response.ContentType != "application/json" {
		t.Errorf("Expected ContentType to be 'application/json', got %q", response.ContentType)
	}
	if response.BaseResponse != "JSON" {
		t.Errorf("Expected BaseResponse to be 'JSON', got %q", response.BaseResponse)
	}
}
