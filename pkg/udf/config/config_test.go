package config

import (
	"fmt"
	"testing"

	"github.com/itchyny/gojq"
)

func run(t *testing.T, query string, input any, options ...gojq.CompilerOption) any {
	t.Helper()
	q, err := gojq.Parse(query)
	if err != nil {
		t.Fatalf("parse %q: %v", query, err)
	}
	code, err := gojq.Compile(q, options...)
	if err != nil {
		t.Fatalf("compile %q: %v", query, err)
	}
	iter := code.Run(input)
	v, ok := iter.Next()
	if !ok {
		t.Fatalf("%q produced no result", query)
	}
	if e, isErr := v.(error); isErr {
		t.Fatalf("%q: %v", query, e)
	}
	return v
}

func TestIni(t *testing.T) {
	got := run(t, `ini_parse`, `[server]
host = 10.0.0.1
port = 8080
; comment
[log]
level = "debug"`, RegisterAll()...)
	m, ok := got.(map[string]any)
	if !ok {
		t.Fatalf("ini_parse = %T", got)
	}
	server, ok := m["server"].(map[string]any)
	if !ok {
		t.Fatalf("ini server = %T", m["server"])
	}
	if server["host"] != "10.0.0.1" || server["port"] != "8080" {
		t.Errorf("ini server = %v", server)
	}
	logSection := m["log"].(map[string]any)
	if logSection["level"] != "debug" {
		t.Errorf("ini log level = %v", logSection["level"])
	}

	round := run(t, `ini_parse | ini_stringify`, `[server]
host = x`, RegisterAll()...)
	if fmt.Sprint(round) != "[server]\nhost=x\n" {
		t.Errorf("ini round-trip = %q", round)
	}
}

func TestProperties(t *testing.T) {
	got := run(t, `properties_parse`, `# comment
APP_NAME=my app
DB_HOST = db.internal
EMPTY=
escaped\:key=value\:with\:colons
multiline=one\
two`, RegisterAll()...)
	m, ok := got.(map[string]any)
	if !ok {
		t.Fatalf("properties_parse = %T", got)
	}
	if m["APP_NAME"] != "my app" || m["DB_HOST"] != "db.internal" {
		t.Errorf("properties = %v", m)
	}
	if m["escaped:key"] != "value:with:colons" {
		t.Errorf("escaped key = %v", m["escaped:key"])
	}
	if m["multiline"] != "onetwo" {
		t.Errorf("multiline = %v", m["multiline"])
	}

	round := run(t, `properties_parse | properties_stringify`, `a=1`, RegisterAll()...)
	if fmt.Sprint(round) != "a=1\n" {
		t.Errorf("properties round-trip = %q", round)
	}
}

func TestLogfmt(t *testing.T) {
	got := run(t, `logfmt_parse`, `ts=2026-08-10T12:00:00Z level=info msg="user logged in" user=ada retries=3 success=true`, RegisterAll()...)
	m, ok := got.(map[string]any)
	if !ok {
		t.Fatalf("logfmt_parse = %T", got)
	}
	if m["level"] != "info" {
		t.Errorf("level = %v", m["level"])
	}
	if m["msg"] != "user logged in" {
		t.Errorf("msg = %v", m["msg"])
	}
	if fmt.Sprint(m["retries"]) != "3" {
		t.Errorf("retries = %v", m["retries"])
	}
	if m["success"] != true {
		t.Errorf("success = %v", m["success"])
	}

	round := run(t, `logfmt_parse | logfmt_stringify`, `level=info msg="hello world" n=1`, RegisterAll()...)
	if fmt.Sprint(round) != `level=info msg="hello world" n=1` {
		t.Errorf("logfmt round-trip = %q", round)
	}
}
