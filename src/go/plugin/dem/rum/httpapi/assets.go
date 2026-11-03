// SPDX-License-Identifier: GPL-3.0-or-later

package httpapi

import (
	"net/http"
	"strings"
	"time"

	"github.com/netdata/netdata/go/plugins/plugin/dem/rum/faro"
)

// asset serves public, byte-identified assets independently of any site runtime.
func (s *Server) asset(w http.ResponseWriter, r *http.Request) {
	asset, ok := faro.LookupAsset(r.PathValue("asset"))
	if !ok {
		w.Header().Set("Cache-Control", "no-store")
		http.NotFound(w, r)
		return
	}
	w.Header().Set("Content-Type", asset.ContentType)
	w.Header().Set("X-Content-Type-Options", "nosniff")
	w.Header().Set("Access-Control-Allow-Origin", "*")
	w.Header().Set("Cross-Origin-Resource-Policy", "cross-origin")
	w.Header().Set("Cache-Control", "public, max-age=31536000, immutable")
	w.Header().Set("ETag", asset.ETag)
	// A relative link preserves the externally visible reverse-proxy prefix.
	w.Header().Set("Link", "<"+strings.TrimPrefix(faro.NoticesPath, "assets/")+">; rel=\"license\"")
	http.ServeContent(w, r, r.PathValue("asset"), time.Time{}, strings.NewReader(asset.Content))
}
