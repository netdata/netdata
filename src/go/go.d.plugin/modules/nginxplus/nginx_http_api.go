// SPDX-License-Identifier: GPL-3.0-or-later

package nginxplus

import "time"

// https://demo.nginx.com/dashboard.html
// https://demo.nginx.com/swagger-ui/
// http://nginx.org/en/docs/http/ngx_http_api_module.html

type nginxAPIVersions []int64

type (
	nginxInfo struct {
		Version       string    `json:"version"`
		Build         string    `json:"build"`
		Address       string    `json:"address"`
		Generation    int       `json:"generation"`
		LoadTimestamp time.Time `json:"load_timestamp"`
		Timestamp     time.Time `json:"timestamp"`
	}
	nginxConnections struct {
		Accepted int64 `json:"accepted"`
		Dropped  int64 `json:"dropped"`
		Active   int64 `json:"active"`
		Idle     int64 `json:"idle"`
	}
	nginxSSL struct {
		Handshakes       int64 `json:"handshakes"`
		HandshakesFailed int64 `json:"handshakes_failed"`
		SessionReuses    int64 `json:"session_reuses"`
		NoCommonProtocol int64 `json:"no_common_protocol"`
		NoCommonCipher   int64 `json:"no_common_cipher"`
		HandshakeTimeout int64 `json:"handshake_timeout"`
		PeerRejectedCert int64 `json:"peer_rejected_cert"`
		VerifyFailures   struct {
			NoCert           int64 `json:"no_cert"`
			ExpiredCert      int64 `json:"expired_cert"`
			RevokedCert      int64 `json:"revoked_cert"`
			HostnameMismatch int64 `json:"hostname_mismatch"`
			Other            int64 `json:"other"`
		} `json:"verify_failures"`
	}
)

type (
	nginxHTTPRequests struct {
		Total   int64 `json:"total"`
		Current int64 `json:"current"`
	}
	nginxHTTPServerZones map[string]struct {
		Processing int64 `json:"processing"`
		Requests   int64 `json:"requests"`
		Responses  struct {
			Class1xx int64 `json:"1xx"`
			Class2xx int64 `json:"2xx"`
			Class3xx int64 `json:"3xx"`
			Class4xx int64 `json:"4xx"`
			Class5xx int64 `json:"5xx"`
			Total    int64
		} `json:"responses"`
		Discarded int64 `json:"discarded"`
		Received  int64 `json:"received"`
		Sent      int64 `json:"sent"`
	}
	nginxHTTPLocationZones map[string]struct {
		Requests  int64 `json:"requests"`
		Responses struct {
			Class1xx int64 `json:"1xx"`
			Class2xx int64 `json:"2xx"`
			Class3xx int64 `json:"3xx"`
			Class4xx int64 `json:"4xx"`
			Class5xx int64 `json:"5xx"`
			Total    int64
		} `json:"responses"`
		Discarded int64 `json:"discarded"`
		Received  int64 `json:"received"`
		Sent      int64 `json:"sent"`
	}
	nginxHTTPUpstreams map[string]struct {
		Peers []struct {
			Id           int64  `json:"id"`
			Server       string `json:"server"`
			Name         string `json:"name"`
			Backup       bool   `json:"backup"`
			Weight       int64  `json:"weight"`
			State        string `json:"state"`
			Active       int64  `json:"active"`
			Requests     int64  `json:"requests"`
			HeaderTime   int64  `json:"header_time"`
			ResponseTime int64  `json:"response_time"`
			Responses    struct {
				Class1xx int64 `json:"1xx"`
				Class2xx int64 `json:"2xx"`
				Class3xx int64 `json:"3xx"`
				Class4xx int64 `json:"4xx"`
				Class5xx int64 `json:"5xx"`
				Total    int64
			} `json:"responses"`
			Sent         int64 `json:"sent"`
			Received     int64 `json:"received"`
			Fails        int64 `json:"fails"`
			Unavail      int64 `json:"unavail"`
			HealthChecks struct {
				Checks    int64 `json:"checks"`
				Fails     int64 `json:"fails"`
				Unhealthy int64 `json:"unhealthy"`
			} `json:"health_checks"`
			Downtime int64     `json:"downtime"`
			Selected time.Time `json:"selected"`
		} `json:"peers"`
		Keepalive int64  `json:"keepalive"`
		Zombies   int64  `json:"zombies"`
		Zone      string `json:"zone"`
	}
	nginxHTTPCaches map[string]struct {
		Size int64 `json:"size"`
		Cold bool  `json:"cold"`
		Hit  struct {
			Responses int64 `json:"responses"`
			Bytes     int64 `json:"bytes"`
		} `json:"hit"`
		Stale struct {
			Responses int64 `json:"responses"`
			Bytes     int64 `json:"bytes"`
		} `json:"stale"`
		Updating struct {
			Responses int64 `json:"responses"`
			Bytes     int64 `json:"bytes"`
		} `json:"updating"`
		Revalidated struct {
			Responses int64 `json:"responses"`
			Bytes     int64 `json:"bytes"`
		} `json:"revalidated"`
		Miss struct {
			Responses        int64 `json:"responses"`
			Bytes            int64 `json:"bytes"`
			ResponsesWritten int64 `json:"responses_written"`
			BytesWritten     int64 `json:"bytes_written"`
		} `json:"miss"`
		Expired struct {
			Responses        int64 `json:"responses"`
			Bytes            int64 `json:"bytes"`
			ResponsesWritten int64 `json:"responses_written"`
			BytesWritten     int64 `json:"bytes_written"`
		} `json:"expired"`
		Bypass struct {
			Responses        int64 `json:"responses"`
			Bytes            int64 `json:"bytes"`
			ResponsesWritten int64 `json:"responses_written"`
			BytesWritten     int64 `json:"bytes_written"`
		} `json:"bypass"`
	}
)

type (
	nginxStreamServerZones map[string]struct {
		Processing  int64 `json:"processing"`
		Connections int64 `json:"connections"`
		Sessions    struct {
			Class2xx int64 `json:"2xx"`
			Class4xx int64 `json:"4xx"`
			Class5xx int64 `json:"5xx"`
			Total    int64 `json:"total"`
		} `json:"sessions"`
		Discarded int64 `json:"discarded"`
		Received  int64 `json:"received"`
		Sent      int64 `json:"sent"`
	}
	nginxStreamUpstreams map[string]struct {
		Peers []struct {
			Id           int64  `json:"id"`
			Server       string `json:"server"`
			Name         string `json:"name"`
			Backup       bool   `json:"backup"`
			Weight       int64  `json:"weight"`
			State        string `json:"state"`
			Active       int64  `json:"active"`
			Connections  int64  `json:"connections"`
			Sent         int64  `json:"sent"`
			Received     int64  `json:"received"`
			Fails        int64  `json:"fails"`
			Unavail      int64  `json:"unavail"`
			HealthChecks struct {
				Checks    int64 `json:"checks"`
				Fails     int64 `json:"fails"`
				Unhealthy int64 `json:"unhealthy"`
			} `json:"health_checks"`
			Downtime int64 `json:"downtime"`
		} `json:"peers"`
		Zombies int64  `json:"zombies"`
		Zone    string `json:"zone"`
	}
)

type nginxResolvers map[string]struct {
	Requests struct {
		Name int64 `json:"name"`
		Srv  int64 `json:"srv"`
		Addr int64 `json:"addr"`
	} `json:"requests"`
	Responses struct {
		NoError  int64 `json:"noerror"`
		Formerr  int64 `json:"formerr"`
		Servfail int64 `json:"servfail"`
		Nxdomain int64 `json:"nxdomain"`
		Notimp   int64 `json:"notimp"`
		Refused  int64 `json:"refused"`
		TimedOut int64 `json:"timedout"`
		Unknown  int64 `json:"unknown"`
	} `json:"responses"`
}
