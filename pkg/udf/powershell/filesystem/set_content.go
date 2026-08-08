package filesystem

import (
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"

	"github.com/itchyny/gojq"
	"github.com/xen0bit/pwrq/pkg/core/psobject"
	"github.com/xen0bit/pwrq/pkg/udf/common"
	"golang.org/x/text/encoding"
	"golang.org/x/text/encoding/charmap"
	"golang.org/x/text/encoding/unicode"
	"golang.org/x/text/transform"
)

// SetContentOptions represents options for Set-Content
type SetContentOptions struct {
	Path    string
	Value   any
	Encoding string
	Force   bool
}

// parseSetContentArgs parses arguments for set_content
func parseSetContentArgs(args []any) (SetContentOptions, error) {
	opts := SetContentOptions{
		Path:     "",
		Value:    nil,
		Encoding: "utf8",
		Force:    false,
	}

	if len(args) == 0 {
		return opts, fmt.Errorf("set_content: expected at least 2 arguments (path, value)")
	}

	for i, arg := range args {
		argVal := common.BindValue(arg)

		switch v := argVal.(type) {
		case string:
			if opts.Path == "" {
				opts.Path = v
			} else if opts.Value == nil {
				opts.Value = v
			}
		case map[string]any:
			if path, ok := v["Path"].(string); ok {
				opts.Path = path
			}
			if value, ok := v["Value"].(any); ok {
				opts.Value = value
			}
			if encoding, ok := v["Encoding"].(string); ok {
				opts.Encoding = encoding
			}
			if force, ok := v["Force"].(bool); ok {
				opts.Force = force
			}
		default:
			// First non-map argument is path, second is value
			if i == 0 && opts.Path == "" {
				opts.Path = fmt.Sprintf("%v", argVal)
			} else if i == 1 && opts.Value == nil {
				opts.Value = argVal
			}
		}
	}

	if opts.Path == "" {
		return opts, fmt.Errorf("set_content: path is required")
	}
	if opts.Value == nil {
		return opts, fmt.Errorf("set_content: value is required")
	}

	return opts, nil
}

// getEncoding returns the appropriate encoder for the given encoding name
func getEncoding(name string) (encoding.Encoding, error) {
	switch strings.ToLower(name) {
	case "utf8", "utf-8", "utf8mb3":
		return unicode.UTF8, nil
	case "utf16le", "utf-16le", "ucs2le":
		return unicode.UTF16(unicode.LittleEndian, unicode.IgnoreBOM), nil
	case "utf16be", "utf-16be", "ucs2be":
		return unicode.UTF16(unicode.BigEndian, unicode.IgnoreBOM), nil
	case "utf16", "utf-16":
		// PowerShell default is UTF-16LE with BOM
		return unicode.UTF16(unicode.LittleEndian, unicode.UseBOM), nil
	case "ascii", "us-ascii":
		return charmap.ISO8859_1, nil
	case "latin1", "iso-8859-1", "cp819":
		return charmap.ISO8859_1, nil
	case "cp437":
		return charmap.CodePage437, nil
	case "cp850":
		return charmap.CodePage850, nil
	case "cp852":
		return charmap.CodePage852, nil
	case "cp855":
		return charmap.CodePage855, nil
	case "cp858":
		return charmap.CodePage858, nil
	case "cp860":
		return charmap.CodePage860, nil
	case "cp862":
		return charmap.CodePage862, nil
	case "cp863":
		return charmap.CodePage863, nil
	case "cp865":
		return charmap.CodePage865, nil
	case "cp866":
		return charmap.CodePage866, nil
	case "cp1140":
		return charmap.CodePage1140, nil
	case "cp1250", "windows-1250":
		return charmap.Windows1250, nil
	case "cp1251", "windows-1251":
		return charmap.Windows1251, nil
	case "cp1252", "windows-1252":
		return charmap.Windows1252, nil
	case "cp1253", "windows-1253":
		return charmap.Windows1253, nil
	case "cp1254", "windows-1254":
		return charmap.Windows1254, nil
	case "cp1255", "windows-1255":
		return charmap.Windows1255, nil
	case "cp1256", "windows-1256":
		return charmap.Windows1256, nil
	case "cp1257", "windows-1257":
		return charmap.Windows1257, nil
	case "cp1258", "windows-1258":
		return charmap.Windows1258, nil
	case "cp874", "windows-874":
		return charmap.Windows874, nil
	case "ebcdic", "cp037":
		return charmap.CodePage037, nil
	case "cp1047":
		return charmap.CodePage1047, nil
	case "koi8-r":
		return charmap.KOI8R, nil
	case "koi8-u":
		return charmap.KOI8U, nil
	case "macintosh":
		return charmap.Macintosh, nil
	case "macintosh-cyrillic":
		return charmap.MacintoshCyrillic, nil
	case "iso-8859-2":
		return charmap.ISO8859_2, nil
	case "iso-8859-3":
		return charmap.ISO8859_3, nil
	case "iso-8859-4":
		return charmap.ISO8859_4, nil
	case "iso-8859-5":
		return charmap.ISO8859_5, nil
	case "iso-8859-6":
		return charmap.ISO8859_6, nil
	case "iso-8859-7":
		return charmap.ISO8859_7, nil
	case "iso-8859-8":
		return charmap.ISO8859_8, nil
	case "iso-8859-9":
		return charmap.ISO8859_9, nil
	case "iso-8859-10":
		return charmap.ISO8859_10, nil
	case "iso-8859-13":
		return charmap.ISO8859_13, nil
	case "iso-8859-14":
		return charmap.ISO8859_14, nil
	case "iso-8859-15":
		return charmap.ISO8859_15, nil
	case "iso-8859-16":
		return charmap.ISO8859_16, nil
	default:
		return nil, fmt.Errorf("unsupported encoding: %s (supported: utf8, utf16, utf16le, utf16be, ascii, latin1, cp437, cp850, cp1252, windows-1252, etc)", name)
	}
}

// convertToEncoding converts a string to the specified encoding
// Unsupported characters are replaced with '?'
func convertToEncoding(s string, enc encoding.Encoding) ([]byte, error) {
	// First, pre-process the string to replace potentially problematic runes
	// with a safe replacement character
	safe := strings.Map(func(r rune) rune {
		// Keep ASCII and common Unicode, replace others that might cause issues
		if r < 128 || (r >= 0x00A0 && r <= 0xFFFD) {
			return r
		}
		return '?'
	}, s)
	
	encoder := enc.NewEncoder()
	result, _, err := transform.String(encoder, safe)
	if err != nil {
		return nil, fmt.Errorf("encoding conversion failed: %w", err)
	}
	return []byte(result), nil
}

// extractPSObjectValue unwraps a PSObject map to get the actual value.
// If the input is a PSObject (detected by __psobject marker), returns .Value.
// Otherwise returns the input unchanged.
func extractPSObjectValue(v any) any {
	m, ok := v.(map[string]any)
	if !ok {
		return v
	}
	
	// Check for PSObject marker
	if isPSObj, ok := m["__psobject"].(bool); ok && isPSObj {
		if val, ok := m["Value"]; ok {
			// Recursively unwrap in case of nested PSObjects
			return extractPSObjectValue(val)
		}
	}
	return v
}

// getNewLine returns the platform-appropriate newline string.
// PowerShell uses Environment::NewLine which is \r\n on Windows, \n on Unix.
func getNewLine() string {
	if runtime.GOOS == "windows" {
		return "\r\n"
	}
	return "\n"
}

// validatePath checks if the path is valid for writing.
// On Windows, this includes checking for reserved names.
func validatePath(path string) error {
	// Check for empty path
	if path == "" {
		return fmt.Errorf("path cannot be empty")
	}
	
	// On Windows, check for reserved names (CON, PRN, AUX, NUL, COM1-9, LPT1-9)
	if runtime.GOOS == "windows" {
		baseName := strings.ToUpper(filepath.Base(path))
		ext := filepath.Ext(baseName)
		nameWithoutExt := strings.TrimSuffix(baseName, ext)
		
		reservedNames := []string{
			"CON", "PRN", "AUX", "NUL",
			"COM1", "COM2", "COM3", "COM4", "COM5", "COM6", "COM7", "COM8", "COM9",
			"LPT1", "LPT2", "LPT3", "LPT4", "LPT5", "LPT6", "LPT7", "LPT8", "LPT9",
		}
		
		for _, reserved := range reservedNames {
			if nameWithoutExt == reserved {
				return fmt.Errorf("cannot write to reserved device name: %s", nameWithoutExt)
			}
		}
	}
	
	return nil
}

// isReadOnly checks if a file has read-only attribute set
func isReadOnly(path string) bool {
	info, err := os.Stat(path)
	if err != nil {
		return false
	}
	// On Unix, check write permissions
	if runtime.GOOS != "windows" {
		return info.Mode()&0o200 == 0
	}
	// On Windows, we'd need syscall to check FILE_ATTRIBUTE_READONLY
	// For now, just try to open for write and see if it fails
	return false
}

// setContent performs the file write operation with encoding support
func setContent(opts SetContentOptions) (string, error) {
	// Resolve the path
	path, err := filepath.Abs(opts.Path)
	if err != nil {
		return "", fmt.Errorf("invalid path: %w", err)
	}

	// Validate the path (checks for reserved names on Windows, etc.)
	if err := validatePath(path); err != nil {
		return "", err
	}

	// Extract actual value from PSObject if needed
	value := extractPSObjectValue(opts.Value)

	// Platform-aware line endings
	newline := getNewLine()

	// Convert value to content string
	var content string
	switch v := value.(type) {
	case string:
		content = v
	case []any:
		// Join array elements with platform-appropriate newlines
		parts := make([]string, len(v))
		for i, item := range v {
			// Unwrap PSObject from each array element too
			item = extractPSObjectValue(item)
			parts[i] = fmt.Sprintf("%v", item)
		}
		content = strings.Join(parts, newline)
	default:
		content = fmt.Sprintf("%v", v)
	}

	// Get the encoding
	enc, err := getEncoding(opts.Encoding)
	if err != nil {
		return "", err
	}

	// Convert content to the specified encoding
	encodedContent, err := convertToEncoding(content, enc)
	if err != nil {
		return "", err
	}

	// Ensure parent directory exists if Force is set
	if opts.Force {
		parentDir := filepath.Dir(path)
		if err := os.MkdirAll(parentDir, 0o755); err != nil {
			return "", fmt.Errorf("failed to create parent directory: %w", err)
		}
	}

	// Check if file exists and is read-only
	// If Force is set, we attempt to change permissions before writing
	if _, err := os.Stat(path); err == nil {
		if isReadOnly(path) && !opts.Force {
			return "", fmt.Errorf("access denied: %s is read-only (use -Force to override)", opts.Path)
		}
		if isReadOnly(path) && opts.Force {
			// Try to make it writable
			if err := os.Chmod(path, 0o644); err != nil {
				return "", fmt.Errorf("failed to remove read-only attribute: %w", err)
			}
		}
	}

	// Write the file
	if err := os.WriteFile(path, encodedContent, 0o644); err != nil {
		return "", fmt.Errorf("failed to write file: %w", err)
	}

	return path, nil
}

// RegisterSetContent registers the set_content function with gojq
func RegisterSetContent() gojq.CompilerOption {
	return gojq.WithFunction("set_content", 0, 5, func(v any, args []any) any {
		opts, err := parseSetContentArgs(args)
		if err != nil {
			return common.MakeUDFErrorResult(err, nil)
		}

		writtenPath, err := setContent(opts)
		if err != nil {
			return common.MakeUDFErrorResult(err, map[string]any{
				"path": opts.Path,
			})
		}

		// Get file info for metadata
		fileInfo, err := os.Stat(writtenPath)
		var fileSize int64
		if err == nil {
			fileSize = fileInfo.Size()
		}

		// Return PSObject with file info
		psobj := psobject.NewPSObject(writtenPath)
		psobj.TypeName = "System.IO.FileInfo"
		psobj.AddNoteProperty("Path", writtenPath)
		psobj.AddNoteProperty("Length", fileSize)
		psobj.AddNoteProperty("Exists", true)
		psobj.AddNoteProperty("Operation", "Set-Content")

		return psobj.ToMap()
	})
}
