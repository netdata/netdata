// SPDX-License-Identifier: GPL-3.0-or-later

package gcp

import (
	"context"
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"encoding/base64"
	"encoding/json"
	"encoding/pem"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/netdata/netdata/go/plugins/pkg/credentialfile/testutil"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestServiceAccountFileRereadPreservesUnboundedInput(t *testing.T) {
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	require.NoError(t, err)
	keyDER, err := x509.MarshalPKCS8PrivateKey(key)
	require.NoError(t, err)
	keyPEM := pem.EncodeToMemory(&pem.Block{Type: "PRIVATE KEY", Bytes: keyDER})
	path := filepath.Join(t.TempDir(), "service-account.json")
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	local := testutil.New()
	reads := 0
	s := &publishedStore{runtime: &runtime{
		readFile: func(got context.Context, path string) ([]byte, error) {
			assert.Equal(t, ctx, got)
			reads++
			return local.ReadAll(got, path)
		},
		apiClient: &http.Client{Transport: roundTripFunc(func(req *http.Request) (*http.Response, error) {
			require.NoError(t, req.ParseForm())
			parts := strings.Split(req.Form.Get("assertion"), ".")
			require.Len(t, parts, 3)
			payload, err := base64.RawURLEncoding.DecodeString(parts[1])
			require.NoError(t, err)
			var claims map[string]any
			require.NoError(t, json.Unmarshal(payload, &claims))
			body, err := json.Marshal(map[string]any{"access_token": claims["iss"]})
			require.NoError(t, err)
			return &http.Response{StatusCode: http.StatusOK, Header: make(http.Header), Body: io.NopCloser(strings.NewReader(string(body)))}, nil
		})},
	}, mode: "service_account_file", serviceAccountFilePath: path}
	for _, email := range []string{"first@example.test", "second@example.test"} {
		data, err := json.Marshal(map[string]string{
			"client_email": email, "private_key": string(keyPEM), "token_uri": "https://token.example/exchange",
			"unused": strings.Repeat("x", (1<<20)+1),
		})
		require.NoError(t, err)
		require.NoError(t, os.WriteFile(path, data, 0o600))
		token, err := s.accessToken(ctx)
		require.NoError(t, err)
		assert.Equal(t, email, token)
	}
	assert.Equal(t, 2, reads)
}

func TestServiceAccountFileReadErrors(t *testing.T) {
	t.Run("missing file", func(t *testing.T) {
		path := filepath.Join(t.TempDir(), "missing")
		s := &publishedStore{runtime: &runtime{readFile: testutil.New().ReadAll}}
		_, err := s.serviceAccountToken(t.Context(), path)
		require.ErrorIs(t, err, os.ErrNotExist)
	})
	t.Run("context", func(t *testing.T) {
		ctx, cancel := context.WithCancel(context.Background())
		cancel()
		s := &publishedStore{runtime: &runtime{readFile: func(got context.Context, path string) ([]byte, error) {
			assert.Equal(t, ctx, got)
			assert.Equal(t, "account", path)
			return nil, got.Err()
		}}}
		_, err := s.serviceAccountToken(ctx, "account")
		require.ErrorIs(t, err, context.Canceled)
	})
}
