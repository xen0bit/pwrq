package domain

import "github.com/xen0bit/pwrq/pkg/core/shape"

var (
	// CoordinateShape is a latitude and longitude in decimal degrees.
	CoordinateShape = shape.Plain(
		shape.Prop("lat", shape.Number, "latitude in decimal degrees"),
		shape.Prop("lon", shape.Number, "longitude in decimal degrees"),
	)

	// GeohashShape is a decoded geohash: its centre, and the half-width of the
	// cell it names. The error terms are the point of decoding a geohash
	// rather than reading a coordinate, since a geohash is an area.
	GeohashShape = shape.Plain(
		shape.Prop("lat", shape.Number, "latitude of the cell's centre, in decimal degrees"),
		shape.Prop("lon", shape.Number, "longitude of the cell's centre, in decimal degrees"),
		shape.Prop("latErr", shape.Number, "half the cell's height in degrees, so the true latitude is lat ± latErr"),
		shape.Prop("lonErr", shape.Number, "half the cell's width in degrees, so the true longitude is lon ± lonErr"),
	)
)
