package crypto

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/des"
	"crypto/rc4"
	"encoding/base64"
	"encoding/hex"
	"fmt"
	"strings"

	"github.com/itchyny/gojq"
	"github.com/xen0bit/pwrq/pkg/udf/common"
	"golang.org/x/crypto/blowfish"
	"golang.org/x/crypto/chacha20"
)

// Common encryption/decryption helper functions

// parseKey parses a key from string or bytes, with optional hex/base64 decoding
func parseKey(keyInput any, keyFormat string) ([]byte, error) {
	var keyBytes []byte

	keyVal := common.ExtractUDFValue(keyInput)
	switch val := keyVal.(type) {
	case string:
		keyBytes = []byte(val)
	case []byte:
		keyBytes = val
	default:
		return nil, fmt.Errorf("key must be a string or bytes, got %T", val)
	}

	// Decode key if format is specified
	switch strings.ToLower(keyFormat) {
	case "hex":
		decoded, err := hex.DecodeString(string(keyBytes))
		if err != nil {
			return nil, fmt.Errorf("failed to decode hex key: %v", err)
		}
		return decoded, nil
	case "base64":
		decoded, err := base64.StdEncoding.DecodeString(string(keyBytes))
		if err != nil {
			return nil, fmt.Errorf("failed to decode base64 key: %v", err)
		}
		return decoded, nil
	case "raw", "":
		return keyBytes, nil
	default:
		return nil, fmt.Errorf("unsupported key format: %s (use 'hex', 'base64', or 'raw')", keyFormat)
	}
}

// parseData parses input data from string or bytes, with optional hex/base64 decoding
func parseData(dataInput any, dataFormat string) ([]byte, error) {
	var dataBytes []byte

	dataVal := common.ExtractUDFValue(dataInput)
	switch val := dataVal.(type) {
	case string:
		dataBytes = []byte(val)
	case []byte:
		dataBytes = val
	default:
		return nil, fmt.Errorf("data must be a string or bytes, got %T", val)
	}

	// Decode data if format is specified
	switch strings.ToLower(dataFormat) {
	case "hex":
		decoded, err := hex.DecodeString(string(dataBytes))
		if err != nil {
			return nil, fmt.Errorf("failed to decode hex data: %v", err)
		}
		return decoded, nil
	case "base64":
		decoded, err := base64.StdEncoding.DecodeString(string(dataBytes))
		if err != nil {
			return nil, fmt.Errorf("failed to decode base64 data: %v", err)
		}
		return decoded, nil
	case "raw", "":
		return dataBytes, nil
	default:
		return nil, fmt.Errorf("unsupported data format: %s (use 'hex', 'base64', or 'raw')", dataFormat)
	}
}

// pkcs7Pad adds PKCS7 padding
func pkcs7Pad(data []byte, blockSize int) []byte {
	padding := blockSize - len(data)%blockSize
	padtext := make([]byte, padding)
	for i := range padtext {
		padtext[i] = byte(padding)
	}
	return append(data, padtext...)
}

// pkcs7Unpad removes PKCS7 padding
func pkcs7Unpad(data []byte) ([]byte, error) {
	if len(data) == 0 {
		return nil, fmt.Errorf("data is empty")
	}
	padding := int(data[len(data)-1])
	if padding > len(data) || padding == 0 {
		return nil, fmt.Errorf("invalid padding")
	}
	for i := len(data) - padding; i < len(data); i++ {
		if data[i] != byte(padding) {
			return nil, fmt.Errorf("invalid padding")
		}
	}
	return data[:len(data)-padding], nil
}

// AES Encryption/Decryption

// RegisterAESEncrypt registers AES encryption function
func RegisterAESEncrypt() gojq.CompilerOption {
	return gojq.WithFunction("aes_encrypt", 2, 5, func(v any, args []any) any {
		if len(args) < 2 {
			return common.MakeUDFErrorResult(fmt.Errorf("aes_encrypt: requires at least 2 arguments (data, key)"), nil)
		}

		// Parse arguments: data, key, mode (default CBC), keyFormat (default raw), dataFormat (default raw)
		var dataInput any
		if len(args) > 0 {
			dataInput = args[0]
		} else {
			dataInput = v
		}
		dataInput = common.ExtractUDFValue(dataInput)

		if len(args) < 2 {
			return common.MakeUDFErrorResult(fmt.Errorf("aes_encrypt: requires at least 2 arguments (data, key)"), nil)
		}
		keyInput := args[1]
		mode := "CBC"
		keyFormat := "raw"
		dataFormat := "raw"

		if len(args) > 2 {
			if modeStr, ok := args[2].(string); ok {
				mode = strings.ToUpper(modeStr)
			} else {
				modeVal := common.ExtractUDFValue(args[2])
				if modeStr, ok := modeVal.(string); ok {
					mode = strings.ToUpper(modeStr)
				}
			}
		}
		if len(args) > 3 {
			if fmtStr, ok := args[3].(string); ok {
				keyFormat = fmtStr
			}
		}
		if len(args) > 4 {
			if fmtStr, ok := args[4].(string); ok {
				dataFormat = fmtStr
			}
		}

		key, err := parseKey(keyInput, keyFormat)
		if err != nil {
			return common.MakeUDFErrorResult(fmt.Errorf("aes_encrypt: %v", err), nil)
		}

		data, err := parseData(dataInput, dataFormat)
		if err != nil {
			return common.MakeUDFErrorResult(fmt.Errorf("aes_encrypt: %v", err), nil)
		}

		// Validate key size
		validKeySizes := map[int]bool{16: true, 24: true, 32: true} // 128, 192, 256 bits
		if !validKeySizes[len(key)] {
			return common.MakeUDFErrorResult(fmt.Errorf("aes_encrypt: invalid key size %d bytes (must be 16, 24, or 32)", len(key)), nil)
		}

		block, err := aes.NewCipher(key)
		if err != nil {
			return common.MakeUDFErrorResult(fmt.Errorf("aes_encrypt: failed to create cipher: %v", err), nil)
		}

		var ciphertext []byte
		var iv []byte

		switch mode {
		case "ECB":
			// ECB mode (no IV)
			blockSize := block.BlockSize()
			padded := pkcs7Pad(data, blockSize)
			ciphertext = make([]byte, len(padded))
			for i := 0; i < len(padded); i += blockSize {
				block.Encrypt(ciphertext[i:i+blockSize], padded[i:i+blockSize])
			}
		case "CBC":
			// Generate random IV
			iv = make([]byte, aes.BlockSize)
			for i := range iv {
				iv[i] = byte(i) // Simple IV for demo - in production use crypto/rand
			}
			mode := cipher.NewCBCEncrypter(block, iv)
			padded := pkcs7Pad(data, aes.BlockSize)
			ciphertext = make([]byte, len(padded))
			mode.CryptBlocks(ciphertext, padded)
		case "CFB":
			iv = make([]byte, aes.BlockSize)
			for i := range iv {
				iv[i] = byte(i)
			}
			stream := cipher.NewCFBEncrypter(block, iv)
			ciphertext = make([]byte, len(data))
			stream.XORKeyStream(ciphertext, data)
		case "OFB":
			iv = make([]byte, aes.BlockSize)
			for i := range iv {
				iv[i] = byte(i)
			}
			stream := cipher.NewOFB(block, iv)
			ciphertext = make([]byte, len(data))
			stream.XORKeyStream(ciphertext, data)
		case "CTR":
			iv = make([]byte, aes.BlockSize)
			for i := range iv {
				iv[i] = byte(i)
			}
			stream := cipher.NewCTR(block, iv)
			ciphertext = make([]byte, len(data))
			stream.XORKeyStream(ciphertext, data)
		default:
			return common.MakeUDFErrorResult(fmt.Errorf("aes_encrypt: unsupported mode %s (use ECB, CBC, CFB, OFB, or CTR)", mode), nil)
		}

		// Prepend IV for modes that use it
		if iv != nil {
			ciphertext = append(iv, ciphertext...)
		}

		// Return base64 encoded result
		result := base64.StdEncoding.EncodeToString(ciphertext)

		meta := map[string]any{
			"operation": "aes_encrypt",
			"mode":      mode,
			"key_size":  len(key),
		}
		if iv != nil {
			meta["iv_length"] = len(iv)
		}

		return common.MakeUDFSuccessResult(result, meta)
	})
}

// RegisterAESDecrypt registers AES decryption function
func RegisterAESDecrypt() gojq.CompilerOption {
	return gojq.WithFunction("aes_decrypt", 2, 5, func(v any, args []any) any {
		if len(args) < 2 {
			return common.MakeUDFErrorResult(fmt.Errorf("aes_decrypt: requires at least 2 arguments (data, key)"), nil)
		}

		// Parse arguments: data, key, mode (default CBC), keyFormat (default raw), dataFormat (default base64)
		dataInput := common.ExtractUDFValue(v)
		if len(args) > 0 {
			dataInput = common.ExtractUDFValue(args[0])
		}

		keyInput := args[1]
		mode := "CBC"
		keyFormat := "raw"
		dataFormat := "base64" // Default to base64 for encrypted data

		if len(args) > 2 {
			if modeStr, ok := args[2].(string); ok {
				mode = strings.ToUpper(modeStr)
			} else {
				modeVal := common.ExtractUDFValue(args[2])
				if modeStr, ok := modeVal.(string); ok {
					mode = strings.ToUpper(modeStr)
				}
			}
		}
		if len(args) > 3 {
			if fmtStr, ok := args[3].(string); ok {
				keyFormat = fmtStr
			}
		}
		if len(args) > 4 {
			if fmtStr, ok := args[4].(string); ok {
				dataFormat = fmtStr
			}
		}

		key, err := parseKey(keyInput, keyFormat)
		if err != nil {
			return common.MakeUDFErrorResult(fmt.Errorf("aes_decrypt: %v", err), nil)
		}

		ciphertext, err := parseData(dataInput, dataFormat)
		if err != nil {
			return common.MakeUDFErrorResult(fmt.Errorf("aes_decrypt: %v", err), nil)
		}

		// Validate key size
		validKeySizes := map[int]bool{16: true, 24: true, 32: true}
		if !validKeySizes[len(key)] {
			return common.MakeUDFErrorResult(fmt.Errorf("aes_decrypt: invalid key size %d bytes (must be 16, 24, or 32)", len(key)), nil)
		}

		block, err := aes.NewCipher(key)
		if err != nil {
			return common.MakeUDFErrorResult(fmt.Errorf("aes_decrypt: failed to create cipher: %v", err), nil)
		}

		var plaintext []byte
		var iv []byte

		switch mode {
		case "ECB":
			// ECB mode (no IV)
			blockSize := block.BlockSize()
			if len(ciphertext)%blockSize != 0 {
				return common.MakeUDFErrorResult(fmt.Errorf("aes_decrypt: ciphertext length must be multiple of %d", blockSize), nil)
			}
			plaintext = make([]byte, len(ciphertext))
			for i := 0; i < len(ciphertext); i += blockSize {
				block.Decrypt(plaintext[i:i+blockSize], ciphertext[i:i+blockSize])
			}
			plaintext, err = pkcs7Unpad(plaintext)
			if err != nil {
				return common.MakeUDFErrorResult(fmt.Errorf("aes_decrypt: failed to unpad: %v", err), nil)
			}
		case "CBC":
			if len(ciphertext) < aes.BlockSize {
				return common.MakeUDFErrorResult(fmt.Errorf("aes_decrypt: ciphertext too short"), nil)
			}
			iv = ciphertext[:aes.BlockSize]
			ciphertext = ciphertext[aes.BlockSize:]
			if len(ciphertext)%aes.BlockSize != 0 {
				return common.MakeUDFErrorResult(fmt.Errorf("aes_decrypt: ciphertext length must be multiple of %d", aes.BlockSize), nil)
			}
			mode := cipher.NewCBCDecrypter(block, iv)
			plaintext = make([]byte, len(ciphertext))
			mode.CryptBlocks(plaintext, ciphertext)
			plaintext, err = pkcs7Unpad(plaintext)
			if err != nil {
				return common.MakeUDFErrorResult(fmt.Errorf("aes_decrypt: failed to unpad: %v", err), nil)
			}
		case "CFB":
			if len(ciphertext) < aes.BlockSize {
				return common.MakeUDFErrorResult(fmt.Errorf("aes_decrypt: ciphertext too short"), nil)
			}
			iv = ciphertext[:aes.BlockSize]
			ciphertext = ciphertext[aes.BlockSize:]
			stream := cipher.NewCFBDecrypter(block, iv)
			plaintext = make([]byte, len(ciphertext))
			stream.XORKeyStream(plaintext, ciphertext)
		case "OFB":
			if len(ciphertext) < aes.BlockSize {
				return common.MakeUDFErrorResult(fmt.Errorf("aes_decrypt: ciphertext too short"), nil)
			}
			iv = ciphertext[:aes.BlockSize]
			ciphertext = ciphertext[aes.BlockSize:]
			stream := cipher.NewOFB(block, iv)
			plaintext = make([]byte, len(ciphertext))
			stream.XORKeyStream(plaintext, ciphertext)
		case "CTR":
			if len(ciphertext) < aes.BlockSize {
				return common.MakeUDFErrorResult(fmt.Errorf("aes_decrypt: ciphertext too short"), nil)
			}
			iv = ciphertext[:aes.BlockSize]
			ciphertext = ciphertext[aes.BlockSize:]
			stream := cipher.NewCTR(block, iv)
			plaintext = make([]byte, len(ciphertext))
			stream.XORKeyStream(plaintext, ciphertext)
		default:
			return common.MakeUDFErrorResult(fmt.Errorf("aes_decrypt: unsupported mode %s (use ECB, CBC, CFB, OFB, or CTR)", mode), nil)
		}

		result := string(plaintext)

		meta := map[string]any{
			"operation": "aes_decrypt",
			"mode":      mode,
			"key_size":  len(key),
		}

		return common.MakeUDFSuccessResult(result, meta)
	})
}

// XOR Encryption/Decryption (same operation)

// RegisterXOR registers XOR encryption/decryption function
func RegisterXOR() gojq.CompilerOption {
	return gojq.WithFunction("xor", 1, 3, func(v any, args []any) any {
		if len(args) < 1 {
			return common.MakeUDFErrorResult(fmt.Errorf("xor: requires at least 1 argument (key)"), nil)
		}

		dataInput := common.ExtractUDFValue(v)
		keyInput := args[0]
		keyFormat := "raw"
		dataFormat := "raw"

		if len(args) > 1 {
			if fmtStr, ok := args[1].(string); ok {
				keyFormat = fmtStr
			}
		}
		if len(args) > 2 {
			if fmtStr, ok := args[2].(string); ok {
				dataFormat = fmtStr
			}
		}

		key, err := parseKey(keyInput, keyFormat)
		if err != nil {
			return common.MakeUDFErrorResult(fmt.Errorf("xor: %v", err), nil)
		}

		data, err := parseData(dataInput, dataFormat)
		if err != nil {
			return common.MakeUDFErrorResult(fmt.Errorf("xor: %v", err), nil)
		}

		if len(key) == 0 {
			return common.MakeUDFErrorResult(fmt.Errorf("xor: key cannot be empty"), nil)
		}

		// XOR operation
		result := make([]byte, len(data))
		for i := range data {
			result[i] = data[i] ^ key[i%len(key)]
		}

		// Return as hex string
		resultHex := hex.EncodeToString(result)

		meta := map[string]any{
			"operation": "xor",
			"key_size":   len(key),
		}

		return common.MakeUDFSuccessResult(resultHex, meta)
	})
}

// RC4 Encryption/Decryption (same operation)

// RegisterRC4 registers RC4 encryption/decryption function
func RegisterRC4() gojq.CompilerOption {
	return gojq.WithFunction("rc4", 1, 3, func(v any, args []any) any {
		if len(args) < 1 {
			return common.MakeUDFErrorResult(fmt.Errorf("rc4: requires at least 1 argument (key)"), nil)
		}

		dataInput := common.ExtractUDFValue(v)
		keyInput := args[0]
		keyFormat := "raw"
		dataFormat := "raw"

		if len(args) > 1 {
			if fmtStr, ok := args[1].(string); ok {
				keyFormat = fmtStr
			}
		}
		if len(args) > 2 {
			if fmtStr, ok := args[2].(string); ok {
				dataFormat = fmtStr
			}
		}

		key, err := parseKey(keyInput, keyFormat)
		if err != nil {
			return common.MakeUDFErrorResult(fmt.Errorf("rc4: %v", err), nil)
		}

		data, err := parseData(dataInput, dataFormat)
		if err != nil {
			return common.MakeUDFErrorResult(fmt.Errorf("rc4: %v", err), nil)
		}

		cipher, err := rc4.NewCipher(key)
		if err != nil {
			return common.MakeUDFErrorResult(fmt.Errorf("rc4: failed to create cipher: %v", err), nil)
		}

		result := make([]byte, len(data))
		cipher.XORKeyStream(result, data)

		// Return as base64 string
		resultB64 := base64.StdEncoding.EncodeToString(result)

		meta := map[string]any{
			"operation": "rc4",
			"key_size":  len(key),
		}

		return common.MakeUDFSuccessResult(resultB64, meta)
	})
}

// ChaCha20 Encryption/Decryption

// RegisterChaCha20 registers ChaCha20 encryption/decryption function
func RegisterChaCha20() gojq.CompilerOption {
	return gojq.WithFunction("chacha20", 1, 4, func(v any, args []any) any {
		if len(args) < 1 {
			return common.MakeUDFErrorResult(fmt.Errorf("chacha20: requires at least 1 argument (key)"), nil)
		}

		dataInput := common.ExtractUDFValue(v)
		keyInput := args[0]
		keyFormat := "raw"
		dataFormat := "raw"
		var nonce []byte

		if len(args) > 1 {
			// Second arg can be nonce or keyFormat
			if nonceVal, ok := args[1].(string); ok {
				// Try to parse as nonce
				nonceBytes, err := hex.DecodeString(nonceVal)
				if err == nil && len(nonceBytes) == 12 {
					nonce = nonceBytes
				} else {
					keyFormat = nonceVal
				}
			}
		}
		if len(args) > 2 {
			if fmtStr, ok := args[2].(string); ok {
				if nonce == nil {
					keyFormat = fmtStr
				} else {
					dataFormat = fmtStr
				}
			}
		}
		if len(args) > 3 {
			if fmtStr, ok := args[3].(string); ok {
				dataFormat = fmtStr
			}
		}

		key, err := parseKey(keyInput, keyFormat)
		if err != nil {
			return common.MakeUDFErrorResult(fmt.Errorf("chacha20: %v", err), nil)
		}

		if len(key) != 32 {
			return common.MakeUDFErrorResult(fmt.Errorf("chacha20: key must be 32 bytes (256 bits), got %d", len(key)), nil)
		}

		data, err := parseData(dataInput, dataFormat)
		if err != nil {
			return common.MakeUDFErrorResult(fmt.Errorf("chacha20: %v", err), nil)
		}

		// Generate nonce if not provided
		if nonce == nil {
			nonce = make([]byte, 12)
			for i := range nonce {
				nonce[i] = byte(i) // Simple nonce for demo
			}
		}

		cipher, err := chacha20.NewUnauthenticatedCipher(key, nonce)
		if err != nil {
			return common.MakeUDFErrorResult(fmt.Errorf("chacha20: failed to create cipher: %v", err), nil)
		}

		result := make([]byte, len(data))
		cipher.XORKeyStream(result, data)

		// Prepend nonce and return as base64
		resultWithNonce := append(nonce, result...)
		resultB64 := base64.StdEncoding.EncodeToString(resultWithNonce)

		meta := map[string]any{
			"operation": "chacha20",
			"key_size":   len(key),
			"nonce_size": len(nonce),
		}

		return common.MakeUDFSuccessResult(resultB64, meta)
	})
}

// DES Encryption/Decryption

// RegisterDESEncrypt registers DES encryption function
func RegisterDESEncrypt() gojq.CompilerOption {
	return gojq.WithFunction("des_encrypt", 2, 4, func(v any, args []any) any {
		if len(args) < 2 {
			return common.MakeUDFErrorResult(fmt.Errorf("des_encrypt: requires at least 2 arguments (data, key)"), nil)
		}

		dataInput := common.ExtractUDFValue(v)
		if len(args) > 0 {
			dataInput = common.ExtractUDFValue(args[0])
		}

		keyInput := args[1]
		mode := "CBC"
		keyFormat := "raw"
		dataFormat := "raw"

		if len(args) > 2 {
			if modeStr, ok := args[2].(string); ok {
				mode = strings.ToUpper(modeStr)
			}
		}
		if len(args) > 3 {
			if fmtStr, ok := args[3].(string); ok {
				keyFormat = fmtStr
			}
		}

		key, err := parseKey(keyInput, keyFormat)
		if err != nil {
			return common.MakeUDFErrorResult(fmt.Errorf("des_encrypt: %v", err), nil)
		}

		if len(key) != 8 {
			return common.MakeUDFErrorResult(fmt.Errorf("des_encrypt: key must be 8 bytes (64 bits), got %d", len(key)), nil)
		}

		data, err := parseData(dataInput, dataFormat)
		if err != nil {
			return common.MakeUDFErrorResult(fmt.Errorf("des_encrypt: %v", err), nil)
		}

		block, err := des.NewCipher(key)
		if err != nil {
			return common.MakeUDFErrorResult(fmt.Errorf("des_encrypt: failed to create cipher: %v", err), nil)
		}

		var ciphertext []byte
		var iv []byte

		switch mode {
		case "ECB":
			blockSize := block.BlockSize()
			padded := pkcs7Pad(data, blockSize)
			ciphertext = make([]byte, len(padded))
			for i := 0; i < len(padded); i += blockSize {
				block.Encrypt(ciphertext[i:i+blockSize], padded[i:i+blockSize])
			}
		case "CBC":
			iv = make([]byte, des.BlockSize)
			for i := range iv {
				iv[i] = byte(i)
			}
			mode := cipher.NewCBCEncrypter(block, iv)
			padded := pkcs7Pad(data, des.BlockSize)
			ciphertext = make([]byte, len(padded))
			mode.CryptBlocks(ciphertext, padded)
		default:
			return common.MakeUDFErrorResult(fmt.Errorf("des_encrypt: unsupported mode %s (use ECB or CBC)", mode), nil)
		}

		if iv != nil {
			ciphertext = append(iv, ciphertext...)
		}

		result := base64.StdEncoding.EncodeToString(ciphertext)

		meta := map[string]any{
			"operation": "des_encrypt",
			"mode":      mode,
			"key_size":  len(key),
		}

		return common.MakeUDFSuccessResult(result, meta)
	})
}

// RegisterDESDecrypt registers DES decryption function
func RegisterDESDecrypt() gojq.CompilerOption {
	return gojq.WithFunction("des_decrypt", 2, 4, func(v any, args []any) any {
		if len(args) < 2 {
			return common.MakeUDFErrorResult(fmt.Errorf("des_decrypt: requires at least 2 arguments (data, key)"), nil)
		}

		dataInput := common.ExtractUDFValue(v)
		if len(args) > 0 {
			dataInput = common.ExtractUDFValue(args[0])
		}

		keyInput := args[1]
		mode := "CBC"
		keyFormat := "raw"
		dataFormat := "base64"

		if len(args) > 2 {
			if modeStr, ok := args[2].(string); ok {
				mode = strings.ToUpper(modeStr)
			}
		}
		if len(args) > 3 {
			if fmtStr, ok := args[3].(string); ok {
				keyFormat = fmtStr
			}
		}

		key, err := parseKey(keyInput, keyFormat)
		if err != nil {
			return common.MakeUDFErrorResult(fmt.Errorf("des_decrypt: %v", err), nil)
		}

		if len(key) != 8 {
			return common.MakeUDFErrorResult(fmt.Errorf("des_decrypt: key must be 8 bytes (64 bits), got %d", len(key)), nil)
		}

		ciphertext, err := parseData(dataInput, dataFormat)
		if err != nil {
			return common.MakeUDFErrorResult(fmt.Errorf("des_decrypt: %v", err), nil)
		}

		block, err := des.NewCipher(key)
		if err != nil {
			return common.MakeUDFErrorResult(fmt.Errorf("des_decrypt: failed to create cipher: %v", err), nil)
		}

		var plaintext []byte
		var iv []byte

		switch mode {
		case "ECB":
			blockSize := block.BlockSize()
			if len(ciphertext)%blockSize != 0 {
				return common.MakeUDFErrorResult(fmt.Errorf("des_decrypt: ciphertext length must be multiple of %d", blockSize), nil)
			}
			plaintext = make([]byte, len(ciphertext))
			for i := 0; i < len(ciphertext); i += blockSize {
				block.Decrypt(plaintext[i:i+blockSize], ciphertext[i:i+blockSize])
			}
			plaintext, err = pkcs7Unpad(plaintext)
			if err != nil {
				return common.MakeUDFErrorResult(fmt.Errorf("des_decrypt: failed to unpad: %v", err), nil)
			}
		case "CBC":
			if len(ciphertext) < des.BlockSize {
				return common.MakeUDFErrorResult(fmt.Errorf("des_decrypt: ciphertext too short"), nil)
			}
			iv = ciphertext[:des.BlockSize]
			ciphertext = ciphertext[des.BlockSize:]
			if len(ciphertext)%des.BlockSize != 0 {
				return common.MakeUDFErrorResult(fmt.Errorf("des_decrypt: ciphertext length must be multiple of %d", des.BlockSize), nil)
			}
			mode := cipher.NewCBCDecrypter(block, iv)
			plaintext = make([]byte, len(ciphertext))
			mode.CryptBlocks(plaintext, ciphertext)
			plaintext, err = pkcs7Unpad(plaintext)
			if err != nil {
				return common.MakeUDFErrorResult(fmt.Errorf("des_decrypt: failed to unpad: %v", err), nil)
			}
		default:
			return common.MakeUDFErrorResult(fmt.Errorf("des_decrypt: unsupported mode %s (use ECB or CBC)", mode), nil)
		}

		result := string(plaintext)

		meta := map[string]any{
			"operation": "des_decrypt",
			"mode":      mode,
			"key_size":  len(key),
		}

		return common.MakeUDFSuccessResult(result, meta)
	})
}

// 3DES Encryption/Decryption

// Register3DESEncrypt registers 3DES encryption function
func Register3DESEncrypt() gojq.CompilerOption {
	return gojq.WithFunction("3des_encrypt", 2, 4, func(v any, args []any) any {
		if len(args) < 2 {
			return common.MakeUDFErrorResult(fmt.Errorf("3des_encrypt: requires at least 2 arguments (data, key)"), nil)
		}

		dataInput := common.ExtractUDFValue(v)
		if len(args) > 0 {
			dataInput = common.ExtractUDFValue(args[0])
		}

		keyInput := args[1]
		mode := "CBC"
		keyFormat := "raw"
		dataFormat := "raw"

		if len(args) > 2 {
			if modeStr, ok := args[2].(string); ok {
				mode = strings.ToUpper(modeStr)
			}
		}
		if len(args) > 3 {
			if fmtStr, ok := args[3].(string); ok {
				keyFormat = fmtStr
			}
		}

		key, err := parseKey(keyInput, keyFormat)
		if err != nil {
			return common.MakeUDFErrorResult(fmt.Errorf("3des_encrypt: %v", err), nil)
		}

		// 3DES key can be 16 or 24 bytes
		if len(key) != 16 && len(key) != 24 {
			return common.MakeUDFErrorResult(fmt.Errorf("3des_encrypt: key must be 16 or 24 bytes, got %d", len(key)), nil)
		}

		data, err := parseData(dataInput, dataFormat)
		if err != nil {
			return common.MakeUDFErrorResult(fmt.Errorf("3des_encrypt: %v", err), nil)
		}

		block, err := des.NewTripleDESCipher(key)
		if err != nil {
			return common.MakeUDFErrorResult(fmt.Errorf("3des_encrypt: failed to create cipher: %v", err), nil)
		}

		var ciphertext []byte
		var iv []byte

		switch mode {
		case "ECB":
			blockSize := block.BlockSize()
			padded := pkcs7Pad(data, blockSize)
			ciphertext = make([]byte, len(padded))
			for i := 0; i < len(padded); i += blockSize {
				block.Encrypt(ciphertext[i:i+blockSize], padded[i:i+blockSize])
			}
		case "CBC":
			iv = make([]byte, des.BlockSize)
			for i := range iv {
				iv[i] = byte(i)
			}
			mode := cipher.NewCBCEncrypter(block, iv)
			padded := pkcs7Pad(data, des.BlockSize)
			ciphertext = make([]byte, len(padded))
			mode.CryptBlocks(ciphertext, padded)
		default:
			return common.MakeUDFErrorResult(fmt.Errorf("3des_encrypt: unsupported mode %s (use ECB or CBC)", mode), nil)
		}

		if iv != nil {
			ciphertext = append(iv, ciphertext...)
		}

		result := base64.StdEncoding.EncodeToString(ciphertext)

		meta := map[string]any{
			"operation": "3des_encrypt",
			"mode":      mode,
			"key_size":  len(key),
		}

		return common.MakeUDFSuccessResult(result, meta)
	})
}

// Register3DESDecrypt registers 3DES decryption function
func Register3DESDecrypt() gojq.CompilerOption {
	return gojq.WithFunction("3des_decrypt", 2, 4, func(v any, args []any) any {
		if len(args) < 2 {
			return common.MakeUDFErrorResult(fmt.Errorf("3des_decrypt: requires at least 2 arguments (data, key)"), nil)
		}

		dataInput := common.ExtractUDFValue(v)
		if len(args) > 0 {
			dataInput = common.ExtractUDFValue(args[0])
		}

		keyInput := args[1]
		mode := "CBC"
		keyFormat := "raw"
		dataFormat := "base64"

		if len(args) > 2 {
			if modeStr, ok := args[2].(string); ok {
				mode = strings.ToUpper(modeStr)
			}
		}
		if len(args) > 3 {
			if fmtStr, ok := args[3].(string); ok {
				keyFormat = fmtStr
			}
		}

		key, err := parseKey(keyInput, keyFormat)
		if err != nil {
			return common.MakeUDFErrorResult(fmt.Errorf("3des_decrypt: %v", err), nil)
		}

		if len(key) != 16 && len(key) != 24 {
			return common.MakeUDFErrorResult(fmt.Errorf("3des_decrypt: key must be 16 or 24 bytes, got %d", len(key)), nil)
		}

		ciphertext, err := parseData(dataInput, dataFormat)
		if err != nil {
			return common.MakeUDFErrorResult(fmt.Errorf("3des_decrypt: %v", err), nil)
		}

		block, err := des.NewTripleDESCipher(key)
		if err != nil {
			return common.MakeUDFErrorResult(fmt.Errorf("3des_decrypt: failed to create cipher: %v", err), nil)
		}

		var plaintext []byte
		var iv []byte

		switch mode {
		case "ECB":
			blockSize := block.BlockSize()
			if len(ciphertext)%blockSize != 0 {
				return common.MakeUDFErrorResult(fmt.Errorf("3des_decrypt: ciphertext length must be multiple of %d", blockSize), nil)
			}
			plaintext = make([]byte, len(ciphertext))
			for i := 0; i < len(ciphertext); i += blockSize {
				block.Decrypt(plaintext[i:i+blockSize], ciphertext[i:i+blockSize])
			}
			plaintext, err = pkcs7Unpad(plaintext)
			if err != nil {
				return common.MakeUDFErrorResult(fmt.Errorf("3des_decrypt: failed to unpad: %v", err), nil)
			}
		case "CBC":
			if len(ciphertext) < des.BlockSize {
				return common.MakeUDFErrorResult(fmt.Errorf("3des_decrypt: ciphertext too short"), nil)
			}
			iv = ciphertext[:des.BlockSize]
			ciphertext = ciphertext[des.BlockSize:]
			if len(ciphertext)%des.BlockSize != 0 {
				return common.MakeUDFErrorResult(fmt.Errorf("3des_decrypt: ciphertext length must be multiple of %d", des.BlockSize), nil)
			}
			mode := cipher.NewCBCDecrypter(block, iv)
			plaintext = make([]byte, len(ciphertext))
			mode.CryptBlocks(plaintext, ciphertext)
			plaintext, err = pkcs7Unpad(plaintext)
			if err != nil {
				return common.MakeUDFErrorResult(fmt.Errorf("3des_decrypt: failed to unpad: %v", err), nil)
			}
		default:
			return common.MakeUDFErrorResult(fmt.Errorf("3des_decrypt: unsupported mode %s (use ECB or CBC)", mode), nil)
		}

		result := string(plaintext)

		meta := map[string]any{
			"operation": "3des_decrypt",
			"mode":      mode,
			"key_size":  len(key),
		}

		return common.MakeUDFSuccessResult(result, meta)
	})
}

// Blowfish Encryption/Decryption

// RegisterBlowfishEncrypt registers Blowfish encryption function
func RegisterBlowfishEncrypt() gojq.CompilerOption {
	return gojq.WithFunction("blowfish_encrypt", 2, 4, func(v any, args []any) any {
		if len(args) < 2 {
			return common.MakeUDFErrorResult(fmt.Errorf("blowfish_encrypt: requires at least 2 arguments (data, key)"), nil)
		}

		dataInput := common.ExtractUDFValue(v)
		if len(args) > 0 {
			dataInput = common.ExtractUDFValue(args[0])
		}

		keyInput := args[1]
		mode := "CBC"
		keyFormat := "raw"
		dataFormat := "raw"

		if len(args) > 2 {
			if modeStr, ok := args[2].(string); ok {
				mode = strings.ToUpper(modeStr)
			}
		}
		if len(args) > 3 {
			if fmtStr, ok := args[3].(string); ok {
				keyFormat = fmtStr
			}
		}

		key, err := parseKey(keyInput, keyFormat)
		if err != nil {
			return common.MakeUDFErrorResult(fmt.Errorf("blowfish_encrypt: %v", err), nil)
		}

		if len(key) < 4 || len(key) > 56 {
			return common.MakeUDFErrorResult(fmt.Errorf("blowfish_encrypt: key must be 4-56 bytes, got %d", len(key)), nil)
		}

		data, err := parseData(dataInput, dataFormat)
		if err != nil {
			return common.MakeUDFErrorResult(fmt.Errorf("blowfish_encrypt: %v", err), nil)
		}

		block, err := blowfish.NewCipher(key)
		if err != nil {
			return common.MakeUDFErrorResult(fmt.Errorf("blowfish_encrypt: failed to create cipher: %v", err), nil)
		}

		var ciphertext []byte
		var iv []byte

		switch mode {
		case "ECB":
			blockSize := block.BlockSize()
			padded := pkcs7Pad(data, blockSize)
			ciphertext = make([]byte, len(padded))
			for i := 0; i < len(padded); i += blockSize {
				block.Encrypt(ciphertext[i:i+blockSize], padded[i:i+blockSize])
			}
		case "CBC":
			iv = make([]byte, blowfish.BlockSize)
			for i := range iv {
				iv[i] = byte(i)
			}
			mode := cipher.NewCBCEncrypter(block, iv)
			padded := pkcs7Pad(data, blowfish.BlockSize)
			ciphertext = make([]byte, len(padded))
			mode.CryptBlocks(ciphertext, padded)
		default:
			return common.MakeUDFErrorResult(fmt.Errorf("blowfish_encrypt: unsupported mode %s (use ECB or CBC)", mode), nil)
		}

		if iv != nil {
			ciphertext = append(iv, ciphertext...)
		}

		result := base64.StdEncoding.EncodeToString(ciphertext)

		meta := map[string]any{
			"operation": "blowfish_encrypt",
			"mode":      mode,
			"key_size":  len(key),
		}

		return common.MakeUDFSuccessResult(result, meta)
	})
}

// RegisterBlowfishDecrypt registers Blowfish decryption function
func RegisterBlowfishDecrypt() gojq.CompilerOption {
	return gojq.WithFunction("blowfish_decrypt", 2, 4, func(v any, args []any) any {
		if len(args) < 2 {
			return common.MakeUDFErrorResult(fmt.Errorf("blowfish_decrypt: requires at least 2 arguments (data, key)"), nil)
		}

		dataInput := common.ExtractUDFValue(v)
		if len(args) > 0 {
			dataInput = common.ExtractUDFValue(args[0])
		}

		keyInput := args[1]
		mode := "CBC"
		keyFormat := "raw"
		dataFormat := "base64"

		if len(args) > 2 {
			if modeStr, ok := args[2].(string); ok {
				mode = strings.ToUpper(modeStr)
			}
		}
		if len(args) > 3 {
			if fmtStr, ok := args[3].(string); ok {
				keyFormat = fmtStr
			}
		}

		key, err := parseKey(keyInput, keyFormat)
		if err != nil {
			return common.MakeUDFErrorResult(fmt.Errorf("blowfish_decrypt: %v", err), nil)
		}

		if len(key) < 4 || len(key) > 56 {
			return common.MakeUDFErrorResult(fmt.Errorf("blowfish_decrypt: key must be 4-56 bytes, got %d", len(key)), nil)
		}

		ciphertext, err := parseData(dataInput, dataFormat)
		if err != nil {
			return common.MakeUDFErrorResult(fmt.Errorf("blowfish_decrypt: %v", err), nil)
		}

		block, err := blowfish.NewCipher(key)
		if err != nil {
			return common.MakeUDFErrorResult(fmt.Errorf("blowfish_decrypt: failed to create cipher: %v", err), nil)
		}

		var plaintext []byte
		var iv []byte

		switch mode {
		case "ECB":
			blockSize := block.BlockSize()
			if len(ciphertext)%blockSize != 0 {
				return common.MakeUDFErrorResult(fmt.Errorf("blowfish_decrypt: ciphertext length must be multiple of %d", blockSize), nil)
			}
			plaintext = make([]byte, len(ciphertext))
			for i := 0; i < len(ciphertext); i += blockSize {
				block.Decrypt(plaintext[i:i+blockSize], ciphertext[i:i+blockSize])
			}
			plaintext, err = pkcs7Unpad(plaintext)
			if err != nil {
				return common.MakeUDFErrorResult(fmt.Errorf("blowfish_decrypt: failed to unpad: %v", err), nil)
			}
		case "CBC":
			if len(ciphertext) < blowfish.BlockSize {
				return common.MakeUDFErrorResult(fmt.Errorf("blowfish_decrypt: ciphertext too short"), nil)
			}
			iv = ciphertext[:blowfish.BlockSize]
			ciphertext = ciphertext[blowfish.BlockSize:]
			if len(ciphertext)%blowfish.BlockSize != 0 {
				return common.MakeUDFErrorResult(fmt.Errorf("blowfish_decrypt: ciphertext length must be multiple of %d", blowfish.BlockSize), nil)
			}
			mode := cipher.NewCBCDecrypter(block, iv)
			plaintext = make([]byte, len(ciphertext))
			mode.CryptBlocks(plaintext, ciphertext)
			plaintext, err = pkcs7Unpad(plaintext)
			if err != nil {
				return common.MakeUDFErrorResult(fmt.Errorf("blowfish_decrypt: failed to unpad: %v", err), nil)
			}
		default:
			return common.MakeUDFErrorResult(fmt.Errorf("blowfish_decrypt: unsupported mode %s (use ECB or CBC)", mode), nil)
		}

		result := string(plaintext)

		meta := map[string]any{
			"operation": "blowfish_decrypt",
			"mode":      mode,
			"key_size":  len(key),
		}

		return common.MakeUDFSuccessResult(result, meta)
	})
}

