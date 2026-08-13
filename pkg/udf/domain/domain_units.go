package domain

import (
	"fmt"
	"math"
	"strings"

	"github.com/itchyny/gojq"
	"github.com/xen0bit/pwrq/pkg/udf/common"
)

// unitConversion is a linear map to and from a base unit: value*factor+offset
// gives the base, and the inverse gives the value back. Temperature needs the
// offset; everything else leaves it zero.
type unitConversion struct {
	quantity string
	factor   float64
	offset   float64
}

// units maps a unit name to its quantity and its conversion to that quantity's
// base unit — kelvin, metre, kilogram, litre, metre per second, or litres per
// 100km. Conversion between any two units of the same quantity is then to the
// base and back, which is why this table replaces the pairwise converters:
// adding a unit here makes it convertible to every other unit of its quantity,
// rather than requiring two more functions per existing unit.
var units = map[string]unitConversion{
	// Temperature, base kelvin.
	"c": {"temperature", 1, 273.15}, "celsius": {"temperature", 1, 273.15},
	"f": {"temperature", 5.0 / 9.0, 273.15 - 32*5.0/9.0}, "fahrenheit": {"temperature", 5.0 / 9.0, 273.15 - 32*5.0/9.0},
	"k": {"temperature", 1, 0}, "kelvin": {"temperature", 1, 0},

	// Length, base metre.
	"m": {"length", 1, 0}, "metre": {"length", 1, 0}, "meter": {"length", 1, 0},
	"km": {"length", 1000, 0}, "cm": {"length", 0.01, 0}, "mm": {"length", 0.001, 0},
	"mi": {"length", 1609.344, 0}, "mile": {"length", 1609.344, 0},
	"ft": {"length", 0.3048, 0}, "foot": {"length", 0.3048, 0},
	"in": {"length", 0.0254, 0}, "inch": {"length", 0.0254, 0},
	"yd": {"length", 0.9144, 0}, "nmi": {"length", 1852, 0},

	// Mass, base kilogram.
	"kg": {"mass", 1, 0}, "g": {"mass", 0.001, 0}, "mg": {"mass", 1e-6, 0},
	"lb": {"mass", 0.45359237, 0}, "pound": {"mass", 0.45359237, 0},
	"oz": {"mass", 0.028349523125, 0}, "st": {"mass", 6.35029318, 0},
	"t": {"mass", 1000, 0}, "tonne": {"mass", 1000, 0},

	// Volume, base litre.
	"l": {"volume", 1, 0}, "litre": {"volume", 1, 0}, "liter": {"volume", 1, 0},
	"ml": {"volume", 0.001, 0}, "gal": {"volume", 3.785411784, 0},
	"qt": {"volume", 0.946352946, 0}, "pt": {"volume", 0.473176473, 0},
	"floz": {"volume", 0.0295735295625, 0},

	// Speed, base metre per second.
	"mps": {"speed", 1, 0}, "kph": {"speed", 1 / 3.6, 0}, "kmh": {"speed", 1 / 3.6, 0},
	"mph": {"speed", 0.44704, 0}, "knot": {"speed", 0.514444, 0}, "kn": {"speed", 0.514444, 0},

	// Duration, base second.
	"s": {"duration", 1, 0}, "sec": {"duration", 1, 0}, "second": {"duration", 1, 0},
	"ms": {"duration", 0.001, 0}, "min": {"duration", 60, 0}, "minute": {"duration", 60, 0},
	"h": {"duration", 3600, 0}, "hour": {"duration", 3600, 0},
	"d": {"duration", 86400, 0}, "day": {"duration", 86400, 0},
	"wk": {"duration", 604800, 0}, "week": {"duration", 604800, 0},
}

// fuelEfficiency is its own case: mpg and l/100km are reciprocal, not linear,
// so they cannot share the table above.
var fuelEfficiency = map[string]bool{"mpg": true, "l100km": true}

// RegisterConvertUnit registers convert_unit, converting a number between two
// units of the same quantity:
//
//	20 | convert_unit("C"; "F")        -> 68
//	convert_unit(5; "mi"; "km")        -> 8.04672
//
// Unit names are case-insensitive. Converting between different quantities is
// an error rather than a silently wrong number.
func RegisterConvertUnit() gojq.CompilerOption {
	return common.WithFunction("convert_unit", 2, 3, func(v any, args []any) any {
		in, rest := common.SplitInput(v, args, 2)
		value, ok := common.ToFloat64(common.BindValue(in))
		if !ok {
			return common.MakeUDFErrorResult(fmt.Errorf("convert_unit: expected a number, got %T", common.BindValue(in)), nil)
		}
		from, err := unitName(rest[0])
		if err != nil {
			return common.MakeUDFErrorResult(err, nil)
		}
		to, err := unitName(rest[1])
		if err != nil {
			return common.MakeUDFErrorResult(err, nil)
		}
		out, err := convertUnit(value, from, to)
		if err != nil {
			return common.MakeUDFErrorResult(err, nil)
		}
		return common.MakeUDFSuccessResult(out, nil)
	})
}

func unitName(a any) (string, error) {
	s, ok := common.BindValue(a).(string)
	if !ok {
		return "", fmt.Errorf("convert_unit: unit must be a string, got %T", common.BindValue(a))
	}
	return strings.ToLower(strings.TrimSpace(s)), nil
}

func convertUnit(value float64, from, to string) (float64, error) {
	if fuelEfficiency[from] && fuelEfficiency[to] {
		if from == to {
			return value, nil
		}
		if value == 0 {
			return 0, fmt.Errorf("convert_unit: cannot convert 0 %s", from)
		}
		return tidy(235.214583 / value), nil
	}
	src, ok := units[from]
	if !ok {
		return 0, fmt.Errorf("convert_unit: unknown unit %q", from)
	}
	dst, ok := units[to]
	if !ok {
		return 0, fmt.Errorf("convert_unit: unknown unit %q", to)
	}
	if src.quantity != dst.quantity {
		return 0, fmt.Errorf("convert_unit: cannot convert %s (%s) to %s (%s)", from, src.quantity, to, dst.quantity)
	}
	if from == to {
		return value, nil
	}
	base := value*src.factor + src.offset
	return tidy((base - dst.offset) / dst.factor), nil
}

// tidy rounds off the representation error a trip through the base unit
// introduces. Converting 20°C to °F is exactly 68, but going via kelvin with
// factors of 5/9 — which no binary float holds exactly — lands on
// 67.99999999999993. Rounding to 12 significant figures recovers the intended
// answer while leaving genuinely precise conversions (5 mi = 8.04672 km) alone,
// since they need far fewer digits than that.
func tidy(f float64) float64 {
	if f == 0 || math.IsInf(f, 0) || math.IsNaN(f) {
		return f
	}
	mag := math.Pow(10, 12-math.Ceil(math.Log10(math.Abs(f))))
	return math.Round(f*mag) / mag
}

// RegisterParseSize registers parse_size, a size string like "1.5 MiB" to its
// byte count. It is the inverse of human_bytes, so it uses the same binary
// units: suffixes b, k/kb/kib, m/mb/mib, g/gb/gib, t/tb/tib and their
// p/e larger forms, all powers of 1024.
func RegisterParseSize() gojq.CompilerOption {
	return common.WithFunction("parse_size", 0, 1, func(v any, args []any) any {
		input := v
		if len(args) > 0 {
			input = args[0]
		}
		s, ok := common.BindValue(input).(string)
		if !ok {
			return common.MakeUDFErrorResult(fmt.Errorf("parse_size: expected a size string, got %T", input), nil)
		}
		bytes, err := parseSize(s)
		if err != nil {
			return common.MakeUDFErrorResult(fmt.Errorf("parse_size: %v", err), nil)
		}
		return common.MakeUDFSuccessResult(bytes, nil)
	})
}

func parseSize(s string) (int64, error) {
	s = strings.TrimSpace(s)
	if s == "" {
		return 0, fmt.Errorf("empty size string")
	}
	numPart := s
	unitPart := ""
	for i := 0; i < len(s); i++ {
		if s[i] < '0' || s[i] > '9' || s[i] == '.' {
			if s[i] == '.' {
				continue
			}
			numPart = strings.TrimSpace(s[:i])
			unitPart = strings.TrimSpace(s[i:])
			break
		}
	}
	mult := float64(1)
	if unitPart != "" {
		var ok bool
		mult, ok = sizeUnit(strings.ToLower(unitPart))
		if !ok {
			return 0, fmt.Errorf("unknown unit %q", unitPart)
		}
	}
	var num float64
	if _, err := fmt.Sscanf(numPart, "%g", &num); err != nil {
		return 0, fmt.Errorf("cannot parse %q as a number", numPart)
	}
	return int64(num * mult), nil
}

func sizeUnit(unit string) (float64, bool) {
	table := map[string]float64{
		"b":   1,
		"k":   1 << 10,
		"kb":  1 << 10,
		"kib": 1 << 10,
		"m":   1 << 20,
		"mb":  1 << 20,
		"mib": 1 << 20,
		"g":   1 << 30,
		"gb":  1 << 30,
		"gib": 1 << 30,
		"t":   1 << 40,
		"tb":  1 << 40,
		"tib": 1 << 40,
		"p":   1 << 50,
		"pb":  1 << 50,
		"pib": 1 << 50,
		"e":   1 << 60,
		"eb":  1 << 60,
		"eib": 1 << 60,
	}
	m, ok := table[unit]
	return m, ok
}
