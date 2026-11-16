// SPDX-License-Identifier: GPL-3.0-or-later

package httpclient

import (
	"context"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestRequest(t *testing.T) {
	type capturedRequest struct {
		Method, ContentType, UserAgent, Body string
		Header                               []string
	}
	headers := http.Header{"X-Test": {"one", "two"}}
	for name, test := range map[string]struct {
		send func(context.Context, *http.Client, string) (*http.Response, error)
		want capturedRequest
	}{
		"post JSON": {
			send: func(ctx context.Context, client *http.Client, endpoint string) (*http.Response, error) {
				return PostJSON(ctx, client, "test", endpoint, headers, map[string]string{"text": "<hello>"})
			},
			want: capturedRequest{"POST", "application/json", "netdata-alarm-notify", `{"text":"\u003chello\u003e"}`, []string{"one", "two"}},
		},
		"put JSON": {
			send: func(ctx context.Context, client *http.Client, endpoint string) (*http.Response, error) {
				return RequestJSON(ctx, client, "test", http.MethodPut, endpoint, headers, map[string]string{"text": "hello"})
			},
			want: capturedRequest{"PUT", "application/json", "netdata-alarm-notify", `{"text":"hello"}`, []string{"one", "two"}},
		},
		"post form": {
			send: func(ctx context.Context, client *http.Client, endpoint string) (*http.Response, error) {
				return Post(ctx, client, "test", endpoint, "application/x-www-form-urlencoded", headers, strings.NewReader("text=hello+world"))
			},
			want: capturedRequest{"POST", "application/x-www-form-urlencoded", "netdata-alarm-notify", "text=hello+world", []string{"one", "two"}},
		},
		"delete body": {
			send: func(ctx context.Context, client *http.Client, endpoint string) (*http.Response, error) {
				return Request(ctx, client, "test", http.MethodDelete, endpoint, "text/plain", headers, strings.NewReader("hello"))
			},
			want: capturedRequest{"DELETE", "text/plain", "netdata-alarm-notify", "hello", []string{"one", "two"}},
		},
	} {
		t.Run(name, func(t *testing.T) {
			captured := make(chan capturedRequest, 1)
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				body, err := io.ReadAll(r.Body)
				assert.NoError(t, err)
				captured <- capturedRequest{r.Method, r.Header.Get("Content-Type"), r.UserAgent(), string(body), r.Header.Values("X-Test")}
				w.WriteHeader(http.StatusTeapot)
				_, _ = io.WriteString(w, "provider acknowledgment")
			}))
			defer server.Close()
			client := New(time.Second)
			defer client.CloseIdleConnections()
			response, err := test.send(t.Context(), client, server.URL)
			require.NoError(t, err)
			defer response.Body.Close()
			assert.Equal(t, test.want, <-captured)
			assert.Equal(t, http.StatusTeapot, response.StatusCode) // Provider owns status acceptance.
			body, err := io.ReadAll(response.Body)
			require.NoError(t, err)
			assert.Equal(t, "provider acknowledgment", string(body))
		})
	}
}

func TestClientDoesNotRedirect(t *testing.T) {
	var calls atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls.Add(1)
		http.Redirect(w, r, "/redirected", http.StatusTemporaryRedirect)
	}))
	defer server.Close()
	client := New(time.Second)
	defer client.CloseIdleConnections()
	response, err := PostJSON(t.Context(), client, "test", server.URL, nil, map[string]string{"text": "hello"})
	require.NoError(t, err)
	defer response.Body.Close()
	assert.Equal(t, http.StatusTemporaryRedirect, response.StatusCode)
	assert.EqualValues(t, 1, calls.Load())
}

func TestRequestCancellation(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		<-r.Context().Done()
	}))
	defer server.Close()
	for name, test := range map[string]struct {
		canceled bool
		want     string
	}{
		"canceled": {true, "notification canceled"},
		"deadline": {false, "notification timed out"},
	} {
		t.Run(name, func(t *testing.T) {
			ctx, cancel := context.WithTimeout(t.Context(), 20*time.Millisecond)
			defer cancel()
			if test.canceled {
				cancel()
			}
			client := New(time.Second)
			defer client.CloseIdleConnections()
			response, err := Request(ctx, client, "test", http.MethodGet, server.URL+"/synthetic-secret", "text/plain", nil, nil)
			require.EqualError(t, err, test.want)
			assert.Nil(t, response)
		})
	}
}

func TestDecodeResponse(t *testing.T) {
	type acknowledgment struct {
		Accepted bool `json:"accepted"`
	}
	for name, test := range map[string]struct {
		body string
		want acknowledgment
		err  string
	}{
		"valid":          {body: `{"accepted":true}`, want: acknowledgment{true}},
		"boundary":       {body: `{"accepted":true}` + strings.Repeat(" ", ResponseLimit-len(`{"accepted":true}`)), want: acknowledgment{true}},
		"over limit":     {body: strings.Repeat(" ", ResponseLimit+1), err: "test response exceeds the 256 KiB limit"},
		"invalid JSON":   {body: "synthetic-secret", err: "invalid test response"},
		"extra document": {body: `{"accepted":true}{}`, err: "invalid test response"},
	} {
		t.Run(name, func(t *testing.T) {
			var got acknowledgment
			err := DecodeResponse("test", strings.NewReader(test.body), &got)
			if test.err != "" {
				require.EqualError(t, err, test.err)
			} else {
				require.NoError(t, err)
			}
			assert.Equal(t, test.want, got)
		})
	}
}

func TestSafeError(t *testing.T) {
	for name, test := range map[string]struct {
		err  error
		want string
	}{
		"cancellation":  {context.Canceled, "notification canceled"},
		"deadline":      {context.DeadlineExceeded, "notification timed out"},
		"URL wrapper":   {&url.Error{Op: "Post", URL: "https://example.com/synthetic-secret", Err: context.DeadlineExceeded}, "notification timed out"},
		"other failure": {errors.New("synthetic-secret"), "test transport failed; check connectivity, TLS, and proxy settings"},
	} {
		t.Run(name, func(t *testing.T) { assert.EqualError(t, SafeError("test", test.err), test.want) })
	}
}

func TestValidURL(t *testing.T) {
	for name, test := range map[string]struct {
		value          string
		fragment, want bool
	}{
		"HTTP":              {"http://example.com/notify", false, true},
		"HTTPS":             {"https://example.com/notify?key=synthetic", false, true},
		"event link":        {"https://example.com/chart#details", true, true},
		"endpoint fragment": {"https://example.com/notify#details", false, false},
		"userinfo":          {"https://synthetic@example.com/notify", true, false},
		"relative":          {"/notify", false, false},
		"missing host":      {"https:///notify", false, false},
		"other scheme":      {"file:///notify", false, false},
		"opaque":            {"https:opaque", false, false},
	} {
		t.Run(name, func(t *testing.T) { assert.Equal(t, test.want, ValidURL(test.value, test.fragment)) })
	}
}
