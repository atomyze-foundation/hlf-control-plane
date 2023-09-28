package matcher

import (
	"fmt"
	"sync"
)

// Matcher is a simple key-value map with thread-safe access.
type Matcher struct {
	v  map[string]string
	mx sync.RWMutex
}

// Match returns the value associated with the provided host.
// If the host is not found in the map, it returns an error.
func (m *Matcher) Match(host string) (string, error) {
	m.mx.RLock()
	defer m.mx.RUnlock()
	if v, ok := m.v[host]; ok {
		return v, nil
	}
	return "", fmt.Errorf("not found")
}

// NewMatcher creates a new Matcher with the provided key-value map.
func NewMatcher(m map[string]string) *Matcher {
	return &Matcher{v: m}
}
