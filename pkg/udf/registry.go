package udf

import (
	"github.com/itchyny/gojq"
	"github.com/xen0bit/pwrq/pkg/udf/aggregate"
	"github.com/xen0bit/pwrq/pkg/udf/archive"
	"github.com/xen0bit/pwrq/pkg/udf/base32"
	"github.com/xen0bit/pwrq/pkg/udf/base64"
	"github.com/xen0bit/pwrq/pkg/udf/base85"
	"github.com/xen0bit/pwrq/pkg/udf/binary"
	"github.com/xen0bit/pwrq/pkg/udf/cat"
	"github.com/xen0bit/pwrq/pkg/udf/checksum"
	"github.com/xen0bit/pwrq/pkg/udf/collection"
	"github.com/xen0bit/pwrq/pkg/udf/compress"
	"github.com/xen0bit/pwrq/pkg/udf/config"
	"github.com/xen0bit/pwrq/pkg/udf/crypto"
	"github.com/xen0bit/pwrq/pkg/udf/csv"
	"github.com/xen0bit/pwrq/pkg/udf/discovery"
	"github.com/xen0bit/pwrq/pkg/udf/domain"
	"github.com/xen0bit/pwrq/pkg/udf/duration"
	"github.com/xen0bit/pwrq/pkg/udf/entropy"
	"github.com/xen0bit/pwrq/pkg/udf/find"
	"github.com/xen0bit/pwrq/pkg/udf/hex"
	"github.com/xen0bit/pwrq/pkg/udf/hmac"
	"github.com/xen0bit/pwrq/pkg/udf/html"
	"github.com/xen0bit/pwrq/pkg/udf/http"
	"github.com/xen0bit/pwrq/pkg/udf/json"
	"github.com/xen0bit/pwrq/pkg/udf/logfile"
	md5udf "github.com/xen0bit/pwrq/pkg/udf/md5"
	"github.com/xen0bit/pwrq/pkg/udf/mkdir"
	"github.com/xen0bit/pwrq/pkg/udf/net"
	"github.com/xen0bit/pwrq/pkg/udf/number"
	"github.com/xen0bit/pwrq/pkg/udf/path"
	"github.com/xen0bit/pwrq/pkg/udf/random"
	"github.com/xen0bit/pwrq/pkg/udf/rm"
	"github.com/xen0bit/pwrq/pkg/udf/sh"
	"github.com/xen0bit/pwrq/pkg/udf/sha1"
	"github.com/xen0bit/pwrq/pkg/udf/sha224"
	"github.com/xen0bit/pwrq/pkg/udf/sha256"
	"github.com/xen0bit/pwrq/pkg/udf/sha384"
	"github.com/xen0bit/pwrq/pkg/udf/sha512"
	"github.com/xen0bit/pwrq/pkg/udf/sha512_224"
	"github.com/xen0bit/pwrq/pkg/udf/sha512_256"
	"github.com/xen0bit/pwrq/pkg/udf/similarity"
	"github.com/xen0bit/pwrq/pkg/udf/sniff"
	"github.com/xen0bit/pwrq/pkg/udf/ssdeep"
	"github.com/xen0bit/pwrq/pkg/udf/stats"
	stringudf "github.com/xen0bit/pwrq/pkg/udf/string"
	"github.com/xen0bit/pwrq/pkg/udf/system"
	"github.com/xen0bit/pwrq/pkg/udf/tee"
	"github.com/xen0bit/pwrq/pkg/udf/tempdir"
	"github.com/xen0bit/pwrq/pkg/udf/timestamp"
	"github.com/xen0bit/pwrq/pkg/udf/token"
	"github.com/xen0bit/pwrq/pkg/udf/url"
	"github.com/xen0bit/pwrq/pkg/udf/validate"
	"github.com/xen0bit/pwrq/pkg/udf/xml"
	yamllib "github.com/xen0bit/pwrq/pkg/udf/yaml"

	// PowerShell cmdlets
	"github.com/xen0bit/pwrq/pkg/udf/powershell/datetime"
	"github.com/xen0bit/pwrq/pkg/udf/powershell/filesystem"
	"github.com/xen0bit/pwrq/pkg/udf/powershell/formatting"
	"github.com/xen0bit/pwrq/pkg/udf/powershell/location"
	"github.com/xen0bit/pwrq/pkg/udf/powershell/objects"
	"github.com/xen0bit/pwrq/pkg/udf/powershell/process"
	"github.com/xen0bit/pwrq/pkg/udf/powershell/service"
	"github.com/xen0bit/pwrq/pkg/udf/powershell/variables"
	"github.com/xen0bit/pwrq/pkg/udf/powershell/web"
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
	reg.Register(cat.RegisterCat())
	reg.Register(mkdir.RegisterMkdir())
	reg.Register(rm.RegisterRm())

	// Encoding/Decoding
	reg.Register(base64.RegisterBase64Encode())
	reg.Register(base64.RegisterBase64Decode())
	reg.Register(hex.RegisterHexEncode())
	reg.Register(hex.RegisterHexDecode())
	reg.Register(url.RegisterURLEncode())
	reg.Register(url.RegisterURLDecode())
	reg.Register(html.RegisterHTMLEncode())
	reg.Register(html.RegisterHTMLDecode())

	// Additional encodings
	reg.Register(base32.RegisterBase32Encode())
	reg.Register(base32.RegisterBase32Decode())
	reg.Register(base85.RegisterBase85Encode())
	reg.Register(base85.RegisterBase85Decode())
	reg.Register(binary.RegisterBinaryEncode())
	reg.Register(binary.RegisterBinaryDecode())

	// Compression
	reg.Register(compress.RegisterGzipCompress())
	reg.Register(compress.RegisterGzipDecompress())
	reg.Register(compress.RegisterZlibCompress())
	reg.Register(compress.RegisterZlibDecompress())
	reg.Register(compress.RegisterDeflateCompress())
	reg.Register(compress.RegisterDeflateDecompress())

	// String operations
	for _, opt := range stringudf.RegisterAll() {
		reg.Register(opt)
	}
	// split, join and trim are jq builtins. Registering them here had no
	// effect - gojq resolves builtins first - so pwrq's versions never ran.
	// Use jq's, and `cat("f") | split(",")` for the file case pwrq's took a
	// flag for.

	// Timestamp operations
	reg.Register(timestamp.RegisterTimestampToDate())
	reg.Register(timestamp.RegisterDateToTimestamp())

	// JSON operations
	for _, opt := range json.RegisterAll() {
		reg.Register(opt)
	}

	// CSV operations
	reg.Register(csv.RegisterCSVParse())
	reg.Register(csv.RegisterCSVStringify())
	reg.Register(csv.RegisterTSVParse())
	reg.Register(csv.RegisterTSVStringify())

	// XML operations
	reg.Register(xml.RegisterXMLParse())
	reg.Register(xml.RegisterXMLStringify())

	// Entropy
	reg.Register(entropy.RegisterEntropy())

	// SSDeep (fuzzy hashing)
	reg.Register(ssdeep.RegisterSSDeep())
	reg.Register(ssdeep.RegisterSSDeepCompare())

	// Tee (write to stderr or file)
	reg.Register(tee.RegisterTee())

	// Shell command execution
	reg.Register(sh.RegisterSh())

	// Temporary directory
	reg.Register(tempdir.RegisterTempDir())

	// HTTP requests
	reg.Register(http.RegisterHTTP())
	reg.Register(http.RegisterHTTPServe())

	// Encryption/Decryption functions
	reg.Register(crypto.RegisterAESEncrypt())
	reg.Register(crypto.RegisterAESDecrypt())
	reg.Register(crypto.RegisterDESEncrypt())
	reg.Register(crypto.RegisterDESDecrypt())
	reg.Register(crypto.Register3DESEncrypt())
	reg.Register(crypto.Register3DESDecrypt())
	reg.Register(crypto.RegisterBlowfishEncrypt())
	reg.Register(crypto.RegisterBlowfishDecrypt())
	reg.Register(crypto.RegisterRC4())
	reg.Register(crypto.RegisterChaCha20())
	reg.Register(crypto.RegisterXOR())

	// Hash functions (all support optional file argument)
	reg.Register(md5udf.RegisterMD5())
	reg.Register(sha1.RegisterSHA1())
	reg.Register(sha224.RegisterSHA224())
	reg.Register(sha256.RegisterSHA256())
	reg.Register(sha384.RegisterSHA384())
	reg.Register(sha512.RegisterSHA512())
	reg.Register(sha512_224.RegisterSHA512_224())
	reg.Register(sha512_256.RegisterSHA512_256())

	// HMAC functions (key, message, optional file flag)
	reg.Register(hmac.RegisterHMACMD5())
	reg.Register(hmac.RegisterHMACSHA1())
	reg.Register(hmac.RegisterHMACSHA224())
	reg.Register(hmac.RegisterHMACSHA256())
	reg.Register(hmac.RegisterHMACSHA384())
	reg.Register(hmac.RegisterHMACSHA512())
	reg.Register(hmac.RegisterHMACSHA512_224())
	reg.Register(hmac.RegisterHMACSHA512_256())

	// Command discovery, over the catalog assembled below
	reg.Register(discovery.RegisterGetCommand())
	reg.Register(discovery.RegisterGetHelp())

	// Utilitarian cmdlets: text, numbers, paths, statistics, durations,
	// randomness, IP and network, identifiers and tokens, validation, text
	// similarity, YAML and checksums. Each is a pure transform, so the whole
	// set also appears in WebRegistry.
	for _, opt := range number.RegisterAll() {
		reg.Register(opt)
	}
	for _, opt := range path.RegisterAll() {
		reg.Register(opt)
	}
	for _, opt := range stats.RegisterAll() {
		reg.Register(opt)
	}
	for _, opt := range duration.RegisterAll() {
		reg.Register(opt)
	}
	for _, opt := range random.RegisterAll() {
		reg.Register(opt)
	}
	for _, opt := range net.RegisterAll() {
		reg.Register(opt)
	}
	for _, opt := range token.RegisterAll() {
		reg.Register(opt)
	}
	for _, opt := range validate.RegisterAll() {
		reg.Register(opt)
	}
	for _, opt := range similarity.RegisterAll() {
		reg.Register(opt)
	}
	for _, opt := range collection.RegisterAll() {
		reg.Register(opt)
	}
	for _, opt := range sniff.RegisterAll() {
		reg.Register(opt)
	}
	for _, opt := range yamllib.RegisterAll() {
		reg.Register(opt)
	}
	for _, opt := range checksum.RegisterAll() {
		reg.Register(opt)
	}

	// Fourth round: grouping and summarising over arrays, and a domain
	// package of unit, geo and finance cmdlets. All pure, so both appear in
	// the browser registry too.
	for _, opt := range aggregate.RegisterAll() {
		reg.Register(opt)
	}
	for _, opt := range domain.RegisterAll() {
		reg.Register(opt)
	}

	// Archives. They read and write files, so like the log readers they are a
	// CLI-only capability and WebRegistry leaves them out.
	for _, opt := range archive.RegisterAll() {
		reg.Register(opt)
	}

	// Config formats: INI, .env / properties and logfmt, as pure transforms.
	for _, opt := range config.RegisterAll() {
		reg.Register(opt)
	}

	// Line-oriented log readers. They touch the filesystem, so they exist in
	// the CLI only; WebRegistry leaves them out and the IDE marks them
	// unavailable.
	for _, opt := range logfile.RegisterAll() {
		reg.Register(opt)
	}

	// Host lookups and PATH searches. They need the network or the
	// filesystem, so they are CLI-only like the log readers.
	for _, opt := range system.RegisterAll() {
		reg.Register(opt)
	}

	// PowerShell cmdlets
	for _, opt := range filesystem.RegisterAll() {
		reg.Register(opt)
	}
	for _, opt := range objects.RegisterAll() {
		reg.Register(opt)
	}
	for _, opt := range formatting.RegisterAll() {
		reg.Register(opt)
	}
	for _, opt := range variables.RegisterAll() {
		reg.Register(opt)
	}
	for _, opt := range location.RegisterAll() {
		reg.Register(opt)
	}
	for _, opt := range process.RegisterAll() {
		reg.Register(opt)
	}
	for _, opt := range service.RegisterAll() {
		reg.Register(opt)
	}
	for _, opt := range web.RegisterAll() {
		reg.Register(opt)
	}
	for _, opt := range datetime.RegisterAll() {
		reg.Register(opt)
	}

	// Publish the catalog get_command and get_help report on. Aliases are
	// attached to the cmdlet they name so a search finds either.
	discovery.SetCatalog(buildCatalog())

	return reg
}

// buildCatalog joins the documented function list to the alias table.
func buildCatalog() []discovery.Command {
	aliasesFor := make(map[string][]string)
	for _, alias := range StandardAliases {
		aliasesFor[alias.Target] = append(aliasesFor[alias.Target], alias.Name)
	}

	metadata := GetFunctionMetadata()
	commands := make([]discovery.Command, 0, len(metadata))
	for _, meta := range metadata {
		commands = append(commands, discovery.Command{
			Name:        meta.Name,
			Aliases:     aliasesFor[meta.Name],
			MinArgs:     meta.MinArgs,
			MaxArgs:     meta.MaxArgs,
			Category:    meta.Category,
			Description: meta.Description,
			Examples:    meta.Examples,
			// The CLI registers every documented command, so its catalog marks
			// them all runnable. browserCatalog overrides this per command.
			Available: true,
		})
	}
	return commands
}
