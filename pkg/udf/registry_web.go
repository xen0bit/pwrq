package udf

import (
	"github.com/xen0bit/pwrq/pkg/udf/aggregate"
	"github.com/xen0bit/pwrq/pkg/udf/base32"
	"github.com/xen0bit/pwrq/pkg/udf/base64"
	"github.com/xen0bit/pwrq/pkg/udf/base85"
	"github.com/xen0bit/pwrq/pkg/udf/binary"
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
	"github.com/xen0bit/pwrq/pkg/udf/hex"
	"github.com/xen0bit/pwrq/pkg/udf/hmac"
	"github.com/xen0bit/pwrq/pkg/udf/html"
	"github.com/xen0bit/pwrq/pkg/udf/json"
	md5udf "github.com/xen0bit/pwrq/pkg/udf/md5"
	"github.com/xen0bit/pwrq/pkg/udf/net"
	"github.com/xen0bit/pwrq/pkg/udf/number"
	"github.com/xen0bit/pwrq/pkg/udf/path"
	"github.com/xen0bit/pwrq/pkg/udf/random"
	"github.com/xen0bit/pwrq/pkg/udf/rncd"
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
	"github.com/xen0bit/pwrq/pkg/udf/timestamp"
	"github.com/xen0bit/pwrq/pkg/udf/token"
	"github.com/xen0bit/pwrq/pkg/udf/url"
	"github.com/xen0bit/pwrq/pkg/udf/validate"
	"github.com/xen0bit/pwrq/pkg/udf/xml"
	yamllib "github.com/xen0bit/pwrq/pkg/udf/yaml"

	"github.com/xen0bit/pwrq/pkg/udf/powershell/formatting"
	"github.com/xen0bit/pwrq/pkg/udf/powershell/objects"
)

// WebRegistry returns the functions the browser IDE evaluates queries against:
// everything that is a pure in-memory transform, and nothing else.
//
// The exclusions are not about trust - the page runs the user's own query on
// the user's own data. They are about honesty. A browser tab has no filesystem,
// no process table and no service manager, so get_childitem, get_process and
// sh cannot do anything but fail; offering them would advertise a pipeline the
// page can never run. Network cmdlets are excluded for a subtler reason: they
// would work, but only against origins that allow CORS, so they would fail on
// most URLs for a reason that has nothing to do with the query.
//
// The result is a vocabulary where every listed function does what it says:
// codecs, hashes, ciphers, compression, format conversion, and the object and
// formatting cmdlets that operate purely on their input.
func WebRegistry() *Registry {
	reg := NewRegistry()

	// Encoding/Decoding
	reg.Register(base64.RegisterBase64Encode())
	reg.Register(base64.RegisterBase64Decode())
	reg.Register(base32.RegisterBase32Encode())
	reg.Register(base32.RegisterBase32Decode())
	reg.Register(base85.RegisterBase85Encode())
	reg.Register(base85.RegisterBase85Decode())
	reg.Register(binary.RegisterBinaryEncode())
	reg.Register(binary.RegisterBinaryDecode())
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
	for _, opt := range stringudf.RegisterAll() {
		reg.Register(opt)
	}

	// Format conversion
	for _, opt := range json.RegisterAll() {
		reg.Register(opt)
	}
	reg.Register(csv.RegisterCSVParse())
	reg.Register(csv.RegisterCSVStringify())
	reg.Register(csv.RegisterTSVParse())
	reg.Register(csv.RegisterTSVStringify())
	reg.Register(xml.RegisterXMLParse())
	reg.Register(xml.RegisterXMLStringify())
	reg.Register(timestamp.RegisterTimestampToDate())
	reg.Register(timestamp.RegisterDateToTimestamp())

	// Analysis
	reg.Register(entropy.RegisterEntropy())
	reg.Register(ssdeep.RegisterSSDeep())
	reg.Register(ssdeep.RegisterSSDeepCompare())

	// Ciphers
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

	// Hashes
	reg.Register(md5udf.RegisterMD5())
	reg.Register(sha1.RegisterSHA1())
	reg.Register(sha224.RegisterSHA224())
	reg.Register(sha256.RegisterSHA256())
	reg.Register(sha384.RegisterSHA384())
	reg.Register(sha512.RegisterSHA512())
	reg.Register(sha512_224.RegisterSHA512_224())
	reg.Register(sha512_256.RegisterSHA512_256())

	reg.Register(hmac.RegisterHMACMD5())
	reg.Register(hmac.RegisterHMACSHA1())
	reg.Register(hmac.RegisterHMACSHA224())
	reg.Register(hmac.RegisterHMACSHA256())
	reg.Register(hmac.RegisterHMACSHA384())
	reg.Register(hmac.RegisterHMACSHA512())
	reg.Register(hmac.RegisterHMACSHA512_224())
	reg.Register(hmac.RegisterHMACSHA512_256())

	// PowerShell cmdlets that only ever touch their input
	for _, opt := range objects.RegisterAll() {
		reg.Register(opt)
	}
	for _, opt := range formatting.RegisterAll() {
		reg.Register(opt)
	}

	// Utilitarian cmdlets. Each is a pure in-memory transform, so everything
	// here is available in the browser; the CLI registers the same set plus
	// the filesystem-bound logfile cmdlets.
	for _, opt := range number.RegisterAll() {
		reg.Register(opt)
	}
	for _, opt := range path.RegisterWeb() {
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
	// rncd measures bytes, not paths, so nothing it does needs a filesystem.
	for _, opt := range rncd.RegisterAll() {
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
	for _, opt := range aggregate.RegisterAll() {
		reg.Register(opt)
	}
	for _, opt := range domain.RegisterAll() {
		reg.Register(opt)
	}
	for _, opt := range config.RegisterAll() {
		reg.Register(opt)
	}

	// Discovery, so the page can answer "what can I call here?" with the
	// vocabulary it actually has rather than the CLI's.
	//
	// The catalog is a package-level hook, so the registry constructed last
	// owns it. That is fine because a process builds one: the CLI builds
	// DefaultRegistry, the browser builds this.
	reg.Register(discovery.RegisterGetCommand())
	reg.Register(discovery.RegisterGetHelp())
	discovery.SetCatalog(browserCatalog(reg))

	return reg
}

// browserCatalog is the command list the page's get_command and get_help
// report on, and the one the IDE's Catalog tab reads.
//
// Unlike the CLI's catalog, it documents the whole vocabulary, including the
// commands this registry excluded: a browser tab has no filesystem, process
// table or service manager, so get_childitem and get_process can only fail,
// and a reader should be able to see that they exist rather than wonder why
// they are missing. Each command carries its own answer - Available is decided
// by asking the registry which names it added, the same source of truth
// --udf-list uses, so the flag cannot drift from what the page can run.
func browserCatalog(reg *Registry) []discovery.Command {
	signatures, err := reg.Signatures()
	if err != nil {
		// Signature discovery compiles a trivial program; if that fails there
		// is nothing useful to report, and an empty catalog is honest.
		return nil
	}
	registered := make(map[string]bool, len(signatures))
	for sig := range signatures {
		registered[sig.Name] = true
	}

	commands := buildCatalog()
	for i := range commands {
		commands[i].Available = registered[commands[i].Name]
	}
	return commands
}
