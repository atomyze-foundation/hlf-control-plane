package util

import "context"

// NewContext adds Logger to the Context.
func NewContext(parent context.Context, ready chan struct{}) context.Context {
	return context.WithValue(parent, ctxReady, ready)
}

// FromContext gets Logger from the Context.
func FromContext(ctx context.Context) chan struct{} {
	if val, ok := ctx.Value(ctxReady).(chan struct{}); ok {
		return val
	}

	return nil
}

type ctxKey int

const (
	ctxReady ctxKey = iota
)
