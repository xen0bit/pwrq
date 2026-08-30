// Package checksum provides fast integrity checksums (CRC, FNV, Adler), the
// BLAKE2b hash family, and bcrypt hashing and verification for passwords.
package checksum

import (
	"crypto/rand"
	sha3std "crypto/sha3"
	"fmt"
	"hash"
	"hash/adler32"
	"hash/crc32"
	"hash/crc64"
	"hash/fnv"

	"github.com/itchyny/gojq"
	"github.com/xen0bit/pwrq/pkg/udf/common"
	"golang.org/x/crypto/argon2"
	"golang.org/x/crypto/bcrypt"
	"golang.org/x/crypto/blake2b"
	"golang.org/x/crypto/pbkdf2"
	xsha3 "golang.org/x/crypto/sha3"
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
		RegisterSHA3_256(),
		RegisterSHA3_512(),
		RegisterKeccak256(),
		RegisterCRC16(),
		RegisterPBKDF2SHA256(),
		RegisterArgon2ID(),
		RegisterRandomHex(),
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
	// The %x below is the whole reason the catalogue can say these are hex, so
	// the declaration goes beside it rather than at each of the four call
	// sites, where one of them would eventually be forgotten.
	common.DeclareEncoding(name, common.EncodingHex, "")
	return common.WithFunction(name, 0, 2, func(v any, args []any) any {
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
	common.DeclareEncoding("blake2b_256", common.EncodingHex, "")
	return common.WithFunction("blake2b_256", 0, 2, func(v any, args []any) any {
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
	common.DeclareEncoding("blake2b_512", common.EncodingHex, "")
	return common.WithFunction("blake2b_512", 0, 2, func(v any, args []any) any {
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
	return common.WithFunction("bcrypt_hash", 0, 2, func(v any, args []any) any {
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
	return common.WithFunction("bcrypt_verify", 1, 2, func(v any, args []any) any {
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

// RegisterSHA3_256 registers sha3_256, the SHA-3-256 hash.
func RegisterSHA3_256() gojq.CompilerOption {
	return registerDigest("sha3_256", func() hash.Hash { return sha3std.New256() })
}

// RegisterSHA3_512 registers sha3_512, the SHA-3-512 hash.
func RegisterSHA3_512() gojq.CompilerOption {
	return registerDigest("sha3_512", func() hash.Hash { return sha3std.New512() })
}

// RegisterKeccak256 registers keccak_256, the pre-standardization Keccak-256
// (what Ethereum uses).
func RegisterKeccak256() gojq.CompilerOption {
	return registerDigest("keccak_256", xsha3.NewLegacyKeccak256)
}

// crc16 is CRC-16/CCITT-FALSE, the checksum used by XMODEM and many legacy
// protocols.
type crc16 struct {
	crc uint16
}

var crc16Table = func() [256]uint16 {
	var table [256]uint16
	for i := 0; i < 256; i++ {
		crc := uint16(i) << 8
		for j := 0; j < 8; j++ {
			if crc&0x8000 != 0 {
				crc = crc<<1 ^ 0x1021
			} else {
				crc <<= 1
			}
		}
		table[i] = crc
	}
	return table
}()

func (c *crc16) Write(p []byte) (int, error) {
	for _, b := range p {
		c.crc = c.crc<<8 ^ crc16Table[byte(c.crc>>8)^b]
	}
	return len(p), nil
}

func (c *crc16) Sum(b []byte) []byte {
	return append(b, byte(c.crc>>8), byte(c.crc))
}

func (c *crc16) Reset() { c.crc = 0xFFFF }

func (c *crc16) Size() int { return 2 }

func (c *crc16) BlockSize() int { return 1 }

// RegisterCRC16 registers crc16, the CRC-16/CCITT-FALSE checksum.
func RegisterCRC16() gojq.CompilerOption {
	return registerDigest("crc16", func() hash.Hash { return &crc16{crc: 0xFFFF} })
}

// RegisterPBKDF2SHA256 registers pbkdf2_sha256, a password-derived key from the
// PBKDF2 key derivation function with SHA-256, as hex.
func RegisterPBKDF2SHA256() gojq.CompilerOption {
	common.DeclareEncoding("pbkdf2_sha256", common.EncodingHex, "")
	return common.WithFunction("pbkdf2_sha256", 1, 3, func(v any, args []any) any {
		salt, ok := common.BindValue(args[0]).(string)
		if !ok {
			return common.MakeUDFErrorResult(fmt.Errorf("pbkdf2_sha256: salt must be a string, got %T", args[0]), nil)
		}
		iterations := 100000
		if len(args) > 1 {
			if n, ok := common.ToInt(args[1]); ok {
				iterations = n
			}
		}
		keyLen := 32
		if len(args) > 2 {
			if n, ok := common.ToInt(args[2]); ok {
				keyLen = n
			}
		}
		if iterations <= 0 || keyLen <= 0 {
			return common.MakeUDFErrorResult(fmt.Errorf("pbkdf2_sha256: iterations and key length must be positive"), nil)
		}
		password, ok := common.BindValue(v).(string)
		if !ok {
			return common.MakeUDFErrorResult(fmt.Errorf("pbkdf2_sha256: password must be a string, got %T", v), nil)
		}
		key := pbkdf2.Key([]byte(password), []byte(salt), iterations, keyLen, func() hash.Hash { return sha3std.New256() })
		return common.MakeUDFSuccessResult(fmt.Sprintf("%x", key), nil)
	})
}

// RegisterArgon2ID registers argon2id_hash, a password-derived key from the
// Argon2id memory-hard function, as hex.
func RegisterArgon2ID() gojq.CompilerOption {
	common.DeclareEncoding("argon2id_hash", common.EncodingHex, "")
	return common.WithFunction("argon2id_hash", 1, 3, func(v any, args []any) any {
		salt, ok := common.BindValue(args[0]).(string)
		if !ok {
			return common.MakeUDFErrorResult(fmt.Errorf("argon2id_hash: salt must be a string, got %T", args[0]), nil)
		}
		argonTime := uint32(1)
		argonMemory := uint32(8 << 10) // 8 MiB in KiB
		keyLen := uint32(32)
		if len(args) > 1 {
			if n, ok := common.ToInt(args[1]); ok {
				argonTime = uint32(n)
			}
		}
		if len(args) > 2 {
			if n, ok := common.ToInt(args[2]); ok {
				argonMemory = uint32(n) << 10
			}
		}
		if len(args) > 3 {
			if n, ok := common.ToInt(args[3]); ok {
				keyLen = uint32(n)
			}
		}
		password, ok := common.BindValue(v).(string)
		if !ok {
			return common.MakeUDFErrorResult(fmt.Errorf("argon2id_hash: password must be a string, got %T", v), nil)
		}
		key := argon2.IDKey([]byte(password), []byte(salt), argonTime, argonMemory, 1, keyLen)
		return common.MakeUDFSuccessResult(fmt.Sprintf("%x", key), nil)
	})
}

// RegisterRandomHex registers random_hex, n cryptographically random bytes as a
// hex string.
func RegisterRandomHex() gojq.CompilerOption {
	common.DeclareEncoding("random_hex", common.EncodingHex, "")
	return common.WithFunction("random_hex", 0, 1, func(v any, args []any) any {
		n := 16
		if len(args) > 0 {
			if m, ok := common.ToInt(args[0]); ok && m >= 0 {
				n = m
			}
		}
		bytes := make([]byte, n)
		if _, err := rand.Read(bytes); err != nil {
			return common.MakeUDFErrorResult(err, nil)
		}
		return common.MakeUDFSuccessResult(fmt.Sprintf("%x", bytes), nil)
	})
}
