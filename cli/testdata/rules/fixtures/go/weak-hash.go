package fixture

import (
	"crypto/md5"
	"crypto/sha1"
	"crypto/sha256"
	"fmt"
)

func fingerprint(data []byte) string {
	// ruleid: go-weak-hash
	h := md5.New()
	h.Write(data)
	// ruleid: go-weak-hash
	return fmt.Sprintf("%x %x", h.Sum(nil), md5.Sum(data))
}

func legacyTag(data []byte) string {
	// ruleid: go-weak-hash
	return fmt.Sprintf("%x", sha1.Sum(data))
}

func digest(data []byte) string {
	// ok: go-weak-hash
	return fmt.Sprintf("%x", sha256.Sum256(data))
}

// A comment naming md5.New() is not a call, and neither is a string.
func mention() string {
	// ok: go-weak-hash
	return "md5.New()"
}
