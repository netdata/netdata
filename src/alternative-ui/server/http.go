// SPDX-License-Identifier: GPL-3.0-or-later

package server

import (
	"embed"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/netdata/netdata/src/alternative-ui/metrics"
)

// Server is the HTTP server for the alternative UI
type Server struct {
	store      *metrics.Store
	addr       string
	apiKey     string
	wsHub      *WebSocketHub
	webFS      embed.FS
}

// NewServer creates a new HTTP server
func NewServer(store *metrics.Store, addr string, apiKey string, webFS embed.FS) *Server {
	return &Server{
		store:  store,
		addr:   addr,
		apiKey: apiKey,
		wsHub:  NewWebSocketHub(store),
		webFS:  webFS,
	}
}

// Start starts the HTTP server
func (s *Server) Start() error {
	go s.wsHub.Run()

	mux := http.NewServeMux()

	// API routes
	mux.HandleFunc("/api/v1/push", s.corsMiddleware(s.authMiddleware(s.handlePush)))
	mux.HandleFunc("/api/v1/nodes", s.corsMiddleware(s.handleNodes))
	mux.HandleFunc("/api/v1/node/", s.corsMiddleware(s.handleNode))
	mux.HandleFunc("/api/v1/charts/", s.corsMiddleware(s.handleCharts))
	mux.HandleFunc("/api/v1/data/", s.corsMiddleware(s.handleData))
	mux.HandleFunc("/api/v1/health", s.corsMiddleware(s.handleHealth))

	// WebSocket
	mux.HandleFunc("/ws", s.handleWebSocket)

	// Static files
	mux.HandleFunc("/", s.handleStatic)

	log.Printf("Starting alternative UI server on %s", s.addr)
	return http.ListenAndServe(s.addr, mux)
}

// corsMiddleware adds CORS headers
func (s *Server) corsMiddleware(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Access-Control-Allow-Origin", "*")
		w.Header().Set("Access-Control-Allow-Methods", "GET, POST, OPTIONS")
		w.Header().Set("Access-Control-Allow-Headers", "Content-Type, X-API-Key, Authorization")

		if r.Method == "OPTIONS" {
			w.WriteHeader(http.StatusOK)
			return
		}

		next(w, r)
	}
}

// authMiddleware checks API key for push endpoints
func (s *Server) authMiddleware(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if s.apiKey != "" {
			key := r.Header.Get("X-API-Key")
			if key == "" {
				key = r.URL.Query().Get("api_key")
			}
			if key != s.apiKey {
				http.Error(w, "Unauthorized", http.StatusUnauthorized)
				return
			}
		}
		next(w, r)
	}
}

// handlePush handles incoming metric pushes
func (s *Server) handlePush(w http.ResponseWriter, r *http.Request) {
	if r.Method != "POST" {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	body, err := io.ReadAll(io.LimitReader(r.Body, 10*1024*1024)) // 10MB limit
	if err != nil {
		http.Error(w, "Failed to read body", http.StatusBadRequest)
		return
	}
	defer r.Body.Close()

	var push metrics.MetricPush
	if err := json.Unmarshal(body, &push); err != nil {
		http.Error(w, fmt.Sprintf("Invalid JSON: %v", err), http.StatusBadRequest)
		return
	}

	if push.NodeID == "" {
		http.Error(w, "node_id is required", http.StatusBadRequest)
		return
	}

	if err := s.store.Push(&push); err != nil {
		http.Error(w, fmt.Sprintf("Failed to store metrics: %v", err), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(map[string]string{"status": "ok"})
}

// handleNodes returns a list of all nodes
func (s *Server) handleNodes(w http.ResponseWriter, r *http.Request) {
	if r.Method != "GET" {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	nodes := s.store.GetNodes()
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(nodes)
}

// handleNode returns details for a specific node
func (s *Server) handleNode(w http.ResponseWriter, r *http.Request) {
	if r.Method != "GET" {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	nodeID := strings.TrimPrefix(r.URL.Path, "/api/v1/node/")
	if nodeID == "" {
		http.Error(w, "node_id is required", http.StatusBadRequest)
		return
	}

	node, ok := s.store.GetNode(nodeID)
	if !ok {
		http.Error(w, "Node not found", http.StatusNotFound)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(node)
}

// handleCharts returns charts for a node
func (s *Server) handleCharts(w http.ResponseWriter, r *http.Request) {
	if r.Method != "GET" {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	nodeID := strings.TrimPrefix(r.URL.Path, "/api/v1/charts/")
	if nodeID == "" {
		http.Error(w, "node_id is required", http.StatusBadRequest)
		return
	}

	charts, ok := s.store.GetCharts(nodeID)
	if !ok {
		http.Error(w, "Node not found", http.StatusNotFound)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(charts)
}

// handleData returns chart data
func (s *Server) handleData(w http.ResponseWriter, r *http.Request) {
	if r.Method != "GET" {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	// Parse path: /api/v1/data/{node_id}/{chart_id}
	path := strings.TrimPrefix(r.URL.Path, "/api/v1/data/")
	parts := strings.SplitN(path, "/", 2)
	if len(parts) < 2 {
		http.Error(w, "node_id and chart_id are required", http.StatusBadRequest)
		return
	}

	nodeID := parts[0]
	chartID := parts[1]

	// Parse query params
	query := r.URL.Query()
	after, _ := strconv.ParseInt(query.Get("after"), 10, 64)
	before, _ := strconv.ParseInt(query.Get("before"), 10, 64)
	points, _ := strconv.Atoi(query.Get("points"))

	data, ok := s.store.GetChartData(nodeID, chartID, after, before, points)
	if !ok {
		http.Error(w, "Chart not found", http.StatusNotFound)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(data)
}

// handleHealth returns server health status
func (s *Server) handleHealth(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"status":    "healthy",
		"timestamp": time.Now().UnixMilli(),
		"nodes":     len(s.store.GetNodes()),
	})
}

// handleWebSocket handles WebSocket connections
func (s *Server) handleWebSocket(w http.ResponseWriter, r *http.Request) {
	s.wsHub.HandleConnection(w, r)
}

// handleStatic serves static files
func (s *Server) handleStatic(w http.ResponseWriter, r *http.Request) {
	path := r.URL.Path
	if path == "/" {
		path = "/index.html"
	}

	// Try to serve from embedded files
	content, err := s.webFS.ReadFile("web" + path)
	if err != nil {
		// Try index.html for SPA routing
		content, err = s.webFS.ReadFile("web/index.html")
		if err != nil {
			http.NotFound(w, r)
			return
		}
		path = "/index.html"
	}

	// Set content type
	contentType := "text/plain"
	switch {
	case strings.HasSuffix(path, ".html"):
		contentType = "text/html; charset=utf-8"
	case strings.HasSuffix(path, ".css"):
		contentType = "text/css; charset=utf-8"
	case strings.HasSuffix(path, ".js"):
		contentType = "application/javascript; charset=utf-8"
	case strings.HasSuffix(path, ".json"):
		contentType = "application/json"
	case strings.HasSuffix(path, ".svg"):
		contentType = "image/svg+xml"
	case strings.HasSuffix(path, ".png"):
		contentType = "image/png"
	case strings.HasSuffix(path, ".ico"):
		contentType = "image/x-icon"
	}

	w.Header().Set("Content-Type", contentType)
	w.Header().Set("Cache-Control", "public, max-age=3600")
	w.Write(content)
}
