// SPDX-License-Identifier: GPL-3.0-or-later

package httpcheck

type metrics struct {
	Status         status `stm:""`
	InState        int    `stm:"in_state"`
	ResponseTime   int    `stm:"time"`
	ResponseLength int    `stm:"length"`
}

type status struct {
	Success       bool `stm:"success"` // No error on request, body reading and checking its content
	Timeout       bool `stm:"timeout"`
	Redirect      bool `stm:"redirect"`
	BadContent    bool `stm:"bad_content"`
	BadStatusCode bool `stm:"bad_status"`
	BadHeader     bool `stm:"bad_header"`
	NoConnection  bool `stm:"no_connection"` // All other errors basically
}
