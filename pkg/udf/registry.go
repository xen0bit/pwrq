package udf

import (
	"github.com/itchyny/gojq"
	"github.com/xen0bit/pwrq/pkg/udf/base64"
	"github.com/xen0bit/pwrq/pkg/udf/compress"
	"github.com/xen0bit/pwrq/pkg/udf/find"
	"github.com/xen0bit/pwrq/pkg/udf/hex"
	"github.com/xen0bit/pwrq/pkg/udf/html"
	md5udf "github.com/xen0bit/pwrq/pkg/udf/md5"
	"github.com/xen0bit/pwrq/pkg/udf/sha1"
	"github.com/xen0bit/pwrq/pkg/udf/sha224"
	"github.com/xen0bit/pwrq/pkg/udf/sha256"
	"github.com/xen0bit/pwrq/pkg/udf/sha384"
	"github.com/xen0bit/pwrq/pkg/udf/sha512"
	"github.com/xen0bit/pwrq/pkg/udf/sha512_224"
	"github.com/xen0bit/pwrq/pkg/udf/sha512_256"
	"github.com/xen0bit/pwrq/pkg/udf/string"
	"github.com/xen0bit/pwrq/pkg/udf/url"
)

// Registry holds all user-defined functions
type Registry struct {
	functions []gojq.CompilerOption
}

// NewRegistry creates a new UDF registry
func NewRegistry() *Registry {
	return &Registry{
		functions: make([]gojq.CompilerOption, 0),
	}
}

// Register adds a compiler option to the registry
func (r *Registry) Register(option gojq.CompilerOption) {
	r.functions = append(r.functions, option)
}

// Options returns all registered compiler options
func (r *Registry) Options() []gojq.CompilerOption {
	return r.functions
}

// DefaultRegistry returns the default registry with all built-in UDFs
func DefaultRegistry() *Registry {
	reg := NewRegistry()
	
	// Register all built-in UDFs
	reg.Register(find.RegisterFind())
	
	// Encoding/Decoding
	reg.Register(base64.RegisterBase64Encode())
	reg.Register(base64.RegisterBase64Decode())
	reg.Register(hex.RegisterHexEncode())
	reg.Register(hex.RegisterHexDecode())
	reg.Register(url.RegisterURLEncode())
	reg.Register(url.RegisterURLDecode())
	reg.Register(html.RegisterHTMLEncode())
	reg.Register(html.RegisterHTMLDecode())
	
	// Compression
	reg.Register(compress.RegisterGzipCompress())
	reg.Register(compress.RegisterGzipDecompress())
	reg.Register(compress.RegisterZlibCompress())
	reg.Register(compress.RegisterZlibDecompress())
	reg.Register(compress.RegisterDeflateCompress())
	reg.Register(compress.RegisterDeflateDecompress())
	
	// String operations
	reg.Register(string.RegisterUpper())
	reg.Register(string.RegisterLower())
	reg.Register(string.RegisterReverse())
	reg.Register(string.RegisterReplace())
	
	// Hash functions (all support optional file argument)
	reg.Register(md5udf.RegisterMD5())
	reg.Register(sha1.RegisterSHA1())
	reg.Register(sha224.RegisterSHA224())
	reg.Register(sha256.RegisterSHA256())
	reg.Register(sha384.RegisterSHA384())
	reg.Register(sha512.RegisterSHA512())
	reg.Register(sha512_224.RegisterSHA512_224())
	reg.Register(sha512_256.RegisterSHA512_256())

	return reg
}
