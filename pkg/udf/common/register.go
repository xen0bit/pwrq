package common

import (
	"fmt"
	"sync"

	"github.com/itchyny/gojq"
	"github.com/xen0bit/pwrq/pkg/core/shape"
	"github.com/xen0bit/pwrq/pkg/core/typed"
)

// This file is the boundary between cmdlet code and the query engine.
//
// gojq's value space is nil, bool, int, float64, *big.Int, json.Number, string,
// []any and map[string]any. gojq panics - it does not error - on anything else,
// and not only when encoding: `type`, arithmetic, comparison and most other
// builtins panic the moment they inspect such a value. A cmdlet that returns an
// int32 in a map therefore plants a crash that goes off whenever some later
// query happens to touch that field, which is a long way from where the mistake
// was made.
//
// Every cmdlet is registered through the wrappers below, so its results are
// normalized on the way out and the mistake stops being possible. Registering
// with gojq.WithFunction directly bypasses this; use these instead.

// WithFunction registers a cmdlet, normalizing whatever it returns into gojq's
// value space. It is a drop-in replacement for gojq.WithFunction.
func WithFunction(name string, minArity, maxArity int, f func(any, []any) any) gojq.CompilerOption {
	recordEmission(name, false)
	return gojq.WithFunction(name, minArity, maxArity, func(v any, args []any) any {
		return normalizeResult(f(v, args))
	})
}

// WithIterFunction registers a streaming cmdlet, normalizing each value it
// yields. It is a drop-in replacement for gojq.WithIterFunction.
func WithIterFunction(name string, minArity, maxArity int, f func(any, []any) gojq.Iter) gojq.CompilerOption {
	recordEmission(name, true)
	return gojq.WithIterFunction(name, minArity, maxArity, func(v any, args []any) gojq.Iter {
		inner := f(v, args)
		if inner == nil {
			return gojq.NewIter[any]()
		}
		return &normalizingIter{inner: inner}
	})
}

// WithFunctionOf is WithFunction for a cmdlet that emits an object, declaring
// the shape of that object as it registers.
//
// The declaration belongs here for the same reason the streaming flag does: a
// shape written down anywhere else is a second copy of a fact, and a second
// copy is a copy that can fall behind. Registering is the one thing every
// cmdlet does, so it is the one place a declaration cannot be forgotten.
func WithFunctionOf(name string, minArity, maxArity int, s *shape.Shape, f func(any, []any) any) gojq.CompilerOption {
	recordShape(name, s)
	return WithFunction(name, minArity, maxArity, f)
}

// WithIterFunctionOf is WithIterFunction for a streaming cmdlet that emits
// objects, declaring the shape of one emitted value - not of the stream.
func WithIterFunctionOf(name string, minArity, maxArity int, s *shape.Shape, f func(any, []any) gojq.Iter) gojq.CompilerOption {
	recordShape(name, s)
	return WithIterFunction(name, minArity, maxArity, f)
}

// Whether a cmdlet emits one value or a stream of them is the single fact
// callers most often get wrong: it decides whether a query needs to collect
// with [...] or must not. It is also the fact most likely to rot in a
// hand-written table, because nothing forces the table to agree with the code.
//
// So it is not written down anywhere. Choosing WithIterFunction over
// WithFunction *is* the declaration, and it is recorded here as the cmdlet is
// registered. get_help reports what these wrappers observed, which means the
// documentation cannot disagree with the behaviour.
var (
	emissionMu   sync.RWMutex
	streamingUDF = make(map[string]bool)
)

func recordEmission(name string, streaming bool) {
	emissionMu.Lock()
	defer emissionMu.Unlock()
	streamingUDF[name] = streaming
}

// IsStreaming reports whether the named cmdlet emits a stream of values rather
// than a single one, and whether it was registered through these wrappers at
// all.
func IsStreaming(name string) (streaming, known bool) {
	emissionMu.RLock()
	defer emissionMu.RUnlock()
	streaming, known = streamingUDF[name]
	return streaming, known
}

// The shape a cmdlet emits is recorded beside the streaming flag, for the same
// reason and with the same guarantee: it is written down where the cmdlet is
// registered, so it cannot name a cmdlet that does not exist and cannot be
// left behind when one is renamed.
//
// Most cmdlets have no entry here, and that is deliberate rather than
// unfinished. Roughly 300 of them are transforms returning a string or a
// number, where a property list would be an invention; declaring one for each
// would be 300 chances to write something untrue. They report nothing, and
// their output is described by observation at the point of use instead.
var (
	shapeMu       sync.RWMutex
	shapeOfUDF    = make(map[string]*shape.Shape)
	inputOfUDF    = make(map[string]InputForm)
	encodingOfUDF = make(map[string]encodingNote)
)

func recordShape(name string, s *shape.Shape) {
	shapeMu.Lock()
	defer shapeMu.Unlock()
	shapeOfUDF[name] = s
}

// ShapeOf reports the declared shape of the named cmdlet's output. A cmdlet
// that declared none returns nil, which every Shape method handles: an
// undeclared shape describes itself as nothing rather than as a guess.
func ShapeOf(name string) *shape.Shape {
	shapeMu.RLock()
	defer shapeMu.RUnlock()
	return shapeOfUDF[name]
}

// InputForm says where a cmdlet takes its input from.
type InputForm int

const (
	// InputUnspecified is a cmdlet that has not said.
	InputUnspecified InputForm = iota
	// InputPipeline is the SplitInput convention: at the cmdlet's lowest
	// arity the input arrives from the pipeline and every argument is an
	// operand, and at its highest the first argument is the input.
	InputPipeline
	// InputArguments is a cmdlet that ignores the piped value and reads
	// everything from its arguments.
	InputArguments
)

// DeclareInput records where a cmdlet's input comes from.
//
// This is the fact SplitInput encodes and no catalogue can see. `chunks(2)`
// takes its input from the pipeline and `chunks([1,2,3,4]; 2)` takes it as the
// leading argument, and the arity range - 1 to 2 - says neither. A caller
// reading `chunks/1-2` cannot tell which argument is the data, which is the
// input-side twin of not knowing whether to collect the output with [...].
func DeclareInput(name string, form InputForm) {
	shapeMu.Lock()
	defer shapeMu.Unlock()
	inputOfUDF[name] = form
}

// Encoding says how a cmdlet's output is written down, for the cmdlets whose
// result is a text rendering of bytes rather than the bytes themselves.
//
// This is not a cosmetic fact. `zlib_compress` returns "789c…" because one line
// of it reads fmt.Sprintf("%x", compressed), and nothing said so: not the name,
// not the description, not the examples, not get_help. A model over MCP spent
// roughly twenty tool calls and three successive wrong theories - the server
// strips whitespace, hex_decode yields bytes, the endpoint decodes twice -
// working out by experiment what one sentence would have told it.
//
// The declaration covers the whole family rather than the cmdlets that happen
// to have caused trouble. If only zlib_compress declared hex, a caller would
// reasonably infer that sha256 does not, and a catalogue that is right in one
// place and silent in the next is worse than one that is silent throughout.
type Encoding int

const (
	// EncodingUnspecified is a cmdlet that has not said, which is nearly all
	// of them: a cmdlet returning a number, an object or ordinary text has
	// nothing to explain.
	EncodingUnspecified Encoding = iota
	// EncodingHex is lower-case hexadecimal, two characters per byte.
	EncodingHex
	// EncodingBase64 is standard base64 with padding.
	EncodingBase64
	// EncodingBase64URL is unpadded URL-safe base64.
	EncodingBase64URL
	// EncodingBase32 is standard base32.
	EncodingBase32
	// EncodingBase58 is bitcoin-alphabet base58.
	EncodingBase58
	// EncodingBase85 is Ascii85.
	EncodingBase85
	// EncodingBinary is one group of eight '0' and '1' characters per byte,
	// separated by spaces.
	EncodingBinary
	// EncodingBytesAsText is raw bytes carried in a string, which is what the
	// decoders return. It is worth declaring because the bytes are usually not
	// valid text: `length` counts runes and will disagree with the byte count,
	// and printing the value shows replacement characters that are an artefact
	// of the display rather than of the data.
	EncodingBytesAsText
)

// Describe explains an encoding in one line, in the terms a caller writing the
// next stage of the pipeline needs.
func (e Encoding) Describe() string {
	switch e {
	case EncodingHex:
		return "a lower-case hex string, two characters per byte - not the raw bytes"
	case EncodingBase64:
		return "a standard base64 string, with padding"
	case EncodingBase64URL:
		return "an unpadded URL-safe base64 string"
	case EncodingBase32:
		return "a standard base32 string"
	case EncodingBase58:
		return "a base58 string"
	case EncodingBase85:
		return "an Ascii85 string"
	case EncodingBinary:
		return "groups of eight 0 and 1 characters, one group per byte, separated by spaces"
	case EncodingBytesAsText:
		return "raw bytes in a string, which may not be valid text - count them with utf8bytelength, not length"
	default:
		return ""
	}
}

// DeclareEncoding records how a cmdlet's output is encoded, and optionally the
// cmdlet that reverses it.
//
// The inverse is half the value. "returns hex" tells a caller what they are
// holding; "pair with zlib_decompress" tells them what to do with it, and it is
// the sentence that would have ended the session described above in one call
// rather than twenty.
func DeclareEncoding(name string, e Encoding, inverse string) {
	shapeMu.Lock()
	defer shapeMu.Unlock()
	encodingOfUDF[name] = encodingNote{encoding: e, inverse: inverse}
}

// EncodingOf reports the declared encoding of the named cmdlet's output, and
// the cmdlet that reverses it. A cmdlet that declared none returns
// EncodingUnspecified, which describes itself as nothing rather than as a
// guess.
func EncodingOf(name string) (Encoding, string) {
	shapeMu.RLock()
	defer shapeMu.RUnlock()
	note := encodingOfUDF[name]
	return note.encoding, note.inverse
}

// DescribeEncoding renders a cmdlet's declared encoding as the sentence
// get_help and list_functions print, or "" when it declared none.
func DescribeEncoding(name string) string {
	encoding, inverse := EncodingOf(name)
	described := encoding.Describe()
	if described == "" {
		return ""
	}
	if inverse == "" {
		return described
	}
	return described + "; reverse it with " + inverse
}

type encodingNote struct {
	encoding Encoding
	inverse  string
}

// InputOf reports where the named cmdlet takes its input from.
func InputOf(name string) InputForm {
	shapeMu.RLock()
	defer shapeMu.RUnlock()
	return inputOfUDF[name]
}

// Describe explains an input form in the terms a caller writing the call needs,
// given the arity range the cmdlet was registered with.
func (f InputForm) Describe(minArity, maxArity int) string {
	switch f {
	case InputPipeline:
		if maxArity > minArity {
			return fmt.Sprintf("the value from the pipeline, or the first of %d arguments", maxArity)
		}
		return "the value from the pipeline"
	case InputArguments:
		return "its arguments; the piped value is ignored"
	default:
		return ""
	}
}

// normalizeResult puts one cmdlet result into gojq's value space.
func normalizeResult(v any) any {
	// An error is how a cmdlet reports failure: gojq routes it to jq's error
	// channel, where try/catch and the exit status can see it. NormalizeJSON
	// would turn it into its message string, so a failure would come back as
	// an ordinary successful value.
	if _, isErr := v.(error); isErr {
		return v
	}
	// A value too deep to normalize is passed through untouched rather than
	// dropped: the encoder refuses it with a query error, which is a better
	// answer than a silently mangled result.
	normalized, _ := typed.EnsureJSON(v)
	return normalized
}

// normalizingIter normalizes each value a streaming cmdlet yields.
type normalizingIter struct {
	inner gojq.Iter
}

func (it *normalizingIter) Next() (any, bool) {
	v, ok := it.inner.Next()
	if !ok {
		return nil, false
	}
	return normalizeResult(v), true
}
