package checksum

import (
	"crypto/rand"
	sha3std "crypto/sha3"
	"fmt"
	"hash"

	"github.com/itchyny/gojq"
	"github.com/xen0bit/pwrq/pkg/udf/common"
	"golang.org/x/crypto/argon2"
	"golang.org/x/crypto/pbkdf2"
	xsha3 "golang.org/x/crypto/sha3"
)

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
	return gojq.WithFunction("pbkdf2_sha256", 1, 3, func(v any, args []any) any {
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
	return gojq.WithFunction("argon2id_hash", 1, 3, func(v any, args []any) any {
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
	return gojq.WithFunction("random_hex", 0, 1, func(v any, args []any) any {
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
