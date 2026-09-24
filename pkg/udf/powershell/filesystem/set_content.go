package filesystem

import (
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"

	"github.com/itchyny/gojq"
	"github.com/xen0bit/pwrq/pkg/core/typed"
	"github.com/xen0bit/pwrq/pkg/udf/common"
	"golang.org/x/text/encoding"
	"golang.org/x/text/encoding/charmap"
	"golang.org/x/text/encoding/unicode"
	"golang.org/x/text/transform"
)

// SetContentOptions represents options for Set-Content
type SetContentOptions struct {
	Path     string
	Value    any
	Encoding string
	Force    bool
}

// parseSetContentArgs binds set_content's path, value and options the same way
// add_content does, so the two writers read alike: the value comes from the
// pipeline unless an explicit one is supplied, and anything past the options
// object is an error rather than a silently dropped argument.
func parseSetContentArgs(v any, args []any) (SetContentOptions, error) {
	o := appendOptions{SetContentOptions: SetContentOptions{Encoding: "utf8"}}
	if len(args) == 0 {
		return o.SetContentOptions, fmt.Errorf("set_content: path is required")
	}
	o, err := parseAppendArgs(v, args, "set_content")
	if err != nil {
		return o.SetContentOptions, err
	}
	if o.Append {
		return o.SetContentOptions, fmt.Errorf("set_content: does not append; use add_content")
	}
	return o.SetContentOptions, nil
}

// getEncoding returns the appropriate encoder for the given encoding name
func getEncoding(name string) (encoding.Encoding, error) {
	switch strings.ToLower(name) {
	case "utf8", "utf-8", "utf8mb3":
		// nil means "no transcoding": see convertToEncoding. Returning
		// unicode.UTF8 here would route the content through a decoder that
		// cannot round-trip bytes which are not valid UTF-8.
		return nil, nil
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

// convertToEncoding converts a string to the specified encoding. A nil
// encoding means UTF-8, which is written verbatim; runes the target encoding
// cannot represent are replaced with its substitute character.
func convertToEncoding(s string, enc encoding.Encoding) ([]byte, error) {
	// UTF-8 is the identity transform on a jq string, so the bytes go to disk
	// as they are. This is not an optimization, it is the only correct answer:
	// a jq string is a byte string, and the bytes reaching here may not be
	// valid UTF-8 at all. read_bytes and http's .Content both hand over raw
	// bytes, and decoding those to runes to re-encode them would rewrite every
	// invalid byte as U+FFFD — silently corrupting the very content the caller
	// asked to be written unchanged.
	if enc == nil {
		return []byte(s), nil
	}

	// Every other encoding is a real transcode, and some runes genuinely have
	// no spelling in the target. ReplaceUnsupported substitutes those and
	// leaves the rest alone, which is what the encoding's own tables say is
	// representable — the previous hand-rolled rune filter guessed at that
	// range and mangled everything above U+FFFD, so an emoji became '?' even
	// when writing UTF-16, which represents it perfectly well.
	encoder := encoding.ReplaceUnsupported(enc.NewEncoder())
	result, _, err := transform.String(encoder, s)
	if err != nil {
		return nil, fmt.Errorf("encoding conversion failed: %w", err)
	}
	return []byte(result), nil
}

// getNewLine returns the platform-appropriate newline string: \r\n on Windows,
// \n elsewhere.
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

	content := renderContent(opts.Value)

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
	return common.WithFunctionOf("set_content", 1, 3, WrittenFile, func(v any, args []any) any {
		opts, err := parseSetContentArgs(v, args)
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

		// Return object with file info
		obj := typed.New(writtenPath)
		obj.AddNoteProperty("Path", writtenPath)
		obj.AddNoteProperty("Length", fileSize)
		obj.AddNoteProperty("Exists", true)
		obj.AddNoteProperty("Operation", "Set-Content")

		return WrittenFile.Build(obj.ToMap())
	})
}

// renderContent turns a value into the text to write. A string goes as-is, an
// array becomes one line per element, and anything else is formatted.
//
// Every element is bound first, so a stream of cmdlet output writes its values
// rather than its object representation. That is what this comment always
// claimed; the code looked for a __psobject marker belonging to a wire format
// pwrq stopped emitting long ago, which no cmdlet has ever produced, so the
// unwrap never fired. Binding is the rule the rest of the pipeline uses.
func renderContent(value any) string {
	value = common.BindValue(value)
	switch v := value.(type) {
	case string:
		return v
	case []any:
		parts := make([]string, len(v))
		for i, item := range v {
			parts[i] = fmt.Sprintf("%v", common.BindValue(item))
		}
		return strings.Join(parts, getNewLine())
	default:
		return fmt.Sprintf("%v", v)
	}
}
