// SPDX-License-Identifier: GPL-3.0-or-later

package collecttest

import (
	"crypto/ed25519"
	"crypto/rand"
	"crypto/x509"
	"encoding/pem"
	"errors"
	"math/big"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/netdata/netdata/go/plugins/plugin/go.d/pkg/logs"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// AssertErrorOmits checks the complete error chain, not just its outer message.
func AssertErrorOmits(t *testing.T, err error, marker string) {
	t.Helper()
	for cause := err; cause != nil; cause = errors.Unwrap(cause) {
		assert.NotContains(t, cause.Error(), marker)
	}
}

// TestLogLineAssignmentDiagnostics exercises a real collector line through every parser.
func TestLogLineAssignmentDiagnostics(t *testing.T, newLine func() logs.LogLine, checkField logs.CSVCheckFieldFunc, field, marker string, wantCause error) {
	t.Helper()
	for name, test := range map[string]struct {
		config logs.ParserConfig
		row    string
	}{
		"CSV": {
			config: logs.ParserConfig{LogType: logs.TypeCSV, CSV: logs.CSVConfig{CheckField: checkField, Format: "$" + field}},
			row:    marker,
		},
		"regexp": {
			config: logs.ParserConfig{LogType: logs.TypeRegExp, RegExp: logs.RegExpConfig{Pattern: "(?P<" + field + ">.*)"}},
			row:    marker,
		},
		"LTSV": {config: logs.ParserConfig{LogType: logs.TypeLTSV}, row: field + ":" + marker},
		"JSON": {config: logs.ParserConfig{LogType: logs.TypeJSON}, row: `{"` + field + `":"` + marker + `"}`},
	} {
		t.Run(name, func(t *testing.T) {
			parser, err := logs.NewParser(test.config, strings.NewReader(""))
			require.NoError(t, err)
			err = parser.Parse([]byte(test.row), newLine())
			require.Error(t, err)
			assert.True(t, logs.IsParseError(err))
			assert.ErrorIs(t, err, wantCause)
			AssertErrorOmits(t, err, marker)
		})
	}
}

// CertificateWithInvalidURI makes a signed PEM whose rejected URI carries the marker.
func CertificateWithInvalidURI(t *testing.T, marker string) []byte {
	t.Helper()
	pub, key, err := ed25519.GenerateKey(rand.Reader)
	require.NoError(t, err)
	cert := &x509.Certificate{
		SerialNumber: big.NewInt(1),
		NotBefore:    time.Unix(1, 0),
		NotAfter:     time.Unix(2, 0),
		URIs:         []*url.URL{{Scheme: "https", Host: "[" + marker}},
	}
	der, err := x509.CreateCertificate(rand.Reader, cert, cert, pub, key)
	require.NoError(t, err)
	return pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: der})
}
