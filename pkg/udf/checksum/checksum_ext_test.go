package checksum

import (
	sha3std "crypto/sha3"
	"fmt"
	"testing"

	xsha3 "golang.org/x/crypto/sha3"
)

func TestSHA3Keccak(t *testing.T) {
	sum256 := sha3std.Sum256([]byte("abc"))
	if got := fmt.Sprint(run(t, `"abc" | sha3_256`)); got != fmt.Sprintf("%x", sum256) {
		t.Errorf("sha3_256 = %s", got)
	}
	sum512 := sha3std.Sum512([]byte("abc"))
	if got := fmt.Sprint(run(t, `"abc" | sha3_512`)); got != fmt.Sprintf("%x", sum512) {
		t.Errorf("sha3_512 = %s", got)
	}
	h := xsha3.NewLegacyKeccak256()
	h.Write([]byte("abc"))
	if got := fmt.Sprint(run(t, `"abc" | keccak_256`)); got != fmt.Sprintf("%x", h.Sum(nil)) {
		t.Errorf("keccak_256 = %s", got)
	}
}

func TestCRC16(t *testing.T) {
	// "123456789" is the CRC-CCITT check value: 0x29B1.
	if got := fmt.Sprint(run(t, `"123456789" | crc16`)); got != "29b1" {
		t.Errorf("crc16 = %s, want 29b1", got)
	}
}

func TestKDFs(t *testing.T) {
	got := fmt.Sprint(run(t, `"password" | pbkdf2_sha256("salt"; 1000; 32)`))
	if len(got) != 64 {
		t.Errorf("pbkdf2_sha256 length = %d, want 64 hex chars", len(got))
	}
	got = fmt.Sprint(run(t, `"password" | argon2id_hash("somesalt"; 1; 8)`))
	if len(got) != 64 {
		t.Errorf("argon2id_hash length = %d, want 64 hex chars", len(got))
	}
}

// TestPBKDF2SHA256Vectors pins pbkdf2_sha256 to the published
// PBKDF2-HMAC-SHA256 vectors (P="password", S="salt", dkLen=32). The cmdlet
// used to derive with SHA3-256, which every vector here would fail.
func TestPBKDF2SHA256Vectors(t *testing.T) {
	cases := []struct {
		iterations int
		want       string
	}{
		{1, "120fb6cffcf8b32c43e7225256c4f837a86548c92ccc35480805987cb70be17b"},
		{2, "ae4d0c95af6b46d32d0adff928f06dd02a303f8ef3c251dfd6e2d85a95474c43"},
		{4096, "c5e478d59288c841aa530db6845c4c8d962893a001ce4e11a4963873aa98134a"},
	}
	for _, c := range cases {
		got := fmt.Sprint(run(t, fmt.Sprintf(`"password" | pbkdf2_sha256("salt"; %d; 32)`, c.iterations)))
		if got != c.want {
			t.Errorf("pbkdf2_sha256(iterations=%d) = %s, want %s", c.iterations, got, c.want)
		}
	}
}

// TestArgon2IDKeyLenAndDeterminism checks the keyLen argument is reachable
// (it used to sit behind an unregistered arity) and that the derivation is
// stable for a fixed salt.
func TestArgon2IDKeyLenAndDeterminism(t *testing.T) {
	got := fmt.Sprint(run(t, `"password" | argon2id_hash("salt"; 1; 8; 64)`))
	if len(got) != 128 {
		t.Errorf("argon2id_hash keyLen=64 length = %d, want 128 hex chars", len(got))
	}
	a := fmt.Sprint(run(t, `"password" | argon2id_hash("salt"; 1; 8)`))
	b := fmt.Sprint(run(t, `"password" | argon2id_hash("salt"; 1; 8)`))
	if a != b {
		t.Errorf("argon2id_hash is not deterministic: %s != %s", a, b)
	}
}

func TestRandomHex(t *testing.T) {
	got := fmt.Sprint(run(t, `random_hex(8)`))
	if len(got) != 16 {
		t.Errorf("random_hex(8) length = %d, want 16", len(got))
	}
}
