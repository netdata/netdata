// SPDX-License-Identifier: GPL-3.0-or-later

package httpcheck

import (
	"context"
	"errors"
	"io"
	"net/http"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/netdata/netdata/go/plugins/pkg/credentialfile"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type cookieReader struct {
	credentialfile.FileReader
	content     string
	modTime     time.Time
	opens       int
	wantContext context.Context
	t           *testing.T
	stream      io.Reader
}

func (r *cookieReader) Open(ctx context.Context, _ string) (io.ReadCloser, error) {
	if r.wantContext != nil {
		require.Equal(r.t, r.wantContext, ctx)
	}
	r.opens++
	if r.stream != nil {
		return io.NopCloser(r.stream), nil
	}
	return io.NopCloser(strings.NewReader(r.content)), nil
}

func (r *cookieReader) Stat(ctx context.Context, _ string) (time.Time, error) {
	if r.wantContext != nil {
		require.Equal(r.t, r.wantContext, ctx)
	}
	return r.modTime, nil
}

func TestLoadCookieJarRejectsMalformedInputWithoutContents(t *testing.T) {
	const sentinel = "private-cookie-sentinel"
	tests := map[string]string{
		"field count":        sentinel,
		"secure flag":        ".example.com TRUE / " + sentinel + " 0 session value",
		"expiry":             ".example.com TRUE / FALSE " + sentinel + " session value",
		"scanner line limit": strings.Repeat(sentinel, 5000),
	}
	for name, content := range tests {
		t.Run(name, func(t *testing.T) {
			jar, err := loadCookieJar(context.Background(), "cookies", &cookieReader{content: content})
			require.Error(t, err)
			assert.Nil(t, jar)
			assert.NotContains(t, err.Error(), sentinel)
		})
	}
}

type cookieReadError struct{ err error }

func (r cookieReadError) Read([]byte) (int, error) { return 0, r.err }

func TestLoadCookieJarReturnsStreamError(t *testing.T) {
	expected := errors.New("synthetic read failure")
	jar, err := loadCookieJar(context.Background(), "cookies", &cookieReader{stream: cookieReadError{expected}})
	require.ErrorIs(t, err, expected)
	assert.Nil(t, jar)
}

func TestReadCookieFilePreservesModificationTimeReload(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	files := &cookieReader{
		content: ".example.com TRUE / FALSE 0 session first\n",
		modTime: time.Unix(100, 0), wantContext: ctx, t: t,
	}
	c := New()
	c.CookieFile = "cookies"
	c.httpClient = &http.Client{}
	c.SetCredentialFiles(files)
	cookieURL := &url.URL{Scheme: "http", Host: "www.example.com"}
	require.NoError(t, c.readCookieFile(ctx))
	require.Len(t, c.httpClient.Jar.Cookies(cookieURL), 1)
	assert.Equal(t, "first", c.httpClient.Jar.Cookies(cookieURL)[0].Value)
	files.content = ".example.com TRUE / FALSE 0 session second\n"
	require.NoError(t, c.readCookieFile(ctx))
	assert.Equal(t, 1, files.opens)
	assert.Equal(t, "first", c.httpClient.Jar.Cookies(cookieURL)[0].Value)
	files.modTime = files.modTime.Add(time.Second)
	require.NoError(t, c.readCookieFile(ctx))
	assert.Equal(t, 2, files.opens)
	assert.Equal(t, "second", c.httpClient.Jar.Cookies(cookieURL)[0].Value)
	files.content = "invalid"
	files.modTime = files.modTime.Add(time.Second)
	require.Error(t, c.readCookieFile(ctx))
	assert.Equal(t, "second", c.httpClient.Jar.Cookies(cookieURL)[0].Value)
	assert.NotEqual(t, files.modTime, c.cookieFileModTime)
}

func TestLoadCookieJarDoesNotBoundTotalInput(t *testing.T) {
	files := &cookieReader{content: strings.Repeat("# comment\n", 120000) + ".example.com TRUE / FALSE 0 session value\n"}
	jar, err := loadCookieJar(context.Background(), "cookies", files)
	require.NoError(t, err)
	assert.Len(t, jar.Cookies(&url.URL{Scheme: "http", Host: "www.example.com"}), 1)
}
