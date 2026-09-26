package fixture

import "sync"

func f(mu *sync.Mutex) {
	// ruleid: go-concurrency
	done := make(chan bool)
	// ruleid: go-concurrency
	go work(done)
	// ruleid: go-concurrency
	select {
	case <-done:
	}
	// ruleid: go-concurrency
	mu.Lock()
	// ok: go-concurrency
	buf := make([]byte, 10)
	_ = buf
}

func work(done chan bool) { done <- true }
