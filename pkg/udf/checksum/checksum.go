// Package checksum provides fast integrity checksums (CRC, FNV, Adler), the
// BLAKE2b hash family, and bcrypt hashing and verification for passwords.
package checksum

import (
	"fmt"
	"hash"
	"hash/adler32"
	"hash/crc32"
	"hash/crc64"
	"hash/fnv"

	"github.com/itchyny/gojq"
	"github.com/xen0bit/pwrq/pkg/udf/common"
	"golang.org/x/crypto/bcrypt"
	"golang.org/x/crypto/blake2b"
)

// RegisterAll registers every checksum cmdlet.
func RegisterAll() []gojq.CompilerOption {
	return []gojq.CompilerOption{
		RegisterCRC32(),
		RegisterCRC32C(),
		RegisterCRC64(),
		RegisterFNV1a(),
		RegisterAdler32(),
		RegisterBlake2b256(),
		RegisterBlake2b512(),
		RegisterBcryptHash(),
		RegisterBcryptVerify(),
	}
}

// inputBytes resolves a string (or its bytes) from the pipeline, with an
// optional file flag.
func inputBytes(v any, args []any, name string) ([]byte, error) {
	inputVal, _, err := common.ParseFileArgs(v, args)
	if err != nil {
		return nil, err
	}
	switch val := common.BindValue(inputVal).(type) {
	case string:
		return []byte(val), nil
	case []byte:
		return val, nil
	default:
		return nil, fmt.Errorf("expected a string, got %T", inputVal)
	}
}

// registerDigest builds a 0-2 arity cmdlet that hex-encodes a hash over its
// input.
func registerDigest(name string, newHash func() hash.Hash) gojq.CompilerOption {
	return gojq.WithFunction(name, 0, 2, func(v any, args []any) any {
		data, err := inputBytes(v, args, name)
		if err != nil {
			return common.MakeUDFErrorResult(fmt.Errorf("%s: %v", name, err), nil)
		}
		h := newHash()
		h.Write(data)
		return common.MakeUDFSuccessResult(fmt.Sprintf("%x", h.Sum(nil)), nil)
	})
}

// RegisterCRC32 registers crc32, the IEEE CRC-32 checksum.
func RegisterCRC32() gojq.CompilerOption {
	return registerDigest("crc32", func() hash.Hash { return crc32.NewIEEE() })
}

// RegisterCRC32C registers crc32c, the Castagnoli CRC-32 checksum.
func RegisterCRC32C() gojq.CompilerOption {
	return registerDigest("crc32c", func() hash.Hash {
		return crc32.New(crc32.MakeTable(crc32.Castagnoli))
	})
}

// RegisterCRC64 registers crc64, the ECMA CRC-64 checksum.
func RegisterCRC64() gojq.CompilerOption {
	return registerDigest("crc64", func() hash.Hash {
		return crc64.New(crc64.MakeTable(crc64.ECMA))
	})
}

// RegisterFNV1a registers fnv1a, the 64-bit FNV-1a hash.
func RegisterFNV1a() gojq.CompilerOption {
	return registerDigest("fnv1a", func() hash.Hash { return fnv.New64a() })
}

// RegisterAdler32 registers adler32, the Adler-32 checksum.
func RegisterAdler32() gojq.CompilerOption {
	return registerDigest("adler32", func() hash.Hash { return adler32.New() })
}

// RegisterBlake2b256 registers blake2b_256, BLAKE2b truncated to 256 bits.
func RegisterBlake2b256() gojq.CompilerOption {
	return gojq.WithFunction("blake2b_256", 0, 2, func(v any, args []any) any {
		data, err := inputBytes(v, args, "blake2b_256")
		if err != nil {
			return common.MakeUDFErrorResult(fmt.Errorf("blake2b_256: %v", err), nil)
		}
		sum := blake2b.Sum256(data)
		return common.MakeUDFSuccessResult(fmt.Sprintf("%x", sum[:]), nil)
	})
}

// RegisterBlake2b512 registers blake2b_512, full BLAKE2b-512.
func RegisterBlake2b512() gojq.CompilerOption {
	return gojq.WithFunction("blake2b_512", 0, 2, func(v any, args []any) any {
		data, err := inputBytes(v, args, "blake2b_512")
		if err != nil {
			return common.MakeUDFErrorResult(fmt.Errorf("blake2b_512: %v", err), nil)
		}
		sum := blake2b.Sum512(data)
		return common.MakeUDFSuccessResult(fmt.Sprintf("%x", sum[:]), nil)
	})
}

// RegisterBcryptHash registers bcrypt_hash, a bcrypt password hash with an
// optional cost (default 10).
func RegisterBcryptHash() gojq.CompilerOption {
	return gojq.WithFunction("bcrypt_hash", 0, 2, func(v any, args []any) any {
		cost := bcrypt.DefaultCost
		isFile := false
		if len(args) > 0 {
			if b, ok := args[0].(bool); ok {
				isFile = b
			} else if n, ok := common.ToInt(args[0]); ok {
				cost = n
			}
		}
		if len(args) > 1 {
			if b, ok := args[1].(bool); ok {
				isFile = b
			}
		}
		var data []byte
		if isFile {
			path, ok := common.BindPath(v)
			if !ok {
				return common.MakeUDFErrorResult(fmt.Errorf("bcrypt_hash: file argument requires a string path"), nil)
			}
			fileData, _, _, err := common.ReadFileFromPath(path)
			if err != nil {
				return common.MakeUDFErrorResult(fmt.Errorf("bcrypt_hash: %v", err), nil)
			}
			data = fileData
		} else {
			switch val := common.BindValue(v).(type) {
			case string:
				data = []byte(val)
			case []byte:
				data = val
			default:
				return common.MakeUDFErrorResult(fmt.Errorf("bcrypt_hash: expected a string, got %T", v), nil)
			}
		}
		hashed, err := bcrypt.GenerateFromPassword(data, cost)
		if err != nil {
			return common.MakeUDFErrorResult(fmt.Errorf("bcrypt_hash: %v", err), nil)
		}
		return common.MakeUDFSuccessResult(string(hashed), nil)
	})
}

// RegisterBcryptVerify registers bcrypt_verify, whether a password matches a
// bcrypt hash.
func RegisterBcryptVerify() gojq.CompilerOption {
	return gojq.WithFunction("bcrypt_verify", 1, 2, func(v any, args []any) any {
		hashValue, ok := common.BindValue(args[0]).(string)
		if !ok {
			return common.MakeUDFErrorResult(fmt.Errorf("bcrypt_verify: hash must be a string, got %T", args[0]), nil)
		}
		password, ok := common.BindValue(v).(string)
		if !ok {
			return common.MakeUDFErrorResult(fmt.Errorf("bcrypt_verify: expected a password string, got %T", v), nil)
		}
		err := bcrypt.CompareHashAndPassword([]byte(hashValue), []byte(password))
		return common.MakeUDFSuccessResult(err == nil, nil)
	})
}
