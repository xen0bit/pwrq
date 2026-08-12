package censys

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"

	"github.com/itchyny/gojq"
	"github.com/xen0bit/pwrq/pkg/core/psobject"
)

// request records what a handler was asked for, so a test can assert on the
// query string and body the SDK actually produced rather than only on what
// came back.
type request struct {
	Method string
	Path   string
	Query  url.Values
	Body   string
}

// server stands in for the Censys Platform.
//
// Every cmdlet reaches the API through the SDK, so a fake HTTP server is the
// only place a test can see the whole path: parameter binding, URL building,
// authentication, decoding and the shape that reaches the pipeline.
type server struct {
	t        *testing.T
	mux      map[string]http.HandlerFunc
	requests []request
}

func newServer(t *testing.T) *server {
	t.Helper()
	s := &server{t: t, mux: map[string]http.HandlerFunc{}}

	httpSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		// The body is recorded and then put back, so a handler can still read
		// it to decide what to answer.
		r.Body = io.NopCloser(bytes.NewReader(body))
		s.requests = append(s.requests, request{
			Method: r.Method,
			Path:   r.URL.Path,
			Query:  r.URL.Query(),
			Body:   string(body),
		})
		// The API answers success with application/json unless an asset
		// endpoint overrides it, so that is the default here too.
		w.Header().Set("Content-Type", "application/json")
		handler, ok := s.mux[r.Method+" "+r.URL.Path]
		if !ok {
			handler, ok = s.mux[r.URL.Path]
		}
		if !ok {
			s.t.Errorf("unexpected request %s %s", r.Method, r.URL.Path)
			w.WriteHeader(http.StatusNotFound)
			return
		}
		handler(w, r)
	}))
	t.Cleanup(httpSrv.Close)

	// The cmdlets read their credentials from the environment, so pointing the
	// environment at the fake server is also the test for that resolution.
	t.Setenv(EnvToken, "test-token")
	t.Setenv(EnvOrgID, "test-org")
	t.Setenv(EnvServer, httpSrv.URL)
	return s
}

// handle registers a canned JSON response for a path.
func (s *server) handle(path, body string) {
	s.mux[path] = func(w http.ResponseWriter, _ *http.Request) {
		_, _ = io.WriteString(w, body)
	}
}

// handleAsset is handle for the Global Data asset endpoints, which answer with
// a versioned vendor media type. The SDK dispatches on it, so a test that sent
// plain application/json would exercise the wrong branch.
func (s *server) handleAsset(path, contentType, body string) {
	s.mux[path] = func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", contentType)
		_, _ = io.WriteString(w, body)
	}
}

// handleCreated is handle for the endpoints that answer 201 rather than 200.
func (s *server) handleCreated(path, body string) {
	s.mux[path] = func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusCreated)
		_, _ = io.WriteString(w, body)
	}
}

// handleFunc registers a handler that can vary its answer per request.
func (s *server) handleFunc(path string, fn http.HandlerFunc) {
	s.mux[path] = fn
}

// fail registers a problem+json error response, the API's failure shape.
func (s *server) fail(path string, status int, body string) {
	s.mux[path] = func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/problem+json")
		w.WriteHeader(status)
		_, _ = io.WriteString(w, body)
	}
}

// run evaluates a query against the Censys cmdlets and collects every output.
func run(t *testing.T, query string, input any) ([]any, error) {
	t.Helper()
	parsed, err := gojq.Parse(query)
	if err != nil {
		t.Fatalf("parse %q: %v", query, err)
	}
	code, err := gojq.Compile(parsed, RegisterAll()...)
	if err != nil {
		t.Fatalf("compile %q: %v", query, err)
	}

	var out []any
	iter := code.Run(input)
	for {
		value, ok := iter.Next()
		if !ok {
			return out, nil
		}
		if err, isErr := value.(error); isErr {
			return out, err
		}
		out = append(out, value)
	}
}

// runOne evaluates a query that must produce exactly one value.
func runOne(t *testing.T, query string, input any) any {
	t.Helper()
	out, err := run(t, query, input)
	if err != nil {
		t.Fatalf("%s: %v", query, err)
	}
	if len(out) != 1 {
		t.Fatalf("%s: expected one output, got %d", query, len(out))
	}
	return out[0]
}

// runErr evaluates a query that must fail, returning the message.
func runErr(t *testing.T, query string, input any) string {
	t.Helper()
	_, err := run(t, query, input)
	if err == nil {
		t.Fatalf("%s: expected an error", query)
	}
	return err.Error()
}

func obj(t *testing.T, v any) map[string]any {
	t.Helper()
	m, ok := v.(map[string]any)
	if !ok {
		t.Fatalf("expected an object, got %T", v)
	}
	return m
}

// hostType is the media type the host endpoint answers with.
const hostType = "application/vnd.censys.api.v3.host.v1+json"

const hostBody = `{"result":{"resource":{"ip":"1.1.1.1","service_count":2,
	"location":{"country":"Australia"}}}}`

func TestGetHostReturnsTheAssetWithATypeName(t *testing.T) {
	srv := newServer(t)
	srv.handleAsset("/v3/global/asset/host/1.1.1.1", hostType, hostBody)

	host := obj(t, runOne(t, `get_censys_host("1.1.1.1")`, nil))
	if host[psobject.PSTypeNameKey] != "Censys.Platform.Host" {
		t.Errorf("PSTypeName = %v", host[psobject.PSTypeNameKey])
	}
	resource := obj(t, host["resource"])
	if resource["ip"] != "1.1.1.1" {
		t.Errorf("resource.ip = %v", resource["ip"])
	}

	// The API's own field names survive: a caller who knows CenQL can reach
	// straight for them without learning a second spelling.
	if _, ok := resource["service_count"]; !ok {
		t.Errorf("service_count was renamed or dropped: %v", resource)
	}
}

// TestCallingFormsAgree is the binding rule the rest of pwrq is held to: the
// piped form and the argument form must be the same call.
func TestCallingFormsAgree(t *testing.T) {
	srv := newServer(t)
	srv.handleAsset("/v3/global/asset/host/1.1.1.1", hostType, hostBody)

	piped := runOne(t, `"1.1.1.1" | get_censys_host`, nil)
	explicit := runOne(t, `get_censys_host("1.1.1.1")`, nil)
	if diff := jsonOf(t, piped) != jsonOf(t, explicit); diff {
		t.Errorf("piped %s differs from explicit %s", jsonOf(t, piped), jsonOf(t, explicit))
	}

	// An options object as the only argument leaves the identifier to the
	// pipeline, which is the third form the cmdlets accept.
	withOpts := runOne(t, `"1.1.1.1" | get_censys_host({Timeout: 5})`, nil)
	if jsonOf(t, withOpts) != jsonOf(t, explicit) {
		t.Errorf("piped-with-options %s differs from explicit %s", jsonOf(t, withOpts), jsonOf(t, explicit))
	}
}

func jsonOf(t *testing.T, v any) string {
	t.Helper()
	encoded, err := json.Marshal(v)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	return string(encoded)
}

func TestAtTimeIsSentAsAQueryParameter(t *testing.T) {
	srv := newServer(t)
	srv.handleAsset("/v3/global/asset/host/1.1.1.1", hostType, hostBody)

	runOne(t, `get_censys_host("1.1.1.1"; {AtTime: "2026-01-02T03:04:05Z"})`, nil)
	if got := srv.requests[0].Query.Get("at_time"); !strings.HasPrefix(got, "2026-01-02T03:04:05") {
		t.Errorf("at_time = %q", got)
	}
}

func TestAtTimeMustBeRFC3339(t *testing.T) {
	newServer(t)
	msg := runErr(t, `get_censys_host("1.1.1.1"; {AtTime: "2026-01-02"})`, nil)
	if !strings.Contains(msg, "RFC3339") {
		t.Errorf("error should explain the format, got %q", msg)
	}
}

// TestUnknownOptionIsRejected covers the failure an API cmdlet must not have:
// a misspelled option that is silently ignored, leaving the caller believing
// they filtered something.
func TestUnknownOptionIsRejected(t *testing.T) {
	newServer(t)
	msg := runErr(t, `search_censys("host.services.port=22"; {PageSze: 5})`, nil)
	if !strings.Contains(msg, "unknown option") || !strings.Contains(msg, "PageSze") {
		t.Errorf("error should name the unknown option, got %q", msg)
	}
	// The suggestion keeps the declared spelling, so it can be pasted back in.
	if !strings.Contains(msg, "PageSize") {
		t.Errorf("error should suggest the real option names, got %q", msg)
	}
}

func TestOptionsBindCaseInsensitively(t *testing.T) {
	srv := newServer(t)
	srv.handleAsset("/v3/global/asset/host/1.1.1.1", hostType, hostBody)

	runOne(t, `get_censys_host("1.1.1.1"; {attime: "2026-01-02T03:04:05Z"})`, nil)
	if got := srv.requests[0].Query.Get("at_time"); got == "" {
		t.Error("a lowercased parameter name did not bind")
	}
}

func TestMissingTokenIsReportedBeforeTheRequest(t *testing.T) {
	newServer(t)
	t.Setenv(EnvToken, "")

	msg := runErr(t, `get_censys_host("1.1.1.1")`, nil)
	if !strings.Contains(msg, EnvToken) {
		t.Errorf("error should name the environment variable, got %q", msg)
	}
}

func TestOrganizationIsSentOnDataRequests(t *testing.T) {
	srv := newServer(t)
	srv.handleAsset("/v3/global/asset/host/1.1.1.1", hostType, hostBody)

	runOne(t, `get_censys_host("1.1.1.1")`, nil)
	if got := srv.requests[0].Query.Get("organization_id"); got != "test-org" {
		t.Errorf("organization_id = %q", got)
	}

	// An explicit option beats the environment, which is what lets one query
	// reach two organizations.
	srv.requests = nil
	runOne(t, `get_censys_host("1.1.1.1"; {OrganizationId: "other"})`, nil)
	if got := srv.requests[0].Query.Get("organization_id"); got != "other" {
		t.Errorf("organization_id = %q", got)
	}
}

func TestBearerTokenIsSent(t *testing.T) {
	srv := newServer(t)
	var auth string
	srv.handleFunc("/v3/global/asset/host/1.1.1.1", func(w http.ResponseWriter, r *http.Request) {
		auth = r.Header.Get("Authorization")
		w.Header().Set("Content-Type", hostType)
		_, _ = io.WriteString(w, hostBody)
	})

	runOne(t, `get_censys_host("1.1.1.1")`, nil)
	if auth != "Bearer test-token" {
		t.Errorf("Authorization = %q", auth)
	}
}

func TestAPIErrorCarriesStatusAndTitle(t *testing.T) {
	srv := newServer(t)
	srv.fail("/v3/global/asset/host/9.9.9.9", http.StatusNotFound,
		`{"status":404,"title":"Not Found","detail":"no such host"}`)

	msg := runErr(t, `get_censys_host("9.9.9.9")`, nil)
	for _, want := range []string{"get_censys_host", "404", "Not Found", "no such host"} {
		if !strings.Contains(msg, want) {
			t.Errorf("error %q is missing %q", msg, want)
		}
	}
}

// TestErrorsAreJqErrors pins the package to pwrq's error contract: a failed
// cmdlet raises, so try/catch and the exit status behave as they do for jq.
func TestErrorsAreJqErrors(t *testing.T) {
	srv := newServer(t)
	srv.fail("/v3/global/asset/host/9.9.9.9", http.StatusNotFound, `{"status":404,"title":"Not Found"}`)

	if got := runOne(t, `try get_censys_host("9.9.9.9") catch "missing"`, nil); got != "missing" {
		t.Errorf("catch produced %v", got)
	}
}

func TestSearchEmitsOneObjectPerHit(t *testing.T) {
	srv := newServer(t)
	srv.handle("/v3/global/search/query", `{"result":{"hits":[
		{"host_v1":{"resource":{"ip":"1.1.1.1"}}},
		{"host_v1":{"resource":{"ip":"2.2.2.2"}}}],"total_hits":2}}`)

	hits, err := run(t, `[search_censys("host.services.port=22") | .host_v1.resource.ip]`, nil)
	if err != nil {
		t.Fatal(err)
	}
	got := jsonOf(t, hits[0])
	if got != `["1.1.1.1","2.2.2.2"]` {
		t.Errorf("hits = %s", got)
	}

	var body map[string]any
	if err := json.Unmarshal([]byte(srv.requests[0].Body), &body); err != nil {
		t.Fatalf("request body: %v", err)
	}
	if body["query"] != "host.services.port=22" {
		t.Errorf("query = %v", body["query"])
	}
}

// TestSearchStopsAtOnePageByDefault guards the credit bill: these calls cost
// money, so following every cursor must be something the caller asked for.
func TestSearchStopsAtOnePageByDefault(t *testing.T) {
	srv := newServer(t)
	pages := 0
	srv.handleFunc("/v3/global/search/query", func(w http.ResponseWriter, _ *http.Request) {
		pages++
		_, _ = io.WriteString(w, `{"result":{"hits":[{"host_v1":{"resource":{"ip":"1.1.1.1"}}}],"next_page_token":"more"}}`)
	})

	out, err := run(t, `[search_censys("x")] | length`, nil)
	if err != nil {
		t.Fatal(err)
	}
	if pages != 1 {
		t.Errorf("made %d requests, want 1", pages)
	}
	if out[0] != 1 {
		t.Errorf("emitted %v hits, want 1", out[0])
	}
}

func TestSearchFollowsTheCursorWhenAsked(t *testing.T) {
	srv := newServer(t)
	tokens := []string{}
	srv.handleFunc("/v3/global/search/query", func(w http.ResponseWriter, r *http.Request) {
		var body map[string]any
		raw, _ := io.ReadAll(r.Body)
		_ = json.Unmarshal(raw, &body)
		token, _ := body["page_token"].(string)
		tokens = append(tokens, token)

		switch token {
		case "":
			_, _ = io.WriteString(w, `{"result":{"hits":[{"host_v1":{"resource":{"ip":"1.1.1.1"}}}],"next_page_token":"p2"}}`)
		case "p2":
			_, _ = io.WriteString(w, `{"result":{"hits":[{"host_v1":{"resource":{"ip":"2.2.2.2"}}}],"next_page_token":""}}`)
		default:
			t.Errorf("unexpected page token %q", token)
		}
	})

	out, err := run(t, `[search_censys("x"; {Pages: 0}) | .host_v1.resource.ip]`, nil)
	if err != nil {
		t.Fatal(err)
	}
	if got := jsonOf(t, out[0]); got != `["1.1.1.1","2.2.2.2"]` {
		t.Errorf("hits = %s", got)
	}
	if len(tokens) != 2 || tokens[1] != "p2" {
		t.Errorf("page tokens = %v", tokens)
	}
}

// TestPagingStopsOnARepeatedCursor keeps a misbehaving server from turning a
// query into an infinite loop.
func TestPagingStopsOnARepeatedCursor(t *testing.T) {
	srv := newServer(t)
	calls := 0
	srv.handleFunc("/v3/global/search/query", func(w http.ResponseWriter, _ *http.Request) {
		calls++
		if calls > 10 {
			t.Fatal("the cursor never terminated")
		}
		_, _ = io.WriteString(w, `{"result":{"hits":[],"next_page_token":"stuck"}}`)
	})

	if _, err := run(t, `[search_censys("x"; {Pages: 0, PageToken: "stuck"})]`, nil); err != nil {
		t.Fatal(err)
	}
	if calls != 1 {
		t.Errorf("made %d requests, want 1", calls)
	}
}

func TestSearchWithinACollectionUsesTheCollectionEndpoint(t *testing.T) {
	srv := newServer(t)
	srv.handle("/v3/collections/abc/search/query", `{"result":{"hits":[]}}`)

	if _, err := run(t, `[search_censys("x"; {CollectionId: "abc"})]`, nil); err != nil {
		t.Fatal(err)
	}
	if srv.requests[0].Path != "/v3/collections/abc/search/query" {
		t.Errorf("path = %s", srv.requests[0].Path)
	}
}

func TestAggregateTakesQueryAndFieldInEitherForm(t *testing.T) {
	srv := newServer(t)
	srv.handle("/v3/global/search/aggregate",
		`{"result":{"buckets":[{"key":"443","count":7}],"total_count":7}}`)

	explicit := runOne(t, `get_censys_aggregate("x"; "host.services.port")`, nil)
	piped := runOne(t, `"x" | get_censys_aggregate("host.services.port")`, nil)
	if jsonOf(t, explicit) != jsonOf(t, piped) {
		t.Errorf("explicit %s differs from piped %s", jsonOf(t, explicit), jsonOf(t, piped))
	}

	agg := obj(t, explicit)
	if agg[psobject.PSTypeNameKey] != "Censys.Platform.Aggregate" {
		t.Errorf("PSTypeName = %v", agg[psobject.PSTypeNameKey])
	}

	var body map[string]any
	if err := json.Unmarshal([]byte(srv.requests[0].Body), &body); err != nil {
		t.Fatalf("request body: %v", err)
	}
	if body["field"] != "host.services.port" {
		t.Errorf("field = %v", body["field"])
	}
	if body["number_of_buckets"] != float64(50) {
		t.Errorf("number_of_buckets = %v, want the documented default", body["number_of_buckets"])
	}
}

func TestTimelineDefaultsToAWindowEndingNow(t *testing.T) {
	srv := newServer(t)
	srv.handleAsset("/v3/global/asset/host/1.1.1.1/timeline",
		"application/vnd.censys.api.v3.host_timeline_event.v1+json",
		`{"result":{"events":[{"event_type":"service_added"},{"event_type":"service_removed"}]}}`)

	out, err := run(t, `[get_censys_host_timeline("1.1.1.1")] | length`, nil)
	if err != nil {
		t.Fatal(err)
	}
	if out[0] != 2 {
		t.Errorf("emitted %v events, want 2", out[0])
	}

	query := srv.requests[0].Query
	if query.Get("start_time") == "" || query.Get("end_time") == "" {
		t.Errorf("both ends of the window must be sent, got %v", query)
	}
	if query.Get("start_time") <= query.Get("end_time") {
		t.Errorf("start_time should be the recent end of the range, got start=%q end=%q",
			query.Get("start_time"), query.Get("end_time"))
	}
}

// TestTimelineRejectsAnInvertedWindow catches the mistake the API's parameter
// names invite, and says which end is which.
func TestTimelineRejectsAnInvertedWindow(t *testing.T) {
	newServer(t)

	msg := runErr(t, `get_censys_host_timeline("1.1.1.1";
		{StartTime: "2026-01-01T00:00:00Z", EndTime: "2026-02-01T00:00:00Z"})`, nil)
	if !strings.Contains(msg, "nearest to now") {
		t.Errorf("error should explain the ordering, got %q", msg)
	}
}

func TestListingCmdletsEmitOneObjectPerRecord(t *testing.T) {
	srv := newServer(t)
	srv.handle("/v3/tags", `{"result":{"tags":[
		{"id":"t1","name":"alpha"},{"id":"t2","name":"beta"}],"total_size":2}}`)

	out, err := run(t, `[get_censys_tag | .name]`, nil)
	if err != nil {
		t.Fatal(err)
	}
	if got := jsonOf(t, out[0]); got != `["alpha","beta"]` {
		t.Errorf("tags = %s", got)
	}
}

func TestGettingOneTagUsesTheSingleEndpoint(t *testing.T) {
	srv := newServer(t)
	srv.handle("/v3/tags/t1", `{"result":{"id":"t1","name":"alpha"}}`)

	tag := obj(t, runOne(t, `get_censys_tag("t1")`, nil))
	if tag["name"] != "alpha" {
		t.Errorf("name = %v", tag["name"])
	}
	if srv.requests[0].Path != "/v3/tags/t1" {
		t.Errorf("path = %s", srv.requests[0].Path)
	}
}

func TestNewTagDefaultsToShared(t *testing.T) {
	srv := newServer(t)
	srv.handleCreated("POST /v3/tags", `{"result":{"id":"t1","name":"alpha","privacy":"shared"}}`)

	runOne(t, `new_censys_tag({Name: "alpha"})`, nil)

	var body map[string]any
	if err := json.Unmarshal([]byte(srv.requests[0].Body), &body); err != nil {
		t.Fatalf("request body: %v", err)
	}
	if body["privacy"] != "shared" {
		t.Errorf("privacy = %v, want the shared default", body["privacy"])
	}
}

func TestSetTagRefusesAnEmptyPatch(t *testing.T) {
	newServer(t)
	msg := runErr(t, `set_censys_tag("t1"; {})`, nil)
	if !strings.Contains(msg, "at least one") {
		t.Errorf("error should say what is missing, got %q", msg)
	}
}

// TestRemoveReturnsTheIdentifier keeps a deletion visible in a pipeline
// instead of collapsing it to null.
func TestRemoveReturnsTheIdentifier(t *testing.T) {
	srv := newServer(t)
	srv.handleFunc("/v3/tags/t1", func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	})

	if got := runOne(t, `remove_censys_tag("t1")`, nil); got != "t1" {
		t.Errorf("remove_censys_tag returned %v", got)
	}
	if srv.requests[0].Method != http.MethodDelete {
		t.Errorf("method = %s", srv.requests[0].Method)
	}
}

// TestAddTagAssignmentTakesTheAssetFromThePipeline is the calling form the
// cmdlet exists for: tagging everything a search found.
func TestAddTagAssignmentTakesTheAssetFromThePipeline(t *testing.T) {
	srv := newServer(t)
	srv.handleCreated("POST /v3/tags/t1/assignments", `{"result":{"id":"a1","asset_id":"1.1.1.1"}}`)

	assignment := obj(t, runOne(t, `"1.1.1.1" | add_censys_tag_assignment("t1")`, nil))
	if assignment["asset_id"] != "1.1.1.1" {
		t.Errorf("asset_id = %v", assignment["asset_id"])
	}

	var body map[string]any
	if err := json.Unmarshal([]byte(srv.requests[0].Body), &body); err != nil {
		t.Fatalf("request body: %v", err)
	}
	if body["asset_id"] != "1.1.1.1" {
		t.Errorf("asset_id in body = %v", body["asset_id"])
	}

	// Both explicit arguments must mean the same thing as the piped form.
	srv.requests = nil
	explicit := obj(t, runOne(t, `add_censys_tag_assignment("t1"; "1.1.1.1")`, nil))
	if jsonOf(t, explicit) != jsonOf(t, assignment) {
		t.Errorf("explicit %s differs from piped %s", jsonOf(t, explicit), jsonOf(t, assignment))
	}
}

func TestCenseyeJobTargetsTheAssetKindAsked(t *testing.T) {
	srv := newServer(t)
	srv.handle("POST /v3/threat-hunting/censeye/jobs", `{"result":{"job_id":"j1","status":"pending"}}`)

	runOne(t, `new_censys_censeye_job("example.com:443"; {Type: "webproperty"})`, nil)

	var body map[string]any
	if err := json.Unmarshal([]byte(srv.requests[0].Body), &body); err != nil {
		t.Fatalf("request body: %v", err)
	}
	target := obj(t, body["target"])
	if target["webproperty_id"] != "example.com:443" {
		t.Errorf("target = %v", target)
	}
}

func TestCenseyeJobRejectsAnUnknownAssetKind(t *testing.T) {
	newServer(t)
	msg := runErr(t, `new_censys_censeye_job("1.1.1.1"; {Type: "router"})`, nil)
	if !strings.Contains(msg, "host, webproperty or certificate") {
		t.Errorf("error should list the kinds, got %q", msg)
	}
}

func TestCreditsFollowTheConfiguredOrganization(t *testing.T) {
	srv := newServer(t)
	srv.handle("/v3/accounts/organizations/test-org/credits", `{"result":{"balance":100}}`)
	srv.handle("/v3/accounts/users/credits", `{"result":{"balance":5}}`)

	org := obj(t, runOne(t, `get_censys_credits`, nil))
	if srv.requests[0].Path != "/v3/accounts/organizations/test-org/credits" {
		t.Errorf("path = %s", srv.requests[0].Path)
	}
	if org[psobject.PSTypeNameKey] != "Censys.Platform.Credits" {
		t.Errorf("PSTypeName = %v", org[psobject.PSTypeNameKey])
	}

	srv.requests = nil
	runOne(t, `get_censys_credits({Scope: "user"})`, nil)
	if srv.requests[0].Path != "/v3/accounts/users/credits" {
		t.Errorf("Scope: user went to %s", srv.requests[0].Path)
	}
}

func TestOrganizationEndpointNeedsAnOrganization(t *testing.T) {
	newServer(t)
	t.Setenv(EnvOrgID, "")

	msg := runErr(t, `get_censys_organization`, nil)
	if !strings.Contains(msg, EnvOrgID) {
		t.Errorf("error should name the environment variable, got %q", msg)
	}
}

// TestContextNeverRevealsTheToken is a security property, not a formatting
// one: query output lands in logs and scrollback.
func TestContextNeverRevealsTheToken(t *testing.T) {
	newServer(t)

	context := obj(t, runOne(t, `get_censys_context`, nil))
	for key, value := range context {
		if s, ok := value.(string); ok && strings.Contains(s, "test-token") {
			t.Errorf("%s leaked the token: %q", key, s)
		}
	}
	if context["HasToken"] != true {
		t.Errorf("HasToken = %v", context["HasToken"])
	}
	if context["TokenSource"] != EnvToken {
		t.Errorf("TokenSource = %v", context["TokenSource"])
	}
	if context["OrganizationId"] != "test-org" {
		t.Errorf("OrganizationId = %v", context["OrganizationId"])
	}
}

func TestContextReportsMissingCredentials(t *testing.T) {
	newServer(t)
	t.Setenv(EnvToken, "")
	t.Setenv(EnvOrgID, "")

	context := obj(t, runOne(t, `get_censys_context`, nil))
	if context["HasToken"] != false {
		t.Errorf("HasToken = %v", context["HasToken"])
	}
	if context["TokenSource"] != "" {
		t.Errorf("TokenSource = %v, want empty when there is no token", context["TokenSource"])
	}
}

func TestNumbersSurviveAsExactValues(t *testing.T) {
	srv := newServer(t)
	// A certificate serial is the case float64 would quietly round.
	srv.handle("/v3/global/search/aggregate",
		`{"result":{"buckets":[],"total_count":9007199254740993}}`)

	agg := obj(t, runOne(t, `get_censys_aggregate("x"; "f")`, nil))
	if got := jsonOf(t, agg["total_count"]); got != "9007199254740993" {
		t.Errorf("total_count = %s", got)
	}
}

func TestEmptyResponseIsAnError(t *testing.T) {
	srv := newServer(t)
	srv.handleAsset("/v3/global/asset/host/1.1.1.1", hostType, `{}`)

	msg := runErr(t, `get_censys_host("1.1.1.1")`, nil)
	if !strings.Contains(msg, "empty response") {
		t.Errorf("error = %q", msg)
	}
}
