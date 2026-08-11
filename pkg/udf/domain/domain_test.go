package domain

import (
	"fmt"
	"testing"

	"github.com/itchyny/gojq"
)

func run(t *testing.T, query string, input any, options ...gojq.CompilerOption) any {
	t.Helper()
	q, err := gojq.Parse(query)
	if err != nil {
		t.Fatalf("parse %q: %v", query, err)
	}
	code, err := gojq.Compile(q, options...)
	if err != nil {
		t.Fatalf("compile %q: %v", query, err)
	}
	iter := code.Run(input)
	v, ok := iter.Next()
	if !ok {
		t.Fatalf("%q produced no result", query)
	}
	if e, isErr := v.(error); isErr {
		t.Fatalf("%q: %v", query, e)
	}
	return v
}

// runErr is run for queries expected to fail: it returns the error rather than
// ending the test with it.
func runErr(t *testing.T, query string, input any, options ...gojq.CompilerOption) (any, error) {
	t.Helper()
	q, err := gojq.Parse(query)
	if err != nil {
		return nil, err
	}
	code, err := gojq.Compile(q, options...)
	if err != nil {
		return nil, err
	}
	iter := code.Run(input)
	v, ok := iter.Next()
	if !ok {
		t.Fatalf("%q produced no result", query)
	}
	if e, isErr := v.(error); isErr {
		return nil, e
	}
	return v, nil
}

func approx(want, got float64) bool {
	d := want - got
	if d < 0 {
		d = -d
	}
	// Relative tolerance: 0.1% or 0.5 units, whichever is larger.
	tol := 0.5
	if want > 0 && want*0.001 > tol {
		tol = want * 0.001
	}
	return d < tol
}

func toF64(v any) (float64, bool) {
	switch val := v.(type) {
	case float64:
		return val, true
	case int:
		return float64(val), true
	case int64:
		return float64(val), true
	}
	return 0, false
}

func TestParseSize(t *testing.T) {
	tests := []struct {
		query string
		want  int64
	}{
		{`"1024" | parse_size`, 1024},
		{`"1.5 MiB" | parse_size`, 1572864},
		{`"1 KiB" | parse_size`, 1024},
		{`"2G" | parse_size`, 2147483648},
		{`"512 b" | parse_size`, 512},
	}
	for _, tt := range tests {
		got := run(t, tt.query, nil, RegisterAll()...)
		if fmt.Sprint(got) != fmt.Sprint(tt.want) {
			t.Errorf("%s = %v, want %d", tt.query, got, tt.want)
		}
	}
	if _, err := parseSize("1.5 banana"); err == nil {
		t.Error("parse_size accepted an unknown unit")
	}
}

func TestGeo(t *testing.T) {
	dist := run(t, `haversine_distance(51.5007; -0.1246; 40.7128; -74.0060)`, nil, RegisterAll()...)
	f, _ := toF64(dist)
	if !approx(5570, f) {
		t.Errorf("haversine_distance London-NY = %v, want ~5570 km", dist)
	}

	bearing := run(t, `bearing(37.7749; -122.4194; 34.0522; -118.2437)`, nil, RegisterAll()...)
	bf, _ := toF64(bearing)
	if !approx(136.3, bf) {
		t.Errorf("bearing = %v, want ~136.3", bearing)
	}

	mid := run(t, `geo_midpoint(0; 0; 0; 10)`, nil, RegisterAll()...)
	m, ok := mid.(map[string]any)
	if !ok {
		t.Fatalf("geo_midpoint = %T", mid)
	}
	mlat, _ := toF64(m["lat"])
	if !approx(0, mlat) {
		t.Errorf("geo_midpoint lat = %v, want 0", m["lat"])
	}
	mlon, _ := toF64(m["lon"])
	if !approx(5, mlon) {
		t.Errorf("geo_midpoint lon = %v, want 5", m["lon"])
	}

	if got := run(t, `within_radius(51.5; -0.12; 51.5007; -0.1246; 1)`, nil, RegisterAll()...); got != true {
		t.Errorf("within_radius = %v, want true", got)
	}
	if got := run(t, `within_radius(40.7; -74.0; 51.5007; -0.1246; 1)`, nil, RegisterAll()...); got != false {
		t.Errorf("within_radius = %v, want false", got)
	}

	coords := run(t, `parse_coords("51.5007, -0.1246")`, nil, RegisterAll()...)
	c, ok := coords.(map[string]any)
	if !ok {
		t.Fatalf("parse_coords = %T", coords)
	}
	if fmt.Sprint(c["lat"]) != "51.5007" {
		t.Errorf("parse_coords lat = %v", c["lat"])
	}

	enc := run(t, `geohash_encode(42.6; -5.6; 6)`, nil, RegisterAll()...)
	if got := fmt.Sprint(enc); got != "ezs42e" {
		t.Errorf("geohash_encode = %v, want ezs42e", enc)
	}

	dec := run(t, `"ezs42" | geohash_decode`, nil, RegisterAll()...)
	dm, ok := dec.(map[string]any)
	if !ok {
		t.Fatalf("geohash_decode = %T", dec)
	}
	dlat, _ := toF64(dm["lat"])
	dlon, _ := toF64(dm["lon"])
	if !approx(42.6, dlat) || !approx(-5.6, dlon) {
		t.Errorf("geohash_decode = lat %v lon %v, want ~42.6, ~-5.6", dm["lat"], dm["lon"])
	}
}

func TestFinance(t *testing.T) {
	tests := []struct {
		query string
		want  float64
	}{
		{`cagr(100; 200; 1)`, 1.0},
		{`future_value(100; 0.1; 10)`, 259.3742},
		{`present_value(259.3742; 0.1; 10)`, 100},
		{`compound_interest(100; 0.1; 10)`, 159.3742},
		{`simple_interest(100; 0.1; 3)`, 30},
		{`monthly_payment(10000; 0.06; 36)`, 304.219},
	}
	for _, tt := range tests {
		got := run(t, tt.query, nil, RegisterAll()...)
		f, ok := toF64(got)
		if !ok || !approx(tt.want, f) {
			t.Errorf("%s = %v, want %v", tt.query, got, tt.want)
		}
	}
}

func TestConvertUnit(t *testing.T) {
	tests := []struct {
		query string
		want  float64
	}{
		{`20 | convert_unit("C"; "F")`, 68},
		{`68 | convert_unit("F"; "C")`, 20},
		{`0 | convert_unit("C"; "K")`, 273.15},
		{`convert_unit(5; "mi"; "km")`, 8.04672},
		{`convert_unit(8.04672; "km"; "mi")`, 5},
		{`10 | convert_unit("lb"; "kg")`, 4.5359237},
		{`90 | convert_unit("min"; "h")`, 1.5},
		{`convert_unit(2; "d"; "h")`, 48},
		{`30 | convert_unit("mpg"; "l100km")`, 7.8404861},
		{`5 | convert_unit("m"; "m")`, 5},
		{`100 | convert_unit("CELSIUS"; "fahrenheit")`, 212},
	}
	for _, tt := range tests {
		got := run(t, tt.query, nil, RegisterAll()...)
		f, ok := toF64(got)
		if !ok || !approx(tt.want, f) {
			t.Errorf("%s = %v, want %v", tt.query, got, tt.want)
		}
	}
}

// TestConvertUnitRejectsMismatch is the property the pairwise converters could
// not have: a conversion between different quantities is an error, not a
// plausible-looking number.
func TestConvertUnitRejectsMismatch(t *testing.T) {
	for _, q := range []string{
		`5 | convert_unit("kg"; "m")`,
		`5 | convert_unit("C"; "km")`,
		`5 | convert_unit("furlong"; "m")`,
	} {
		if _, err := runErr(t, q, nil, RegisterAll()...); err == nil {
			t.Errorf("%s should be an error", q)
		}
	}
}
