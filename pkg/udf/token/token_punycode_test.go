package token

import (
	"fmt"
	"testing"
)

func TestPunycode(t *testing.T) {
	// "bücher.example" encodes to "xn--bcher-kva.example".
	if got := fmt.Sprint(run(t, `"bücher.example" | punycode_encode`)); got != "xn--bcher-kva.example" {
		t.Errorf("punycode_encode = %s", got)
	}
	if got := fmt.Sprint(run(t, `"xn--bcher-kva.example" | punycode_decode`)); got != "bücher.example" {
		t.Errorf("punycode_decode = %s", got)
	}
}
