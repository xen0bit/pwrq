package llm

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"sync"
	"time"
)

// hbbHTTPProtocol is the protocol number of `hbb serve --listen` this client speaks.
const hbbHTTPProtocol = 1

// hbbHTTP is an hbb on another machine. It answers what a child answers, so
// everything above hbbBackend is the same; what differs is how it fails. A
// connection that cannot be made, a wrong token or a server error is the whole
// hbb being unavailable and is raised, as a dead child is. A request the server
// refuses as too large is that file's error, as an unreadable file is.
type hbbHTTP struct {
	base    string
	token   string
	client  *http.Client
	doc     map[string]any
	mu      sync.Mutex
	maxBody int64
}

var (
	hbbRemoteMu   sync.Mutex
	hbbRemotePool = map[string]*hbbHTTP{}
)

func hbbRemote(op string, o hbbOptions) (hbbBackend, error) {
	base := strings.TrimRight(o.Url, "/")
	key := base + "\x00" + o.Token
	hbbRemoteMu.Lock()
	defer hbbRemoteMu.Unlock()
	if h := hbbRemotePool[key]; h != nil {
		return h, nil
	}
	h := &hbbHTTP{base: base, token: o.Token, client: &http.Client{}}
	doc, err := h.get("/v1/info", o.timeout())
	if err != nil {
		return nil, fmt.Errorf("%s: asking %s what it is: %w", op, base, err)
	}
	if p, _ := doc["protocol"].(float64); int(p) != hbbHTTPProtocol {
		return nil, fmt.Errorf("%s: %s speaks hbb protocol %v and this pwrq speaks %d; update the older of the two",
			op, base, doc["protocol"], hbbHTTPProtocol)
	}
	h.doc = doc
	if n, ok := doc["max_body"].(float64); ok {
		h.maxBody = int64(n)
	}
	hbbRemotePool[key] = h
	return h, nil
}

func (h *hbbHTTP) info() map[string]any { return h.doc }

func (h *hbbHTTP) get(path string, timeout time.Duration) (map[string]any, error) {
	m, _, err := h.do(http.MethodGet, path, nil, timeout)
	return m, err
}

// do sends one request, once more after a pause when the server could not be
// reached or said it was busy: scoring is the same answer asked twice, and the
// server's score cache makes the second cheap. It returns the decoded body and the
// status of a reply the server meant (anything but a 5xx).
func (h *hbbHTTP) do(method, path string, body []byte, timeout time.Duration) (map[string]any, int, error) {
	var last error
	for attempt := 0; attempt < 2; attempt++ {
		if attempt > 0 {
			time.Sleep(2 * time.Second)
		}
		req, err := http.NewRequest(method, h.base+path, bytes.NewReader(body))
		if err != nil {
			return nil, 0, err
		}
		req.Header.Set("Content-Type", "application/json")
		if h.token != "" {
			req.Header.Set("Authorization", "Bearer "+h.token)
		}
		h.client.Timeout = timeout
		resp, err := h.client.Do(req)
		if err != nil {
			last = err
			continue
		}
		raw, rerr := io.ReadAll(io.LimitReader(resp.Body, 64<<20))
		_ = resp.Body.Close()
		if rerr != nil {
			last = rerr
			continue
		}
		if resp.StatusCode >= 500 {
			last = fmt.Errorf("%s: %s", resp.Status, truncateForDebug(strings.TrimSpace(string(raw))))
			continue
		}
		var m map[string]any
		if err := json.Unmarshal(raw, &m); err != nil {
			return nil, resp.StatusCode, fmt.Errorf("%s: unreadable reply %q", resp.Status, truncateForDebug(string(raw)))
		}
		if resp.StatusCode == http.StatusUnauthorized || resp.StatusCode == http.StatusForbidden {
			return nil, resp.StatusCode, fmt.Errorf("%s: check the Token (%v)", resp.Status, m["error"])
		}
		return m, resp.StatusCode, nil
	}
	return nil, 0, last
}

func (h *hbbHTTP) ask(req map[string]any, timeout time.Duration) (map[string]any, error) {
	h.mu.Lock()
	defer h.mu.Unlock()
	body, err := json.Marshal(req)
	if err != nil {
		return nil, err
	}
	path, _ := req["path"].(string)
	if h.maxBody > 0 && int64(len(body)) > h.maxBody {
		return map[string]any{"path": path, "error": fmt.Sprintf(
			"file is %d bytes and the server takes %d per request", len(body), h.maxBody)}, nil
	}
	resp, status, err := h.do(http.MethodPost, "/v1/score", body, timeout)
	if err != nil {
		return nil, err
	}
	switch {
	case status == http.StatusRequestEntityTooLarge:
		resp["path"] = path
		return resp, nil
	case status != http.StatusOK:
		return nil, fmt.Errorf("server refused the request (%d): %v", status, resp["error"])
	}
	return resp, nil
}
