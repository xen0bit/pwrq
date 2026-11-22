package llm

import (
	"encoding/base64"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

// clearRemote unsets everything that routes System One to the remote
// backend, so a call with no options is a local one.
func clearRemote(t *testing.T) {
	t.Helper()
	for _, k := range []string{EnvSystemOneModel, EnvTypeSafeBase, EnvTypeSafeKey, EnvModel, EnvSystemOneBackend, EnvCache} {
		t.Setenv(k, "")
	}
	// Never let a test download two gigabytes: a load that gets this far
	// fails offline instead.
	t.Setenv("PWRQ_D1_OFFLINE", "1")
	t.Setenv("PWRQ_D1_CACHE_DIR", t.TempDir())
}

func TestChooseBackend(t *testing.T) {
	clearRemote(t)
	for _, c := range []struct {
		opts map[string]any
		env  map[string]string
		want string
		err  string
	}{
		{opts: nil, want: backendLocal},
		{opts: map[string]any{"Device": "cpu"}, want: backendLocal},
		{opts: map[string]any{"Model": "systemone/jev-latest"}, want: backendRemote},
		{opts: map[string]any{"BaseUrl": "http://localhost:8080"}, want: backendRemote},
		{opts: map[string]any{"Backend": "remote"}, want: backendRemote},
		{env: map[string]string{EnvSystemOneModel: "systemone/x"}, want: backendRemote},
		{env: map[string]string{EnvTypeSafeBase: "http://x"}, want: backendRemote},
		{env: map[string]string{EnvSystemOneBackend: "remote"}, want: backendRemote},
		{opts: map[string]any{"Backend": "local", "Model": "x"}, err: "belongs to the remote backend"},
		{opts: map[string]any{"Retries": 3}, err: "belongs to the remote backend"},
		{opts: map[string]any{"Backend": "remote", "Device": "gpu"}, err: "belongs to the local d1-2B backend"},
		{opts: map[string]any{"Backend": "cloud"}, err: "is not local or remote"},
	} {
		for k, v := range c.env {
			t.Setenv(k, v)
		}
		o := defaults()
		if err := bindOptions("op", c.opts, &o, groupSystemOne); err != nil {
			t.Fatal(err)
		}
		got, err := chooseBackend("op", o, c.opts)
		switch {
		case c.err != "" && (err == nil || !strings.Contains(err.Error(), c.err)):
			t.Errorf("%v %v: got %v, want an error containing %q", c.opts, c.env, err, c.err)
		case c.err == "" && (err != nil || got != c.want):
			t.Errorf("%v %v: got %q, %v; want %q", c.opts, c.env, got, err, c.want)
		}
		for k := range c.env {
			t.Setenv(k, "")
		}
	}
}

// TestLocalRefusesBeforeLoading checks that a malformed call fails on the
// question, the state or the media - before two gigabytes are loaded.
func TestLocalRefusesBeforeLoading(t *testing.T) {
	clearRemote(t)
	for _, c := range []struct{ query, want string }{
		{`invoke_systemone({q: {type: "noul"}})`, `"instructions" is the question as text`},
		{`invoke_systemone({q: {type: "maybe", instructions: "?"}})`, `"type" is one of noul, choice, score, text, number, date, box, boxes`},
		{`invoke_systemone({q: {type: "noul", instructions: "?", criterion: {}}})`, `unknown key "criterion"`},
		{`invoke_systemone({q: {type: "score", instructions: "?", criteria: ["one"]}})`, `lists 2 to 10 levels`},
		{`invoke_systemone({q: {type: "text", instructions: "?", range: [0, 1]}})`, `an estimate is {"type": "number", "range": [low, high]}`},
		{`invoke_systemone({q: {type: "boxes", instructions: "?", max: 40}})`, `"max" belongs to boxes, a whole number from 1 to 16`},
		{`invoke_systemone({q: {type: "number", instructions: "?", range: [0, 10], step: 3}})`, `its step, above 0, divides it`},
		{`invoke_systemone(null; {q: {type: "noul", instructions: "?"}}; {})`, `the state is null and there is no Media`},
		{`invoke_systemone({q: {type: "noul", instructions: "?"}}; {Media: [{Type: "audio", Path: "a.wav"}]})`, `audio needs d1-Omni`},
		{`invoke_systemone({q: {type: "noul", instructions: "?"}}; {Media: [{Type: "video", Frames: []}]})`, `a video is a file's path, {Type: "video", Path, Start, End}`},
		{`invoke_systemone({q: {type: "noul", instructions: "?"}}; {Media: [{Data: "not base64!"}]})`, `the Data is not base64`},
		{`invoke_systemone({q: {type: "noul", instructions: "?"}}; {Media: ["/no/such/picture.png"]})`, `no such file`},
		{`invoke_systemone({q: {type: "noul", instructions: "?"}}; {Mdia: []})`, `unknown option "Mdia"`},
		{`[{State: "x"}] | invoke_systemone_batch`, `questions are an object of name to question`},
		{`[{State: "x", Questions: {}, Extra: 1}] | invoke_systemone_batch`, `unknown key "Extra"`},
		{`get_systemone({Model: "x"})`, `unknown option "Model"`},
	} {
		_, err := run(t, c.query, "a state")
		if err == nil || !strings.Contains(err.Error(), c.want) {
			t.Errorf("%s:\n got %v\nwant an error containing %q", c.query, err, c.want)
		}
	}
}

// TestLocalOfflineSaysHowToFetch is what a slim build with no model in its
// cache says, instead of hanging on a download nobody asked for.
func TestLocalOfflineSaysHowToFetch(t *testing.T) {
	clearRemote(t)
	if runtime.GOOS == "js" {
		t.Skip()
	}
	t.Setenv("PWRQ_D1_DEVICE", "cpu")
	_, err := run(t, `invoke_systemone({q: {type: "noul", instructions: "Is it urgent?"}})`, "now!")
	if err == nil || !strings.Contains(err.Error(), "offline") && !strings.Contains(err.Error(), "no build") {
		t.Fatalf("got %v, want an offline error naming how to fetch", err)
	}
}

// localModel runs the real model when PWRQ_TEST_D1_DIR and
// PWRQ_TEST_ORT_LIB name it; CI skips these.
func localModelEnv(t *testing.T) {
	t.Helper()
	dir, lib := os.Getenv("PWRQ_TEST_D1_DIR"), os.Getenv("PWRQ_TEST_ORT_LIB")
	if dir == "" || lib == "" {
		t.Skip("set PWRQ_TEST_D1_DIR and PWRQ_TEST_ORT_LIB to run the model")
	}
	clearRemote(t)
	t.Setenv("PWRQ_D1_MODEL_DIR", dir)
	t.Setenv("PWRQ_D1_ORT_LIB", lib)
	t.Setenv("PWRQ_D1_DEVICE", "cpu")
}

func TestLocalAnswersEveryForm(t *testing.T) {
	localModelEnv(t)
	got, err := run(t, `invoke_systemone({
		refund: {type: "noul", instructions: "Is this a refund request?"},
		team: {type: "choice", instructions: "Which team handles it?", criteria: {billing: "Charges and refunds", shipping: "Delivery"}}
	}) | [(.refund.noul > 0.5), .team.choice]`, "Customer: my package never arrived and I want my money back NOW.")
	if err != nil {
		t.Fatal(err)
	}
	if g := got.([]any); g[0] != true || g[1] != "shipping" {
		t.Errorf("got %v", got)
	}

	got, err = run(t, `invoke_systemone_request({total: {type: "number", instructions: "What is the total?", unit: "EUR"}})
		| [.Answers.total.number, .Provider, .Device, .OutputTokens]`, map[string]any{"total": "56.00 EUR", "vendor": "Acme"})
	if err != nil {
		t.Fatal(err)
	}
	if g := got.([]any); g[0] != 56 || g[1] != "local" || g[2] != "cpu" || g[3] != 0 {
		t.Errorf("got %v", got)
	}

	got, err = run(t, `invoke_systemone_batch([
		{State: "I love it", Questions: {happy: {type: "noul", instructions: "Is the writer happy?"}}},
		{State: "I hate it", Questions: {happy: {type: "noul", instructions: "Is the writer happy?"}}}
	]) | map(.happy.noul > 0.5)`, nil)
	if err != nil {
		t.Fatal(err)
	}
	if g := got.([]any); g[0] != true || g[1] != false {
		t.Errorf("batch got %v", got)
	}
}

func TestLocalReadsAPicture(t *testing.T) {
	localModelEnv(t)
	shapes := filepath.Join("..", "..", "d1", "testdata", "shapes.png")
	raw, err := os.ReadFile(shapes)
	if err != nil {
		t.Fatal(err)
	}
	got, err := run(t, `invoke_systemone(null; {red: {type: "noul", instructions: "Is there a red square?"},
		where: {type: "box", instructions: "Where is the red square?"}}; {Media: ["`+shapes+`"]}) | [.red.noul > 0.5, .where.box.left < 0.5]`, nil)
	if err != nil {
		t.Fatal(err)
	}
	if g := got.([]any); g[0] != true || g[1] != true {
		t.Errorf("got %v", got)
	}
	got, err = run(t, `invoke_systemone(null; {red: {type: "noul", instructions: "Is there a red square?"}}; {Media: [{Data: "`+base64.StdEncoding.EncodeToString(raw)+`"}]}) | .red.noul > 0.5`, nil)
	if err != nil || got != true {
		t.Errorf("base64 picture: %v, %v", got, err)
	}
	got, err = run(t, `invoke_systemone(null; {red: {type: "noul", instructions: "Is there a red square?"}}; {Media: [{FullName: "`+shapes+`"}]}) | .red.noul > 0.5`, nil)
	if err != nil || got != true {
		t.Errorf("file object: %v, %v", got, err)
	}
}

// TestMediaReadsVideoFiles: an MP4, QuickTime, Matroska or WebM path is a video, decoded
// here and sampled as d1 samples, by path, file object or {Type, Path,
// Start, End}; another container is refused with the way out, before any
// model is loaded.
func TestMediaReadsVideoFiles(t *testing.T) {
	clip := filepath.Join("..", "..", "d1", "video", "testdata", "h264.mp4")
	for _, item := range []any{
		clip,
		map[string]any{"FullName": clip},
		map[string]any{"Type": "video", "Path": clip},
	} {
		media, digest, _, err := parseMedia("invoke_systemone", []any{item})
		if err != nil {
			t.Fatalf("%v: %v", item, err)
		}
		m := media[0]
		if m.Picture != nil || len(m.Frames) != 6 || len(m.Times) != 6 {
			t.Fatalf("%v: %d frames, %d times, want 6 of each (two a second for 3s)", item, len(m.Frames), len(m.Times))
		}
		if !strings.HasPrefix(digest[0], "video-file:") {
			t.Errorf("digest %q", digest[0])
		}
	}
	media, _, _, err := parseMedia("invoke_systemone", []any{map[string]any{"Type": "video", "Path": clip, "Start": 1, "End": 2}})
	if err != nil {
		t.Fatal(err)
	}
	if n := len(media[0].Frames); n != 4 || media[0].Times[0] != 0 {
		t.Errorf("1s..2s: %d frames from %v, want 4 from 0", n, media[0].Times[0])
	}

	for _, name := range []string{"vp9.webm", "av1.webm", "h264.mkv"} {
		media, _, _, err := parseMedia("invoke_systemone", []any{filepath.Join("..", "..", "d1", "video", "testdata", name)})
		if err != nil || len(media[0].Frames) < 4 {
			t.Errorf("%s: %v", name, err)
		}
	}
	avi := filepath.Join("..", "..", "d1", "video", "testdata", "other.avi")
	_, _, _, err = parseMedia("invoke_systemone", []any{avi})
	if err == nil || !strings.Contains(err.Error(), "Frames") || !strings.Contains(err.Error(), "ffmpeg") {
		t.Errorf("an AVI: %v, want the frames route and ffmpeg named", err)
	}
	_, _, _, err = parseMedia("invoke_systemone", []any{map[string]any{"Type": "video"}})
	if err == nil {
		t.Error("a video with neither Path nor Frames should be refused")
	}
}
