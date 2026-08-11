package token

import (
	"crypto/rand"
	"encoding/base64"
	"encoding/binary"
	"fmt"
	"math/big"
	"strings"
	"time"

	"github.com/itchyny/gojq"
	"github.com/xen0bit/pwrq/pkg/udf/common"
)

// RegisterUUID7 registers uuid7, a time-ordered version-7 UUID whose leading
// bytes sort by creation time.
func RegisterUUID7() gojq.CompilerOption {
	return gojq.WithFunction("uuid7", 0, 0, func(v any, args []any) any {
		ms := uint64(time.Now().UnixMilli())
		var b [16]byte
		binary.BigEndian.PutUint64(b[0:8], ms<<16) // 48 bits of milliseconds
		if _, err := rand.Read(b[6:]); err != nil {
			return common.MakeUDFErrorResult(err, nil)
		}
		b[6] = (b[6] & 0x0f) | 0x70 // version 7
		b[8] = (b[8] & 0x3f) | 0x80 // RFC 4122 variant
		return common.MakeUDFSuccessResult(formatUUID(b[:]), nil)
	})
}

const nanoidAlphabet = "_-0123456789abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ"

// randBelow returns a uniform index in [0, n).
func randBelow(n int) (int, error) {
	v, err := rand.Int(rand.Reader, big.NewInt(int64(n)))
	if err != nil {
		return 0, err
	}
	return int(v.Int64()), nil
}

// RegisterNanoID registers nanoid, n URL-safe characters (21 by default) drawn
// from the nanoid alphabet.
func RegisterNanoID() gojq.CompilerOption {
	return gojq.WithFunction("nanoid", 0, 1, func(v any, args []any) any {
		n := 21
		if len(args) > 0 {
			if m, ok := common.ToInt(args[0]); ok && m >= 0 {
				n = m
			}
		}
		out := make([]byte, n)
		for i := 0; i < n; i++ {
			idx, err := randBelow(len(nanoidAlphabet))
			if err != nil {
				return common.MakeUDFErrorResult(err, nil)
			}
			out[i] = nanoidAlphabet[idx]
		}
		return common.MakeUDFSuccessResult(string(out), nil)
	})
}

// RegisterIsBase64 registers is_base64, whether a string decodes as standard
// base64 (padded or unpadded).
func RegisterIsBase64() gojq.CompilerOption {
	return registerBoolToken("is_base64", func(s string) bool {
		clean := strings.TrimSpace(s)
		_, err := base64.StdEncoding.DecodeString(clean)
		if err == nil {
			return true
		}
		_, err = base64.RawStdEncoding.DecodeString(clean)
		return err == nil
	})
}

// RegisterIsBase64URL registers is_base64url, whether a string decodes as
// URL-safe base64 (padded or unpadded).
func RegisterIsBase64URL() gojq.CompilerOption {
	return registerBoolToken("is_base64url", func(s string) bool {
		clean := strings.TrimSpace(s)
		_, err := base64.RawURLEncoding.DecodeString(clean)
		if err == nil {
			return true
		}
		_, err = base64.URLEncoding.DecodeString(clean)
		return err == nil
	})
}

const base58Alphabet = "123456789ABCDEFGHJKLMNPQRSTUVWXYZabcdefghijkmnopqrstuvwxyz"

// RegisterBase58Encode registers base58_encode, the byte string as the compact
// base58 alphabet used by Bitcoin addresses.
func RegisterBase58Encode() gojq.CompilerOption {
	return gojq.WithFunction("base58_encode", 0, 2, func(v any, args []any) any {
		s, err := strInput(v, args, "base58_encode")
		if err != nil {
			return common.MakeUDFErrorResult(fmt.Errorf("base58_encode: %v", err), nil)
		}
		return common.MakeUDFSuccessResult(base58Encode([]byte(s)), nil)
	})
}

// RegisterBase58Decode registers base58_decode, the inverse of base58_encode.
func RegisterBase58Decode() gojq.CompilerOption {
	return gojq.WithFunction("base58_decode", 0, 2, func(v any, args []any) any {
		s, err := strInput(v, args, "base58_decode")
		if err != nil {
			return common.MakeUDFErrorResult(fmt.Errorf("base58_decode: %v", err), nil)
		}
		decoded, err := base58Decode(strings.TrimSpace(s))
		if err != nil {
			return common.MakeUDFErrorResult(fmt.Errorf("base58_decode: %v", err), nil)
		}
		return common.MakeUDFSuccessResult(string(decoded), nil)
	})
}

func base58Encode(data []byte) string {
	// Count leading zero bytes, which become leading '1's.
	zeros := 0
	for zeros < len(data) && data[zeros] == 0 {
		zeros++
	}
	n := new(big.Int).SetBytes(data)
	base := big.NewInt(58)
	mod := new(big.Int)
	var b strings.Builder
	for n.Sign() > 0 {
		n.DivMod(n, base, mod)
		b.WriteByte(base58Alphabet[mod.Int64()])
	}
	for i := 0; i < zeros; i++ {
		b.WriteByte('1')
	}
	// The alphabet is built least-significant first; reverse.
	runes := []byte(b.String())
	for i, j := 0, len(runes)-1; i < j; i, j = i+1, j-1 {
		runes[i], runes[j] = runes[j], runes[i]
	}
	return string(runes)
}

func base58Decode(s string) ([]byte, error) {
	n := big.NewInt(0)
	base := big.NewInt(58)
	for i := 0; i < len(s); i++ {
		idx := strings.IndexByte(base58Alphabet, s[i])
		if idx < 0 {
			return nil, fmt.Errorf("invalid base58 character %q", s[i])
		}
		n.Mul(n, base)
		n.Add(n, big.NewInt(int64(idx)))
	}
	bytes := n.Bytes()
	// Leading '1's are zero bytes.
	zeros := 0
	for zeros < len(s) && s[zeros] == '1' {
		zeros++
	}
	out := make([]byte, zeros+len(bytes))
	copy(out[zeros:], bytes)
	return out, nil
}

// registerBoolToken registers a 0-2 arity string-in, boolean-out cmdlet.
func registerBoolToken(name string, fn func(string) bool) gojq.CompilerOption {
	return gojq.WithFunction(name, 0, 2, func(v any, args []any) any {
		s, err := strInput(v, args, name)
		if err != nil {
			return common.MakeUDFErrorResult(fmt.Errorf("%s: %v", name, err), nil)
		}
		return common.MakeUDFSuccessResult(fn(s), nil)
	})
}
