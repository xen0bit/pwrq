package http

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/itchyny/gojq"
	"github.com/xen0bit/pwrq/pkg/udf/common"
)

// RegisterHTTP registers the http function with gojq
func RegisterHTTP() gojq.CompilerOption {
	return gojq.WithFunction("http", 0, 2, func(v any, args []any) any {
		var method string = "POST" // default method
		var url string

		// Parse arguments
		if len(args) == 0 {
			// No arguments: URL from pipeline, method = POST
			inputVal := common.ExtractUDFValue(v)
			if urlStr, ok := inputVal.(string); ok {
				url = urlStr
			} else {
				return common.MakeUDFErrorResult(fmt.Errorf("http: URL must be provided as argument or from pipeline, got %T", inputVal), nil)
			}
		} else if len(args) == 1 {
			// One argument: could be method or URL
			// If it's a string, treat it as URL (method = POST)
			// If it's a method name, we'd need URL from pipeline
			argVal := common.ExtractUDFValue(args[0])
			if urlStr, ok := argVal.(string); ok {
				url = urlStr
				// Method stays as default POST
			} else {
				return common.MakeUDFErrorResult(fmt.Errorf("http: URL argument must be a string, got %T", argVal), nil)
			}
		} else if len(args) == 2 {
			// Two arguments: method, url
			methodVal := common.ExtractUDFValue(args[0])
			urlVal := common.ExtractUDFValue(args[1])

			if methodStr, ok := methodVal.(string); ok {
				method = strings.ToUpper(methodStr)
			} else {
				return common.MakeUDFErrorResult(fmt.Errorf("http: method argument must be a string, got %T", methodVal), nil)
			}

			if urlStr, ok := urlVal.(string); ok {
				url = urlStr
			} else {
				return common.MakeUDFErrorResult(fmt.Errorf("http: URL argument must be a string, got %T", urlVal), nil)
			}
		}

		// Validate URL is provided
		if url == "" {
			return common.MakeUDFErrorResult(fmt.Errorf("http: URL is required but was not provided"), nil)
		}

		// Validate method
		validMethods := map[string]bool{
			"GET": true, "POST": true, "PUT": true, "PATCH": true,
			"DELETE": true, "HEAD": true, "OPTIONS": true,
		}
		if !validMethods[method] {
			return common.MakeUDFErrorResult(fmt.Errorf("http: invalid method %q, must be one of: GET, POST, PUT, PATCH, DELETE, HEAD, OPTIONS", method), nil)
		}

		// Prepare request body from pipeline input
		var bodyReader io.Reader
		var bodyBytes []byte
		var bodyString string

		// Extract body from pipeline (v)
		// If we used args for method/url, v might still contain body
		// But if URL came from pipeline, v is the URL, not the body
		// So we need to check: if URL came from args, v might be body
		// If URL came from v, there's no body

		// Determine if we have a body
		hasBody := false
		if len(args) == 0 {
			// URL came from pipeline, no body
			hasBody = false
		} else if len(args) == 1 {
			// URL came from arg, v might be body
			bodyVal := common.ExtractUDFValue(v)
			if bodyVal != nil {
				hasBody = true
				// Convert body to string/bytes
				switch b := bodyVal.(type) {
				case string:
					bodyString = b
					bodyBytes = []byte(b)
				case []byte:
					bodyBytes = b
					bodyString = string(b)
				case map[string]any, []any:
					// JSON object or array - stringify it
					jsonBytes, err := json.Marshal(b)
					if err != nil {
						return common.MakeUDFErrorResult(fmt.Errorf("http: failed to marshal request body to JSON: %v", err), nil)
					}
					bodyBytes = jsonBytes
					bodyString = string(jsonBytes)
				default:
					// Try to convert to string
					bodyString = fmt.Sprintf("%v", b)
					bodyBytes = []byte(bodyString)
				}
			}
		} else if len(args) == 2 {
			// Method and URL from args, v is body
			bodyVal := common.ExtractUDFValue(v)
			if bodyVal != nil {
				hasBody = true
				// Convert body to string/bytes
				switch b := bodyVal.(type) {
				case string:
					bodyString = b
					bodyBytes = []byte(b)
				case []byte:
					bodyBytes = b
					bodyString = string(b)
				case map[string]any, []any:
					// JSON object or array - stringify it
					jsonBytes, err := json.Marshal(b)
					if err != nil {
						return common.MakeUDFErrorResult(fmt.Errorf("http: failed to marshal request body to JSON: %v", err), nil)
					}
					bodyBytes = jsonBytes
					bodyString = string(jsonBytes)
				default:
					// Try to convert to string
					bodyString = fmt.Sprintf("%v", b)
					bodyBytes = []byte(bodyString)
				}
			}
		}

		if hasBody {
			bodyReader = bytes.NewReader(bodyBytes)
		}

		// Create HTTP request
		req, err := http.NewRequest(method, url, bodyReader)
		if err != nil {
			return common.MakeUDFErrorResult(fmt.Errorf("http: failed to create request: %v", err), nil)
		}

		// Set Content-Type header if we have a body
		if hasBody {
			// Check if body looks like JSON
			if len(bodyBytes) > 0 {
				var testJSON any
				if json.Unmarshal(bodyBytes, &testJSON) == nil {
					req.Header.Set("Content-Type", "application/json")
				} else {
					req.Header.Set("Content-Type", "text/plain")
				}
			}
		}

		// Create HTTP client with timeout
		client := &http.Client{
			Timeout: 30 * time.Second,
		}

		// Make the request
		resp, err := client.Do(req)
		if err != nil {
			meta := map[string]any{
				"operation": "http",
				"method":    method,
				"url":       url,
			}
			return common.MakeUDFErrorResult(fmt.Errorf("http: request failed: %v", err), meta)
		}
		defer resp.Body.Close()

		// Read response body
		respBody, err := io.ReadAll(resp.Body)
		if err != nil {
			meta := map[string]any{
				"operation":  "http",
				"method":     method,
				"url":        url,
				"status":     resp.StatusCode,
				"statusText": resp.Status,
			}
			return common.MakeUDFErrorResult(fmt.Errorf("http: failed to read response body: %v", err), meta)
		}

		// Convert response headers to map
		headers := make(map[string]any)
		for key, values := range resp.Header {
			if len(values) == 1 {
				headers[key] = values[0]
			} else {
				headers[key] = values
			}
		}

		// Return response body as string
		responseBody := string(respBody)

		meta := map[string]any{
			"operation":  "http",
			"method":     method,
			"url":        url,
			"status":     resp.StatusCode,
			"statusText": resp.Status,
			"headers":    headers,
		}

		if hasBody {
			meta["requestBody"] = bodyString
			meta["requestBodySize"] = len(bodyBytes)
		}

		meta["responseBodySize"] = len(respBody)

		return common.MakeUDFSuccessResult(responseBody, meta)
	})
}

