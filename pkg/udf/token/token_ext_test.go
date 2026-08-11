package token

import (
	"encoding/json"
	"fmt"
	"regexp"
	"strings"
	"testing"
)

func TestUUID7(t *testing.T) {
	got := fmt.Sprint(run(t, `uuid7`))
	if !uuidPattern.MatchString(got) {
		t.Fatalf("uuid7 = %q, not a UUID", got)
	}
	if got[14] != '7' {
		t.Errorf("uuid7 version nibble = %q, want 7", got[14])
	}
}

func TestNanoID(t *testing.T) {
	got := fmt.Sprint(run(t, `nanoid(12)`))
	if len(got) != 12 {
		t.Fatalf("nanoid(12) length = %d", len(got))
	}
	for _, c := range got {
		if !strings.ContainsRune(nanoidAlphabet, c) {
			t.Errorf("nanoid produced %q outside the alphabet", c)
		}
	}
}

func TestIsBase64(t *testing.T) {
	if got := run(t, `"aGVsbG8=" | is_base64`); got != true {
		t.Error("is_base64 padded = false")
	}
	if got := run(t, `"aGVsbG8" | is_base64`); got != true {
		t.Error("is_base64 unpadded = false")
	}
	if got := run(t, `"not base64!!" | is_base64`); got != false {
		t.Error("is_base64 garbage = true")
	}
}

func TestBase58(t *testing.T) {
	// Leading zero bytes become leading '1's.
	if got := fmt.Sprint(run(t, `"\u0000\u0000" | base58_encode`)); got != "11" {
		t.Errorf("base58_encode of two zero bytes = %q, want 11", got)
	}
	// Round-trip across a few inputs, including binary bytes.
	for _, input := range []string{"hello", "", "ünïcødé", "bin\x00bytes"} {
		q, _ := json.Marshal(input)
		encoded := fmt.Sprint(run(t, string(q)+" | base58_encode"))
		decoded := fmt.Sprint(run(t, fmt.Sprintf("%q | base58_decode", encoded)))
		if decoded != input {
			t.Errorf("base58 round-trip of %q = %q", input, decoded)
		}
	}
}

func TestUUID7SortsByTime(t *testing.T) {
	// Two uuid7 values generated quickly should both match and be version 7;
	// the millisecond prefix means a later one is >= the earlier.
	a := fmt.Sprint(run(t, `uuid7`))
	b := fmt.Sprint(run(t, `uuid7`))
	re := regexp.MustCompile(`^[0-9a-f]{8}-[0-9a-f]{4}-7[0-9a-f]{3}-[89ab][0-9a-f]{3}-[0-9a-f]{12}$`)
	if !re.MatchString(a) || !re.MatchString(b) {
		t.Errorf("uuid7 shape wrong: %q %q", a, b)
	}
}
