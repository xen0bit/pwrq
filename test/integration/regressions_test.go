package integration

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// TestNewItemCreatesWhatItSays is the regression for the second positional
// argument being read as a name: new_item("/tmp/f"; "file") created a directory
// at /tmp/f containing a file called "file".
func TestNewItemCreatesWhatItSays(t *testing.T) {
	dir := t.TempDir()

	// The permission bits are the umask's business, so the assertion is on the
	// leading character of the mode - the kind of thing that was created -
	// which is what the bug got wrong.
	got := strings.TrimSpace(mustRunDir(t, dir, "", "-n", "-c",
		`new_item("f.txt"; "file") | {Name, Kind: (.Mode[0:1])}`))
	if got != `{"Kind":"-","Name":"f.txt"}` {
		t.Errorf("new_item file: got %s", got)
	}
	if info, err := os.Stat(filepath.Join(dir, "f.txt")); err != nil {
		t.Fatal(err)
	} else if info.IsDir() {
		t.Errorf("new_item made a directory where a file was asked for")
	}

	got = strings.TrimSpace(mustRunDir(t, dir, "", "-n", "-c",
		`new_item("d"; "directory") | {Name, Kind: (.Mode[0:1])}`))
	if got != `{"Kind":"d","Name":"d"}` {
		t.Errorf("new_item directory: got %s", got)
	}
	if info, err := os.Stat(filepath.Join(dir, "d")); err != nil {
		t.Fatal(err)
	} else if !info.IsDir() {
		t.Errorf("new_item made a file where a directory was asked for")
	}

	// The name remains available through the options object.
	got = strings.TrimSpace(mustRunDir(t, dir, "", "-n", "-c",
		`new_item("d2"; {Name: "inner", ItemType: "file"}) | .Name`))
	if got != `"inner"` {
		t.Errorf("new_item Name option: got %s", got)
	}
}

// TestSetContentReadsThePipeline is the regression for set_content not binding
// its value from the pipeline while add_content does.
func TestSetContentReadsThePipeline(t *testing.T) {
	dir := t.TempDir()

	got := strings.TrimSpace(mustRunDir(t, dir, "", "-n", "-c",
		`"hello from the pipeline" | set_content("piped.txt") | .Length`))
	if want := "23"; got != want {
		t.Errorf("piped set_content Length: got %s, want %s", got, want)
	}
	got = strings.TrimSpace(mustRunDir(t, dir, "", "-n", "-c", `cat("piped.txt")`))
	if got != `"hello from the pipeline"` {
		t.Errorf("piped set_content content: got %s", got)
	}

	// An extra argument is an error now, not a silent drop.
	_, stderr, code := runDir(t, dir, "", "-n", "-c", `set_content("x.txt"; "v"; "extra")`)
	if code == 0 {
		t.Errorf("a third positional argument should be rejected, got: %s", stderr)
	}
}

// TestFormatPropsArgument covers the property list the synopsis prints as
// [properties], which used to be ignored unless it was an options object.
func TestFormatPropsArgument(t *testing.T) {
	dir := t.TempDir()
	query := `format_table([{Name:"a",Age:30}]; ["Name"])`

	got := strings.TrimSpace(mustRunDir(t, dir, "", "-n", "-c", query))
	if strings.Contains(got, "Age") {
		t.Errorf("the bare property array was ignored:\n%s", got)
	}
	if !strings.Contains(got, "Name") {
		t.Errorf("the property list should still show Name:\n%s", got)
	}
}

// TestStartProcessPassThru is the regression for PassThru being dead: the
// second-argument options object was ignored, so the call returned null.
func TestStartProcessPassThru(t *testing.T) {
	dir := t.TempDir()

	got := strings.TrimSpace(mustRunDir(t, dir, "", "-n", "-c",
		`start_process("/bin/sleep"; {ArgumentList: ["1"], PassThru: true})
		 | {ok: (.Id > 0), type: .PwrqType, exited: (.HasExited == false)}`))
	want := `{"exited":true,"ok":true,"type":"Pwrq.Process.Started"}`
	if got != want {
		t.Errorf("start_process PassThru: got %s\nwant %s", got, want)
	}
}

// TestGetVariableShapeDescribesBothForms pins the shape Note that reconciles
// the exact-name object return with the wildcard array.
func TestGetVariableShapeDescribesBothForms(t *testing.T) {
	got := strings.TrimSpace(mustRun(t, "", "-n", "-r",
		`get_command("get_variable") | .Shape`))
	if !strings.Contains(got, "single object for an exact name") {
		t.Errorf("get_variable shape does not describe the named form: %s", got)
	}
}
