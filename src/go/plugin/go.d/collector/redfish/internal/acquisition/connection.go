// SPDX-License-Identifier: GPL-3.0-or-later

package acquisition

import (
	"net/http"
	"net/url"
)

// connection is immutable after setup. Authentication belongs to each SDK
// client; only the transport and its per-job request admission are shared.
type connection struct {
	config Options
	http   *http.Client
	root   *url.URL
	origin string
}
