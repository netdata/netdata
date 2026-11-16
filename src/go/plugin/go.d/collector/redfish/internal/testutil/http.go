// SPDX-License-Identifier: GPL-3.0-or-later

package testutil

import (
	"encoding/json"
	"fmt"
	"maps"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
)

type ServerConfig struct {
	RequireBasic         bool
	SupportSession       bool
	SessionStatus        int
	RootRedirect         string
	RootHits             *atomic.Int64
	FailSystems          *atomic.Bool
	ExpireSessionOnce    *atomic.Bool
	MalformedSessionBody bool
	SessionDeleteStatus  int
	HandleRequest        func(http.ResponseWriter, *http.Request) bool
}

type Server struct {
	*httptest.Server
	SessionCreates atomic.Int64
	SessionDeletes atomic.Int64
	ActiveSessions atomic.Int64
	BasicRequests  atomic.Int64
}

func NewServer(t *testing.T, cfg ServerConfig) *Server {
	t.Helper()

	state := &Server{}
	var mu sync.Mutex
	sessions := map[string]string{}
	issuedSessions := map[string]string{}
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if cfg.HandleRequest != nil && cfg.HandleRequest(w, r) {
			return
		}
		if token := r.Header.Get("X-Auth-Token"); token != "" && cfg.ExpireSessionOnce != nil &&
			cfg.ExpireSessionOnce.CompareAndSwap(true, false) {
			http.Error(w, "expired", http.StatusUnauthorized)
			return
		}
		if cfg.RootHits != nil && r.URL.Path == "/redfish/v1/" {
			cfg.RootHits.Add(1)
		}
		if cfg.RootRedirect != "" && r.URL.Path == "/redfish/v1/" {
			http.Redirect(w, r, cfg.RootRedirect, http.StatusTemporaryRedirect)
			return
		}
		if r.URL.Path == "/redfish/v1/redirected" {
			ServeServiceRootAt(w, r.URL.Path, cfg.SupportSession || cfg.SessionStatus != 0)
			return
		}

		if r.URL.Path == "/redfish/v1/SessionService/Sessions" && r.Method == http.MethodPost {
			sessionID := state.SessionCreates.Add(1)
			status := cfg.SessionStatus
			if status == 0 && cfg.SupportSession {
				status = http.StatusCreated
			}
			if status == 0 {
				status = http.StatusNotFound
			}
			if status != http.StatusCreated {
				http.Error(w, http.StatusText(status), status)
				return
			}
			if !cfg.SupportSession {
				w.WriteHeader(http.StatusCreated)
				_, _ = w.Write([]byte(`{}`))
				return
			}
			var got map[string]string
			if err := json.NewDecoder(r.Body).Decode(&got); err != nil {
				t.Errorf("decode session request body: %v", err)
				http.Error(w, "invalid request body", http.StatusBadRequest)
				return
			}
			want := map[string]string{"UserName": "user", "Password": "test-password"}
			if !maps.Equal(got, want) {
				t.Errorf("session request body = %v, want %v", got, want)
				http.Error(w, "unexpected request body", http.StatusBadRequest)
				return
			}
			token := fmt.Sprintf("test-token-%d", sessionID)
			mu.Lock()
			sessionURI := fmt.Sprintf("/redfish/v1/SessionService/Sessions/%d", sessionID)
			sessions[token] = sessionURI
			issuedSessions[token] = sessionURI
			state.ActiveSessions.Add(1)
			mu.Unlock()
			w.Header().Set("X-Auth-Token", token)
			w.Header().Set("Location", sessionURI)
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusCreated)
			if cfg.MalformedSessionBody {
				WriteJSONBody(w, map[string]any{
					"@odata.id":   sessionURI,
					"@odata.type": "#Session.v1_6_0.Session",
					"Id":          "1",
				})
				return
			}
			WriteJSONBody(w, map[string]any{
				"@odata.id":   sessionURI,
				"@odata.type": "#Session.v1_6_0.Session",
				"Id":          "1",
				"Name":        "Session",
			})
			return
		}
		if strings.HasPrefix(r.URL.Path, "/redfish/v1/SessionService/Sessions/") && r.Method == http.MethodDelete {
			token := r.Header.Get("X-Auth-Token")
			mu.Lock()
			defer mu.Unlock()
			if token == "" || issuedSessions[token] != r.URL.Path {
				t.Errorf("session cleanup used the wrong token/session pair")
				http.Error(w, "unauthorized", http.StatusUnauthorized)
				return
			}
			state.SessionDeletes.Add(1)
			status := cfg.SessionDeleteStatus
			if status == 0 {
				status = http.StatusNoContent
			}
			switch status {
			case http.StatusOK, http.StatusNoContent, http.StatusUnauthorized, http.StatusNotFound, http.StatusGone:
				// Unauthorized/gone injection models a session already expired on the BMC.
				if _, active := sessions[token]; active {
					delete(sessions, token)
					state.ActiveSessions.Add(-1)
				}
			}
			w.WriteHeader(status)
			return
		}

		if r.URL.Path != "/redfish/v1/" {
			authenticated := false
			if user, password, ok := r.BasicAuth(); ok && user == "user" && password == "test-password" {
				state.BasicRequests.Add(1)
				authenticated = true
			}
			mu.Lock()
			authenticated = authenticated || sessions[r.Header.Get("X-Auth-Token")] != ""
			mu.Unlock()
			if cfg.RequireBasic || cfg.SupportSession {
				if !authenticated {
					http.Error(w, "unauthorized", http.StatusUnauthorized)
					return
				}
			}
		}

		switch r.URL.Path {
		case "/redfish/v1/":
			ServeServiceRootAt(
				w,
				"/redfish/v1/",
				cfg.SupportSession || cfg.SessionStatus != 0,
			)
		case "/redfish/v1/Systems":
			if cfg.FailSystems != nil && cfg.FailSystems.Load() {
				http.Error(w, "temporary failure", http.StatusServiceUnavailable)
				return
			}
			ServeCollection(w, r.URL.Path, "/redfish/v1/Systems/1")
		case "/redfish/v1/Chassis":
			ServeCollection(w, r.URL.Path, "/redfish/v1/Chassis/1")
		case "/redfish/v1/Managers":
			ServeCollection(w, r.URL.Path, "/redfish/v1/Managers/1")
		case "/redfish/v1/Systems/1":
			ServeBaseResource(w, r.URL.Path, "#ComputerSystem.v1_21_0.ComputerSystem", "System")
		case "/redfish/v1/Chassis/1":
			ServeBaseResource(w, r.URL.Path, "#Chassis.v1_27_0.Chassis", "Chassis")
		case "/redfish/v1/Managers/1":
			ServeBaseResource(w, r.URL.Path, "#Manager.v1_20_0.Manager", "Manager")
		default:
			http.NotFound(w, r)
		}
	})
	state.Server = httptest.NewServer(handler)
	return state
}

func ServeServiceRoot(w http.ResponseWriter, session bool) {
	ServeServiceRootAt(w, "/redfish/v1/", session)
}

func ServeServiceRootAt(
	w http.ResponseWriter,
	resourceURI string,
	session bool,
) {
	document := map[string]any{
		"@odata.id":      resourceURI,
		"@odata.type":    "#ServiceRoot.v1_16_0.ServiceRoot",
		"Id":             "RootService",
		"Name":           "Root Service",
		"RedfishVersion": "1.20.0",
		"Systems":        map[string]any{"@odata.id": "/redfish/v1/Systems"},
		"Chassis":        map[string]any{"@odata.id": "/redfish/v1/Chassis"},
		"Managers":       map[string]any{"@odata.id": "/redfish/v1/Managers"},
	}
	if session {
		document["SessionService"] = map[string]any{"@odata.id": "/redfish/v1/SessionService"}
		document["Links"] = map[string]any{
			"Sessions": map[string]any{"@odata.id": "/redfish/v1/SessionService/Sessions"},
		}
	}
	WriteJSON(w, document)
}

func ServeCollection(w http.ResponseWriter, resourceURI string, members ...string) {
	values := make([]map[string]any, 0, len(members))
	for _, member := range members {
		values = append(values, map[string]any{"@odata.id": member})
	}
	collectionType := map[string]string{
		"/redfish/v1/Systems":  "ComputerSystemCollection",
		"/redfish/v1/Chassis":  "ChassisCollection",
		"/redfish/v1/Managers": "ManagerCollection",
	}[strings.TrimSuffix(resourceURI, "/")]
	if collectionType == "" {
		collectionType = "ResourceCollection"
	}
	WriteJSON(w, map[string]any{
		"@odata.id":           resourceURI,
		"@odata.type":         "#" + collectionType + "." + collectionType,
		"Members@odata.count": len(values),
		"Members":             values,
	})
}

func ServeBaseResource(w http.ResponseWriter, resourceURI, schemaType, name string) {
	WriteJSON(w, map[string]any{
		"@odata.id":   resourceURI,
		"@odata.type": schemaType,
		"Id":          "1",
		"Name":        name,
		"Status":      map[string]any{"Health": "OK", "State": "Enabled"},
	})
}

func WriteJSON(w http.ResponseWriter, value any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	WriteJSONBody(w, value)
}

func WriteJSONBody(w http.ResponseWriter, value any) {
	_ = json.NewEncoder(w).Encode(value)
}

func Link(uri string) map[string]any { return map[string]any{"@odata.id": uri} }

func Resource(uri, schema, name string, properties map[string]any) map[string]any {
	document := map[string]any{
		"@odata.id":   uri,
		"@odata.type": "#" + schema + ".v1_0_0." + schema,
		"Id":          name,
		"Name":        name,
	}
	for key, value := range properties {
		document[key] = value
	}
	return document
}

func Collection(uri, schema string, members ...string) map[string]any {
	values := make([]any, 0, len(members))
	for _, member := range members {
		values = append(values, Link(member))
	}
	return map[string]any{
		"@odata.id":           uri,
		"@odata.type":         "#" + schema + "Collection." + schema + "Collection",
		"Members@odata.count": len(values),
		"Members":             values,
		"Name":                strings.TrimPrefix(uri, "/redfish/v1/"),
	}
}

func ServeDocuments(t *testing.T, docs map[string]map[string]any) string {
	t.Helper()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		doc := docs[r.URL.Path]
		if doc == nil {
			http.NotFound(w, r)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		w.Header().Set("OData-Version", "4.0")
		_ = json.NewEncoder(w).Encode(doc)
	}))
	t.Cleanup(server.Close)
	return server.URL
}

func NewResourceServer(handler http.Handler) *httptest.Server {
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/redfish/v1/" {
			ServeServiceRoot(w, false)
			return
		}
		handler.ServeHTTP(w, r)
	}))
}
