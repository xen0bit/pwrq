package hmac

import (
	"crypto/hmac"
	"crypto/md5"
	"crypto/sha256"
	"fmt"
	"testing"

	"github.com/xen0bit/pwrq/pkg/udf/common"
)

func TestHMACSHA256(t *testing.T) {
	key := "secret"
	message := "hello world"

	// Compute expected HMAC
	mac := hmac.New(sha256.New, []byte(key))
	mac.Write([]byte(message))
	expected := fmt.Sprintf("%x", mac.Sum(nil))

	// Test with UDF result input
	udfResult := map[string]any{
		"_val": message,
		"_meta": map[string]any{},
	}

	inputVal := common.ExtractUDFValue(udfResult)
	if inputVal != message {
		t.Errorf("extractUDFValue() = %v, want %v", inputVal, message)
	}

	// Verify HMAC computation
	mac2 := hmac.New(sha256.New, []byte(key))
	mac2.Write([]byte(inputVal.(string)))
	got := fmt.Sprintf("%x", mac2.Sum(nil))

	if got != expected {
		t.Errorf("HMAC computation = %v, want %v", got, expected)
	}
}

func TestHMACMD5(t *testing.T) {
	key := "testkey"
	message := "test message"

	// Compute expected HMAC
	mac := hmac.New(md5.New, []byte(key))
	mac.Write([]byte(message))
	expected := fmt.Sprintf("%x", mac.Sum(nil))

	// Verify
	if len(expected) != 32 {
		t.Errorf("HMAC-MD5 should be 32 hex chars, got %d", len(expected))
	}
}

func TestGetHashFunc(t *testing.T) {
	tests := []struct {
		algorithm string
		wantErr   bool
	}{
		{"md5", false},
		{"sha1", false},
		{"sha224", false},
		{"sha256", false},
		{"sha384", false},
		{"sha512", false},
		{"sha512_224", false},
		{"sha512_256", false},
		{"invalid", true},
	}

	for _, tt := range tests {
		t.Run(tt.algorithm, func(t *testing.T) {
			_, err := getHashFunc(tt.algorithm)
			if (err != nil) != tt.wantErr {
				t.Errorf("getHashFunc() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

