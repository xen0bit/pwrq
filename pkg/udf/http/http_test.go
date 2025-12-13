package http

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

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

func TestHTTPGet(t *testing.T) {
	// Create a test HTTP server
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != "GET" {
			t.Errorf("Expected GET, got %s", r.Method)
		}
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("Hello, World!"))
	}))
	defer server.Close()

	// Test GET request with URL as argument
	result := runGojqQuery(t, fmt.Sprintf(`http("GET"; "%s")`, server.URL), nil, RegisterHTTP())
	
	resultMap, ok := result.(map[string]any)
	if !ok {
		t.Fatalf("Expected map, got %T", result)
	}

	val := resultMap["_val"]
	if valStr, ok := val.(string); !ok || valStr != "Hello, World!" {
		t.Errorf("Expected response body 'Hello, World!', got %v", val)
	}

	meta := resultMap["_meta"].(map[string]any)
	if meta["method"] != "GET" {
		t.Errorf("Expected method GET, got %v", meta["method"])
	}
	status, ok := meta["status"].(int)
	if !ok {
		statusFloat, ok := meta["status"].(float64)
		if !ok {
			t.Errorf("Expected status to be int or float64, got %T", meta["status"])
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
		r.Body.Read(body)
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(fmt.Sprintf("Received: %s", string(body))))
	}))
	defer server.Close()

	// Test POST request (default method) with URL from pipeline
	result := runGojqQuery(t, fmt.Sprintf(`"%s" | http`, server.URL), nil, RegisterHTTP())
	
	resultMap, ok := result.(map[string]any)
	if !ok {
		t.Fatalf("Expected map, got %T", result)
	}

	meta := resultMap["_meta"].(map[string]any)
	if meta["method"] != "POST" {
		t.Errorf("Expected method POST (default), got %v", meta["method"])
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
		r.Body.Read(body)
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(fmt.Sprintf("Received: %s", string(body))))
	}))
	defer server.Close()

	// Test POST request with body from pipeline
	result := runGojqQuery(t, fmt.Sprintf(`"test body" | http("POST"; "%s")`, server.URL), nil, RegisterHTTP())
	
	resultMap, ok := result.(map[string]any)
	if !ok {
		t.Fatalf("Expected map, got %T", result)
	}

	val := resultMap["_val"].(string)
	if val != "Received: test body" {
		t.Errorf("Expected 'Received: test body', got %q", val)
	}

	meta := resultMap["_meta"].(map[string]any)
	if meta["method"] != "POST" {
		t.Errorf("Expected method POST, got %v", meta["method"])
	}
	if meta["requestBody"] != "test body" {
		t.Errorf("Expected requestBody 'test body', got %v", meta["requestBody"])
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
		r.Body.Read(body)
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(fmt.Sprintf("Received: %s", string(body))))
	}))
	defer server.Close()

	// Test POST request with JSON body from pipeline
	testJSON := map[string]any{"key": "value", "number": float64(42)}
	result := runGojqQuery(t, fmt.Sprintf(`http("POST"; "%s")`, server.URL), testJSON, RegisterHTTP())
	
	resultMap, ok := result.(map[string]any)
	if !ok {
		t.Fatalf("Expected map, got %T", result)
	}

	val := resultMap["_val"].(string)
	// Response should contain the JSON string
	if len(val) == 0 {
		t.Errorf("Expected non-empty response, got empty")
	}

	meta := resultMap["_meta"].(map[string]any)
	requestBody := meta["requestBody"].(string)
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
	result := runGojqQuery(t, `. | http`, nil, RegisterHTTP())
	
	resultMap, ok := result.(map[string]any)
	if !ok {
		t.Fatalf("Expected map, got %T", result)
	}

	if _, hasErr := resultMap["_err"]; !hasErr {
		t.Errorf("Expected error when URL is not provided")
	}

	val := resultMap["_val"]
	if val != nil {
		t.Errorf("Expected _val to be null on error, got %v", val)
	}
}

func TestHTTPErrorInvalidMethod(t *testing.T) {
	// Test error when method is invalid
	result := runGojqQuery(t, `http("INVALID"; "https://example.com")`, nil, RegisterHTTP())
	
	resultMap, ok := result.(map[string]any)
	if !ok {
		t.Fatalf("Expected map, got %T", result)
	}

	if _, hasErr := resultMap["_err"]; !hasErr {
		t.Errorf("Expected error when method is invalid")
	}
}

func TestHTTPWithURLFromArg(t *testing.T) {
	// Create a test HTTP server
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("Success"))
	}))
	defer server.Close()

	// Test with URL as single argument (default POST)
	result := runGojqQuery(t, fmt.Sprintf(`http("%s")`, server.URL), nil, RegisterHTTP())
	
	resultMap, ok := result.(map[string]any)
	if !ok {
		t.Fatalf("Expected map, got %T", result)
	}

	meta := resultMap["_meta"].(map[string]any)
	if meta["method"] != "POST" {
		t.Errorf("Expected method POST (default), got %v", meta["method"])
	}
	if meta["url"] != server.URL {
		t.Errorf("Expected URL %s, got %v", server.URL, meta["url"])
	}
}

func TestHTTPChaining(t *testing.T) {
	// Create a test HTTP server
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("test response"))
	}))
	defer server.Close()

	// Test chaining: URL from pipeline, then extract _val
	result := runGojqQuery(t, fmt.Sprintf(`"%s" | http | ._val`, server.URL), nil, RegisterHTTP())
	
	if resultStr, ok := result.(string); !ok || resultStr != "test response" {
		t.Errorf("Expected 'test response', got %v", result)
	}
}

func TestHTTPResponseMetadata(t *testing.T) {
	// Create a test HTTP server
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("X-Custom-Header", "test-value")
		w.WriteHeader(http.StatusCreated)
		w.Write([]byte("Created"))
	}))
	defer server.Close()

	result := runGojqQuery(t, fmt.Sprintf(`http("POST"; "%s")`, server.URL), nil, RegisterHTTP())
	
	resultMap, ok := result.(map[string]any)
	if !ok {
		t.Fatalf("Expected map, got %T", result)
	}

	meta := resultMap["_meta"].(map[string]any)
	status, ok := meta["status"].(int)
	if !ok {
		statusFloat, ok := meta["status"].(float64)
		if !ok {
			t.Errorf("Expected status to be int or float64, got %T", meta["status"])
		} else {
			status = int(statusFloat)
		}
	}
	if status != 201 {
		t.Errorf("Expected status 201, got %v", status)
	}

	headers := meta["headers"].(map[string]any)
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
				w.Write([]byte("OK"))
			}))
			defer server.Close()

			result := runGojqQuery(t, fmt.Sprintf(`http("%s"; "%s")`, method, server.URL), nil, RegisterHTTP())
			
			resultMap, ok := result.(map[string]any)
			if !ok {
				t.Fatalf("Expected map, got %T", result)
			}

			meta := resultMap["_meta"].(map[string]any)
			if meta["method"] != method {
				t.Errorf("Expected method %s, got %v", method, meta["method"])
			}
		})
	}
}

