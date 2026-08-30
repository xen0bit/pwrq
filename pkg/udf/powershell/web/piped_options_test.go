package web

import (
	"strings"
	"testing"

	"github.com/itchyny/gojq"
)

// A cmdlet documents an options object, and a caller who has one writes it
// down and pipes it in. That used to fail, and fail in the one way that sends
// the caller looking in the wrong place:
//
//	{Uri: "https://example.com"} | invoke_web_request
//	  -> invoke_web_request: Uri is required
//
// Uri is right there in the object. A message that contradicts what the caller
// can see makes them doubt the option table, which was correct, rather than the
// call form, which was not.
//
// These check the reading of the arguments, not the request: a piped object
// carrying no Uri at all must still be rejected, and one carrying a Uri must
// get far enough to be judged on the URL rather than on the call form.

func compile(t *testing.T, query string) *gojq.Code {
	t.Helper()
	parsed, err := gojq.Parse(query)
	if err != nil {
		t.Fatalf("parse %q: %v", query, err)
	}
	code, err := gojq.Compile(parsed, RegisterAll()...)
	if err != nil {
		t.Fatalf("compile %q: %v", query, err)
	}
	return code
}

// failure runs a query expected not to succeed and returns why.
func failure(t *testing.T, query string) string {
	t.Helper()
	v, ok := compile(t, query).Run(nil).Next()
	if !ok {
		t.Fatalf("%q produced no result", query)
	}
	err, isErr := v.(error)
	if !isErr {
		t.Fatalf("%q succeeded with %v, expected a failure", query, v)
	}
	return err.Error()
}

func TestPipedOptionsCarryTheUri(t *testing.T) {
	for _, name := range []string{"invoke_web_request", "invoke_rest_method"} {
		t.Run(name, func(t *testing.T) {
			// A scheme nothing can dial, so the run stops at the URL rather
			// than at the network - and stopping at the URL is the proof that
			// the Uri was read out of the piped object.
			query := `{Uri: "not-a-url", Method: "GET"} | ` + name
			got := failure(t, query)
			if strings.Contains(got, "Uri is required") {
				t.Errorf("%s ignored the Uri in the object it was piped: %s", name, got)
			}
			if !strings.Contains(got, "not-a-url") {
				t.Errorf("%s failed without naming the Uri it read: %s", name, got)
			}
		})
	}
}

func TestPipedOptionsCarryTheTarget(t *testing.T) {
	got := failure(t, `{Count: 1} | test_connection`)
	if !strings.Contains(got, "Target is required") {
		t.Errorf("test_connection accepted an options object with no Target: %s", got)
	}
	// The message has to say where the target may go, because the caller who
	// sees it has just tried one of the three places.
	if !strings.Contains(got, "down the pipe") {
		t.Errorf("test_connection said %q, which does not say where the Target goes", got)
	}
}

func TestAMissingUriStillSaysSo(t *testing.T) {
	got := failure(t, `{Method: "POST"} | invoke_web_request`)
	if !strings.Contains(got, "Uri is required") {
		t.Errorf("an options object with no Uri was not reported as such: %s", got)
	}
}

// TestAPipedStringIsStillTheUri pins the form that always worked, so that
// reading objects out of the pipe cannot cost it.
func TestAPipedStringIsStillTheUri(t *testing.T) {
	got := failure(t, `"not-a-url" | invoke_web_request`)
	if !strings.Contains(got, "not-a-url") {
		t.Errorf("a piped string is no longer read as the Uri: %s", got)
	}
}
