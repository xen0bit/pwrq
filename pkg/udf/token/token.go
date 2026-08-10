// Package token provides identifiers and token helpers: UUID generation and
// validation, JWT decoding, URL-safe base64, and ROT ciphers.
package token

import (
	"crypto/rand"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"regexp"
	"strings"

	"github.com/itchyny/gojq"
	"github.com/xen0bit/pwrq/pkg/udf/common"
)

// RegisterAll registers every token cmdlet.
func RegisterAll() []gojq.CompilerOption {
	return []gojq.CompilerOption{
		RegisterUUID4(),
		RegisterIsUUID(),
		RegisterUUIDVersion(),
		RegisterJWTDecode(),
		RegisterIsJWT(),
		RegisterBase64URLEncode(),
		RegisterBase64URLDecode(),
		RegisterRot13(),
		RegisterRot(),
		RegisterUUID7(),
		RegisterNanoID(),
		RegisterIsBase64(),
		RegisterIsBase64URL(),
		RegisterBase58Encode(),
		RegisterBase58Decode(),
	}
}

// strInput resolves a string from the pipeline or first argument.
func strInput(v any, args []any, name string) (string, error) {
	inputVal, _, err := common.ParseFileArgs(v, args)
	if err != nil {
		return "", err
	}
	switch val := common.BindValue(inputVal).(type) {
	case string:
		return val, nil
	case []byte:
		return string(val), nil
	default:
		return "", fmt.Errorf("%s: expected a string, got %T", name, inputVal)
	}
}

// RegisterUUID4 registers uuid4, a freshly generated version-4 UUID.
func RegisterUUID4() gojq.CompilerOption {
	return gojq.WithFunction("uuid4", 0, 0, func(v any, args []any) any {
		var b [16]byte
		if _, err := rand.Read(b[:]); err != nil {
			return common.MakeUDFErrorResult(err, nil)
		}
		b[6] = (b[6] & 0x0f) | 0x40 // version 4
		b[8] = (b[8] & 0x3f) | 0x80 // RFC 4122 variant
		return common.MakeUDFSuccessResult(formatUUID(b[:]), nil)
	})
}

func formatUUID(b []byte) string {
	hexBytes := hex.EncodeToString(b)
	return hexBytes[0:8] + "-" + hexBytes[8:12] + "-" + hexBytes[12:16] + "-" + hexBytes[16:20] + "-" + hexBytes[20:32]
}

var uuidPattern = regexp.MustCompile(`^[0-9a-fA-F]{8}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{12}$`)

// RegisterIsUUID registers is_uuid, whether a string is a UUID in canonical
// hyphenated form.
func RegisterIsUUID() gojq.CompilerOption {
	return gojq.WithFunction("is_uuid", 0, 2, func(v any, args []any) any {
		s, err := strInput(v, args, "is_uuid")
		if err != nil {
			return common.MakeUDFErrorResult(fmt.Errorf("is_uuid: %v", err), nil)
		}
		return common.MakeUDFSuccessResult(uuidPattern.MatchString(s), nil)
	})
}

// RegisterUUIDVersion registers uuid_version, the version nibble of a UUID, or
// null when the string is not a UUID.
func RegisterUUIDVersion() gojq.CompilerOption {
	return gojq.WithFunction("uuid_version", 0, 2, func(v any, args []any) any {
		s, err := strInput(v, args, "uuid_version")
		if err != nil {
			return common.MakeUDFErrorResult(fmt.Errorf("uuid_version: %v", err), nil)
		}
		if !uuidPattern.MatchString(s) {
			return nil
		}
		var version int
		if _, err := fmt.Sscanf(s[14:15], "%x", &version); err != nil {
			return common.MakeUDFErrorResult(fmt.Errorf("uuid_version: cannot read version"), nil)
		}
		return common.MakeUDFSuccessResult(version, nil)
	})
}

// base64urlDecode tolerates both padded and unpadded URL-safe base64.
func base64urlDecode(s string) ([]byte, error) {
	if b, err := base64.RawURLEncoding.DecodeString(s); err == nil {
		return b, nil
	}
	if b, err := base64.URLEncoding.DecodeString(s); err == nil {
		return b, nil
	}
	return nil, fmt.Errorf("not valid base64url")
}

// RegisterJWTDecode registers jwt_decode, splitting a JWT into its decoded
// header and payload and its signature.
func RegisterJWTDecode() gojq.CompilerOption {
	return gojq.WithFunction("jwt_decode", 0, 2, func(v any, args []any) any {
		s, err := strInput(v, args, "jwt_decode")
		if err != nil {
			return common.MakeUDFErrorResult(fmt.Errorf("jwt_decode: %v", err), nil)
		}
		parts := strings.Split(strings.TrimSpace(s), ".")
		if len(parts) != 3 {
			return common.MakeUDFErrorResult(fmt.Errorf("jwt_decode: expected three dot-separated segments"), nil)
		}
		header, err := base64urlDecode(parts[0])
		if err != nil {
			return common.MakeUDFErrorResult(fmt.Errorf("jwt_decode: header: %v", err), nil)
		}
		payload, err := base64urlDecode(parts[1])
		if err != nil {
			return common.MakeUDFErrorResult(fmt.Errorf("jwt_decode: payload: %v", err), nil)
		}
		return common.MakeUDFSuccessResult(map[string]any{
			"header":    jsonValue(header),
			"payload":   jsonValue(payload),
			"signature": parts[2],
		}, nil)
	})
}

// jsonValue decodes a JSON document, falling back to the raw string.
func jsonValue(raw []byte) any {
	var v any
	if err := json.Unmarshal(raw, &v); err == nil {
		return v
	}
	return string(raw)
}

// RegisterIsJWT registers is_jwt, whether a string is three base64url segments.
func RegisterIsJWT() gojq.CompilerOption {
	return gojq.WithFunction("is_jwt", 0, 2, func(v any, args []any) any {
		s, err := strInput(v, args, "is_jwt")
		if err != nil {
			return common.MakeUDFErrorResult(fmt.Errorf("is_jwt: %v", err), nil)
		}
		parts := strings.Split(strings.TrimSpace(s), ".")
		if len(parts) != 3 {
			return common.MakeUDFSuccessResult(false, nil)
		}
		for _, part := range parts {
			if _, err := base64urlDecode(part); err != nil {
				return common.MakeUDFSuccessResult(false, nil)
			}
		}
		return common.MakeUDFSuccessResult(true, nil)
	})
}

// RegisterBase64URLEncode registers base64url_encode, unpadded URL-safe base64.
func RegisterBase64URLEncode() gojq.CompilerOption {
	return gojq.WithFunction("base64url_encode", 0, 2, func(v any, args []any) any {
		s, err := strInput(v, args, "base64url_encode")
		if err != nil {
			return common.MakeUDFErrorResult(fmt.Errorf("base64url_encode: %v", err), nil)
		}
		return common.MakeUDFSuccessResult(base64.RawURLEncoding.EncodeToString([]byte(s)), nil)
	})
}

// RegisterBase64URLDecode registers base64url_decode, the inverse of
// base64url_encode.
func RegisterBase64URLDecode() gojq.CompilerOption {
	return gojq.WithFunction("base64url_decode", 0, 2, func(v any, args []any) any {
		s, err := strInput(v, args, "base64url_decode")
		if err != nil {
			return common.MakeUDFErrorResult(fmt.Errorf("base64url_decode: %v", err), nil)
		}
		decoded, err := base64urlDecode(strings.TrimSpace(s))
		if err != nil {
			return common.MakeUDFErrorResult(fmt.Errorf("base64url_decode: %v", err), nil)
		}
		return common.MakeUDFSuccessResult(string(decoded), nil)
	})
}

// RegisterRot13 registers rot13, ROT-13 over ASCII letters.
func RegisterRot13() gojq.CompilerOption {
	return gojq.WithFunction("rot13", 0, 2, func(v any, args []any) any {
		s, err := strInput(v, args, "rot13")
		if err != nil {
			return common.MakeUDFErrorResult(fmt.Errorf("rot13: %v", err), nil)
		}
		return common.MakeUDFSuccessResult(rot(s, 13), nil)
	})
}

// RegisterRot registers rot, a Caesar cipher over ASCII letters with a shift.
func RegisterRot() gojq.CompilerOption {
	return gojq.WithFunction("rot", 1, 2, func(v any, args []any) any {
		shift, ok := common.ToInt(args[0])
		if !ok {
			return common.MakeUDFErrorResult(fmt.Errorf("rot: shift must be an integer, got %v", args[0]), nil)
		}
		isFile := false
		if len(args) > 1 {
			if b, ok := args[1].(bool); ok {
				isFile = b
			}
		}
		var input any
		if isFile {
			path, ok := common.BindPath(v)
			if !ok {
				return common.MakeUDFErrorResult(fmt.Errorf("rot: file argument requires a string path"), nil)
			}
			data, _, _, err := common.ReadFileFromPath(path)
			if err != nil {
				return common.MakeUDFErrorResult(fmt.Errorf("rot: %v", err), nil)
			}
			input = string(data)
		} else {
			input = common.BindValue(v)
		}
		s, err := stringValue(input)
		if err != nil {
			return common.MakeUDFErrorResult(fmt.Errorf("rot: %v", err), nil)
		}
		return common.MakeUDFSuccessResult(rot(s, shift), nil)
	})
}

func stringValue(v any) (string, error) {
	switch val := v.(type) {
	case string:
		return val, nil
	case []byte:
		return string(val), nil
	default:
		return "", fmt.Errorf("expected a string, got %T", v)
	}
}

func rot(s string, shift int) string {
	shift = ((shift % 26) + 26) % 26
	runes := []rune(s)
	for i, r := range runes {
		switch {
		case r >= 'a' && r <= 'z':
			runes[i] = 'a' + (r-'a'+rune(shift))%26
		case r >= 'A' && r <= 'Z':
			runes[i] = 'A' + (r-'A'+rune(shift))%26
		}
	}
	return string(runes)
}
