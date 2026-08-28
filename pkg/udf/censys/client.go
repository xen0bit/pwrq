package censys

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"math/big"
	"os"
	"reflect"
	"sort"
	"strings"
	"time"

	censyssdkgo "github.com/censys/censys-sdk-go"
	"github.com/censys/censys-sdk-go/models/sdkerrors"
	"github.com/xen0bit/pwrq/pkg/core/pipeline"
	"github.com/xen0bit/pwrq/pkg/core/typed"
	"github.com/xen0bit/pwrq/pkg/udf/common"
)

// The environment variables the Censys Platform documents for a personal
// access token and an organization ID. Using the documented names means a
// shell already set up for `censys` or for the SDK works for pwrq unchanged.
const (
	EnvToken  = "CENSYS_PLATFORM_TOKEN"
	EnvOrgID  = "CENSYS_PLATFORM_ORGID"
	EnvServer = "CENSYS_PLATFORM_URL"
)

// defaultTimeout matches the SDK's own default and the http cmdlet's.
const defaultTimeout = 30 * time.Second

// censysServerList is the SDK's own list of base URLs, so get_censys_context
// can report where an unconfigured request would go.
var censysServerList = censyssdkgo.ServerList

// connection is where a request goes and what it authenticates with.
//
// Every cmdlet accepts these four as options, so a single query can reach two
// organizations, and a test can point at a local server. Anything not given
// falls back to the environment.
type connection struct {
	Token          string `param:"Token"`
	OrganizationID string `param:"OrganizationId"`
	ServerURL      string `param:"ServerUrl"`
	Timeout        int    `param:"Timeout"`

	// tokenSource and orgSource record where the values came from, so
	// get_censys_context can explain a misconfiguration without printing the
	// token itself.
	tokenSource string
	orgSource   string
}

// connectionParams are the option names connection claims, lowercased for the
// case-insensitive match PowerShell parameters use.
var connectionParams = map[string]bool{
	"token":          true,
	"organizationid": true,
	"serverurl":      true,
	"timeout":        true,
}

// splitOptions separates the connection options from the cmdlet's own, so each
// cmdlet only has to declare the parameters that are actually about its call.
func splitOptions(opts map[string]any) (conn, rest map[string]any) {
	conn = make(map[string]any)
	rest = make(map[string]any)
	for k, v := range opts {
		if connectionParams[strings.ToLower(k)] {
			conn[k] = v
		} else {
			rest[k] = v
		}
	}
	return conn, rest
}

// resolveConnection builds a connection from the options and the environment.
//
// A missing token is reported here rather than as a 401 from the API, because
// "CENSYS_PLATFORM_TOKEN is not set" is the actionable version of that message.
func resolveConnection(op string, opts map[string]any) (connection, error) {
	conn := connection{}
	if err := bindOptions(op, opts, &conn); err != nil {
		return conn, err
	}

	conn.tokenSource = "Token option"
	if conn.Token == "" {
		conn.Token, conn.tokenSource = os.Getenv(EnvToken), EnvToken
	}
	conn.orgSource = "OrganizationId option"
	if conn.OrganizationID == "" {
		conn.OrganizationID, conn.orgSource = os.Getenv(EnvOrgID), EnvOrgID
	}
	if conn.ServerURL == "" {
		conn.ServerURL = os.Getenv(EnvServer)
	}
	if conn.Timeout < 0 {
		return conn, fmt.Errorf("%s: Timeout must not be negative, got %d", op, conn.Timeout)
	}
	return conn, nil
}

// requireToken fails a call that has nothing to authenticate with.
func (c connection) requireToken(op string) error {
	if c.Token == "" {
		return fmt.Errorf("%s: no personal access token; set %s or pass {Token: \"...\"}", op, EnvToken)
	}
	return nil
}

// requireOrg fails a call whose endpoint puts the organization in the path, so
// there is nothing for the API to fall back to.
func (c connection) requireOrg(op string) error {
	if c.OrganizationID == "" {
		return fmt.Errorf("%s: no organization; set %s or pass {OrganizationId: \"...\"}", op, EnvOrgID)
	}
	return nil
}

// sdk builds the vendor client for this connection.
func (c connection) sdk() *censyssdkgo.SDK {
	timeout := defaultTimeout
	if c.Timeout > 0 {
		timeout = time.Duration(c.Timeout) * time.Second
	}
	opts := []censyssdkgo.SDKOption{
		censyssdkgo.WithSecurity(c.Token),
		censyssdkgo.WithTimeout(timeout),
	}
	// The SDK sends the organization as a query parameter on every endpoint
	// that takes one; leaving it unset is meaningful, since the API then bills
	// the caller's free wallet.
	if c.OrganizationID != "" {
		opts = append(opts, censyssdkgo.WithOrganizationID(c.OrganizationID))
	}
	if c.ServerURL != "" {
		opts = append(opts, censyssdkgo.WithServerURL(c.ServerURL))
	}
	return censyssdkgo.New(opts...)
}

// call is the preamble every cmdlet shares: resolve the connection, insist on a
// token, and hand back a client and a context to make the request with.
func call(op string, opts map[string]any) (*censyssdkgo.SDK, connection, context.Context, error) {
	conn, err := resolveConnection(op, opts)
	if err != nil {
		return nil, conn, nil, err
	}
	if err := conn.requireToken(op); err != nil {
		return nil, conn, nil, err
	}
	return conn.sdk(), conn, context.Background(), nil
}

// bindOptions binds an options object to a cmdlet's parameter struct.
//
// Unlike a bare pipeline.BindParameters this rejects names the struct does not
// declare. A silently ignored option is the worst failure mode an API cmdlet
// has: {PageSze: 5} would quietly fetch the default page and the caller would
// believe they had asked for something.
func bindOptions(op string, opts map[string]any, target any) error {
	if len(opts) == 0 {
		return nil
	}
	known := paramNames(target)
	normalized := make(map[string]any, len(opts))
	for k, v := range opts {
		if _, ok := known[strings.ToLower(k)]; !ok {
			if len(known) == 0 {
				return fmt.Errorf("%s: unknown option %q; this cmdlet takes only the connection options (Token, OrganizationId, ServerUrl, Timeout)", op, k)
			}
			return fmt.Errorf("%s: unknown option %q; expected one of %s, or a connection option (Token, OrganizationId, ServerUrl, Timeout)",
				op, k, strings.Join(sortedNames(known), ", "))
		}
		normalized[k] = normalizeNumbers(v)
	}
	if err := pipeline.BindParameters(normalized, target); err != nil {
		return fmt.Errorf("%s: %w", op, err)
	}
	return nil
}

// paramNames maps a parameter struct's option names, lowercased for the
// case-insensitive match, to the spelling they were declared with — which is
// the one an error message should suggest.
func paramNames(target any) map[string]string {
	names := make(map[string]string)
	t := reflect.TypeOf(target)
	if t.Kind() == reflect.Pointer {
		t = t.Elem()
	}
	if t.Kind() != reflect.Struct {
		return names
	}
	for i := range t.NumField() {
		tag := t.Field(i).Tag.Get("param")
		if tag == "" {
			continue
		}
		declared := strings.TrimSpace(strings.Split(tag, ",")[0])
		names[strings.ToLower(declared)] = declared
	}
	return names
}

func sortedNames(known map[string]string) []string {
	names := make([]string, 0, len(known))
	for _, declared := range known {
		names = append(names, declared)
	}
	sort.Strings(names)
	return names
}

// normalizeNumbers converts the number representations the pipeline carries
// into the ones the parameter binder converts from. The CLI decodes input with
// UseNumber, so a page size read out of a JSON document arrives as a
// json.Number and would otherwise fail to bind to an int.
func normalizeNumbers(v any) any {
	switch n := v.(type) {
	case json.Number:
		if i, err := n.Int64(); err == nil {
			return int(i)
		}
		if f, err := n.Float64(); err == nil {
			return f
		}
		return n.String()
	case *big.Int:
		if n.IsInt64() {
			return int(n.Int64())
		}
		return n.String()
	case []any:
		out := make([]any, len(n))
		for i, item := range n {
			out[i] = normalizeNumbers(item)
		}
		return out
	default:
		return v
	}
}

// optionsArg reads a trailing options object out of a cmdlet's arguments.
//
// Cmdlets take their options as the last argument, and jq has no way to say
// "an object or nothing", so an explicit null is accepted as "no options".
func optionsArg(op string, arg any) (map[string]any, error) {
	value := common.BindValue(arg)
	if value == nil {
		return nil, nil
	}
	m, ok := value.(map[string]any)
	if !ok {
		return nil, fmt.Errorf("%s: options must be an object, got %T", op, value)
	}
	return m, nil
}

// asOptions reports whether an argument is an options object rather than a
// value, which is how the cmdlets tell `f("id"; {…})` from `f("id"; "other")`.
func asOptions(arg any) (map[string]any, bool) {
	m, ok := common.BindValue(arg).(map[string]any)
	return m, ok
}

// bindArgString resolves an argument to a required string parameter.
func bindArgString(op, name string, v any) (string, error) {
	s, err := common.BindString(v, name)
	if err != nil {
		return "", fmt.Errorf("%s: %w", op, err)
	}
	if s == "" {
		return "", fmt.Errorf("%s: %s is required", op, name)
	}
	return s, nil
}

// identifier resolves a cmdlet's subject from its argument or, when it has
// none, from the pipeline — so `get_censys_host("1.1.1.1")` and
// `"1.1.1.1" | get_censys_host` are the same call.
func identifier(op, name string, v any, args []any) (string, map[string]any, error) {
	id, opts, err := optionalIdentifier(op, name, v, args)
	if err != nil {
		return "", nil, err
	}
	if id == "" {
		return "", nil, fmt.Errorf("%s: %s is required", op, name)
	}
	return id, opts, nil
}

// optionalIdentifier is identifier for the cmdlets where leaving the subject
// out means something — get_censys_tag with no tag lists them all.
func optionalIdentifier(op, name string, v any, args []any) (string, map[string]any, error) {
	// A leading object is an options bag, not an identifier, which is what
	// `"1.1.1.1" | get_censys_host({AtTime: ...})` looks like from in here.
	subject, optArgs := v, args
	if len(args) > 0 {
		if _, isOpts := asOptions(args[0]); !isOpts {
			subject, optArgs = args[0], args[1:]
		}
	}
	if len(optArgs) > 1 {
		return "", nil, fmt.Errorf("%s: expected at most one options object, got %d extra arguments", op, len(optArgs))
	}

	var opts map[string]any
	if len(optArgs) == 1 {
		var err error
		if opts, err = optionsArg(op, optArgs[0]); err != nil {
			return "", nil, err
		}
	}

	if common.BindValue(subject) == nil {
		return "", opts, nil
	}
	id, err := common.BindString(subject, name)
	if err != nil {
		return "", nil, fmt.Errorf("%s: %w", op, err)
	}
	return id, opts, nil
}

// pairArgs unpicks the calling forms of a cmdlet that takes two identifiers.
//
// Both can be given as arguments, or one can be piped in and the other passed;
// pipedSlot says which of the two the pipeline supplies in that case. An
// options object may follow either form.
func pairArgs(op, firstName, secondName string, pipedSlot int, v any, args []any) (first, second string, opts map[string]any, err error) {
	pipedInto := func(arg any) (any, any) {
		if pipedSlot == 0 {
			return v, arg
		}
		return arg, v
	}

	var firstVal, secondVal any
	var optArgs []any
	switch len(args) {
	case 0:
		return "", "", nil, fmt.Errorf("%s: expected %s and %s", op, firstName, secondName)
	case 1:
		firstVal, secondVal = pipedInto(args[0])
	case 2:
		if _, isOpts := asOptions(args[1]); isOpts {
			firstVal, secondVal = pipedInto(args[0])
			optArgs = args[1:]
		} else {
			firstVal, secondVal = args[0], args[1]
		}
	default:
		firstVal, secondVal, optArgs = args[0], args[1], args[2:]
	}
	if len(optArgs) > 1 {
		return "", "", nil, fmt.Errorf("%s: expected at most one options object, got %d extra arguments", op, len(optArgs))
	}

	if first, err = bindArgString(op, firstName, firstVal); err != nil {
		return "", "", nil, err
	}
	if second, err = bindArgString(op, secondName, secondVal); err != nil {
		return "", "", nil, err
	}
	if len(optArgs) == 1 {
		if opts, err = optionsArg(op, optArgs[0]); err != nil {
			return "", "", nil, err
		}
	}
	return first, second, opts, nil
}

// propertyArgs reads the object of properties a create or update cmdlet is
// given, from the argument or from the pipeline.
func propertyArgs(op string, v any, args []any) (map[string]any, error) {
	source := v
	if len(args) == 1 {
		source = args[0]
	}
	if len(args) > 1 {
		return nil, fmt.Errorf("%s: expected at most one object of properties, got %d arguments", op, len(args))
	}
	props, ok := asOptions(source)
	if !ok {
		return nil, fmt.Errorf("%s: expected an object of properties, got %T", op, common.BindValue(source))
	}
	return props, nil
}

// deref reads an optional string the SDK models as a pointer.
func deref(s *string) string {
	if s == nil {
		return ""
	}
	return *s
}

// apiError turns an SDK failure into the message a person can act on.
//
// The SDK reports problems as three different types whose Error() strings are
// a JSON blob, so the status code and the human-readable title get buried.
func apiError(op string, err error) error {
	var model *sdkerrors.ErrorModel
	if errors.As(err, &model) {
		parts := make([]string, 0, 2)
		if model.Title != nil && *model.Title != "" {
			parts = append(parts, *model.Title)
		}
		if model.Detail != nil && *model.Detail != "" {
			parts = append(parts, *model.Detail)
		}
		msg := strings.Join(parts, ": ")
		if msg == "" {
			msg = model.Error()
		}
		if model.Status != nil {
			return fmt.Errorf("%s: HTTP %d: %s", op, *model.Status, msg)
		}
		return fmt.Errorf("%s: %s", op, msg)
	}

	var authErr *sdkerrors.AuthenticationError
	if errors.As(err, &authErr) {
		return fmt.Errorf("%s: authentication failed: %s", op, authErr.Error())
	}

	var sdkErr *sdkerrors.SDKError
	if errors.As(err, &sdkErr) {
		body := strings.TrimSpace(sdkErr.Body)
		if body == "" {
			body = sdkErr.Message
		}
		return fmt.Errorf("%s: HTTP %d: %s", op, sdkErr.StatusCode, body)
	}

	return fmt.Errorf("%s: %w", op, err)
}

// object converts an SDK payload into the plain JSON the pipeline carries and
// stamps it with a PowerShell type name.
//
// Numbers are decoded with UseNumber so a 64-bit certificate serial or a large
// hit count survives the trip; float64 would round it.
func object(op, typeName string, payload any) (map[string]any, error) {
	if isNil(payload) {
		return nil, fmt.Errorf("%s: the API returned an empty response", op)
	}
	encoded, err := json.Marshal(payload)
	if err != nil {
		return nil, fmt.Errorf("%s: encoding the response: %w", op, err)
	}
	decoder := json.NewDecoder(bytes.NewReader(encoded))
	decoder.UseNumber()
	var decoded any
	if err := decoder.Decode(&decoded); err != nil {
		return nil, fmt.Errorf("%s: decoding the response: %w", op, err)
	}
	m, ok := decoded.(map[string]any)
	if !ok {
		return nil, fmt.Errorf("%s: expected an object from the API, got %T", op, decoded)
	}
	m[typed.TypeKey] = typeName
	return m, nil
}

// isNil reports whether an interface holds a nil pointer, which a plain
// `payload == nil` misses for the typed nils the SDK hands back.
func isNil(v any) bool {
	if v == nil {
		return true
	}
	rv := reflect.ValueOf(v)
	switch rv.Kind() {
	case reflect.Pointer, reflect.Interface, reflect.Map, reflect.Slice:
		return rv.IsNil()
	default:
		return false
	}
}

// items converts a list payload into one object per element.
func items(op, typeName string, list any) ([]any, error) {
	if isNil(list) {
		return nil, nil
	}
	rv := reflect.ValueOf(list)
	if rv.Kind() != reflect.Slice {
		return nil, fmt.Errorf("%s: expected a list from the API, got %T", op, list)
	}
	out := make([]any, 0, rv.Len())
	for i := range rv.Len() {
		obj, err := object(op, typeName, rv.Index(i).Interface())
		if err != nil {
			return nil, err
		}
		out = append(out, obj)
	}
	return out, nil
}
