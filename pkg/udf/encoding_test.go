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

	// Everything registered through checksum.registerDigest, which declares hex
	// once beside the %x that makes it true. All nine were found by that
	// declaration rather than by anyone remembering them.
	"crc32":      `"hello" | crc32`,
	"crc32c":     `"hello" | crc32c`,
	"crc64":      `"hello" | crc64`,
	"crc16":      `"hello" | crc16`,
	"adler32":    `"hello" | adler32`,
	"fnv1a":      `"hello" | fnv1a`,
	"sha3_256":   `"hello" | sha3_256`,
	"sha3_512":   `"hello" | sha3_512`,
	"keccak_256": `"hello" | keccak_256`,

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

// TestEveryConsumerIsACmdlet checks that nothing declares an input encoding for
// a name that is not registered - the way a declaration goes stale is a rename
// that leaves the DeclareConsumes call behind, and a side table nobody checks
// is exactly what the registration-site rule exists to avoid.
func TestEveryConsumerIsACmdlet(t *testing.T) {
	DefaultRegistry()

	known := make(map[string]bool)
	for _, meta := range GetFunctionMetadata() {
		known[meta.Name] = true
	}

	for _, meta := range GetFunctionMetadata() {
		for _, e := range common.ConsumesOf(meta.Name) {
			if e == common.EncodingUnspecified {
				t.Errorf("%s declares an unspecified input encoding, which says nothing", meta.Name)
			}
		}
	}

	// The declarations live beside the registrations, so anything declared for
	// an unregistered name is a leftover.
	for _, name := range declaredConsumers(t) {
		if !known[name] {
			t.Errorf("%s declares an input encoding but is not a registered cmdlet", name)
		}
	}
}

// TestInversesAgreeAboutTheirEncoding is the guard that makes the mismatch
// check trustworthy. If zlib_compress says "reverse it with zlib_decompress"
// and returns hex, then zlib_decompress must accept hex - otherwise the
// catalogue advises a round trip that the checker will then warn about, and the
// two halves of the same fact contradict each other.
func TestInversesAgreeAboutTheirEncoding(t *testing.T) {
	DefaultRegistry()

	for _, meta := range GetFunctionMetadata() {
		produced, inverse := common.EncodingOf(meta.Name)
		if inverse == "" || produced == common.EncodingUnspecified {
			continue
		}
		name, _, _ := strings.Cut(inverse, ",")
		name = strings.TrimSpace(name)
		if len(common.ConsumesOf(name)) == 0 {
			continue
		}
		if !common.Accepts(name, produced) {
			t.Errorf("%s returns %s and names %s as its inverse, but %s does not accept that",
				meta.Name, produced.Describe(), name, name)
		}
	}
}

// TestDeclaredConsumersAcceptWhatTheyClaim runs the claim rather than reading
// it: each decoder is fed the output of the encoder it names, and has to
// return the original.
func TestDeclaredConsumersAcceptWhatTheyClaim(t *testing.T) {
	DefaultRegistry()

	roundTrips := map[string]string{
		"hex_encode":       "hex_decode",
		"base64_encode":    "base64_decode",
		"base64url_encode": "base64url_decode",
		"base32_encode":    "base32_decode",
		"base58_encode":    "base58_decode",
		"base85_encode":    "base85_decode",
		"binary_encode":    "binary_decode",
	}

	reg := DefaultRegistry()
	options := reg.Options()
	for encoder, decoder := range roundTrips {
		if !common.Accepts(decoder, mustEncoding(t, encoder)) {
			t.Errorf("%s does not accept what %s returns", decoder, encoder)
		}
		got, err := runProbe(options, `"round trip" | `+encoder+` | `+decoder)
		if err != nil {
			t.Errorf("%s | %s: %v", encoder, decoder, err)
			continue
		}
		if got != "round trip" {
			t.Errorf("%s | %s returned %v, want the original", encoder, decoder, got)
		}
	}
}

func mustEncoding(t *testing.T, name string) common.Encoding {
	t.Helper()
	e, _ := common.EncodingOf(name)
	if e == common.EncodingUnspecified {
		t.Fatalf("%s declares no output encoding", name)
	}
	return e
}

// declaredConsumers lists the cmdlets that declared an input encoding, by
// asking the registry rather than by grepping for the call.
func declaredConsumers(t *testing.T) []string {
	t.Helper()
	var out []string
	for _, meta := range GetFunctionMetadata() {
		if len(common.ConsumesOf(meta.Name)) > 0 {
			out = append(out, meta.Name)
		}
	}
	return out
}
