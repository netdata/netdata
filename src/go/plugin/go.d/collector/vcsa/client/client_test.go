// SPDX-License-Identifier: GPL-3.0-or-later

package client

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"context"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const (
	testUser        = "user"
	testPass        = "pass"
	testSessToken   = "sessToken"
	testHealthValue = "green"
)

func newTestClient(srvURL string) *Client {
	return New(nil, srvURL, testUser, testPass)
}

func TestClient_Login(t *testing.T) {
	ts := newTestHTTPServer()
	defer ts.Close()
	cl := newTestClient(ts.URL)

	assert.NoError(t, cl.Login(context.Background()))
	assert.Equal(t, testSessToken, cl.token.get())
}

func TestClient_LoginWrongCredentials(t *testing.T) {
	ts := newTestHTTPServer()
	defer ts.Close()
	cl := newTestClient(ts.URL)
	cl.username += "!"

	assert.Error(t, cl.Login(context.Background()))
}

func TestClient_Logout(t *testing.T) {
	ts := newTestHTTPServer()
	defer ts.Close()
	cl := newTestClient(ts.URL)

	assert.NoError(t, cl.Login(context.Background()))
	assert.NoError(t, cl.Logout(context.Background()))
	assert.Zero(t, cl.token.get())
}

func TestClient_Ping(t *testing.T) {
	ts := newTestHTTPServer()
	defer ts.Close()
	cl := newTestClient(ts.URL)

	require.NoError(t, cl.Login(context.Background()))
	assert.NoError(t, cl.Ping(context.Background()))
}

func TestClient_PingWithReAuthentication(t *testing.T) {
	ts := newTestHTTPServer()
	defer ts.Close()
	cl := newTestClient(ts.URL)

	require.NoError(t, cl.Login(context.Background()))
	cl.token.set("")
	assert.NoError(t, cl.Ping(context.Background()))
	assert.Equal(t, testSessToken, cl.token.get())
}

func TestClient_ApplMgmt(t *testing.T) {
	ts := newTestHTTPServer()
	defer ts.Close()
	cl := newTestClient(ts.URL)

	require.NoError(t, cl.Login(context.Background()))
	v, err := cl.ApplMgmt(context.Background())
	assert.NoError(t, err)
	assert.Equal(t, testHealthValue, v)
}

func TestClient_DatabaseStorage(t *testing.T) {
	ts := newTestHTTPServer()
	defer ts.Close()
	cl := newTestClient(ts.URL)

	require.NoError(t, cl.Login(context.Background()))
	v, err := cl.DatabaseStorage(context.Background())
	assert.NoError(t, err)
	assert.Equal(t, testHealthValue, v)
}

func TestClient_Load(t *testing.T) {
	ts := newTestHTTPServer()
	defer ts.Close()
	cl := newTestClient(ts.URL)

	require.NoError(t, cl.Login(context.Background()))
	v, err := cl.Load(context.Background())
	assert.NoError(t, err)
	assert.Equal(t, testHealthValue, v)
}

func TestClient_Mem(t *testing.T) {
	ts := newTestHTTPServer()
	defer ts.Close()
	cl := newTestClient(ts.URL)

	require.NoError(t, cl.Login(context.Background()))
	v, err := cl.Mem(context.Background())
	assert.NoError(t, err)
	assert.Equal(t, testHealthValue, v)
}

func TestClient_SoftwarePackages(t *testing.T) {
	ts := newTestHTTPServer()
	defer ts.Close()
	cl := newTestClient(ts.URL)

	require.NoError(t, cl.Login(context.Background()))
	v, err := cl.SoftwarePackages(context.Background())
	assert.NoError(t, err)
	assert.Equal(t, testHealthValue, v)
}

func TestClient_Storage(t *testing.T) {
	ts := newTestHTTPServer()
	defer ts.Close()
	cl := newTestClient(ts.URL)

	require.NoError(t, cl.Login(context.Background()))
	v, err := cl.Storage(context.Background())
	assert.NoError(t, err)
	assert.Equal(t, testHealthValue, v)
}

func TestClient_Swap(t *testing.T) {
	ts := newTestHTTPServer()
	defer ts.Close()
	cl := newTestClient(ts.URL)

	require.NoError(t, cl.Login(context.Background()))
	v, err := cl.Swap(context.Background())
	assert.NoError(t, err)
	assert.Equal(t, testHealthValue, v)
}

func TestClient_System(t *testing.T) {
	ts := newTestHTTPServer()
	defer ts.Close()
	cl := newTestClient(ts.URL)

	require.NoError(t, cl.Login(context.Background()))
	v, err := cl.System(context.Background())
	assert.NoError(t, err)
	assert.Equal(t, testHealthValue, v)
}

func TestClient_InvalidDataOnLogin(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte("hello\n and goodbye!"))
	}))
	defer ts.Close()
	cl := newTestClient(ts.URL)

	assert.Error(t, cl.Login(context.Background()))
}

func TestClient_404OnLogin(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(404)
	}))
	defer ts.Close()
	cl := newTestClient(ts.URL)

	assert.Error(t, cl.Login(context.Background()))
}

func newTestHTTPServer() *httptest.Server {
	return httptest.NewServer(&mockVCSAServer{
		username:  testUser,
		password:  testPass,
		sessionID: testSessToken,
	})
}

type mockVCSAServer struct {
	username  string
	password  string
	sessionID string
}

func (m mockVCSAServer) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	switch r.URL.Path {
	default:
		w.WriteHeader(http.StatusNotFound)
	case pathCISSession:
		m.handleSession(w, r)
	case
		pathHealthApplMgmt,
		pathHealthDatabaseStorage,
		pathHealthLoad,
		pathHealthMem,
		pathHealthSoftwarePackager,
		pathHealthStorage,
		pathHealthSwap,
		pathHealthSystem:
		m.handleHealth(w, r)
	}
}

func (m mockVCSAServer) handleHealth(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		w.WriteHeader(http.StatusBadRequest)
		return
	}

	if !m.isSessionAuthenticated(r) {
		w.WriteHeader(http.StatusUnauthorized)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)

	s := struct{ Value string }{Value: testHealthValue}
	b, _ := json.Marshal(s)
	_, _ = w.Write(b)
}

func (m mockVCSAServer) handleSession(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	default:
		w.WriteHeader(http.StatusBadRequest)
	case http.MethodDelete:
		m.handleSessionDelete(w, r)
	case http.MethodPost:
		if r.URL.RawQuery == "" {
			m.handleSessionCreate(w, r)
		} else {
			m.handleSessionGet(w, r)
		}
	}
}

func (m mockVCSAServer) handleSessionCreate(w http.ResponseWriter, r *http.Request) {
	if !m.isReqAuthenticated(r) {
		w.WriteHeader(http.StatusUnauthorized)
		return
	}

	w.WriteHeader(http.StatusOK)
	s := struct{ Value string }{Value: m.sessionID}
	b, _ := json.Marshal(s)
	_, _ = w.Write(b)
}

func (m mockVCSAServer) handleSessionGet(w http.ResponseWriter, r *http.Request) {
	if !m.isSessionAuthenticated(r) {
		w.WriteHeader(http.StatusUnauthorized)
		return
	}

	w.WriteHeader(http.StatusOK)
	s := struct{ Value struct{ User string } }{Value: struct{ User string }{User: m.username}}
	b, _ := json.Marshal(s)
	_, _ = w.Write(b)
}

func (m mockVCSAServer) handleSessionDelete(w http.ResponseWriter, r *http.Request) {
	if !m.isSessionAuthenticated(r) {
		w.WriteHeader(http.StatusUnauthorized)
		return
	}
	w.WriteHeader(http.StatusOK)
}

func (m mockVCSAServer) isReqAuthenticated(r *http.Request) bool {
	u, p, ok := r.BasicAuth()
	return ok && m.username == u && p == m.password
}

func (m mockVCSAServer) isSessionAuthenticated(r *http.Request) bool {
	return r.Header.Get(apiSessIDKey) == m.sessionID
}
