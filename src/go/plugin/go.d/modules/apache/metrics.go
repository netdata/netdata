// SPDX-License-Identifier: GPL-3.0-or-later

package apache

type (
	serverStatus struct {
		// ExtendedStatus
		Total struct {
			// Total number of accesses.
			Accesses *int64 `stm:"accesses"`
			// Total number of byte count served.
			// This metric reflects the bytes that should have been served,
			// which is not necessarily equal to the bytes actually (successfully) served.
			KBytes *int64 `stm:"kBytes"`
		} `stm:"total"`
		Averages struct {
			//Average number of requests per second.
			ReqPerSec *float64 `stm:"req_per_sec,100000,1"`
			// Average number of bytes served per second.
			BytesPerSec *float64 `stm:"bytes_per_sec,100000,1"`
			// Average number of bytes per request.
			BytesPerReq *float64 `stm:"bytes_per_req,100000,1"`
		} `stm:""`
		Uptime *int64 `stm:"uptime"`

		Workers struct {
			// Total number of busy worker threads/processes.
			// A worker is considered “busy” if it is in any of the following states:
			// reading, writing, keep-alive, logging, closing, or gracefully finishing.
			Busy *int64 `stm:"busy_workers"`
			// Total number of idle worker threads/processes.
			// An “idle” worker is not in any of the busy states.
			Idle *int64 `stm:"idle_workers"`
		} `stm:""`
		Connections struct {
			Total *int64 `stm:"total"`
			Async struct {
				// Number of async connections in writing state (only applicable to event MPM).
				Writing *int64 `stm:"writing"`
				// Number of async connections in keep-alive state (only applicable to event MPM).
				KeepAlive *int64 `stm:"keep_alive"`
				// Number of async connections in closing state (only applicable to event MPM).
				Closing *int64 `stm:"closing"`
			} `stm:"async"`
		} `stm:"conns"`
		Scoreboard *scoreboard `stm:"scoreboard"`
	}
	scoreboard struct {
		Waiting     int64 `stm:"waiting"`
		Starting    int64 `stm:"starting"`
		Reading     int64 `stm:"reading"`
		Sending     int64 `stm:"sending"`
		KeepAlive   int64 `stm:"keepalive"`
		DNSLookup   int64 `stm:"dns_lookup"`
		Closing     int64 `stm:"closing"`
		Logging     int64 `stm:"logging"`
		Finishing   int64 `stm:"finishing"`
		IdleCleanup int64 `stm:"idle_cleanup"`
		Open        int64 `stm:"open"`
	}
)
