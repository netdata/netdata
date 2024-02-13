// SPDX-License-Identifier: GPL-3.0-or-later

package nginx

type stubStatus struct {
	Connections struct {
		// The current number of active client connections including Waiting connections.
		Active int64 `stm:"active"`

		// The total number of accepted client connections.
		Accepts int64 `stm:"accepts"`

		// The total number of handled connections.
		// Generally, the parameter value is the same as accepts unless some resource limits have been reached.
		Handled int64 `stm:"handled"`

		// The current number of connections where nginx is reading the request header.
		Reading int64 `stm:"reading"`

		// The current number of connections where nginx is writing the response back to the client.
		Writing int64 `stm:"writing"`

		// The current number of idle client connections waiting for a request.
		Waiting int64 `stm:"waiting"`
	} `stm:""`
	Requests struct {
		// The total number of client requests.
		Total int64 `stm:"requests"`

		// Note: tengine specific
		// The total requests' response time, which is in millisecond
		Time *int64 `stm:"request_time"`
	} `stm:""`
}
