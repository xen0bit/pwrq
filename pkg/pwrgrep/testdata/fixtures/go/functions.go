package fixture

// ruleid: go-functions
func helper() {}

// ruleid: go-functions
func parse(s string) (int, error) {
	// ok: go-functions
	f := func() {}
	f()
	return 0, nil
}

type server struct{}

// ruleid: go-functions
func (s *server) Start(port int) error {
	return nil
}

// ruleid: go-functions
func (server) stop() {}

// ruleid: go-functions
func add(a int, b int) (sum int) {
	return a + b
}
