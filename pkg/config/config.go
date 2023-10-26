package config

import (
	"bytes"
	"os"

	"github.com/atomyze-foundation/hlf-control-plane/pkg/peer"
	"go.uber.org/config"
)

// Config defines the configuration structure for your hlf-control-plane app.
type Config struct {
	LogLevel    string          `yaml:"logLevel"`
	AccessToken string          `yaml:"accessToken"`
	MspID       string          `yaml:"mspId"`
	Identity    *Identity       `yaml:"identity"`
	TLS         *TLSCredentials `yaml:"tls"`
	Peers       []*peer.Peer    `yaml:"peers"`

	Listen struct {
		HTTP string `yaml:"http"`
		GRPC string `yaml:"grpc"`
	} `yaml:"listen"`

	HostMatcher map[string]string `yaml:"hostMatcher"`
}

// Load config by file path
func Load(path string) (*Config, error) {
	var c Config
	pr, err := config.NewYAML(config.Expand(os.LookupEnv), config.File(path))
	if err != nil {
		return nil, err
	}
	return &c, pr.Get(config.Root).Populate(&c)
}

// LoadBytes creates config from byte slice
func LoadBytes(b []byte) (*Config, error) {
	var c Config
	pr, err := config.NewYAML(config.Expand(os.LookupEnv), config.Source(bytes.NewBuffer(b)))
	if err != nil {
		return nil, err
	}
	return &c, pr.Get(config.Root).Populate(&c)
}
