package objects

import (
	"strconv"
	"testing"
)

// where_object filters a pipeline, so everything it does per object it does
// once per row. These measure the two operators that have to build something
// before they can compare: a regex, and a wildcard turned into one.

func benchRows(n int) []any {
	rows := make([]any, n)
	for i := range rows {
		rows[i] = map[string]any{
			"Name":  "service-" + strconv.Itoa(i) + ".log",
			"Size":  i * 37,
			"Owner": "user" + strconv.Itoa(i%17),
		}
	}
	return rows
}

var benchWhereRows = benchRows(10000)

func benchWhere(b *testing.B, op FilterOperator, value any) {
	b.Helper()
	opts := WhereObjectOptions{Property: "Name", Operator: op, Value: value}
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		out, err := whereObject(benchWhereRows, opts)
		if err != nil {
			b.Fatal(err)
		}
		if len(out) == 0 {
			b.Fatal("nothing kept, so this measures the wrong path")
		}
	}
}

func BenchmarkWhereMatch(b *testing.B) {
	benchWhere(b, FilterOperatorMatch, `service-\d*7\.log`)
}

func BenchmarkWhereLike(b *testing.B) {
	benchWhere(b, FilterOperatorLike, "service-*7.log")
}

func BenchmarkWhereEq(b *testing.B) {
	benchWhere(b, FilterOperatorEq, "service-42.log")
}

// sort_object reads each object's sort key, so what it reads and how often is
// the whole of what it costs beyond the comparisons themselves.
func BenchmarkSortObject(b *testing.B) {
	opts := SortObjectOptions{Properties: []SortProperty{
		{Name: "Owner"}, {Name: "Size", Direction: SortDirectionDescending},
	}}
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		rows := append([]any(nil), benchWhereRows...)
		out, err := sortObject(rows, opts)
		if err != nil {
			b.Fatal(err)
		}
		if len(out) != len(benchWhereRows) {
			b.Fatalf("sorted %d of %d", len(out), len(benchWhereRows))
		}
	}
}
