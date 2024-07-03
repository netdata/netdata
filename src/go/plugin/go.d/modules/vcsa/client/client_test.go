// SPDX-License-Identifier: GPL-3.0-or-later

package client

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

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

	assert.NoError(t, cl.Login())
	assert.Equal(t, testSessToken, cl.token.get())
}

func TestClient_LoginWrongCredentials(t *testing.T) {
	ts := newTestHTTPServer()
	defer ts.Close()
	cl := newTestClient(ts.URL)
	cl.username += "!"

	assert.Error(t, cl.Login())
}

func TestClient_Logout(t *testing.T) {
	ts := newTestHTTPServer()
	defer ts.Close()
	cl := newTestClient(ts.URL)

	assert.NoError(t, cl.Login())
	assert.NoError(t, cl.Logout())
	assert.Zero(t, cl.token.get())
}

func TestClient_Ping(t *testing.T) {
	ts := newTestHTTPServer()
	defer ts.Close()
	cl := newTestClient(ts.URL)

	require.NoError(t, cl.Login())
	assert.NoError(t, cl.Ping())
}

func TestClient_PingWithReAuthentication(t *testing.T) {
	ts := newTestHTTPServer()
	defer ts.Close()
	cl := newTestClient(ts.URL)

	require.NoError(t, cl.Login())
	cl.token.set("")
	assert.NoError(t, cl.Ping())
	assert.Equal(t, testSessToken, cl.token.get())
}

func TestClient_ApplMgmt(t *testing.T) {
	ts := newTestHTTPServer()
	defer ts.Close()
	cl := newTestClient(ts.URL)

	require.NoError(t, cl.Login())
	v, err := cl.ApplMgmt()
	assert.NoError(t, err)
	assert.Equal(t, testHealthValue, v)
}

func TestClient_DatabaseStorage(t *testing.T) {
	ts := newTestHTTPServer()
	defer ts.Close()
	cl := newTestClient(ts.URL)

	require.NoError(t, cl.Login())
	v, err := cl.DatabaseStorage()
	assert.NoError(t, err)
	assert.Equal(t, testHealthValue, v)
}

func TestClient_Load(t *testing.T) {
	ts := newTestHTTPServer()
	defer ts.Close()
	cl := newTestClient(ts.URL)

	require.NoError(t, cl.Login())
	v, err := cl.Load()
	assert.NoError(t, err)
	assert.Equal(t, testHealthValue, v)
}

func TestClient_Mem(t *testing.T) {
	ts := newTestHTTPServer()
	defer ts.Close()
	cl := newTestClient(ts.URL)

	require.NoError(t, cl.Login())
	v, err := cl.Mem()
	assert.NoError(t, err)
	assert.Equal(t, testHealthValue, v)
}

func TestClient_SoftwarePackages(t *testing.T) {
	ts := newTestHTTPServer()
	defer ts.Close()
	cl := newTestClient(ts.URL)

	require.NoError(t, cl.Login())
	v, err := cl.SoftwarePackages()
	assert.NoError(t, err)
	assert.Equal(t, testHealthValue, v)
}

func TestClient_Storage(t *testing.T) {
	ts := newTestHTTPServer()
	defer ts.Close()
	cl := newTestClient(ts.URL)

	require.NoError(t, cl.Login())
	v, err := cl.Storage()
	assert.NoError(t, err)
	assert.Equal(t, testHealthValue, v)
}

func TestClient_Swap(t *testing.T) {
	ts := newTestHTTPServer()
	defer ts.Close()
	cl := newTestClient(ts.URL)

	require.NoError(t, cl.Login())
	v, err := cl.Swap()
	assert.NoError(t, err)
	assert.Equal(t, testHealthValue, v)
}

func TestClient_System(t *testing.T) {
	ts := newTestHTTPServer()
	defer ts.Close()
	cl := newTestClient(ts.URL)

	require.NoError(t, cl.Login())
	v, err := cl.System()
	assert.NoError(t, err)
	assert.Equal(t, testHealthValue, v)
}

func TestClient_InvalidDataOnLogin(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte("hello\n and goodbye!"))
	}))
	defer ts.Close()
	cl := newTestClient(ts.URL)

	assert.Error(t, cl.Login())
}

func TestClient_404OnLogin(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(404)
	}))
	defer ts.Close()
	cl := newTestClient(ts.URL)

	assert.Error(t, cl.Login())
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
