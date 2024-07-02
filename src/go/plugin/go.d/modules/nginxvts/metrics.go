// SPDX-License-Identifier: GPL-3.0-or-later

package nginxvts

// NginxVTS metrics: https://github.com/vozlt/nginx-module-vts#json

type vtsMetrics struct {
	// HostName     string
	// NginxVersion string
	LoadMsec    int64
	NowMsec     int64
	Uptime      int64
	Connections struct {
		Active   int64 `stm:"active"`
		Reading  int64 `stm:"reading"`
		Writing  int64 `stm:"writing"`
		Waiting  int64 `stm:"waiting"`
		Accepted int64 `stm:"accepted"`
		Handled  int64 `stm:"handled"`
		Requests int64 `stm:"requests"`
	} `stm:"connections"`
	SharedZones struct {
		// Name     string
		MaxSize  int64 `stm:"maxsize"`
		UsedSize int64 `stm:"usedsize"`
		UsedNode int64 `stm:"usednode"`
	}
	ServerZones map[string]Server
}

func (m vtsMetrics) hasServerZones() bool { return m.ServerZones != nil }

// Server is for total Nginx server
type Server struct {
	RequestCounter int64 `stm:"requestcounter"`
	InBytes        int64 `stm:"inbytes"`
	OutBytes       int64 `stm:"outbytes"`
	Responses      struct {
		Resp1xx     int64 `stm:"responses_1xx" json:"1xx"`
		Resp2xx     int64 `stm:"responses_2xx" json:"2xx"`
		Resp3xx     int64 `stm:"responses_3xx" json:"3xx"`
		Resp4xx     int64 `stm:"responses_4xx" json:"4xx"`
		Resp5xx     int64 `stm:"responses_5xx" json:"5xx"`
		Miss        int64 `stm:"cache_miss"`
		Bypass      int64 `stm:"cache_bypass"`
		Expired     int64 `stm:"cache_expired"`
		Stale       int64 `stm:"cache_stale"`
		Updating    int64 `stm:"cache_updating"`
		Revalidated int64 `stm:"cache_revalidated"`
		Hit         int64 `stm:"cache_hit"`
		Scarce      int64 `stm:"cache_scarce"`
	} `stm:""`
}
