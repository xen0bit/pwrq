package mcpserver

import (
	"fmt"
	"slices"
	"strings"

	"github.com/itchyny/gojq"
	"github.com/xen0bit/pwrq/pkg/udf/common"
)

// Encoding mismatches: the failures that do not fail.
//
// `"x" | zlib_compress | base64_encode` compiles, runs, and returns a string
// that looks exactly like what the caller wanted. It is wrong. zlib_compress
// returns hex text, so the base64 covers "789c00130..." rather than the
// compressed bytes: the result is twice the size it should be, and the far end
// decodes it to hex digits instead of to the payload. Nothing raises an error,
// because nothing is broken - the caller asked to base64 a string and got the
// base64 of that string.
//
// This is the worst shape a problem can take over MCP. A query that fails gets
// a message and one more call; a query that quietly answers wrongly gets
// written into a result and believed. The session that prompted this work
// carried the mistake all the way into a showpiece pipeline that reported
// itself verified.
//
// Both halves of the fact are already declared at the registration sites, so
// the check is a comparison rather than a heuristic: what the left of a pipe
// produces against what the right of it expects. When they disagree the caller
// is told which cmdlet reconciles them, which is nearly always the matching
// decoder.

// encodingWarning is one mismatched pipe stage.
type encodingWarning struct {
	// Producer is the cmdlet on the left of the pipe.
	Producer string `json:"producer"`
	// Consumer is the cmdlet on the right of it.
	Consumer string `json:"consumer"`
	// Message says what disagrees and what to insert.
	Message string `json:"message"`
}

// checkEncodings reports the pipe stages whose declared encodings disagree.
//
// It reports rather than refuses. A declaration can be incomplete, a caller can
// have a reason, and a warning that blocks a working query is a worse failure
// than the one it set out to prevent.
func checkEncodings(q *gojq.Query) []encodingWarning {
	var out []encodingWarning
	walkPipes(q, func(left, right *gojq.Query) {
		producer := lastFunc(left)
		consumer := firstFunc(right)
		if producer == "" || consumer == "" {
			return
		}
		produced, _ := common.EncodingOf(producer)
		accepted := common.ConsumesOf(consumer)
		if produced == common.EncodingUnspecified || len(accepted) == 0 {
			return
		}
		if common.Accepts(consumer, produced) {
			return
		}
		out = append(out, encodingWarning{
			Producer: producer,
			Consumer: consumer,
			Message:  mismatch(producer, produced, consumer, accepted),
		})
	})
	return out
}

// mismatch writes the sentence a caller can act on: what they have, what the
// next stage wants, and the cmdlet that turns one into the other.
func mismatch(producer string, produced common.Encoding, consumer string, accepted []common.Encoding) string {
	_, inverse := common.EncodingOf(producer)
	fix := reconciler(produced, accepted)

	msg := fmt.Sprintf("%s returns %s, but %s expects %s",
		producer, produced.Article(), consumer, wants(accepted))
	switch {
	case fix != "":
		return fmt.Sprintf("%s - %s will encode the text rather than the bytes; pipe through %s first",
			msg, consumer, fix)
	case inverse != "":
		return fmt.Sprintf("%s - decode it with %s first", msg, inverse)
	default:
		return msg
	}
}

// reconciler names the cmdlet that turns one declared encoding into the other,
// for the pairs where a single cmdlet does it.
//
// Only the decode direction is listed. A caller holding hex and needing bytes
// wants hex_decode; a caller holding bytes and needing hex has already been
// told, by the cmdlet that produced them, which encoder to use.
func reconciler(produced common.Encoding, accepted []common.Encoding) string {
	if !slices.Contains(accepted, common.EncodingBytesAsText) {
		return ""
	}
	switch produced {
	case common.EncodingHex:
		return "hex_decode"
	case common.EncodingBase64:
		return "base64_decode"
	case common.EncodingBase64URL:
		return "base64url_decode"
	case common.EncodingBase32:
		return "base32_decode"
	case common.EncodingBase58:
		return "base58_decode"
	case common.EncodingBase85:
		return "base85_decode"
	case common.EncodingBinary:
		return "binary_decode"
	default:
		return ""
	}
}

// wants renders what a consumer accepts, listing the alternatives when it takes
// more than one.
func wants(accepted []common.Encoding) string {
	parts := make([]string, 0, len(accepted))
	for _, a := range accepted {
		if described := a.Article(); described != "" {
			parts = append(parts, described)
		}
	}
	return strings.Join(parts, " or ")
}

// walkPipes visits every pipe in a query, including the pipes inside
// parenthesised sub-queries, object values and function bodies - a mismatch
// buried in `{wire: (zlib_compress | base64_encode)}` is the same mistake as
// one at the top level and is harder to spot by eye.
func walkPipes(q *gojq.Query, visit func(left, right *gojq.Query)) {
	if q == nil {
		return
	}
	if q.Op == gojq.OpPipe && q.Left != nil && q.Right != nil {
		visit(q.Left, q.Right)
	}
	walkPipes(q.Left, visit)
	walkPipes(q.Right, visit)
	for _, def := range q.FuncDefs {
		walkPipes(def.Body, visit)
	}
	walkTerm(q.Term, visit)
}

func walkTerm(t *gojq.Term, visit func(left, right *gojq.Query)) {
	if t == nil {
		return
	}
	walkPipes(t.Query, visit)
	for _, arg := range funcArgs(t) {
		walkPipes(arg, visit)
	}
	if t.Object != nil {
		for _, kv := range t.Object.KeyVals {
			walkPipes(kv.Val, visit)
			walkPipes(kv.KeyQuery, visit)
		}
	}
	if t.Array != nil {
		walkPipes(t.Array.Query, visit)
	}
	if t.Unary != nil {
		walkTerm(t.Unary.Term, visit)
	}
	for _, s := range t.SuffixList {
		if s.Index != nil {
			walkPipes(s.Index.Start, visit)
			walkPipes(s.Index.End, visit)
		}
	}
}

// funcArgs returns the argument queries of a function call term, or nothing for
// any other term.
func funcArgs(t *gojq.Term) []*gojq.Query {
	if t == nil || t.Type != gojq.TermTypeFunc || t.Func == nil {
		return nil
	}
	return t.Func.Args
}

// lastFunc names the cmdlet whose value leaves the left of a pipe, following
// the chain to its final stage: in `a | b | c` the left of the outer pipe is
// `a | b`, and what reaches `c` came from `b`.
func lastFunc(q *gojq.Query) string {
	if q == nil {
		return ""
	}
	if q.Op == gojq.OpPipe && q.Right != nil {
		return lastFunc(q.Right)
	}
	return bareFunc(q.Term)
}

// firstFunc names the cmdlet that receives the value on the right of a pipe.
func firstFunc(q *gojq.Query) string {
	if q == nil {
		return ""
	}
	if q.Op == gojq.OpPipe && q.Left != nil {
		return firstFunc(q.Left)
	}
	return bareFunc(q.Term)
}

// bareFunc names a term that is a plain function call and nothing else.
//
// A suffix disqualifies it. `zlib_compress[0:4] | base64_encode` slices the hex
// before encoding it, which is a caller who knows what they are holding, and
// warning them would be noise.
func bareFunc(t *gojq.Term) string {
	if t == nil || t.Type != gojq.TermTypeFunc || t.Func == nil || len(t.SuffixList) > 0 {
		return ""
	}
	return t.Func.Name
}
