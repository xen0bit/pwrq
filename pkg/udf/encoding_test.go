package udf

import (
	"context"
	"encoding/base32"
	"encoding/base64"
	"regexp"
	"strings"
	"testing"
	"time"

	"github.com/itchyny/gojq"
	"github.com/xen0bit/pwrq/pkg/udf/common"
)

// probes are the calls that prove a declared encoding. Each runs against a
// live registry and its output is checked to be what the cmdlet claims.
//
// Every declared cmdlet needs an entry, and TestEveryDeclaredEncodingHasAProbe
// enforces that: a declaration without a probe is a claim nothing checks, and
// this whole table exists because an unchecked claim about zlib_compress cost
// a model twenty tool calls.
var probes = map[string]string{
	// Hashes and MACs.
	"md5":             `"hello" | md5`,
	"sha1":            `"hello" | sha1`,
	"sha224":          `"hello" | sha224`,
	"sha256":          `"hello" | sha256`,
	"sha384":          `"hello" | sha384`,
	"sha512":          `"hello" | sha512`,
	"sha512_224":      `"hello" | sha512_224`,
	"sha512_256":      `"hello" | sha512_256`,
	"hmac_md5":        `"hello" | hmac_md5("key")`,
	"hmac_sha1":       `"hello" | hmac_sha1("key")`,
	"hmac_sha224":     `"hello" | hmac_sha224("key")`,
	"hmac_sha256":     `"hello" | hmac_sha256("key")`,
	"hmac_sha384":     `"hello" | hmac_sha384("key")`,
	"hmac_sha512":     `"hello" | hmac_sha512("key")`,
	"hmac_sha512_224": `"hello" | hmac_sha512_224("key")`,
	"hmac_sha512_256": `"hello" | hmac_sha512_256("key")`,
	"blake2b_256":     `"hello" | blake2b_256`,
	"blake2b_512":     `"hello" | blake2b_512`,
	"pbkdf2_sha256":   `"hello" | pbkdf2_sha256("salt"; 1000; 16)`,
	"argon2id_hash":   `"hello" | argon2id_hash("salt"; 1; 8)`,
	"random_hex":      `random_hex(16)`,

	// Text encodings and their inverses.
	"hex_encode":       `"hello" | hex_encode`,
	"hex_decode":       `"68656c6c6f" | hex_decode`,
	"base64_encode":    `"hello" | base64_encode`,
	"base64_decode":    `"aGVsbG8=" | base64_decode`,
	"base32_encode":    `"hello" | base32_encode`,
	"base32_decode":    `"NBSWY3DP" | base32_decode`,
	"base85_encode":    `"hello" | base85_encode`,
	"base85_decode":    `"hello" | base85_encode | base85_decode`,
	"base58_encode":    `"hello" | base58_encode`,
	"base58_decode":    `"Cn8eVZg" | base58_decode`,
	"base64url_encode": `"hello?" | base64url_encode`,
	"base64url_decode": `"aGVsbG8_" | base64url_decode`,
	"binary_encode":    `"hello" | binary_encode`,
	"binary_decode":    `"hello" | binary_encode | binary_decode`,

	// Compression, the family that started this.
	"gzip_compress":      `"hello" | gzip_compress`,
	"gzip_decompress":    `"hello" | gzip_compress | gzip_decompress`,
	"zlib_compress":      `"hello" | zlib_compress`,
	"zlib_decompress":    `"hello" | zlib_compress | zlib_decompress`,
	"deflate_compress":   `"hello" | deflate_compress`,
	"deflate_decompress": `"hello" | deflate_compress | deflate_decompress`,

	// Ciphers, which are base64 while nearly everything around them is hex.
	"aes_encrypt":        `aes_encrypt("hello"; "0123456789abcdef")`,
	"des_encrypt":        `des_encrypt("hello"; "01234567")`,
	"triple_des_encrypt": `triple_des_encrypt("hello"; "0123456789abcdef01234567")`,
	"blowfish_encrypt":   `blowfish_encrypt("hello"; "secretkey")`,
	"rc4":                `"hello" | rc4("key")`,
	"chacha20":           `"hello" | chacha20("0123456789abcdef0123456789abcdef")`,
	"xor":                `"hello" | xor("key")`,
}

var (
	hexText    = regexp.MustCompile(`^[0-9a-f]*$`)
	binaryText = regexp.MustCompile(`^[01]{8}( [01]{8})*$`)
)

// holds reports whether a value is written the way an encoding says it is.
func holds(e common.Encoding, s string) bool {
	switch e {
	case common.EncodingHex:
		return hexText.MatchString(s)
	case common.EncodingBase64:
		_, err := base64.StdEncoding.DecodeString(s)
		return err == nil
	case common.EncodingBase64URL:
		_, err := base64.RawURLEncoding.DecodeString(s)
		return err == nil
	case common.EncodingBase32:
		_, err := base32.StdEncoding.DecodeString(s)
		return err == nil
	case common.EncodingBase58:
		return !strings.ContainsAny(s, "0OIl+/=")
	case common.EncodingBase85:
		// Ascii85's alphabet is the printable range ! through u, plus z for a
		// run of zero bytes.
		for _, r := range s {
			if (r < '!' || r > 'u') && r != 'z' {
				return false
			}
		}
		return true
	case common.EncodingBinary:
		return binaryText.MatchString(s)
	case common.EncodingBytesAsText:
		// Any string is bytes in a string. The claim being checked is that the
		// cmdlet returns a string at all rather than an array or an object,
		// which the caller has already established by getting here.
		return true
	}
	return false
}

// TestDeclaredEncodingsHold runs every cmdlet that declares an output encoding
// and checks the output is actually written that way.
//
// This is what keeps the declaration from becoming a second, drifting copy of
// a fact. The catalogue now tells a caller that zlib_compress returns hex; if
// someone changes it to return raw bytes, or base64, the sentence would go on
// saying hex and would be worse than the silence it replaced. Here it fails.
func TestDeclaredEncodingsHold(t *testing.T) {
	reg := DefaultRegistry()
	options := reg.Options()

	for _, meta := range GetFunctionMetadata() {
		encoding, _ := common.EncodingOf(meta.Name)
		if encoding == common.EncodingUnspecified {
			continue
		}
		probe, ok := probes[meta.Name]
		if !ok {
			continue // reported by TestEveryDeclaredEncodingHasAProbe
		}

		t.Run(meta.Name, func(t *testing.T) {
			got, err := runProbe(options, probe)
			if err != nil {
				t.Fatalf("%s: %v", probe, err)
			}
			s, isString := got.(string)
			if !isString {
				t.Fatalf("%s returned %T, but declares an encoding, which only a string can have", meta.Name, got)
			}
			if !holds(encoding, s) {
				t.Errorf("%s declares %s but returned %q", meta.Name, encoding.Describe(), truncate(s))
			}
		})
	}
}

// TestEveryDeclaredEncodingHasAProbe stops a declaration being added without
// the check that keeps it true.
func TestEveryDeclaredEncodingHasAProbe(t *testing.T) {
	DefaultRegistry()

	var unchecked []string
	for _, meta := range GetFunctionMetadata() {
		encoding, _ := common.EncodingOf(meta.Name)
		if encoding == common.EncodingUnspecified {
			continue
		}
		if _, ok := probes[meta.Name]; !ok {
			unchecked = append(unchecked, meta.Name)
		}
	}
	if len(unchecked) > 0 {
		t.Errorf("%d cmdlet(s) declare an output encoding that nothing checks; add a call to the probes table:\n    %s",
			len(unchecked), strings.Join(unchecked, "\n    "))
	}
}

// TestDeclaredInversesExist checks that a cmdlet naming its inverse names one
// that is really there, since "reverse it with X" is the half of the sentence
// a caller acts on.
func TestDeclaredInversesExist(t *testing.T) {
	DefaultRegistry()

	known := make(map[string]bool)
	for _, meta := range GetFunctionMetadata() {
		known[meta.Name] = true
	}

	for _, meta := range GetFunctionMetadata() {
		_, inverse := common.EncodingOf(meta.Name)
		if inverse == "" {
			continue
		}
		// The note may carry a clause after the name, as the compressors do.
		name, _, _ := strings.Cut(inverse, ",")
		if !known[strings.TrimSpace(name)] {
			t.Errorf("%s says to reverse it with %q, which is not a cmdlet", meta.Name, name)
		}
	}
}

// TestCompressorsRoundTripThroughTheirDeclaredForm is the specific claim the
// compressors make and the one a model got wrong for twenty calls: the hex
// they return goes straight back into the decompressor, with no hex_decode in
// between.
func TestCompressorsRoundTripThroughTheirDeclaredForm(t *testing.T) {
	reg := DefaultRegistry()
	options := reg.Options()

	for _, pair := range [][2]string{
		{"gzip_compress", "gzip_decompress"},
		{"zlib_compress", "zlib_decompress"},
		{"deflate_compress", "deflate_decompress"},
	} {
		t.Run(pair[0], func(t *testing.T) {
			got, err := runProbe(options, `"hello pwrq" | `+pair[0]+` | `+pair[1])
			if err != nil {
				t.Fatalf("round trip failed: %v", err)
			}
			if got != "hello pwrq" {
				t.Errorf("%s | %s = %v, want the original", pair[0], pair[1], got)
			}
		})
	}
}

// runProbe evaluates one probe and returns its single result.
func runProbe(options []gojq.CompilerOption, probe string) (any, error) {
	query, err := gojq.Parse(probe)
	if err != nil {
		return nil, err
	}
	code, err := gojq.Compile(query, options...)
	if err != nil {
		return nil, err
	}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	iter := code.RunWithContext(ctx, nil)
	v, ok := iter.Next()
	if !ok {
		return nil, errNoOutput
	}
	if err, isErr := v.(error); isErr {
		return nil, err
	}
	return v, nil
}

var errNoOutput = errNoOutputType{}

type errNoOutputType struct{}

func (errNoOutputType) Error() string { return "the probe produced no output" }

func truncate(s string) string {
	if len(s) <= 60 {
		return s
	}
	return s[:60] + "..."
}
