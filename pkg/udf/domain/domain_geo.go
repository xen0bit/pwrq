package domain

import (
	"fmt"
	"math"
	"strings"

	"github.com/itchyny/gojq"
	"github.com/xen0bit/pwrq/pkg/core/shape"
	"github.com/xen0bit/pwrq/pkg/udf/common"
)

// registerCoords4 registers a cmdlet taking four numeric arguments: lat1, lon1,
// lat2, lon2.
func registerCoords4(name string, fn func(lat1, lon1, lat2, lon2 float64) any) gojq.CompilerOption {
	return registerCoords4Of(name, nil, fn)
}

// registerCoords4Of is registerCoords4 for the one member of the family that
// returns an object rather than a number, so its shape is declared where it is
// registered like every other cmdlet's.
func registerCoords4Of(name string, s *shape.Shape, fn func(lat1, lon1, lat2, lon2 float64) any) gojq.CompilerOption {
	return common.WithFunctionOf(name, 4, 4, s, func(v any, args []any) any {
		nums := make([]float64, 4)
		for i := range nums {
			f, ok := common.ToFloat64(common.BindValue(args[i]))
			if !ok {
				return common.MakeUDFErrorResult(fmt.Errorf("%s: argument %d is not a number (%T)", name, i+1, args[i]), nil)
			}
			nums[i] = f
		}
		return common.MakeUDFSuccessResult(fn(nums[0], nums[1], nums[2], nums[3]), nil)
	})
}

const earthRadiusKm = 6371.0088

func degToRad(d float64) float64 { return d * math.Pi / 180 }
func radToDeg(r float64) float64 { return r * 180 / math.Pi }

// RegisterHaversineDistance registers haversine_distance, the great-circle
// distance in kilometres between two coordinates.
func RegisterHaversineDistance() gojq.CompilerOption {
	return registerCoords4("haversine_distance", func(lat1, lon1, lat2, lon2 float64) any {
		dLat := degToRad(lat2 - lat1)
		dLon := degToRad(lon2 - lon1)
		a := math.Sin(dLat/2)*math.Sin(dLat/2) +
			math.Cos(degToRad(lat1))*math.Cos(degToRad(lat2))*
				math.Sin(dLon/2)*math.Sin(dLon/2)
		return 2 * earthRadiusKm * math.Asin(math.Sqrt(a))
	})
}

// RegisterBearing registers bearing, the initial compass bearing in degrees
// (0-360, clockwise from north) from one coordinate to another.
func RegisterBearing() gojq.CompilerOption {
	return registerCoords4("bearing", func(lat1, lon1, lat2, lon2 float64) any {
		dLon := degToRad(lon2 - lon1)
		y := math.Sin(dLon) * math.Cos(degToRad(lat2))
		x := math.Cos(degToRad(lat1))*math.Sin(degToRad(lat2)) -
			math.Sin(degToRad(lat1))*math.Cos(degToRad(lat2))*math.Cos(dLon)
		bearing := radToDeg(math.Atan2(y, x))
		if bearing < 0 {
			bearing += 360
		}
		return bearing
	})
}

// RegisterGeoMidpoint registers geo_midpoint, the halfway point of the
// great-circle arc between two coordinates.
func RegisterGeoMidpoint() gojq.CompilerOption {
	return registerCoords4Of("geo_midpoint", CoordinateShape, func(lat1, lon1, lat2, lon2 float64) any {
		φ1, λ1 := degToRad(lat1), degToRad(lon1)
		φ2, λ2 := degToRad(lat2), degToRad(lon2)
		bx := math.Cos(φ2) * math.Cos(λ2-λ1)
		by := math.Cos(φ2) * math.Sin(λ2-λ1)
		φ3 := math.Atan2(math.Sin(φ1)+math.Sin(φ2),
			math.Sqrt((math.Cos(φ1)+bx)*(math.Cos(φ1)+bx)+by*by))
		λ3 := λ1 + math.Atan2(by, math.Cos(φ1)+bx)
		return CoordinateShape.Build(map[string]any{"lat": radToDeg(φ3), "lon": radToDeg(λ3)})
	})
}

// RegisterWithinRadius registers within_radius, whether a point is within km of
// a centre: within_radius(lat; lon; centerLat; centerLon; km).
func RegisterWithinRadius() gojq.CompilerOption {
	return common.WithFunction("within_radius", 5, 5, func(v any, args []any) any {
		nums := make([]float64, 5)
		for i := range nums {
			f, ok := common.ToFloat64(common.BindValue(args[i]))
			if !ok {
				return common.MakeUDFErrorResult(fmt.Errorf("within_radius: argument %d is not a number (%T)", i+1, args[i]), nil)
			}
			nums[i] = f
		}
		lat, lon, clat, clon, km := nums[0], nums[1], nums[2], nums[3], nums[4]
		dLat := degToRad(lat - clat)
		dLon := degToRad(lon - clon)
		a := math.Sin(dLat/2)*math.Sin(dLat/2) +
			math.Cos(degToRad(lat))*math.Cos(degToRad(clat))*
				math.Sin(dLon/2)*math.Sin(dLon/2)
		distance := 2 * earthRadiusKm * math.Asin(math.Sqrt(a))
		return distance <= km
	})
}

// RegisterParseCoords registers parse_coords, a "lat, lon" string to an object.
func RegisterParseCoords() gojq.CompilerOption {
	return common.WithFunctionOf("parse_coords", 0, 1, CoordinateShape, func(v any, args []any) any {
		input := v
		if len(args) > 0 {
			input = args[0]
		}
		s, ok := common.BindValue(input).(string)
		if !ok {
			return common.MakeUDFErrorResult(fmt.Errorf("parse_coords: expected a coordinate string, got %T", input), nil)
		}
		lat, lon, err := parseCoords(s)
		if err != nil {
			return common.MakeUDFErrorResult(fmt.Errorf("parse_coords: %v", err), nil)
		}
		return common.MakeUDFSuccessResult(CoordinateShape.Build(map[string]any{"lat": lat, "lon": lon}), nil)
	})
}

func parseCoords(s string) (float64, float64, error) {
	parts := strings.FieldsFunc(s, func(r rune) bool {
		return r == ',' || r == ';' || r == ' '
	})
	if len(parts) < 2 {
		return 0, 0, fmt.Errorf("%q is not a lat, lon pair", s)
	}
	var lat, lon float64
	if _, err := fmt.Sscanf(parts[0], "%g", &lat); err != nil {
		return 0, 0, fmt.Errorf("latitude %q is not a number", parts[0])
	}
	if _, err := fmt.Sscanf(parts[1], "%g", &lon); err != nil {
		return 0, 0, fmt.Errorf("longitude %q is not a number", parts[1])
	}
	if lat < -90 || lat > 90 {
		return 0, 0, fmt.Errorf("latitude %g out of range", lat)
	}
	if lon < -180 || lon > 180 {
		return 0, 0, fmt.Errorf("longitude %g out of range", lon)
	}
	return lat, lon, nil
}

const geohashAlphabet = "0123456789bcdefghjkmnpqrstuvwxyz"

// RegisterGeohashEncode registers geohash_encode, a coordinate to a geohash
// string: geohash_encode(lat; lon; [precision]).
func RegisterGeohashEncode() gojq.CompilerOption {
	return common.WithFunction("geohash_encode", 2, 3, func(v any, args []any) any {
		lat, ok1 := common.ToFloat64(common.BindValue(args[0]))
		lon, ok2 := common.ToFloat64(common.BindValue(args[1]))
		if !ok1 || !ok2 {
			return common.MakeUDFErrorResult(fmt.Errorf("geohash_encode: expected lat and lon numbers"), nil)
		}
		precision := 9
		if len(args) > 2 {
			if p, ok := common.ToInt(args[2]); ok && p > 0 {
				precision = p
			}
		}
		if lat < -90 || lat > 90 || lon < -180 || lon > 180 {
			return common.MakeUDFErrorResult(fmt.Errorf("geohash_encode: coordinate out of range"), nil)
		}
		return common.MakeUDFSuccessResult(geohashEncode(lat, lon, precision), nil)
	})
}

func geohashEncode(lat, lon float64, precision int) string {
	latMin, latMax := -90.0, 90.0
	lonMin, lonMax := -180.0, 180.0
	var out strings.Builder
	bit := 0
	even := true
	ch := 0
	for out.Len() < precision {
		if even {
			mid := (lonMin + lonMax) / 2
			if lon >= mid {
				ch = ch<<1 | 1
				lonMin = mid
			} else {
				ch = ch << 1
				lonMax = mid
			}
		} else {
			mid := (latMin + latMax) / 2
			if lat >= mid {
				ch = ch<<1 | 1
				latMin = mid
			} else {
				ch = ch << 1
				latMax = mid
			}
		}
		even = !even
		bit++
		if bit == 5 {
			out.WriteByte(geohashAlphabet[ch])
			bit = 0
			ch = 0
		}
	}
	return out.String()
}

// RegisterGeohashDecode registers geohash_decode, a geohash to its centre
// coordinate as {lat, lon, latErr, lonErr}.
func RegisterGeohashDecode() gojq.CompilerOption {
	return common.WithFunctionOf("geohash_decode", 0, 1, GeohashShape, func(v any, args []any) any {
		input := v
		if len(args) > 0 {
			input = args[0]
		}
		s, ok := common.BindValue(input).(string)
		if !ok {
			return common.MakeUDFErrorResult(fmt.Errorf("geohash_decode: expected a geohash string, got %T", input), nil)
		}
		lat, lon, latErr, lonErr, err := geohashDecode(s)
		if err != nil {
			return common.MakeUDFErrorResult(fmt.Errorf("geohash_decode: %v", err), nil)
		}
		return common.MakeUDFSuccessResult(GeohashShape.Build(map[string]any{
			"lat": lat, "lon": lon, "latErr": latErr, "lonErr": lonErr,
		}), nil)
	})
}

func geohashDecode(hash string) (lat, lon, latErr, lonErr float64, err error) {
	hash = strings.ToLower(strings.TrimSpace(hash))
	if hash == "" {
		return 0, 0, 0, 0, fmt.Errorf("empty geohash")
	}
	latMin, latMax := -90.0, 90.0
	lonMin, lonMax := -180.0, 180.0
	even := true
	for i := 0; i < len(hash); i++ {
		cd := strings.IndexByte(geohashAlphabet, hash[i])
		if cd < 0 {
			return 0, 0, 0, 0, fmt.Errorf("invalid geohash character %q", hash[i])
		}
		for mask := 16; mask > 0; mask >>= 1 {
			if even {
				mid := (lonMin + lonMax) / 2
				if cd&mask != 0 {
					lonMin = mid
				} else {
					lonMax = mid
				}
			} else {
				mid := (latMin + latMax) / 2
				if cd&mask != 0 {
					latMin = mid
				} else {
					latMax = mid
				}
			}
			even = !even
		}
	}
	return (latMin + latMax) / 2, (lonMin + lonMax) / 2, (latMax - latMin) / 2, (lonMax - lonMin) / 2, nil
}
