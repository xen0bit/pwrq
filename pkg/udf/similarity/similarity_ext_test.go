package similarity

import (
	"fmt"
	"testing"
)

func TestSimilarityPercent(t *testing.T) {
	if got := fmt.Sprint(run(t, `similarity_percent("kitten"; "sitting")`)); got != "0.5714285714285714" {
		t.Errorf("similarity_percent = %s", got)
	}
	if got := fmt.Sprint(run(t, `similarity_percent("abc"; "abc")`)); got != "1" {
		t.Errorf("similarity_percent identical = %s", got)
	}
}

func TestNGrams(t *testing.T) {
	got := run(t, `"hello" | n_grams(2)`)
	arr := got.([]any)
	if fmt.Sprint(arr) != "[he el ll lo]" {
		t.Errorf("n_grams = %v", arr)
	}
}

func TestJaroWinkler(t *testing.T) {
	// "MARTHA" vs "MARHTA" has a classic jaro-winkler score of ~0.961.
	if got := fmt.Sprint(run(t, `jaro_winkler("MARTHA"; "MARHTA")`)); got != "0.9611111111111111" {
		t.Errorf("jaro_winkler = %s", got)
	}
	if got := fmt.Sprint(run(t, `jaro_winkler("abc"; "xyz")`)); got != "0" {
		t.Errorf("jaro_winkler disjoint = %s", got)
	}
}
