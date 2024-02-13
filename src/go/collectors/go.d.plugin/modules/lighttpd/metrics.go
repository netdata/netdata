// SPDX-License-Identifier: GPL-3.0-or-later

package lighttpd

type (
	serverStatus struct {
		Total struct {
			Accesses *int64 `stm:"accesses"`
			KBytes   *int64 `stm:"kBytes"`
		} `stm:"total"`
		Servers struct {
			Busy *int64 `stm:"busy_servers"`
			Idle *int64 `stm:"idle_servers"`
		} `stm:""`
		Uptime     *int64      `stm:"uptime"`
		Scoreboard *scoreboard `stm:"scoreboard"`
	}
	scoreboard struct {
		Waiting       int64 `stm:"waiting"`
		Open          int64 `stm:"open"`
		Close         int64 `stm:"close"`
		HardError     int64 `stm:"hard_error"`
		KeepAlive     int64 `stm:"keepalive"`
		Read          int64 `stm:"read"`
		ReadPost      int64 `stm:"read_post"`
		Write         int64 `stm:"write"`
		HandleRequest int64 `stm:"handle_request"`
		RequestStart  int64 `stm:"request_start"`
		RequestEnd    int64 `stm:"request_end"`
		ResponseStart int64 `stm:"response_start"`
		ResponseEnd   int64 `stm:"response_end"`
	}
)
