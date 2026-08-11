package domain

import (
	"github.com/itchyny/gojq"
)

// RegisterAll registers every domain cmdlet: units, geo and finance.
func RegisterAll() []gojq.CompilerOption {
	return []gojq.CompilerOption{
		// Units: temperature
		RegisterCToF(),
		RegisterFToC(),
		RegisterCToK(),
		RegisterKToC(),
		RegisterFToK(),
		RegisterKToF(),
		// Units: length
		RegisterKmToMi(),
		RegisterMiToKm(),
		RegisterMToFt(),
		RegisterFtToM(),
		RegisterCmToIn(),
		RegisterInToCm(),
		// Units: mass
		RegisterKgToLb(),
		RegisterLbToKg(),
		RegisterGToOz(),
		RegisterOzToG(),
		// Units: volume
		RegisterLToGal(),
		RegisterGalToL(),
		// Units: speed
		RegisterMphToKph(),
		RegisterKphToMph(),
		// Units: efficiency
		RegisterMpgToL100km(),
		RegisterL100kmToMpg(),
		// Units: data
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
		RegisterRuleOf72(),
		RegisterAnnualYield(),
		RegisterNetPresentValue(),
	}
}
