package token

import (
	"fmt"

	"github.com/itchyny/gojq"
	"github.com/xen0bit/pwrq/pkg/udf/common"
	"golang.org/x/net/idna"
)

// RegisterPunycodeEncode registers punycode_encode, an internationalized
// domain or label to its ASCII (punycode) form.
func RegisterPunycodeEncode() gojq.CompilerOption {
	return gojq.WithFunction("punycode_encode", 0, 2, func(v any, args []any) any {
		s, err := strInput(v, args, "punycode_encode")
		if err != nil {
			return common.MakeUDFErrorResult(fmt.Errorf("punycode_encode: %v", err), nil)
		}
		ascii, err := idna.Lookup.ToASCII(s)
		if err != nil {
			return common.MakeUDFErrorResult(fmt.Errorf("punycode_encode: %v", err), nil)
		}
		return common.MakeUDFSuccessResult(ascii, nil)
	})
}

// RegisterPunycodeDecode registers punycode_decode, a punycode (ASCII) domain
// back to its internationalized form.
func RegisterPunycodeDecode() gojq.CompilerOption {
	return gojq.WithFunction("punycode_decode", 0, 2, func(v any, args []any) any {
		s, err := strInput(v, args, "punycode_decode")
		if err != nil {
			return common.MakeUDFErrorResult(fmt.Errorf("punycode_decode: %v", err), nil)
		}
		unicode, err := idna.Lookup.ToUnicode(s)
		if err != nil {
			return common.MakeUDFErrorResult(fmt.Errorf("punycode_decode: %v", err), nil)
		}
		return common.MakeUDFSuccessResult(unicode, nil)
	})
}
