package matcher

import (
	"fmt"
	"sync"
)

type Matcher struct {
	v  map[string]string
	mx sync.RWMutex
}

func (m *Matcher) Match(host string) (string, error) {
	m.mx.RLock()
	defer m.mx.RUnlock()
	if v, ok := m.v[host]; ok {
		return v, nil
	}
	return "", fmt.Errorf("not found")
}

func NewMatcher(m map[string]string) *Matcher {
	return &Matcher{v: m}
}
