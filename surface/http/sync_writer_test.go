package httpapi

import (
	"io"
	"sync"
)

// syncWriter lets the server's goroutines log into one buffer.
type syncWriter struct {
	mu sync.Mutex
	w  io.Writer
}

func (s *syncWriter) Write(p []byte) (int, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.w.Write(p)
}
