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

func TestRandomHex(t *testing.T) {
	got := fmt.Sprint(run(t, `random_hex(8)`))
	if len(got) != 16 {
		t.Errorf("random_hex(8) length = %d, want 16", len(got))
	}
}
