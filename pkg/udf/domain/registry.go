package domain

import (
	"github.com/itchyny/gojq"
)

// RegisterAll registers every domain cmdlet: units, geo and finance.
func RegisterAll() []gojq.CompilerOption {
	return []gojq.CompilerOption{
		// Units
		RegisterConvertUnit(),
		RegisterParseSize(),
		// Geo
		RegisterHaversineDistance(),
		RegisterBearing(),
		RegisterGeoMidpoint(),
		RegisterWithinRadius(),
		RegisterParseCoords(),
		RegisterGeohashEncode(),
		RegisterGeohashDecode(),
		// Finance
		RegisterCAGR(),
		RegisterFutureValue(),
		RegisterPresentValue(),
		RegisterMonthlyPayment(),
		RegisterCompoundInterest(),
		RegisterSimpleInterest(),
		RegisterNetPresentValue(),
	}
}
