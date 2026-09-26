package fixture

// ruleid: go-types
type Server struct {
	Name string
}

// ruleid: go-types
type Handler interface {
	Serve()
}

// ruleid: go-types
type Celsius float64

// ok: go-types
var s Server
