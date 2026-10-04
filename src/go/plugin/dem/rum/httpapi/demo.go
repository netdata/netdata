// SPDX-License-Identifier: GPL-3.0-or-later

package httpapi

import (
	_ "embed"
	"html/template"
	"net/http"
)

//go:embed demo.html
var demoHTML string

var rumDemoPage = template.Must(template.New("demo").Parse(demoHTML))

func (s *Server) demo(w http.ResponseWriter, r *http.Request) {
	key := r.URL.Query().Get("key")
	if key == "" {
		key = "demo"
	}
	_, release, ok := s.acquire(w, r, key)
	if !ok {
		return
	}
	defer release()
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.Header().Set("Cache-Control", "no-store")
	_ = rumDemoPage.Execute(w, map[string]string{"Name": key})
}
