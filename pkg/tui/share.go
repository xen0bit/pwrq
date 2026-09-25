package tui

import (
	"bytes"
	"compress/flate"
	"encoding/base64"
	"encoding/json"
	"errors"
	"io"
	"net/url"
	"strings"

	"github.com/xen0bit/pwrq/pkg/ideengine"
)

// Share links, in the browser page's own format (pkg/web/src/js/share.js), so
// a link made here opens in the page and a link made there opens here.
//
// The state rides in the URL fragment, which browsers never send to a server:
//
//	#z=<base64url>   deflate-raw of the JSON state
//	#j=<base64url>   the JSON state, uncompressed
//	#q=…&i=…         plain percent-encoded query and input, for hand-writing

const shareStateVersion = 1

// DefaultShareBase is where a link points when PWRQ_SHARE_URL does not say
// otherwise: the IDE as `pwrq-viz --ide` serves it.
const DefaultShareBase = "http://localhost:8080/tools/pwrq/"

// Shared is what a link carries.
type Shared struct {
	Query   string
	Input   string
	Args    []ideengine.Arg
	Options map[string]any
}

// wireState is the page's compact form: single-letter keys, and nothing at
// its default.
type wireState struct {
	V int            `json:"v"`
	Q string         `json:"q"`
	I string         `json:"i,omitempty"`
	A [][2]string    `json:"a,omitempty"`
	O map[string]any `json:"o,omitempty"`
}

// EncodeShare renders state as a fragment, without the leading #.
func EncodeShare(state Shared) (string, error) {
	wire := wireState{V: shareStateVersion, Q: state.Query, I: state.Input}
	for _, arg := range state.Args {
		if strings.TrimSpace(arg.Name) != "" {
			wire.A = append(wire.A, [2]string{arg.Name, arg.Value})
		}
	}
	for key, value := range state.Options {
		if value == nil || value == "" {
			continue
		}
		if wire.O == nil {
			wire.O = map[string]any{}
		}
		wire.O[key] = value
	}

	// The page encodes with JSON.stringify, which leaves <, > and & alone.
	var payload bytes.Buffer
	enc := json.NewEncoder(&payload)
	enc.SetEscapeHTML(false)
	if err := enc.Encode(wire); err != nil {
		return "", err
	}

	var compressed bytes.Buffer
	w, err := flate.NewWriter(&compressed, flate.BestCompression)
	if err != nil {
		return "", err
	}
	if _, err := w.Write(bytes.TrimRight(payload.Bytes(), "\n")); err != nil {
		return "", err
	}
	if err := w.Close(); err != nil {
		return "", err
	}
	return "z=" + base64.RawURLEncoding.EncodeToString(compressed.Bytes()), nil
}

// ShareLink is a link to base carrying state.
func ShareLink(base string, state Shared) (string, error) {
	fragment, err := EncodeShare(state)
	if err != nil {
		return "", err
	}
	if i := strings.IndexByte(base, '#'); i >= 0 {
		base = base[:i]
	}
	return base + "#" + fragment, nil
}

// LooksLikeShare reports whether text is a link, or a bare fragment, that
// DecodeShare would read - which is how a command-line argument is told
// apart from a query.
func LooksLikeShare(text string) bool {
	text = strings.TrimSpace(text)
	if i := strings.IndexByte(text, '#'); i >= 0 {
		fragment := text[i+1:]
		if strings.Contains(text[:i], "://") || i == 0 {
			return strings.HasPrefix(fragment, "z=") || strings.HasPrefix(fragment, "j=") ||
				strings.HasPrefix(fragment, "q=") || strings.HasPrefix(fragment, "i=")
		}
	}
	return false
}

// maxInflated bounds what a link may decompress to. A link is input from
// whoever sent it, and deflate can expand a few kilobytes into gigabytes.
const maxInflated = 64 << 20

// DecodeShare reads a link or its fragment. Like the page, it treats what it
// reads as untrusted: every field is coerced to the type it should be, and
// what cannot be is dropped.
func DecodeShare(link string) (Shared, error) {
	fragment := strings.TrimSpace(link)
	if i := strings.IndexByte(fragment, '#'); i >= 0 {
		fragment = fragment[i+1:]
	}
	if fragment == "" {
		return Shared{}, errors.New("the link carries no query")
	}
	params, err := url.ParseQuery(fragment)
	if err != nil {
		return Shared{}, errors.New("the link's fragment is malformed")
	}

	var raw []byte
	switch {
	case params.Has("z"):
		data, err := fromBase64URL(params.Get("z"))
		if err != nil {
			return Shared{}, errors.New("the link is not a pwrq share link")
		}
		raw, err = io.ReadAll(io.LimitReader(flate.NewReader(bytes.NewReader(data)), maxInflated+1))
		if err != nil {
			return Shared{}, errors.New("the link is damaged: it does not decompress")
		}
		if len(raw) > maxInflated {
			return Shared{}, errors.New("the link expands to more than 64MB")
		}
	case params.Has("j"):
		raw, err = fromBase64URL(params.Get("j"))
		if err != nil {
			return Shared{}, errors.New("the link is not a pwrq share link")
		}
	case params.Has("q") || params.Has("i"):
		return Shared{Query: params.Get("q"), Input: params.Get("i"), Options: map[string]any{}}, nil
	default:
		return Shared{}, errors.New("the link carries no query")
	}

	var state map[string]any
	if err := json.Unmarshal(raw, &state); err != nil {
		return Shared{}, errors.New("the link is damaged: its state is not JSON")
	}
	return validateShared(state), nil
}

func fromBase64URL(text string) ([]byte, error) {
	text = strings.TrimRight(text, "=")
	return base64.RawURLEncoding.DecodeString(text)
}

// validateShared mirrors share.js's validate: the same limits, the same
// coercions.
func validateShared(state map[string]any) Shared {
	text := func(v any) string {
		s, _ := v.(string)
		return s
	}
	out := Shared{Query: text(state["q"]), Input: text(state["i"]), Options: map[string]any{}}

	if entries, ok := state["a"].([]any); ok {
		for _, entry := range entries {
			pair, ok := entry.([]any)
			if !ok || len(pair) < 1 {
				continue
			}
			if len(out.Args) == 32 {
				break
			}
			name := truncateRunes(text(pair[0]), 64)
			value := ""
			if len(pair) > 1 {
				value = truncateRunes(text(pair[1]), 65536)
			}
			if name != "" {
				out.Args = append(out.Args, ideengine.Arg{Name: name, Value: value})
			}
		}
	}

	if options, ok := state["o"].(map[string]any); ok {
		for key, value := range options {
			switch value.(type) {
			case string, bool, float64:
				out.Options[key] = value
			}
		}
	}
	return out
}

func truncateRunes(s string, n int) string {
	runes := []rune(s)
	if len(runes) > n {
		return string(runes[:n])
	}
	return s
}
