package util

import (
	"crypto/rand"
	"fmt"
)

const nonceLen = 24

// GetRandomNonce generates new random nonce
func GetRandomNonce() ([]byte, error) {
	key := make([]byte, nonceLen)

	_, err := rand.Read(key)
	if err != nil {
		return nil, fmt.Errorf("error getting random bytes: %w", err)
	}
	return key, nil
}
