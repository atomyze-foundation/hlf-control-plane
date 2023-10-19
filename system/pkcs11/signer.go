//go:build pkcs11
// +build pkcs11

package pkcs11

import (
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/sha256"
	"crypto/x509"
	"encoding/pem"
	"fmt"
	"os"

	"github.com/golang/protobuf/proto" //nolint:staticcheck
	"github.com/hyperledger/fabric-protos-go/msp"
	"github.com/hyperledger/fabric/bccsp"
	"github.com/hyperledger/fabric/bccsp/factory"
	"github.com/hyperledger/fabric/protoutil"
	"go.uber.org/zap"
)

type signer struct {
	csp   bccsp.BCCSP
	ski   []byte
	cr    []byte
	mspID string
	l     *zap.Logger
}

func (s *signer) Sign(msg []byte) ([]byte, error) {
	key, err := s.csp.GetKey(s.ski)
	if err != nil {
		return nil, fmt.Errorf("get key: %w", err)
	}
	hash, err := s.csp.Hash(msg, &bccsp.SHA256Opts{})
	if err != nil {
		return nil, fmt.Errorf("hash: %w", err)
	}
	return s.csp.Sign(key, hash, nil)
}

func (s *signer) Serialize() ([]byte, error) {
	mID := &msp.SerializedIdentity{
		Mspid:   s.mspID,
		IdBytes: s.cr,
	}
	return proto.Marshal(mID)
}

func NewPKCS11Signer(log *zap.Logger, mspID string, certPath string, opts *factory.FactoryOpts) (protoutil.Signer, error) {
	var (
		err error
		s   = new(signer)
	)
	s.l = log.Named("pkcs11")
	// init bccsp factory
	if s.csp, err = factory.GetBCCSPFromOpts(opts); err != nil {
		return nil, fmt.Errorf("bccsp init: %w", err)
	}

	certBytes, err := os.ReadFile(certPath)
	if err != nil {
		return nil, fmt.Errorf("read certificate: %w", err)
	}

	s.cr = certBytes

	b, _ := pem.Decode(certBytes)
	if b == nil {
		return nil, fmt.Errorf("no certificate block")
	}

	cert, err := x509.ParseCertificate(b.Bytes)
	if err != nil {
		return nil, fmt.Errorf("parse certificate: %w", err)
	}

	pk, ok := cert.PublicKey.(*ecdsa.PublicKey)
	if !ok {
		return nil, fmt.Errorf("public key is not ecdsa")
	}
	// If AltID is provided, use it for key lookup
	if opts.PKCS11.AltID != "" {
		s.ski = []byte(opts.PKCS11.AltID)
	} else {
		hash := sha256.New()
		hash.Write(elliptic.Marshal(pk.Curve, pk.X, pk.Y))
		s.ski = hash.Sum(nil)
	}
	s.mspID = mspID
	s.l.Debug("using signer", zap.String("mspId", s.mspID), zap.ByteString("ski", s.ski))
	return s, nil
}
