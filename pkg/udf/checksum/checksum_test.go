package checksum

import (
	"fmt"
	"hash"
	"hash/adler32"
	"hash/crc32"
	"hash/crc64"
	"hash/fnv"
	"testing"

	"github.com/itchyny/gojq"
	"golang.org/x/crypto/blake2b"
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

func TestDigests(t *testing.T) {
	const data = "hello"
	sum := func(h hash.Hash) string {
		h.Write([]byte(data))
		return fmt.Sprintf("%x", h.Sum(nil))
	}
	tests := []struct {
		query string
		want  string
	}{
		{`"hello" | crc32`, sum(crc32.NewIEEE())},
		{`"hello" | crc32c`, sum(crc32.New(crc32.MakeTable(crc32.Castagnoli)))},
		{`"hello" | crc64`, sum(crc64.New(crc64.MakeTable(crc64.ECMA)))},
		{`"hello" | fnv1a`, sum(fnv.New64a())},
		{`"hello" | adler32`, sum(adler32.New())},
	}
	for _, tt := range tests {
		got := fmt.Sprint(run(t, tt.query))
		if got != tt.want {
			t.Errorf("%s = %s, want %s", tt.query, got, tt.want)
		}
	}
}

func TestBlake2b(t *testing.T) {
	got := fmt.Sprint(run(t, `"abc" | blake2b_256`))
	sum := blake2b.Sum256([]byte("abc"))
	if got != fmt.Sprintf("%x", sum[:]) {
		t.Errorf("blake2b_256 = %s", got)
	}
	got = fmt.Sprint(run(t, `"abc" | blake2b_512`))
	sum512 := blake2b.Sum512([]byte("abc"))
	if got != fmt.Sprintf("%x", sum512[:]) {
		t.Errorf("blake2b_512 = %s", got)
	}
}

func TestBcrypt(t *testing.T) {
	hashValue := fmt.Sprint(run(t, `"hunter2" | bcrypt_hash(4)`))
	if len(hashValue) < 50 {
		t.Fatalf("bcrypt_hash too short: %q", hashValue)
	}
	if got := run(t, fmt.Sprintf(`"hunter2" | bcrypt_verify("%s")`, hashValue)); got != true {
		t.Error("bcrypt_verify correct password = false")
	}
	if got := run(t, fmt.Sprintf(`"wrong" | bcrypt_verify("%s")`, hashValue)); got != false {
		t.Error("bcrypt_verify wrong password = true")
	}
}
