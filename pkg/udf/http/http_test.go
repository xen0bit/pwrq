package http

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/itchyny/gojq"
)

// Helper to compile and run a gojq query
func runGojqQuery(t *testing.T, query string, input any, options ...gojq.CompilerOption) any {
	q, err := gojq.Parse(query)
	if err != nil {
		t.Fatalf("Failed to parse query %q: %v", query, err)
	}

	code, err := gojq.Compile(q, options...)
	if err != nil {
		t.Fatalf("Failed to compile query %q: %v", query, err)
	}

	var result any
	iter := code.Run(input)
	for {
		v, ok := iter.Next()
		if !ok {
			break
		}
		if err, ok := v.(error); ok {
			t.Fatalf("Query execution failed: %v", err)
		}
		result = v
	}
	return result
}

// runGojqQueryErr runs a query that is expected to fail and returns the error.
// UDF failures now travel on jq's error channel rather than in-band, so a test
// that wants a failure has to look for one there.
func runGojqQueryErr(t *testing.T, query string, input any, options ...gojq.CompilerOption) error {
	t.Helper()
	q, err := gojq.Parse(query)
	if err != nil {
		t.Fatalf("Failed to parse query %q: %v", query, err)
	}
	code, err := gojq.Compile(q, options...)
	if err != nil {
		t.Fatalf("Failed to compile query %q: %v", query, err)
	}
	iter := code.Run(input)
	for {
		v, ok := iter.Next()
		if !ok {
			break
		}
		if err, ok := v.(error); ok {
			return err
		}
	}
	t.Fatalf("expected query %q to fail, but it succeeded", query)
	return nil
}

func TestHTTPGet(t *testing.T) {
	// Create a test HTTP server
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != "GET" {
			t.Errorf("Expected GET, got %s", r.Method)
		}
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("Hello, World!"))
	}))
	defer server.Close()

	// Test GET request with URL as argument
	result := runGojqQuery(t, fmt.Sprintf(`http("GET"; "%s")`, server.URL), nil, RegisterHTTP())

	resultMap, ok := result.(map[string]any)
	if !ok {
		t.Fatalf("Expected response object, got %T", result)
	}

	val := resultMap["Content"]
	if valStr, ok := val.(string); !ok || valStr != "Hello, World!" {
		t.Errorf("Expected response body 'Hello, World!', got %v", val)
	}

	meta := resultMap
	if meta["Method"] != "GET" {
		t.Errorf("Expected method GET, got %v", meta["Method"])
	}
	status, ok := meta["StatusCode"].(int)
	if !ok {
		statusFloat, ok := meta["StatusCode"].(float64)
		if !ok {
			t.Errorf("Expected status to be int or float64, got %T", meta["StatusCode"])
		} else {
			status = int(statusFloat)
		}
	}
	if status != 200 {
		t.Errorf("Expected status 200, got %v", status)
	}
}

func TestHTTPPostDefault(t *testing.T) {
	// Create a test HTTP server
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != "POST" {
			t.Errorf("Expected POST, got %s", r.Method)
		}
		// Read request body
		body := make([]byte, r.ContentLength)
		_, _ = r.Body.Read(body)
		w.WriteHeader(http.StatusOK)
		_, _ = fmt.Fprintf(w, "Received: %s", string(body))
	}))
	defer server.Close()

	// Test POST request (default method) with URL from pipeline
	result := runGojqQuery(t, fmt.Sprintf(`"%s" | http`, server.URL), nil, RegisterHTTP())

	resultMap, ok := result.(map[string]any)
	if !ok {
		t.Fatalf("Expected response object, got %T", result)
	}

	meta := resultMap
	if meta["Method"] != "POST" {
		t.Errorf("Expected method POST (default), got %v", meta["Method"])
	}
}

func TestHTTPPostWithBody(t *testing.T) {
	// Create a test HTTP server
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != "POST" {
			t.Errorf("Expected POST, got %s", r.Method)
		}
		// Read request body
		body := make([]byte, r.ContentLength)
		_, _ = r.Body.Read(body)
		w.WriteHeader(http.StatusOK)
		_, _ = fmt.Fprintf(w, "Received: %s", string(body))
	}))
	defer server.Close()

	// Test POST request with body from pipeline
	result := runGojqQuery(t, fmt.Sprintf(`"test body" | http("POST"; "%s")`, server.URL), nil, RegisterHTTP())

	resultMap, ok := result.(map[string]any)
	if !ok {
		t.Fatalf("Expected response object, got %T", result)
	}

	val := resultMap["Content"].(string)
	if val != "Received: test body" {
		t.Errorf("Expected 'Received: test body', got %q", val)
	}

	meta := resultMap
	if meta["Method"] != "POST" {
		t.Errorf("Expected method POST, got %v", meta["Method"])
	}
	if meta["RequestBody"] != "test body" {
		t.Errorf("Expected requestBody 'test body', got %v", meta["RequestBody"])
	}
}

func TestHTTPPostWithJSONBody(t *testing.T) {
	// Create a test HTTP server
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != "POST" {
			t.Errorf("Expected POST, got %s", r.Method)
		}
		if r.Header.Get("Content-Type") != "application/json" {
			t.Errorf("Expected Content-Type application/json, got %s", r.Header.Get("Content-Type"))
		}
		// Read request body
		body := make([]byte, r.ContentLength)
		_, _ = r.Body.Read(body)
		w.WriteHeader(http.StatusOK)
		_, _ = fmt.Fprintf(w, "Received: %s", string(body))
	}))
	defer server.Close()

	// Test POST request with JSON body from pipeline
	testJSON := map[string]any{"key": "value", "number": float64(42)}
	result := runGojqQuery(t, fmt.Sprintf(`http("POST"; "%s")`, server.URL), testJSON, RegisterHTTP())

	resultMap, ok := result.(map[string]any)
	if !ok {
		t.Fatalf("Expected response object, got %T", result)
	}

	val := resultMap["Content"].(string)
	// Response should contain the JSON string
	if len(val) == 0 {
		t.Errorf("Expected non-empty response, got empty")
	}

	meta := resultMap
	requestBody := meta["RequestBody"].(string)
	var parsedBody map[string]any
	if err := json.Unmarshal([]byte(requestBody), &parsedBody); err != nil {
		t.Errorf("Failed to parse request body as JSON: %v", err)
	}
	if parsedBody["key"] != "value" {
		t.Errorf("Expected key 'value', got %v", parsedBody["key"])
	}
}

func TestHTTPErrorNoURL(t *testing.T) {
	// Test error when URL is not provided (null input)
	qErr := runGojqQueryErr(t, `. | http`, nil, RegisterHTTP())
	if !strings.Contains(qErr.Error(), "URL") {
		t.Errorf("error should say a URL is required, got %q", qErr)
	}
}

func TestHTTPErrorInvalidMethod(t *testing.T) {
	qErr := runGojqQueryErr(t, `http("INVALID"; "https://example.com")`, nil, RegisterHTTP())
	if !strings.Contains(qErr.Error(), "invalid method") {
		t.Errorf("error should reject the method, got %q", qErr)
	}
}

func TestHTTPWithURLFromArg(t *testing.T) {
	// Create a test HTTP server
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("Success"))
	}))
	defer server.Close()

	// Test with URL as single argument (default POST)
	result := runGojqQuery(t, fmt.Sprintf(`http("%s")`, server.URL), nil, RegisterHTTP())

	resultMap, ok := result.(map[string]any)
	if !ok {
		t.Fatalf("Expected response object, got %T", result)
	}

	meta := resultMap
	if meta["Method"] != "POST" {
		t.Errorf("Expected method POST (default), got %v", meta["Method"])
	}
	if meta["Url"] != server.URL {
		t.Errorf("Expected URL %s, got %v", server.URL, meta["Url"])
	}
}

func TestHTTPChaining(t *testing.T) {
	// Create a test HTTP server
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("test response"))
	}))
	defer server.Close()

	// Chaining yields the response object; the body is on .Content
	result := runGojqQuery(t, fmt.Sprintf(`"%s" | http | .Content`, server.URL), nil, RegisterHTTP())

	if resultStr, ok := result.(string); !ok || resultStr != "test response" {
		t.Errorf("Expected 'test response', got %v", result)
	}
}

func TestHTTPResponseMetadata(t *testing.T) {
	// Create a test HTTP server
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("X-Custom-Header", "test-value")
		w.WriteHeader(http.StatusCreated)
		_, _ = w.Write([]byte("Created"))
	}))
	defer server.Close()

	result := runGojqQuery(t, fmt.Sprintf(`http("POST"; "%s")`, server.URL), nil, RegisterHTTP())

	resultMap, ok := result.(map[string]any)
	if !ok {
		t.Fatalf("Expected response object, got %T", result)
	}

	meta := resultMap
	status, ok := meta["StatusCode"].(int)
	if !ok {
		statusFloat, ok := meta["StatusCode"].(float64)
		if !ok {
			t.Errorf("Expected status to be int or float64, got %T", meta["StatusCode"])
		} else {
			status = int(statusFloat)
		}
	}
	if status != 201 {
		t.Errorf("Expected status 201, got %v", status)
	}

	headers := meta["Headers"].(map[string]any)
	if headers["X-Custom-Header"] != "test-value" {
		t.Errorf("Expected X-Custom-Header 'test-value', got %v", headers["X-Custom-Header"])
	}
}

func TestHTTPDifferentMethods(t *testing.T) {
	methods := []string{"GET", "POST", "PUT", "PATCH", "DELETE", "HEAD", "OPTIONS"}

	for _, method := range methods {
		t.Run(method, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.Method != method {
					t.Errorf("Expected %s, got %s", method, r.Method)
				}
				w.WriteHeader(http.StatusOK)
				_, _ = w.Write([]byte("OK"))
			}))
			defer server.Close()

			result := runGojqQuery(t, fmt.Sprintf(`http("%s"; "%s")`, method, server.URL), nil, RegisterHTTP())

			resultMap, ok := result.(map[string]any)
			if !ok {
				t.Fatalf("Expected response object, got %T", result)
			}

			meta := resultMap
			if meta["Method"] != method {
				t.Errorf("Expected method %s, got %v", method, meta["Method"])
			}
		})
	}
}

func TestHTTPServe(t *testing.T) {
	// Test starting a server with GET request
	// Run query in goroutine since it blocks
	resultChan := make(chan any, 1)
	go func() {
		result := runGojqQuery(t, `http_serve("127.0.0.1"; 0)`, nil, RegisterHTTPServe())
		resultChan <- result
	}()

	// Give server time to start
	time.Sleep(200 * time.Millisecond)

	// Make GET request to trigger return
	// We need to know the port, so let's use a fixed port for this test
	testPort := 18082
	resultChan2 := make(chan any, 1)
	go func() {
		result := runGojqQuery(t, fmt.Sprintf(`http_serve("127.0.0.1"; %d)`, testPort), "test-item", RegisterHTTPServe())
		resultChan2 <- result
	}()

	time.Sleep(200 * time.Millisecond)

	// Make GET request
	client := &http.Client{Timeout: 2 * time.Second}
	resp, err := client.Get(fmt.Sprintf("http://127.0.0.1:%d/", testPort))
	if err != nil {
		t.Fatalf("Failed to make request: %v", err)
	}
	defer func() { _ = resp.Body.Close() }()

	// Wait for result
	select {
	case result := <-resultChan2:
		// http_serve hands back the served value itself
		if result != "test-item" {
			t.Errorf("Expected 'test-item', got %v", result)
		}

	case <-time.After(5 * time.Second):
		t.Fatal("Timeout waiting for result")
	}
}

func TestHTTPServeWithRequest(t *testing.T) {
	// Test GET request - server blocks until GET, then returns the input item
	testInput := map[string]any{"test": "value", "number": float64(42)}
	testPort := 18083

	resultChan := make(chan any, 1)
	go func() {
		result := runGojqQuery(t, fmt.Sprintf(`http_serve("127.0.0.1"; %d)`, testPort), testInput, RegisterHTTPServe())
		resultChan <- result
	}()

	// Give server time to start
	time.Sleep(200 * time.Millisecond)

	// Make GET request - this should return the input item and unblock the query
	client := &http.Client{Timeout: 2 * time.Second}
	resp, err := client.Get(fmt.Sprintf("http://127.0.0.1:%d/", testPort))
	if err != nil {
		t.Fatalf("Failed to make request: %v", err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		t.Errorf("Expected status 200, got %d", resp.StatusCode)
	}

	// Parse response - should be the input item
	var response map[string]any
	if err := json.NewDecoder(resp.Body).Decode(&response); err != nil {
		t.Fatalf("Failed to decode response: %v", err)
	}

	if response["test"] != "value" {
		t.Errorf("Expected test='value', got %v", response["test"])
	}

	// Wait for query result
	select {
	case result := <-resultChan:
		// The result should be the input item
		val, ok := result.(map[string]any)
		if !ok {
			t.Fatalf("Expected the served object, got %T", result)
		}
		if val["test"] != "value" {
			t.Errorf("Expected result to contain test='value', got %v", val)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("Timeout waiting for query result")
	}
}

func TestHTTPServeWithPOSTBody(t *testing.T) {
	// Test POST request - server blocks until POST, then returns POST data
	// Use a high port number to avoid conflicts with common services
	testPort := 18084
	resultChan := make(chan any, 1)
	go func() {
		result := runGojqQuery(t, fmt.Sprintf(`http_serve("127.0.0.1"; %d)`, testPort), nil, RegisterHTTPServe())
		resultChan <- result
	}()

	// Give server time to start
	time.Sleep(200 * time.Millisecond)

	// Make POST request with JSON body - this should unblock and return POST data
	client := &http.Client{Timeout: 2 * time.Second}
	body := bytes.NewBufferString(`{"test": "value", "number": 42}`)
	resp, err := client.Post(fmt.Sprintf("http://127.0.0.1:%d/", testPort), "application/json", body)
	if err != nil {
		t.Fatalf("Failed to make request: %v", err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		t.Errorf("Expected status 200, got %d", resp.StatusCode)
	}

	// Parse response
	var response map[string]any
	if err := json.NewDecoder(resp.Body).Decode(&response); err != nil {
		t.Fatalf("Failed to decode response: %v", err)
	}

	if response["status"] != "accepted" {
		t.Errorf("Expected status 'accepted', got %v", response["status"])
	}

	// Wait for query result - should be the POST data
	select {
	case result := <-resultChan:
		// The result should be the POST data
		val, ok := result.(map[string]any)
		if !ok {
			t.Fatalf("Expected the posted object, got %T", result)
		}
		if val["test"] != "value" {
			t.Errorf("Expected result to contain test='value', got %v", val)
		}
		if val["number"].(float64) != 42 {
			t.Errorf("Expected number=42, got %v", val["number"])
		}
	case <-time.After(5 * time.Second):
		t.Fatal("Timeout waiting for query result")
	}
}

func TestHTTPServeErrorInvalidPort(t *testing.T) {
	qErr := runGojqQueryErr(t, `http_serve("127.0.0.1"; 70000)`, nil, RegisterHTTPServe())
	if qErr == nil {
		t.Error("expected http_serve to reject the argument")
	}
}

func TestHTTPServeErrorInvalidHost(t *testing.T) {
	qErr := runGojqQueryErr(t, `http_serve(123; 8080)`, nil, RegisterHTTPServe())
	if qErr == nil {
		t.Error("expected http_serve to reject the argument")
	}
}
