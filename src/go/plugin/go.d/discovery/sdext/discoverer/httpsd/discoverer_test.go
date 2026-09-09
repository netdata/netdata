// SPDX-License-Identifier: GPL-3.0-or-later

package httpsd

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/netdata/netdata/go/plugins/pkg/confopt"
	"github.com/netdata/netdata/go/plugins/pkg/web"
	"github.com/netdata/netdata/go/plugins/plugin/agent/discovery/sd/model"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNewDiscoverer(t *testing.T) {
	tests := map[string]struct {
		cfg      Config
		wantErr  bool
		validate func(*testing.T, *Discoverer)
	}{
		"valid defaults": {
			cfg: Config{
				HTTPConfig: web.HTTPConfig{
					RequestConfig: web.RequestConfig{URL: "http://127.0.0.1"},
				},
			},
			validate: func(t *testing.T, d *Discoverer) {
				assert.Equal(t, defaultInterval, d.interval)
				assert.Equal(t, defaultTimeout, d.client.Timeout)
				assert.Equal(t, formatAuto, d.parser.format)
			},
		},
		"explicit one shot": {
			cfg: Config{
				HTTPConfig: web.HTTPConfig{
					RequestConfig: web.RequestConfig{URL: "http://127.0.0.1"},
				},
				Interval: durationPtr(0),
			},
			validate: func(t *testing.T, d *Discoverer) {
				assert.Zero(t, d.interval)
			},
		},
		"negative interval": {
			cfg: Config{
				HTTPConfig: web.HTTPConfig{
					RequestConfig: web.RequestConfig{URL: "http://127.0.0.1"},
				},
				Interval: durationPtr(-time.Second),
			},
			wantErr: true,
		},
		"explicit format": {
			cfg: Config{
				HTTPConfig: web.HTTPConfig{
					RequestConfig: web.RequestConfig{URL: "http://127.0.0.1"},
				},
				Format: "yaml",
			},
			validate: func(t *testing.T, d *Discoverer) {
				assert.Equal(t, formatYAML, d.parser.format)
			},
		},
		"negative timeout uses default": {
			cfg: Config{
				HTTPConfig: web.HTTPConfig{
					RequestConfig: web.RequestConfig{URL: "http://127.0.0.1"},
					ClientConfig:  web.ClientConfig{Timeout: confopt.Duration(-time.Second)},
				},
			},
			validate: func(t *testing.T, d *Discoverer) {
				assert.Equal(t, defaultTimeout, d.client.Timeout)
			},
		},
		"missing url": {
			wantErr: true,
		},
		"unsupported scheme": {
			cfg: Config{
				HTTPConfig: web.HTTPConfig{
					RequestConfig: web.RequestConfig{URL: "ftp://127.0.0.1"},
				},
			},
			wantErr: true,
		},
		"unsupported format": {
			cfg: Config{
				HTTPConfig: web.HTTPConfig{
					RequestConfig: web.RequestConfig{URL: "http://127.0.0.1"},
				},
				Format: "toml",
			},
			wantErr: true,
		},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			d, err := NewDiscoverer(tc.cfg)

			if tc.wantErr {
				assert.Error(t, err)
				return
			}

			require.NoError(t, err)
			require.NotNil(t, d)
			if tc.validate != nil {
				tc.validate(t, d)
			}
		})
	}
}

func TestDiscoverer_fetchTargetGroup(t *testing.T) {
	var gotMethod, gotBody, gotHeader, gotAuth string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotMethod = r.Method
		gotHeader = r.Header.Get("X-Test")
		gotAuth = r.Header.Get("Authorization")
		bs, _ := ioReadAllString(r)
		gotBody = bs

		w.Header().Set("Content-Type", "application/json")
		_, _ = fmt.Fprint(w, `[{"name":"api","url":"http://127.0.0.1"}]`)
	}))
	defer srv.Close()

	d, err := NewDiscoverer(Config{
		HTTPConfig: web.HTTPConfig{
			RequestConfig: web.RequestConfig{
				URL:      srv.URL,
				Method:   http.MethodPost,
				Body:     "request-body",
				Username: "user",
				Password: "pass",
				Headers:  map[string]string{"X-Test": "value"},
			},
		},
	})
	require.NoError(t, err)

	tgg, err := d.fetchTargetGroup(context.Background())
	require.NoError(t, err)

	assert.Equal(t, http.MethodPost, gotMethod)
	assert.Equal(t, "request-body", gotBody)
	assert.Equal(t, "value", gotHeader)
	assert.NotEmpty(t, gotAuth)
	assert.Equal(t, fullName, tgg.Provider())
	assert.Len(t, tgg.Targets(), 1)
	assert.Contains(t, tgg.Source(), "discoverer=http,url="+srv.URL+",hash=")
}

func TestDiscoverer_NotFollowRedirects(t *testing.T) {
	var finalHit bool
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/":
			http.Redirect(w, r, "/final", http.StatusFound)
		case "/final":
			finalHit = true
			w.Header().Set("Content-Type", "application/json")
			_, _ = fmt.Fprint(w, `[]`)
		default:
			http.NotFound(w, r)
		}
	}))
	defer srv.Close()

	d, err := NewDiscoverer(Config{
		HTTPConfig: web.HTTPConfig{
			RequestConfig: web.RequestConfig{URL: srv.URL},
			ClientConfig:  web.ClientConfig{NotFollowRedirect: true},
		},
	})
	require.NoError(t, err)

	_, err = d.fetchTargetGroup(context.Background())
	require.Error(t, err)
	assert.Contains(t, err.Error(), "redirect")
	assert.False(t, finalHit)
}

func TestDiscoverer_BearerTokenFileReread(t *testing.T) {
	tokenFile := t.TempDir() + "/token"
	require.NoError(t, os.WriteFile(tokenFile, []byte("token-1"), 0o600))

	var mu sync.Mutex
	var auths []string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		auths = append(auths, r.Header.Get("Authorization"))
		mu.Unlock()

		w.Header().Set("Content-Type", "application/json")
		_, _ = fmt.Fprint(w, `[]`)
	}))
	defer srv.Close()

	d, err := NewDiscoverer(Config{
		HTTPConfig: web.HTTPConfig{
			RequestConfig: web.RequestConfig{
				URL:             srv.URL,
				BearerTokenFile: tokenFile,
			},
		},
	})
	require.NoError(t, err)

	_, err = d.fetchTargetGroup(context.Background())
	require.NoError(t, err)
	require.NoError(t, os.WriteFile(tokenFile, []byte("token-2"), 0o600))
	_, err = d.fetchTargetGroup(context.Background())
	require.NoError(t, err)

	mu.Lock()
	defer mu.Unlock()
	require.Len(t, auths, 2)
	assert.Equal(t, "Bearer token-1", auths[0])
	assert.Equal(t, "Bearer token-2", auths[1])
}

func TestDiscoverer_ResponseBodyLimit(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		chunk := strings.Repeat("x", 1024)
		for written := int64(0); written <= responseBodyLimit; written += int64(len(chunk)) {
			if _, err := w.Write([]byte(chunk)); err != nil {
				return
			}
		}
	}))
	defer srv.Close()

	d, err := NewDiscoverer(Config{
		HTTPConfig: web.HTTPConfig{
			RequestConfig: web.RequestConfig{URL: srv.URL},
		},
	})
	require.NoError(t, err)

	_, err = d.fetchTargetGroup(context.Background())
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "response body exceeds limit")
}

func TestDiscoverer_ErrorUsesSanitizedURL(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		http.Error(w, "nope", http.StatusInternalServerError)
	}))
	defer srv.Close()

	rawURL := strings.Replace(srv.URL, "http://", "http://user:pass@", 1) + "/path?token=secret#fragment"
	d, err := NewDiscoverer(Config{
		HTTPConfig: web.HTTPConfig{
			RequestConfig: web.RequestConfig{URL: rawURL},
		},
	})
	require.NoError(t, err)

	_, err = d.fetchTargetGroup(context.Background())
	require.Error(t, err)

	assert.Contains(t, err.Error(), "/path")
	assert.NotContains(t, err.Error(), "user")
	assert.NotContains(t, err.Error(), "pass")
	assert.NotContains(t, err.Error(), "token")
	assert.NotContains(t, err.Error(), "secret")
	assert.NotContains(t, err.Error(), "fragment")
}

func TestDiscoverer_DiscoverFailureDoesNotEmit(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		http.Error(w, "nope", http.StatusInternalServerError)
	}))
	defer srv.Close()

	d, err := NewDiscoverer(Config{
		HTTPConfig: web.HTTPConfig{
			RequestConfig: web.RequestConfig{URL: srv.URL},
		},
		Interval: durationPtr(0),
	})
	require.NoError(t, err)

	ch := make(chan []model.TargetGroup, 1)
	d.Discover(context.Background(), ch)

	select {
	case got := <-ch:
		t.Fatalf("expected no emission, got %v", got)
	default:
	}
}

func TestDiscoverer_DiscoverEmptySuccessEmitsEmptyTargetGroup(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = fmt.Fprint(w, `[]`)
	}))
	defer srv.Close()

	d, err := NewDiscoverer(Config{
		HTTPConfig: web.HTTPConfig{
			RequestConfig: web.RequestConfig{URL: srv.URL},
		},
		Interval: durationPtr(0),
	})
	require.NoError(t, err)

	ch := make(chan []model.TargetGroup, 1)
	d.Discover(context.Background(), ch)

	select {
	case got := <-ch:
		require.Len(t, got, 1)
		assert.Empty(t, got[0].Targets())
	case <-time.After(time.Second):
		t.Fatal("expected empty target group emission")
	}
}

func TestDiscoverer_DiscoverPollsInterval(t *testing.T) {
	var (
		mu    sync.Mutex
		calls int
	)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		mu.Lock()
		calls++
		mu.Unlock()

		w.Header().Set("Content-Type", "application/json")
		_, _ = fmt.Fprint(w, `[]`)
	}))
	defer srv.Close()

	d, err := NewDiscoverer(Config{
		HTTPConfig: web.HTTPConfig{
			RequestConfig: web.RequestConfig{URL: srv.URL},
		},
		Interval: durationPtr(10 * time.Millisecond),
	})
	require.NoError(t, err)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	ch := make(chan []model.TargetGroup, 4)
	done := make(chan struct{})
	go func() {
		defer close(done)
		d.Discover(ctx, ch)
	}()

	for range 2 {
		select {
		case got := <-ch:
			require.Len(t, got, 1)
			assert.Empty(t, got[0].Targets())
		case <-time.After(time.Second):
			t.Fatal("expected target group emission")
		}
	}

	cancel()
	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("discoverer did not stop after context cancellation")
	}

	mu.Lock()
	defer mu.Unlock()
	assert.GreaterOrEqual(t, calls, 2)
}

func durationPtr(d time.Duration) *confopt.LongDuration {
	v := confopt.LongDuration(d)
	return &v
}

func ioReadAllString(r *http.Request) (string, error) {
	bs, err := io.ReadAll(r.Body)
	if err != nil {
		return "", err
	}
	return string(bs), nil
}
