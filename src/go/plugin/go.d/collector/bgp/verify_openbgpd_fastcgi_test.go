// SPDX-License-Identifier: GPL-3.0-or-later

//go:build integration

package bgp

import (
	"fmt"
	"io"
	"net/http"
	"time"

	fcgiclient "github.com/kanocz/fcgi_client"
)

type openbgpdFastCGIProxy struct {
	socketPath string
	timeout    time.Duration
}

type openbgpdFastCGIResponseBody struct {
	body   io.ReadCloser
	client *fcgiclient.FCGIClient
}

func newOpenBGPDFastCGIProxy(socketPath string, timeout time.Duration) http.Handler {
	return &openbgpdFastCGIProxy{
		socketPath: socketPath,
		timeout:    timeout,
	}
}

func (p *openbgpdFastCGIProxy) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	resp, err := p.do(r)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadGateway)
		return
	}
	defer resp.Body.Close()

	copyHTTPHeaders(w.Header(), resp.Header)
	statusCode := resp.StatusCode
	if statusCode == 0 {
		statusCode = http.StatusOK
	}
	w.WriteHeader(statusCode)
	_, _ = io.Copy(w, resp.Body)
}

func (p *openbgpdFastCGIProxy) do(r *http.Request) (*http.Response, error) {
	return openbgpdFastCGIRequest(p.socketPath, p.timeout, r.URL.Path, r.URL.RawQuery)
}

func openbgpdFastCGIRequest(socketPath string, timeout time.Duration, path, rawQuery string) (*http.Response, error) {
	client, err := fcgiclient.DialTimeout("unix", socketPath, timeout)
	if err != nil {
		return nil, fmt.Errorf("open bgplgd socket %q: %w", socketPath, err)
	}

	if err := client.SetTimeout(timeout); err != nil {
		client.Close()
		return nil, fmt.Errorf("set bgplgd socket timeout: %w", err)
	}

	resp, err := client.Get(openbgpdFastCGIEnv(path, rawQuery))
	if err != nil {
		client.Close()
		return nil, fmt.Errorf("query bgplgd over FastCGI: %w", err)
	}
	resp.Body = &openbgpdFastCGIResponseBody{
		body:   resp.Body,
		client: client,
	}
	return resp, nil
}

func readOpenBGPDFastCGIBody(socketPath string, timeout time.Duration, path string) ([]byte, error) {
	return readOpenBGPDFastCGIBodyWithQuery(socketPath, timeout, path, "")
}

func readOpenBGPDFastCGIBodyWithQuery(socketPath string, timeout time.Duration, path, rawQuery string) ([]byte, error) {
	resp, err := openbgpdFastCGIRequest(socketPath, timeout, path, rawQuery)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	return io.ReadAll(resp.Body)
}

func openbgpdFastCGIEnv(path, rawQuery string) map[string]string {
	requestURI := path
	if rawQuery != "" {
		requestURI += "?" + rawQuery
	}

	return map[string]string{
		"DOCUMENT_URI":    path,
		"HTTP_ACCEPT":     "application/json",
		"PATH_INFO":       path,
		"QUERY_STRING":    rawQuery,
		"REMOTE_ADDR":     "127.0.0.1",
		"REQUEST_METHOD":  http.MethodGet,
		"REQUEST_SCHEME":  "http",
		"REQUEST_URI":     requestURI,
		"SCRIPT_FILENAME": path,
		"SCRIPT_NAME":     path,
		"SERVER_NAME":     "openbgpd-test",
		"SERVER_PORT":     "80",
		"SERVER_PROTOCOL": "HTTP/1.1",
		"SERVER_SOFTWARE": "netdata-bgp-integration",
	}
}

func copyHTTPHeaders(dst, src http.Header) {
	for key, values := range src {
		for _, value := range values {
			dst.Add(key, value)
		}
	}
}

func (b *openbgpdFastCGIResponseBody) Read(p []byte) (int, error) {
	return b.body.Read(p)
}

func (b *openbgpdFastCGIResponseBody) Close() error {
	err := b.body.Close()
	b.client.Close()
	return err
}
