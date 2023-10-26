package config

import (
	"crypto/tls"
	"crypto/x509"
	"fmt"
	"os"
)

// TLSCredentials represents the TLS (Transport Layer Security) credentials configuration
// for secure communication.
type TLSCredentials struct {
	Cert string `yaml:"cert"`
	Key  string `yaml:"key"`
	CA   string `yaml:"ca"`
}

// TLSConfig generates a TLS (Transport Layer Security) configuration using the specified TLS
// certificate, private key, and Certificate Authority (CA) information.
// It creates a TLS configuration suitable for secure communication.
func (c *TLSCredentials) TLSConfig() (*tls.Config, error) {
	// Load certificate of the CA who signed server's certificate
	pemServerCA, err := os.ReadFile(c.CA)
	if err != nil {
		return nil, err
	}

	certPool := x509.NewCertPool()

	if !certPool.AppendCertsFromPEM(pemServerCA) {
		return nil, fmt.Errorf("failed to add server CA's certificate")
	}

	cert, err := tls.LoadX509KeyPair(c.Cert, c.Key)
	if err != nil {
		return nil, fmt.Errorf("load keypair: %w", err)
	}

	// Create the credentials and return it
	conf := &tls.Config{
		Certificates: []tls.Certificate{cert},
		RootCAs:      certPool,
	}

	return conf, nil
}
