// SPDX-License-Identifier: GPL-3.0-or-later

package client

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
)

// MockScaleIOAPIServer represents VxFlex OS Gateway.
type MockScaleIOAPIServer struct {
	User       string
	Password   string
	Token      string
	Version    string
	Instances  Instances
	Statistics SelectedStatistics
}

func (s MockScaleIOAPIServer) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	switch r.URL.Path {
	default:
		w.WriteHeader(http.StatusNotFound)
		msg := fmt.Sprintf("unknown URL path: %s", r.URL.Path)
		writeAPIError(w, msg)
	case "/api/login":
		s.handleLogin(w, r)
	case "/api/logout":
		s.handleLogout(w, r)
	case "/api/version":
		s.handleVersion(w, r)
	case "/api/instances":
		s.handleInstances(w, r)
	case "/api/instances/querySelectedStatistics":
		s.handleQuerySelectedStatistics(w, r)
	}
}

func (s MockScaleIOAPIServer) handleLogin(w http.ResponseWriter, r *http.Request) {
	if user, pass, ok := r.BasicAuth(); !ok || user != s.User || pass != s.Password {
		w.WriteHeader(http.StatusUnauthorized)
		msg := fmt.Sprintf("user got/expected: %s/%s, pass got/expected: %s/%s", user, s.User, pass, s.Password)
		writeAPIError(w, msg)
		return
	}
	if r.Method != http.MethodGet {
		w.WriteHeader(http.StatusBadRequest)
		msg := fmt.Sprintf("wrong method: '%s', expected '%s'", r.Method, http.MethodGet)
		writeAPIError(w, msg)
		return
	}
	_, _ = w.Write([]byte(s.Token))
}

func (s MockScaleIOAPIServer) handleLogout(w http.ResponseWriter, r *http.Request) {
	if _, pass, ok := r.BasicAuth(); !ok || pass != s.Token {
		w.WriteHeader(http.StatusUnauthorized)
		msg := fmt.Sprintf("token got/expected: %s/%s", pass, s.Token)
		writeAPIError(w, msg)
		return
	}
	if r.Method != http.MethodGet {
		w.WriteHeader(http.StatusBadRequest)
		msg := fmt.Sprintf("wrong method: '%s', expected '%s'", r.Method, http.MethodGet)
		writeAPIError(w, msg)
		return
	}
}

func (s MockScaleIOAPIServer) handleVersion(w http.ResponseWriter, r *http.Request) {
	if _, pass, ok := r.BasicAuth(); !ok || pass != s.Token {
		w.WriteHeader(http.StatusUnauthorized)
		msg := fmt.Sprintf("token got/expected: %s/%s", pass, s.Token)
		writeAPIError(w, msg)
		return
	}
	if r.Method != http.MethodGet {
		w.WriteHeader(http.StatusBadRequest)
		msg := fmt.Sprintf("wrong method: '%s', expected '%s'", r.Method, http.MethodGet)
		writeAPIError(w, msg)
		return
	}
	_, _ = w.Write([]byte(s.Version))
}

func (s MockScaleIOAPIServer) handleInstances(w http.ResponseWriter, r *http.Request) {
	if _, pass, ok := r.BasicAuth(); !ok || pass != s.Token {
		w.WriteHeader(http.StatusUnauthorized)
		msg := fmt.Sprintf("token got/expected: %s/%s", pass, s.Token)
		writeAPIError(w, msg)
		return
	}
	if r.Method != http.MethodGet {
		w.WriteHeader(http.StatusBadRequest)
		msg := fmt.Sprintf("wrong method: '%s', expected '%s'", r.Method, http.MethodGet)
		writeAPIError(w, msg)
		return
	}
	b, err := json.Marshal(s.Instances)
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		msg := fmt.Sprintf("marshal Instances: %v", err)
		writeAPIError(w, msg)
		return
	}
	_, _ = w.Write(b)
}

func (s MockScaleIOAPIServer) handleQuerySelectedStatistics(w http.ResponseWriter, r *http.Request) {
	if _, pass, ok := r.BasicAuth(); !ok || pass != s.Token {
		w.WriteHeader(http.StatusUnauthorized)
		msg := fmt.Sprintf("token got/expected: %s/%s", pass, s.Token)
		writeAPIError(w, msg)
		return
	}
	if r.Method != http.MethodPost {
		w.WriteHeader(http.StatusBadRequest)
		msg := fmt.Sprintf("wrong method: '%s', expected '%s'", r.Method, http.MethodPost)
		writeAPIError(w, msg)
		return
	}
	if r.Header.Get("Content-Type") != "application/json" {
		w.WriteHeader(http.StatusBadRequest)
		writeAPIError(w, "no \"Content-Type: application/json\" in the header")
		return
	}
	if err := json.NewDecoder(r.Body).Decode(&SelectedStatisticsQuery{}); err != nil {
		w.WriteHeader(http.StatusBadRequest)
		msg := fmt.Sprintf("body decode error: %v", err)
		writeAPIError(w, msg)
		return
	}
	b, err := json.Marshal(s.Statistics)
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		msg := fmt.Sprintf("marshal SelectedStatistics: %v", err)
		writeAPIError(w, msg)
		return
	}
	_, _ = w.Write(b)
}

func writeAPIError(w io.Writer, msg string) {
	err := apiError{Message: msg}
	b, _ := json.Marshal(err)
	_, _ = w.Write(b)
}
