package compress

import (
	"bytes"
	"compress/flate"
	"compress/gzip"
	"compress/zlib"
	"encoding/hex"
	"fmt"
	"io"

	"github.com/itchyny/gojq"
	"github.com/xen0bit/pwrq/pkg/udf/common"
)

// RegisterGzipCompress registers the gzip_compress function with gojq
func RegisterGzipCompress() gojq.CompilerOption {
	return gojq.WithFunction("gzip_compress", 0, 2, func(v any, args []any) any {
		inputVal, isFile, err := common.ParseFileArgs(v, args)
		if err != nil {
			return common.MakeUDFErrorResult(fmt.Errorf("gzip_compress: %v", err), nil)
		}

		inputVal = common.ExtractUDFValue(inputVal)

		var inputBytes []byte
		var filePath string
		var fileSize int64

		if isFile {
			filePathStr, ok := inputVal.(string)
			if !ok {
				return common.MakeUDFErrorResult(fmt.Errorf("gzip_compress: file argument requires string path, got %T", inputVal), nil)
			}

			fileData, absPath, size, err := common.ReadFileFromPath(filePathStr)
			if err != nil {
				return common.MakeUDFErrorResult(fmt.Errorf("gzip_compress: %v", err), nil)
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
					return common.MakeUDFErrorResult(fmt.Errorf("gzip_compress: argument must be a string or bytes, got %T", val), nil)
				}
			}
		}

		// Compress with gzip
		var buf bytes.Buffer
		writer := gzip.NewWriter(&buf)
		if _, err := writer.Write(inputBytes); err != nil {
			return common.MakeUDFErrorResult(fmt.Errorf("gzip_compress: failed to write: %v", err), nil)
		}
		if err := writer.Close(); err != nil {
			return common.MakeUDFErrorResult(fmt.Errorf("gzip_compress: failed to close writer: %v", err), nil)
		}
		compressed := buf.Bytes()

		meta := map[string]any{
			"compression": "gzip",
		}

		if isFile {
			meta["file_path"] = filePath
			meta["file_size"] = int(fileSize)
			meta["compressed_length"] = len(compressed)
		} else {
			meta["original_length"] = len(inputBytes)
			meta["compressed_length"] = len(compressed)
		}

  return common.MakeUDFSuccessResult(fmt.Sprintf("%x", compressed), meta)
	})
}

// RegisterGzipDecompress registers the gzip_decompress function with gojq
func RegisterGzipDecompress() gojq.CompilerOption {
	return gojq.WithFunction("gzip_decompress", 0, 2, func(v any, args []any) any {
		inputVal, isFile, err := common.ParseFileArgs(v, args)
		if err != nil {
			return common.MakeUDFErrorResult(fmt.Errorf("gzip_decompress: %v", err), nil)
		}

		inputVal = common.ExtractUDFValue(inputVal)

		var inputBytes []byte
		var filePath string
		var fileSize int64

		if isFile {
			filePathStr, ok := inputVal.(string)
			if !ok {
				return common.MakeUDFErrorResult(fmt.Errorf("gzip_decompress: file argument requires string path, got %T", inputVal), nil)
			}

			fileData, absPath, size, err := common.ReadFileFromPath(filePathStr)
			if err != nil {
				return common.MakeUDFErrorResult(fmt.Errorf("gzip_decompress: %v", err), nil)
			}

			inputBytes = fileData
			filePath = absPath
			fileSize = size
		} else {
			switch val := inputVal.(type) {
			case string:
				// Try to decode hex string first
				decoded, err := hex.DecodeString(val)
				if err == nil {
					inputBytes = decoded
				} else {
					inputBytes = []byte(val)
				}
			case []byte:
				inputBytes = val
			default:
				return common.MakeUDFErrorResult(fmt.Errorf("gzip_decompress: argument must be a string or bytes, got %T", val), nil)
			}
		}

		// Decompress with gzip
		reader, err := gzip.NewReader(bytes.NewReader(inputBytes))
		if err != nil {
			return common.MakeUDFErrorResult(fmt.Errorf("gzip_decompress: failed to create reader: %v", err), nil)
		}
		defer reader.Close()

		decompressed, err := io.ReadAll(reader)
		if err != nil {
			return common.MakeUDFErrorResult(fmt.Errorf("gzip_decompress: failed to decompress: %v", err), nil)
		}

		meta := map[string]any{
			"compression": "gzip",
		}

		if isFile {
			meta["file_path"] = filePath
			meta["file_size"] = int(fileSize)
			meta["decompressed_length"] = len(decompressed)
		} else {
			meta["original_length"] = len(inputBytes)
			meta["decompressed_length"] = len(decompressed)
		}

  return common.MakeUDFSuccessResult(string(decompressed), meta)
	})
}

// RegisterZlibCompress registers the zlib_compress function with gojq
func RegisterZlibCompress() gojq.CompilerOption {
	return gojq.WithFunction("zlib_compress", 0, 2, func(v any, args []any) any {
		inputVal, isFile, err := common.ParseFileArgs(v, args)
		if err != nil {
			return common.MakeUDFErrorResult(fmt.Errorf("zlib_compress: %v", err), nil)
		}

		inputVal = common.ExtractUDFValue(inputVal)

		var inputBytes []byte
		var filePath string
		var fileSize int64

		if isFile {
			filePathStr, ok := inputVal.(string)
			if !ok {
				return common.MakeUDFErrorResult(fmt.Errorf("zlib_compress: file argument requires string path, got %T", inputVal), nil)
			}

			fileData, absPath, size, err := common.ReadFileFromPath(filePathStr)
			if err != nil {
				return common.MakeUDFErrorResult(fmt.Errorf("zlib_compress: %v", err), nil)
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
					return common.MakeUDFErrorResult(fmt.Errorf("zlib_compress: argument must be a string or bytes, got %T", val), nil)
				}
			}
		}

		// Compress with zlib
		var buf bytes.Buffer
		writer := zlib.NewWriter(&buf)
		if _, err := writer.Write(inputBytes); err != nil {
			return common.MakeUDFErrorResult(fmt.Errorf("zlib_compress: failed to write: %v", err), nil)
		}
		if err := writer.Close(); err != nil {
			return common.MakeUDFErrorResult(fmt.Errorf("zlib_compress: failed to close writer: %v", err), nil)
		}
		compressed := buf.Bytes()

		meta := map[string]any{
			"compression": "zlib",
		}

		if isFile {
			meta["file_path"] = filePath
			meta["file_size"] = int(fileSize)
			meta["compressed_length"] = len(compressed)
		} else {
			meta["original_length"] = len(inputBytes)
			meta["compressed_length"] = len(compressed)
		}

  return common.MakeUDFSuccessResult(fmt.Sprintf("%x", compressed), meta)
	})
}

// RegisterZlibDecompress registers the zlib_decompress function with gojq
func RegisterZlibDecompress() gojq.CompilerOption {
	return gojq.WithFunction("zlib_decompress", 0, 2, func(v any, args []any) any {
		inputVal, isFile, err := common.ParseFileArgs(v, args)
		if err != nil {
			return common.MakeUDFErrorResult(fmt.Errorf("zlib_decompress: %v", err), nil)
		}

		inputVal = common.ExtractUDFValue(inputVal)

		var inputBytes []byte
		var filePath string
		var fileSize int64

		if isFile {
			filePathStr, ok := inputVal.(string)
			if !ok {
				return common.MakeUDFErrorResult(fmt.Errorf("zlib_decompress: file argument requires string path, got %T", inputVal), nil)
			}

			fileData, absPath, size, err := common.ReadFileFromPath(filePathStr)
			if err != nil {
				return common.MakeUDFErrorResult(fmt.Errorf("zlib_decompress: %v", err), nil)
			}

			inputBytes = fileData
			filePath = absPath
			fileSize = size
		} else {
			switch val := inputVal.(type) {
			case string:
				// Try to decode hex string first
				decoded, err := hex.DecodeString(val)
				if err == nil {
					inputBytes = decoded
				} else {
					inputBytes = []byte(val)
				}
			case []byte:
				inputBytes = val
			default:
				return common.MakeUDFErrorResult(fmt.Errorf("zlib_decompress: argument must be a string or bytes, got %T", val), nil)
			}
		}

		// Decompress with zlib
		reader, err := zlib.NewReader(bytes.NewReader(inputBytes))
		if err != nil {
			return common.MakeUDFErrorResult(fmt.Errorf("zlib_decompress: failed to create reader: %v", err), nil)
		}
		defer reader.Close()

		decompressed, err := io.ReadAll(reader)
		if err != nil {
			return common.MakeUDFErrorResult(fmt.Errorf("zlib_decompress: failed to decompress: %v", err), nil)
		}

		meta := map[string]any{
			"compression": "zlib",
		}

		if isFile {
			meta["file_path"] = filePath
			meta["file_size"] = int(fileSize)
			meta["decompressed_length"] = len(decompressed)
		} else {
			meta["original_length"] = len(inputBytes)
			meta["decompressed_length"] = len(decompressed)
		}

  return common.MakeUDFSuccessResult(string(decompressed), meta)
	})
}

// RegisterDeflateCompress registers the deflate_compress function with gojq
func RegisterDeflateCompress() gojq.CompilerOption {
	return gojq.WithFunction("deflate_compress", 0, 2, func(v any, args []any) any {
		inputVal, isFile, err := common.ParseFileArgs(v, args)
		if err != nil {
			return common.MakeUDFErrorResult(fmt.Errorf("deflate_compress: %v", err), nil)
		}

		inputVal = common.ExtractUDFValue(inputVal)

		var inputBytes []byte
		var filePath string
		var fileSize int64

		if isFile {
			filePathStr, ok := inputVal.(string)
			if !ok {
				return common.MakeUDFErrorResult(fmt.Errorf("deflate_compress: file argument requires string path, got %T", inputVal), nil)
			}

			fileData, absPath, size, err := common.ReadFileFromPath(filePathStr)
			if err != nil {
				return common.MakeUDFErrorResult(fmt.Errorf("deflate_compress: %v", err), nil)
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
					return common.MakeUDFErrorResult(fmt.Errorf("deflate_compress: argument must be a string or bytes, got %T", val), nil)
				}
			}
		}

		// Compress with deflate
		var buf bytes.Buffer
		writer, err := flate.NewWriter(&buf, flate.DefaultCompression)
		if err != nil {
			return common.MakeUDFErrorResult(fmt.Errorf("deflate_compress: failed to create writer: %v", err), nil)
		}
		if _, err := writer.Write(inputBytes); err != nil {
			return common.MakeUDFErrorResult(fmt.Errorf("deflate_compress: failed to write: %v", err), nil)
		}
		if err := writer.Close(); err != nil {
			return common.MakeUDFErrorResult(fmt.Errorf("deflate_compress: failed to close writer: %v", err), nil)
		}
		compressed := buf.Bytes()

		meta := map[string]any{
			"compression": "deflate",
		}

		if isFile {
			meta["file_path"] = filePath
			meta["file_size"] = int(fileSize)
			meta["compressed_length"] = len(compressed)
		} else {
			meta["original_length"] = len(inputBytes)
			meta["compressed_length"] = len(compressed)
		}

  return common.MakeUDFSuccessResult(fmt.Sprintf("%x", compressed), meta)
	})
}

// RegisterDeflateDecompress registers the deflate_decompress function with gojq
func RegisterDeflateDecompress() gojq.CompilerOption {
	return gojq.WithFunction("deflate_decompress", 0, 2, func(v any, args []any) any {
		inputVal, isFile, err := common.ParseFileArgs(v, args)
		if err != nil {
			return common.MakeUDFErrorResult(fmt.Errorf("deflate_decompress: %v", err), nil)
		}

		inputVal = common.ExtractUDFValue(inputVal)

		var inputBytes []byte
		var filePath string
		var fileSize int64

		if isFile {
			filePathStr, ok := inputVal.(string)
			if !ok {
				return common.MakeUDFErrorResult(fmt.Errorf("deflate_decompress: file argument requires string path, got %T", inputVal), nil)
			}

			fileData, absPath, size, err := common.ReadFileFromPath(filePathStr)
			if err != nil {
				return common.MakeUDFErrorResult(fmt.Errorf("deflate_decompress: %v", err), nil)
			}

			inputBytes = fileData
			filePath = absPath
			fileSize = size
		} else {
			switch val := inputVal.(type) {
			case string:
				// Try to decode hex string first
				decoded, err := hex.DecodeString(val)
				if err == nil {
					inputBytes = decoded
				} else {
					inputBytes = []byte(val)
				}
			case []byte:
				inputBytes = val
			default:
				return common.MakeUDFErrorResult(fmt.Errorf("deflate_decompress: argument must be a string or bytes, got %T", val), nil)
			}
		}

		// Decompress with deflate
		reader := flate.NewReader(bytes.NewReader(inputBytes))
		defer reader.Close()

		decompressed, err := io.ReadAll(reader)
		if err != nil {
			return common.MakeUDFErrorResult(fmt.Errorf("deflate_decompress: failed to decompress: %v", err), nil)
		}

		meta := map[string]any{
			"compression": "deflate",
		}

		if isFile {
			meta["file_path"] = filePath
			meta["file_size"] = int(fileSize)
			meta["decompressed_length"] = len(decompressed)
		} else {
			meta["original_length"] = len(inputBytes)
			meta["decompressed_length"] = len(decompressed)
		}

  return common.MakeUDFSuccessResult(string(decompressed), meta)
	})
}

