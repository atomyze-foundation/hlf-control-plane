package config

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestLoadBytes(t *testing.T) {
	confBytes := []byte(`{"logLevel": "warn"}`)
	conf, err := LoadBytes(confBytes)
	require.NoError(t, err)
	require.NotNil(t, conf)
	assert.Equal(t, conf.LogLevel, "warn")
}

func TestLoadBytesWithEnv(t *testing.T) {
	confBytes := []byte(`{"logLevel": "$LOG_LEVEL"}`)
	t.Setenv("LOG_LEVEL", "debug")
	conf, err := LoadBytes(confBytes)
	require.NoError(t, err)
	require.NotNil(t, conf)
	assert.Equal(t, conf.LogLevel, "debug")
}
