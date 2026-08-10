package token

import (
	"fmt"
	"testing"

	"github.com/itchyny/gojq"
)

func run(t *testing.T, query string) any {
	t.Helper()
	q, err := gojq.Parse(query)
	if err != nil {
		t.Fatalf("parse %q: %v", query, err)
	}
	code, err := gojq.Compile(q, RegisterAll()...)
	if err != nil {
		t.Fatalf("compile %q: %v", query, err)
	}
	iter := code.Run(nil)
	v, ok := iter.Next()
	if !ok {
		t.Fatalf("%q produced no result", query)
	}
	if e, isErr := v.(error); isErr {
		t.Fatalf("%q: %v", query, e)
	}
	return v
}

const sampleJWT = "eyJhbGciOiJIUzI1NiIsInR5cCI6IkpXVCJ9.eyJzdWIiOiIxMjM0NTY3ODkwIiwibmFtZSI6IkpvaG4gRG9lIiwiaWF0IjoxNTE2MjM5MDIyfQ.SflKxwRJSMeKKF2QT4fwpMeJf36POk6yJV_adQssw5c"

func TestUUID(t *testing.T) {
	got := fmt.Sprint(run(t, `uuid4 | is_uuid`))
	if got != "true" {
		t.Errorf("uuid4 | is_uuid = %s", got)
	}
	if got := fmt.Sprint(run(t, `"550e8400-e29b-41d4-a716-446655440000" | is_uuid`)); got != "true" {
		t.Errorf("is_uuid canonical = %s", got)
	}
	if got := fmt.Sprint(run(t, `"550e8400" | is_uuid`)); got != "false" {
		t.Errorf("is_uuid short = %s", got)
	}
}

func TestUUIDVersion(t *testing.T) {
	if got := fmt.Sprint(run(t, `"550e8400-e29b-41d4-a716-446655440000" | uuid_version`)); got != "4" {
		t.Errorf("uuid_version = %s", got)
	}
}

func TestJWT(t *testing.T) {
	if got := run(t, fmt.Sprintf(`"%s" | is_jwt`, sampleJWT)); got != true {
		t.Error("is_jwt(sample) = false")
	}
	if got := run(t, `"not.a.jwt" | is_jwt`); got != false {
		t.Error("is_jwt(garbage) = true")
	}
	decoded := run(t, fmt.Sprintf(`"%s" | jwt_decode`, sampleJWT))
	m, ok := decoded.(map[string]any)
	if !ok {
		t.Fatalf("jwt_decode = %T, want an object", decoded)
	}
	header, _ := m["header"].(map[string]any)
	if header == nil || header["alg"] != "HS256" {
		t.Errorf("jwt header = %v", m["header"])
	}
	payload, _ := m["payload"].(map[string]any)
	if payload == nil || payload["name"] != "John Doe" {
		t.Errorf("jwt payload = %v", m["payload"])
	}
}

func TestBase64URL(t *testing.T) {
	if got := fmt.Sprint(run(t, `"hello?" | base64url_encode`)); got != "aGVsbG8_" {
		t.Errorf("base64url_encode = %s", got)
	}
	if got := fmt.Sprint(run(t, `"aGVsbG8_" | base64url_decode`)); got != "hello?" {
		t.Errorf("base64url_decode = %s", got)
	}
}

func TestRot(t *testing.T) {
	if got := fmt.Sprint(run(t, `"hello" | rot13`)); got != "uryyb" {
		t.Errorf("rot13 = %s", got)
	}
	if got := fmt.Sprint(run(t, `"hello" | rot(1)`)); got != "ifmmp" {
		t.Errorf("rot(1) = %s", got)
	}
	if got := fmt.Sprint(run(t, `"uryyb" | rot13`)); got != "hello" {
		t.Errorf("rot13 roundtrip = %s", got)
	}
}
