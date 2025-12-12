package hmac

import (
	"crypto/hmac"
	"crypto/md5"
	"crypto/sha1"
	"crypto/sha256"
	"crypto/sha512"
	"fmt"
	"hash"

	"github.com/itchyny/gojq"
	"github.com/xen0bit/pwrq/pkg/udf/common"
)

// getHashFunc returns the hash function for the given algorithm name
func getHashFunc(algorithm string) (func() hash.Hash, error) {
	switch algorithm {
	case "md5":
		return md5.New, nil
	case "sha1":
		return sha1.New, nil
	case "sha224", "sha256":
		return sha256.New, nil
	case "sha384", "sha512":
		return sha512.New, nil
	case "sha512_224":
		return sha512.New512_224, nil
	case "sha512_256":
		return sha512.New512_256, nil
	default:
		return nil, fmt.Errorf("unsupported hash algorithm: %s", algorithm)
	}
}

// RegisterHMAC registers a generic HMAC function with gojq
func RegisterHMAC(algorithm string) gojq.CompilerOption {
	hashFunc, err := getHashFunc(algorithm)
	if err != nil {
		panic(err)
	}

	funcName := fmt.Sprintf("hmac_%s", algorithm)

	return gojq.WithFunction(funcName, 1, 3, func(v any, args []any) any {
		if len(args) < 1 {
			return fmt.Errorf("%s: expected at least 1 argument (key)", funcName)
		}

		// First argument is the key
		keyVal := common.ExtractUDFValue(args[0])
		var key []byte
		switch val := keyVal.(type) {
		case string:
			key = []byte(val)
		case []byte:
			key = val
		default:
			return fmt.Errorf("%s: key must be a string or bytes, got %T", funcName, val)
		}

		// Parse remaining arguments for message and file flag
		var inputVal any
		var isFile bool

		if len(args) > 1 {
			if fileFlag, ok := args[1].(bool); ok {
				isFile = fileFlag
				inputVal = v
			} else {
				inputVal = args[1]
				if len(args) > 2 {
					if fileFlag, ok := args[2].(bool); ok {
						isFile = fileFlag
					}
				}
			}
		} else {
			inputVal = v
		}

		inputVal = common.ExtractUDFValue(inputVal)

		var inputBytes []byte
		var filePath string
		var fileSize int64

		if isFile {
			filePathStr, ok := inputVal.(string)
			if !ok {
				return fmt.Errorf("%s: file argument requires string path, got %T", funcName, inputVal)
			}

			fileData, absPath, size, err := common.ReadFileFromPath(filePathStr)
			if err != nil {
				return fmt.Errorf("%s: %v", funcName, err)
			}

			inputBytes = fileData
			filePath = absPath
			fileSize = size
		} else {
			switch val := inputVal.(type) {
			case string:
				inputBytes = []byte(val)
			case []byte:
				inputBytes = val
			default:
				if str, ok := val.(fmt.Stringer); ok {
					inputBytes = []byte(str.String())
				} else {
					return fmt.Errorf("%s: message must be a string or bytes, got %T", funcName, val)
				}
			}
		}

		// Compute HMAC
		mac := hmac.New(hashFunc, key)
		mac.Write(inputBytes)
		hashBytes := mac.Sum(nil)
		hashHex := fmt.Sprintf("%x", hashBytes)

		meta := map[string]any{
			"algorithm":   fmt.Sprintf("hmac-%s", algorithm),
			"hash_length": len(hashHex),
		}

		if isFile {
			meta["file_path"] = filePath
			meta["file_size"] = int(fileSize)
		} else {
			meta["input_length"] = len(inputBytes)
		}

		return map[string]any{
			"_val":  hashHex,
			"_meta": meta,
		}
	})
}

// RegisterHMACMD5 registers the hmac_md5 function
func RegisterHMACMD5() gojq.CompilerOption {
	return RegisterHMAC("md5")
}

// RegisterHMACSHA1 registers the hmac_sha1 function
func RegisterHMACSHA1() gojq.CompilerOption {
	return RegisterHMAC("sha1")
}

// RegisterHMACSHA224 registers the hmac_sha224 function
func RegisterHMACSHA224() gojq.CompilerOption {
	return RegisterHMAC("sha224")
}

// RegisterHMACSHA256 registers the hmac_sha256 function
func RegisterHMACSHA256() gojq.CompilerOption {
	return RegisterHMAC("sha256")
}

// RegisterHMACSHA384 registers the hmac_sha384 function
func RegisterHMACSHA384() gojq.CompilerOption {
	return RegisterHMAC("sha384")
}

// RegisterHMACSHA512 registers the hmac_sha512 function
func RegisterHMACSHA512() gojq.CompilerOption {
	return RegisterHMAC("sha512")
}

// RegisterHMACSHA512_224 registers the hmac_sha512_224 function
func RegisterHMACSHA512_224() gojq.CompilerOption {
	return RegisterHMAC("sha512_224")
}

// RegisterHMACSHA512_256 registers the hmac_sha512_256 function
func RegisterHMACSHA512_256() gojq.CompilerOption {
	return RegisterHMAC("sha512_256")
}

