// SPDX-License-Identifier: GPL-3.0-or-later

package merakiapi_test

import (
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"runtime"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/netdata/netdata/go/plugins/pkg/buildinfo"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/collector/meraki/merakiapi"
)

// The subprocess speaks the real credentialfile wire protocol, so client tests
// exercise the shipped web/credentialfile adapter. It does not model C privilege
// dropping, which is tested by credentialfile's opt-in C interoperability suite.
func TestMain(m *testing.M) {
	if len(os.Args) == 6 && os.Args[1] == "--file-reader" && os.Args[2] == "read" {
		data, err := os.ReadFile(os.Args[5])
		if err != nil {
			_, _ = fmt.Fprint(os.Stderr, "NDFILE 1 2\n")
			os.Exit(1)
		}
		if _, err := os.Stdout.Write(data); err != nil {
			os.Exit(1)
		}
		_, _ = fmt.Fprint(os.Stderr, "NDFILE 0 0\n")
		os.Exit(0)
	}
	os.Exit(m.Run())
}

func installCredentialPeer(t *testing.T) {
	t.Helper()
	if runtime.GOOS == "windows" {
		return // Windows uses the shared in-process credential-file implementation.
	}
	executable, err := os.Executable()
	require.NoError(t, err)
	dir := t.TempDir()
	require.NoError(t, os.Symlink(executable, filepath.Join(dir, "nd-run")))
	original := buildinfo.NetdataBinDir
	buildinfo.NetdataBinDir = dir
	t.Cleanup(func() { buildinfo.NetdataBinDir = original })
}

func TestBearerFileRereadAcrossRedirectsAndPages(t *testing.T) {
	installCredentialPeer(t)
	path := filepath.Join(t.TempDir(), "token")
	require.NoError(t, os.WriteFile(path, []byte("synthetic-first\n"), 0o600))
	var calls int
	var headers []string
	client, err := merakiapi.New(merakiapi.Config{
		BearerTokenFile: path,
	}, &http.Client{
		Transport: roundTripFunc(func(r *http.Request) (*http.Response, error) {
			calls++
			headers = append(headers, r.Header.Get("Authorization"))
			switch calls {
			case 1:
				require.NoError(t, os.WriteFile(path, []byte("synthetic-redirect\n"), 0o600))
				location := *r.URL
				location.Host = "n23.dashboard.meraki.com"
				return response(r, http.StatusFound, http.Header{
					"Location": {location.String()},
				}, ""), nil
			case 2:
				require.NoError(t, os.WriteFile(path, []byte("synthetic-next-page\n"), 0o600))
				return response(
					r,
					http.StatusOK,
					http.Header{
						"Link": {`<?startingAfter=2>; rel=next`},
					},
					`[{"id":"first"}]`,
				), nil
			default:
				return response(r, http.StatusOK, nil, `[{"id":"last"}]`), nil
			}
		}),
	})
	require.NoError(t, err)
	got, err := client.ListOrganizations(t.Context())
	require.NoError(t, err)
	assert.Equal(t, []merakiapi.Organization{{ID: "first"}, {ID: "last"}}, got)
	assert.Equal(
		t,
		[]string{"Bearer synthetic-first", "Bearer synthetic-redirect", "Bearer synthetic-next-page"},
		headers,
	)
	assert.Equal(t, 3, calls)
}

func TestCredentialFailureNeverFallsBackToUnauthenticatedHTTP(t *testing.T) {
	installCredentialPeer(t)
	for name, content := range map[string]*string{"missing": nil, "empty": new(string)} {
		t.Run(name, func(t *testing.T) {
			path := filepath.Join(t.TempDir(), "SYNTHETIC_PRIVATE_PATH")
			if content != nil {
				require.NoError(t, os.WriteFile(path, []byte(*content), 0o600))
			}
			var calls int
			client, err := merakiapi.New(merakiapi.Config{
				BearerTokenFile: path,
			}, &http.Client{
				Transport: roundTripFunc(func(r *http.Request) (*http.Response, error) {
					calls++
					return response(r, http.StatusOK, nil, `[]`), nil
				}),
			})
			require.NoError(t, err)
			got, err := client.ListOrganizations(t.Context())
			require.Error(t, err)
			assert.Nil(t, got)
			assert.Zero(t, calls)
			assert.NotContains(t, err.Error(), path)
			assert.NotContains(t, err.Error(), "SYNTHETIC_PRIVATE_PATH")
		})
	}
}

func TestCredentialFailureOnRedirectReturnsNoInventory(t *testing.T) {
	installCredentialPeer(t)
	path := filepath.Join(t.TempDir(), "token")
	require.NoError(t, os.WriteFile(path, []byte("synthetic-first"), 0o600))
	var calls int
	client, err := merakiapi.New(merakiapi.Config{
		BearerTokenFile: path,
	}, &http.Client{
		Transport: roundTripFunc(func(r *http.Request) (*http.Response, error) {
			calls++
			require.NoError(t, os.WriteFile(path, nil, 0o600))
			location := *r.URL
			location.Host = "n23.meraki.com"
			return response(r, http.StatusFound, http.Header{
				"Location": {location.String()},
			}, ""), nil
		}),
	})
	require.NoError(t, err)
	got, err := client.ListOrganizations(t.Context())
	require.Error(t, err)
	assert.Nil(t, got)
	assert.Equal(t, 1, calls)
	assert.NotContains(t, err.Error(), path)
}
