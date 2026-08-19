// SPDX-License-Identifier: GPL-3.0-or-later

package tlscfg

import (
	"crypto/tls"
	"crypto/x509"
	"errors"
	"fmt"

	"github.com/netdata/netdata/go/plugins/pkg/safefile"
)

// ErrTLSFile identifies TLS file read and parse failures.
var ErrTLSFile = errors.New("TLS file is invalid")

type fileError struct {
	err error
}

func (e *fileError) Error() string {
	return e.err.Error()
}

func (e *fileError) Unwrap() error {
	return e.err
}

func (e *fileError) Is(target error) bool {
	return target == ErrTLSFile || errors.Is(e.err, target)
}

// TLSConfig represents the standard client TLS configuration.
type TLSConfig struct {
	// TLSCA specifies the certificate authority to use when verifying server certificates.
	TLSCA string `yaml:"tls_ca,omitempty" json:"tls_ca"`

	// TLSCert specifies tls certificate file.
	TLSCert string `yaml:"tls_cert,omitempty" json:"tls_cert"`

	// TLSKey specifies tls key file.
	TLSKey string `yaml:"tls_key,omitempty" json:"tls_key"`

	// InsecureSkipVerify controls whether a client verifies the server's certificate chain and host name.
	InsecureSkipVerify bool `yaml:"tls_skip_verify,omitempty" json:"tls_skip_verify"`
}

// NewTLSConfig creates a tls.Config, may be nil without an error if TLS is not configured.
func NewTLSConfig(cfg TLSConfig) (*tls.Config, error) {
	if cfg.TLSCA == "" && cfg.TLSKey == "" && cfg.TLSCert == "" && !cfg.InsecureSkipVerify {
		return nil, nil
	}

	tlsConfig := &tls.Config{
		InsecureSkipVerify: cfg.InsecureSkipVerify,
		Renegotiation:      tls.RenegotiateNever,
	}

	if cfg.TLSCA != "" {
		pool, err := loadCertPool([]string{cfg.TLSCA})
		if err != nil {
			return nil, err
		}
		tlsConfig.RootCAs = pool
	}

	if cfg.TLSCert != "" && cfg.TLSKey != "" {
		cert, err := loadCertificate(cfg.TLSCert, cfg.TLSKey)
		if err != nil {
			return nil, err
		}
		tlsConfig.Certificates = []tls.Certificate{cert}
	}

	return tlsConfig, nil
}

func loadCertPool(certFiles []string) (*x509.CertPool, error) {
	pool := x509.NewCertPool()
	for _, certFile := range certFiles {
		pem, err := safefile.Read(certFile)
		if err != nil {
			return nil, newFileError(fmt.Errorf("could not read certificate %q: %w", certFile, err))
		}
		if !pool.AppendCertsFromPEM(pem) {
			return nil, newFileError(fmt.Errorf("could not parse any PEM certificates %q", certFile))
		}
	}
	return pool, nil
}

func loadCertificate(certFile, keyFile string) (tls.Certificate, error) {
	certPEM, err := safefile.Read(certFile)
	if err != nil {
		return tls.Certificate{}, newFileError(fmt.Errorf("could not read certificate %q: %w", certFile, err))
	}
	keyPEM, err := safefile.Read(keyFile)
	if err != nil {
		return tls.Certificate{}, newFileError(fmt.Errorf("could not read key %q: %w", keyFile, err))
	}

	cert, err := tls.X509KeyPair(certPEM, keyPEM)
	if err != nil {
		return tls.Certificate{}, newFileError(fmt.Errorf("could not load keypair %s:%s: %w", certFile, keyFile, err))
	}
	return cert, nil
}

func newFileError(err error) error {
	return &fileError{err: err}
}
