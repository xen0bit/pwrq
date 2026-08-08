// Package web provides PowerShell-style web and network cmdlets.
// This file implements Invoke-WebRequest functionality.
package web

import (
	"bytes"
	"crypto/tls"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"strings"
	"time"

	"github.com/itchyny/gojq"
	"github.com/xen0bit/pwrq/pkg/core/sessionstate"
	"github.com/xen0bit/pwrq/pkg/udf/common"
)

// WebResponse holds the response from Invoke-WebRequest
type WebResponse struct {
	Content        string              `json:"Content"`
	StatusCode     int                 `json:"StatusCode"`
	Status         string              `json:"Status"`
	Headers        map[string][]string `json:"Headers"`
	BaseResponse   string              `json:"BaseResponse"`
	RequestMethod  string              `json:"RequestMethod"`
	RequestUri     *url.URL            `json:"RequestUri"`
	ContentLength  int64               `json:"ContentLength"`
	ContentType    string              `json:"ContentType"`
	LastModified   time.Time           `json:"LastModified"`
	ResponseUri    *url.URL            `json:"ResponseUri"`
}

// InvokeWebRequestOptions holds options for invoke_web_request
type InvokeWebRequestOptions struct {
	Uri             string
	Method          string
	Headers         map[string]string
	Body            any
	ContentType     string
	Timeout         int // seconds
	SkipSSLVerify   bool
	AllowAutoRedirect bool
	MaximumRedirection int
	OutFile         string
	PassThru        bool
}

// RegisterInvokeWebRequest registers the invoke_web_request function with gojq
// PowerShell compatibility: Invoke-WebRequest
// Usage:
//   - invoke_web_request("https://example.com")
//   - invoke_web_request({"Uri": "https://api.example.com"; "Method": "POST"; "Body": {"key": "value"}})
//   - invoke_web_request("https://example.com"; {"Headers": {"Authorization": "Bearer token"}})
func RegisterInvokeWebRequest() gojq.CompilerOption {
	return gojq.WithFunction("invoke_web_request", 0, 2, func(v any, args []any) any {
		opts := InvokeWebRequestOptions{
			Method:               "GET",
			Timeout:              30,
			AllowAutoRedirect:    true,
			MaximumRedirection:   5,
			SkipSSLVerify:        false,
		}

		// Parse arguments
		if len(args) > 0 {
			firstArg := common.BindValue(args[0])

			if uriStr, ok := firstArg.(string); ok {
				opts.Uri = uriStr
			} else if optsMap, ok := firstArg.(map[string]any); ok {
				parseInvokeWebRequestOptions(&opts, optsMap)
			}
		}

		// Second argument could be options map
		if len(args) > 1 {
			if secondArg := common.BindValue(args[1]); secondArg != nil {
				if optsMap, ok := secondArg.(map[string]any); ok {
					// Merge options (second arg takes precedence)
					parseInvokeWebRequestOptions(&opts, optsMap)
				}
			}
		}

		// If URI still empty, try to get from pipeline input
		if opts.Uri == "" {
			if pipelineVal := common.BindValue(v); pipelineVal != nil {
				if uriStr, ok := pipelineVal.(string); ok {
					opts.Uri = uriStr
				}
			}
		}

		// Validate URI
		if opts.Uri == "" {
			return common.MakeUDFErrorResult(fmt.Errorf("invoke_web_request: Uri is required"), nil)
		}

		// Validate and parse URI
		if _, err := url.ParseRequestURI(opts.Uri); err != nil {
			return common.MakeUDFErrorResult(fmt.Errorf("invoke_web_request: invalid URI %q: %w", opts.Uri, err), nil)
		}

		// Make the request
		response, err := invokeWebRequest(opts)
		if err != nil {
			// Update $? automatic variable
			ss := common.GetSessionState()
			if ss != nil {
				ss.SetVariable("?", false, sessionstate.None)
			}

			return common.MakeUDFErrorResult(err, map[string]any{
				"operation": "invoke_web_request",
				"uri":       opts.Uri,
				"method":    opts.Method,
			})
		}

		// Update $? automatic variable
		ss := common.GetSessionState()
		if ss != nil {
			ss.SetVariable("?", true, sessionstate.None)
		}

		// If OutFile is specified, write content to file
		if opts.OutFile != "" {
			if err := os.WriteFile(opts.OutFile, []byte(response.Content), 0644); err != nil {
				return common.MakeUDFErrorResult(fmt.Errorf("invoke_web_request: failed to write to %q: %w", opts.OutFile, err), nil)
			}

			if opts.PassThru {
				return common.MakeUDFSuccessResult(webResponseToMap(response), map[string]any{
					"operation": "invoke_web_request",
					"outfile":   opts.OutFile,
				})
			}

			return common.MakeUDFSuccessResult(map[string]any{
				"Path": opts.OutFile,
				"Size": len(response.Content),
			}, map[string]any{
				"operation": "invoke_web_request",
				"outfile":   opts.OutFile,
			})
		}

		return common.MakeUDFSuccessResult(webResponseToMap(response), map[string]any{
			"operation": "invoke_web_request",
		})
	})
}

// parseInvokeWebRequestOptions parses options from a map
func parseInvokeWebRequestOptions(opts *InvokeWebRequestOptions, optsMap map[string]any) {
	if uriVal, exists := optsMap["Uri"]; exists {
		if uStr, ok := uriVal.(string); ok {
			opts.Uri = uStr
		}
	}
	if methodVal, exists := optsMap["Method"]; exists {
		if mStr, ok := methodVal.(string); ok {
			opts.Method = strings.ToUpper(mStr)
		}
	}
	if headersVal, exists := optsMap["Headers"]; exists {
		if hMap, ok := headersVal.(map[string]any); ok {
			opts.Headers = make(map[string]string)
			for k, v := range hMap {
				if vStr, ok := v.(string); ok {
					opts.Headers[k] = vStr
				}
			}
		}
	}
	if bodyVal, exists := optsMap["Body"]; exists {
		opts.Body = bodyVal
	}
	if contentTypeVal, exists := optsMap["ContentType"]; exists {
		if ctStr, ok := contentTypeVal.(string); ok {
			opts.ContentType = ctStr
		}
	}
	if timeoutVal, exists := optsMap["TimeoutSec"]; exists {
		switch t := timeoutVal.(type) {
		case int:
			opts.Timeout = t
		case float64:
			opts.Timeout = int(t)
		}
	}
	if skipSSLVal, exists := optsMap["SkipSSLVerify"]; exists {
		if b, ok := skipSSLVal.(bool); ok {
			opts.SkipSSLVerify = b
		}
	}
	if redirectVal, exists := optsMap["AllowAutoRedirect"]; exists {
		if b, ok := redirectVal.(bool); ok {
			opts.AllowAutoRedirect = b
		}
	}
	if maxRedirectVal, exists := optsMap["MaximumRedirection"]; exists {
		switch m := maxRedirectVal.(type) {
		case int:
			opts.MaximumRedirection = m
		case float64:
			opts.MaximumRedirection = int(m)
		}
	}
	if outFileVal, exists := optsMap["OutFile"]; exists {
		if oStr, ok := outFileVal.(string); ok {
			opts.OutFile = oStr
		}
	}
	if passThruVal, exists := optsMap["PassThru"]; exists {
		if b, ok := passThruVal.(bool); ok {
			opts.PassThru = b
		}
	}
}

// invokeWebRequest performs the HTTP request
func invokeWebRequest(opts InvokeWebRequestOptions) (*WebResponse, error) {
	// Create request body
	var bodyReader io.Reader
	if opts.Body != nil {
		switch b := opts.Body.(type) {
		case string:
			bodyReader = strings.NewReader(b)
		case []byte:
			bodyReader = bytes.NewReader(b)
		case map[string]any, []any:
			jsonBytes, err := json.Marshal(b)
			if err != nil {
				return nil, fmt.Errorf("failed to marshal body to JSON: %w", err)
			}
			bodyReader = bytes.NewReader(jsonBytes)
		default:
			bodyStr := fmt.Sprintf("%v", b)
			bodyReader = strings.NewReader(bodyStr)
		}
	}

	// Create HTTP request
	req, err := http.NewRequest(opts.Method, opts.Uri, bodyReader)
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	// Set headers
	for key, value := range opts.Headers {
		req.Header.Set(key, value)
	}

	// Set Content-Type if specified or if body looks like JSON
	if opts.ContentType != "" {
		req.Header.Set("Content-Type", opts.ContentType)
	} else if opts.Body != nil {
		// Try to detect JSON body
		if _, ok := opts.Body.(map[string]any); ok {
			req.Header.Set("Content-Type", "application/json")
		} else if _, ok := opts.Body.([]any); ok {
			req.Header.Set("Content-Type", "application/json")
		}
	}

	// Create HTTP client
	client := &http.Client{
		Timeout: time.Duration(opts.Timeout) * time.Second,
		CheckRedirect: func(req *http.Request, via []*http.Request) error {
			if !opts.AllowAutoRedirect {
				return http.ErrUseLastResponse
			}
			if len(via) >= opts.MaximumRedirection {
				return fmt.Errorf("maximum redirections (%d) exceeded", opts.MaximumRedirection)
			}
			return nil
		},
	}

	// Configure TLS if SkipSSLVerify is set
	if opts.SkipSSLVerify {
		client.Transport = &http.Transport{
			TLSClientConfig: &tls.Config{InsecureSkipVerify: true},
		}
	}

	// Make the request
	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("request failed: %w", err)
	}
	defer resp.Body.Close()

	// Read response body
	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read response body: %w", err)
	}

	// Parse content type
	contentType := resp.Header.Get("Content-Type")

	// Parse last modified
	var lastModified time.Time
	if lm := resp.Header.Get("Last-Modified"); lm != "" {
		lastModified, _ = time.Parse(time.RFC1123, lm)
	}

	// Create response object
	response := &WebResponse{
		Content:       string(respBody),
		StatusCode:    resp.StatusCode,
		Status:        resp.Status,
		Headers:       resp.Header,
		RequestMethod: resp.Request.Method,
		RequestUri:    resp.Request.URL,
		ContentLength: resp.ContentLength,
		ContentType:   contentType,
		LastModified:  lastModified,
		ResponseUri:   resp.Request.URL,
	}

	// Set BaseResponse based on content type
	if strings.Contains(contentType, "html") {
		response.BaseResponse = "HTML"
	} else if strings.Contains(contentType, "json") {
		response.BaseResponse = "JSON"
	} else if strings.Contains(contentType, "xml") {
		response.BaseResponse = "XML"
	} else if strings.Contains(contentType, "text") {
		response.BaseResponse = "Text"
	} else {
		response.BaseResponse = "Binary"
	}

	return response, nil
}

// RegisterInvokeRestMethod registers the invoke_rest_method function with gojq
// PowerShell compatibility: Invoke-RestMethod
// Similar to Invoke-WebRequest but automatically parses JSON/XML responses
func RegisterInvokeRestMethod() gojq.CompilerOption {
	return gojq.WithFunction("invoke_rest_method", 0, 2, func(v any, args []any) any {
		opts := InvokeWebRequestOptions{
			Method:               "GET",
			Timeout:              30,
			AllowAutoRedirect:    true,
			MaximumRedirection:   5,
			SkipSSLVerify:        false,
		}

		// Parse arguments (same as invoke_web_request)
		if len(args) > 0 {
			firstArg := common.BindValue(args[0])

			if uriStr, ok := firstArg.(string); ok {
				opts.Uri = uriStr
			} else if optsMap, ok := firstArg.(map[string]any); ok {
				parseInvokeWebRequestOptions(&opts, optsMap)
			}
		}

		if len(args) > 1 {
			if secondArg := common.BindValue(args[1]); secondArg != nil {
				if optsMap, ok := secondArg.(map[string]any); ok {
					parseInvokeWebRequestOptions(&opts, optsMap)
				}
			}
		}

		// If URI still empty, try to get from pipeline input
		if opts.Uri == "" {
			if pipelineVal := common.BindValue(v); pipelineVal != nil {
				if uriStr, ok := pipelineVal.(string); ok {
					opts.Uri = uriStr
				}
			}
		}

		// Validate URI
		if opts.Uri == "" {
			return common.MakeUDFErrorResult(fmt.Errorf("invoke_rest_method: Uri is required"), nil)
		}

		// Make the request
		response, err := invokeWebRequest(opts)
		if err != nil {
			ss := common.GetSessionState()
			if ss != nil {
				ss.SetVariable("?", false, sessionstate.None)
			}

			return common.MakeUDFErrorResult(err, map[string]any{
				"operation": "invoke_rest_method",
				"uri":       opts.Uri,
			})
		}

		ss := common.GetSessionState()
		if ss != nil {
			ss.SetVariable("?", true, sessionstate.None)
		}

		// Parse response based on content type
		var parsedContent any

		if strings.Contains(response.ContentType, "application/json") ||
			strings.HasSuffix(opts.Uri, ".json") {
			// Parse as JSON
			if err := json.Unmarshal([]byte(response.Content), &parsedContent); err != nil {
				return common.MakeUDFErrorResult(fmt.Errorf("invoke_rest_method: failed to parse JSON response: %w", err), nil)
			}
		} else if strings.Contains(response.ContentType, "application/xml") ||
			strings.Contains(response.ContentType, "text/xml") {
			// For XML, return as string (XML parsing would require additional library)
			parsedContent = response.Content
		} else {
			// Return as string
			parsedContent = response.Content
		}

		return common.MakeUDFSuccessResult(parsedContent, map[string]any{
			"operation":    "invoke_rest_method",
			"statusCode":   response.StatusCode,
			"contentType":  response.ContentType,
		})
	})
}

// webResponseToMap converts a WebResponse to a map for JSON encoding
func webResponseToMap(r *WebResponse) map[string]any {
	headersMap := make(map[string]any)
	for k, v := range r.Headers {
		if len(v) == 1 {
			headersMap[k] = v[0]
		} else {
			headersMap[k] = v
		}
	}
	
	// Convert int64 to int for encoder compatibility
	contentLength := int(r.ContentLength)
	
	return map[string]any{
		"Content":        r.Content,
		"StatusCode":     r.StatusCode,
		"Status":         r.Status,
		"Headers":        headersMap,
		"BaseResponse":   r.BaseResponse,
		"RequestMethod":  r.RequestMethod,
		"RequestUri":     r.RequestUri.String(),
		"ContentLength":  contentLength,
		"ContentType":    r.ContentType,
		"LastModified":   r.LastModified.Format(time.RFC3339),
		"ResponseUri":    r.ResponseUri.String(),
	}
}
