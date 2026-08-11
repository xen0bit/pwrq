package domain

import (
	"fmt"
	"strings"

	"github.com/itchyny/gojq"
	"github.com/xen0bit/pwrq/pkg/udf/common"
)

// convertUnary registers a one-number converter like c_to_f: it reads a number
// from the pipeline or the first argument and applies fn.
func convertUnary(name string, fn func(float64) float64) gojq.CompilerOption {
	return gojq.WithFunction(name, 0, 1, func(v any, args []any) any {
		input := v
		if len(args) > 0 {
			input = args[0]
		}
		f, ok := common.ToFloat64(common.BindValue(input))
		if !ok {
			return common.MakeUDFErrorResult(fmt.Errorf("%s: expected a number, got %T", name, input), nil)
		}
		return common.MakeUDFSuccessResult(fn(f), nil)
	})
}

// RegisterCToF registers c_to_f, Celsius to Fahrenheit.
func RegisterCToF() gojq.CompilerOption {
	return convertUnary("c_to_f", func(c float64) float64 { return c*9/5 + 32 })
}

// RegisterFToC registers f_to_c, Fahrenheit to Celsius.
func RegisterFToC() gojq.CompilerOption {
	return convertUnary("f_to_c", func(f float64) float64 { return (f - 32) * 5 / 9 })
}

// RegisterCToK registers c_to_k, Celsius to Kelvin.
func RegisterCToK() gojq.CompilerOption {
	return convertUnary("c_to_k", func(c float64) float64 { return c + 273.15 })
}

// RegisterKToC registers k_to_c, Kelvin to Celsius.
func RegisterKToC() gojq.CompilerOption {
	return convertUnary("k_to_c", func(k float64) float64 { return k - 273.15 })
}

// RegisterFToK registers f_to_k, Fahrenheit to Kelvin.
func RegisterFToK() gojq.CompilerOption {
	return convertUnary("f_to_k", func(f float64) float64 { return (f-32)*5/9 + 273.15 })
}

// RegisterKToF registers k_to_f, Kelvin to Fahrenheit.
func RegisterKToF() gojq.CompilerOption {
	return convertUnary("k_to_f", func(k float64) float64 { return (k-273.15)*9/5 + 32 })
}

// RegisterKmToMi registers km_to_mi, kilometres to miles.
func RegisterKmToMi() gojq.CompilerOption {
	return convertUnary("km_to_mi", func(km float64) float64 { return km / 1.609344 })
}

// RegisterMiToKm registers mi_to_km, miles to kilometres.
func RegisterMiToKm() gojq.CompilerOption {
	return convertUnary("mi_to_km", func(mi float64) float64 { return mi * 1.609344 })
}

// RegisterMToFt registers m_to_ft, metres to feet.
func RegisterMToFt() gojq.CompilerOption {
	return convertUnary("m_to_ft", func(m float64) float64 { return m / 0.3048 })
}

// RegisterFtToM registers ft_to_m, feet to metres.
func RegisterFtToM() gojq.CompilerOption {
	return convertUnary("ft_to_m", func(ft float64) float64 { return ft * 0.3048 })
}

// RegisterCmToIn registers cm_to_in, centimetres to inches.
func RegisterCmToIn() gojq.CompilerOption {
	return convertUnary("cm_to_in", func(cm float64) float64 { return cm / 2.54 })
}

// RegisterInToCm registers in_to_cm, inches to centimetres.
func RegisterInToCm() gojq.CompilerOption {
	return convertUnary("in_to_cm", func(in float64) float64 { return in * 2.54 })
}

// RegisterKgToLb registers kg_to_lb, kilograms to pounds.
func RegisterKgToLb() gojq.CompilerOption {
	return convertUnary("kg_to_lb", func(kg float64) float64 { return kg / 0.45359237 })
}

// RegisterLbToKg registers lb_to_kg, pounds to kilograms.
func RegisterLbToKg() gojq.CompilerOption {
	return convertUnary("lb_to_kg", func(lb float64) float64 { return lb * 0.45359237 })
}

// RegisterGToOz registers g_to_oz, grams to ounces.
func RegisterGToOz() gojq.CompilerOption {
	return convertUnary("g_to_oz", func(g float64) float64 { return g / 28.349523125 })
}

// RegisterOzToG registers oz_to_g, ounces to grams.
func RegisterOzToG() gojq.CompilerOption {
	return convertUnary("oz_to_g", func(oz float64) float64 { return oz * 28.349523125 })
}

// RegisterLToGal registers l_to_gal, litres to US gallons.
func RegisterLToGal() gojq.CompilerOption {
	return convertUnary("l_to_gal", func(l float64) float64 { return l / 3.785411784 })
}

// RegisterGalToL registers gal_to_l, US gallons to litres.
func RegisterGalToL() gojq.CompilerOption {
	return convertUnary("gal_to_l", func(gal float64) float64 { return gal * 3.785411784 })
}

// RegisterMphToKph registers mph_to_kph, miles per hour to kilometres per hour.
func RegisterMphToKph() gojq.CompilerOption {
	return convertUnary("mph_to_kph", func(mph float64) float64 { return mph * 1.609344 })
}

// RegisterKphToMph registers kph_to_mph, kilometres per hour to miles per hour.
func RegisterKphToMph() gojq.CompilerOption {
	return convertUnary("kph_to_mph", func(kph float64) float64 { return kph / 1.609344 })
}

// RegisterMpgToL100km registers mpg_to_l100km, US miles per gallon to litres per
// 100 kilometres.
func RegisterMpgToL100km() gojq.CompilerOption {
	return convertUnary("mpg_to_l100km", func(mpg float64) float64 { return 235.214583 / mpg })
}

// RegisterL100kmToMpg registers l100km_to_mpg, litres per 100 kilometres to US
// miles per gallon.
func RegisterL100kmToMpg() gojq.CompilerOption {
	return convertUnary("l100km_to_mpg", func(l float64) float64 { return 235.214583 / l })
}

// RegisterParseSize registers parse_size, a size string like "1.5 MiB" to its
// byte count. It is the inverse of human_bytes, so it uses the same binary
// units: suffixes b, k/kb/kib, m/mb/mib, g/gb/gib, t/tb/tib and their
// p/e larger forms, all powers of 1024.
func RegisterParseSize() gojq.CompilerOption {
	return gojq.WithFunction("parse_size", 0, 1, func(v any, args []any) any {
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
