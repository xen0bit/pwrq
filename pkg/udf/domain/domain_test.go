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

func TestTemperature(t *testing.T) {
	tests := []struct {
		query string
		want  float64
	}{
		{`100 | c_to_f`, 212},
		{`212 | f_to_c`, 100},
		{`0 | c_to_k`, 273.15},
		{`273.15 | k_to_c`, 0},
		{`32 | f_to_k`, 273.15},
		{`300 | k_to_f`, 80.33},
	}
	for _, tt := range tests {
		got := run(t, tt.query, nil, RegisterAll()...)
		f, ok := toF64(got)
		if !ok || !approx(tt.want, f) {
			t.Errorf("%s = %v, want %v", tt.query, got, tt.want)
		}
	}
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

func TestLengthAndMass(t *testing.T) {
	tests := []struct {
		query string
		want  float64
	}{
		{`1 | km_to_mi`, 0.621371},
		{`1 | mi_to_km`, 1.609344},
		{`1 | m_to_ft`, 3.28084},
		{`1 | ft_to_m`, 0.3048},
		{`1 | cm_to_in`, 0.393701},
		{`1 | in_to_cm`, 2.54},
		{`1 | kg_to_lb`, 2.20462},
		{`1 | lb_to_kg`, 0.453592},
		{`1 | g_to_oz`, 0.035274},
		{`1 | oz_to_g`, 28.3495},
		{`1 | l_to_gal`, 0.264172},
		{`1 | gal_to_l`, 3.78541},
		{`1 | mph_to_kph`, 1.609344},
		{`1 | kph_to_mph`, 0.621371},
		{`30 | mpg_to_l100km`, 7.840486},
		{`7.84 | l100km_to_mpg`, 30.0},
	}
	for _, tt := range tests {
		got := run(t, tt.query, nil, RegisterAll()...)
		f, ok := toF64(got)
		if !ok || !approx(tt.want, f) {
			t.Errorf("%s = %v, want %v", tt.query, got, tt.want)
		}
	}
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
		{`rule_of_72(0.08)`, 9},
		{`annual_yield(0.06; 12)`, 0.061678},
	}
	for _, tt := range tests {
		got := run(t, tt.query, nil, RegisterAll()...)
		f, ok := toF64(got)
		if !ok || !approx(tt.want, f) {
			t.Errorf("%s = %v, want %v", tt.query, got, tt.want)
		}
	}
}
