// SPDX-License-Identifier: GPL-3.0-or-later

package boinc

import (
	"encoding/xml"
)

// https://boinc.berkeley.edu/trac/wiki/GuiRpcProtocol

type boincRequest struct {
	XMLName    xml.Name                `xml:"boinc_gui_rpc_request"`
	Auth1      *struct{}               `xml:"auth1"`
	Auth2      *boincRequestAuthNonce  `xml:"auth2"`
	GetResults *boincRequestGetResults `xml:"get_results"`
}

type (
	boincRequestAuthNonce struct {
		Hash string `xml:"nonce_hash"`
	}
	boincRequestGetResults struct {
		ActiveOnly int `xml:"active_only"`
	}
)

type boincReply struct {
	XMLName      xml.Name           `xml:"boinc_gui_rpc_reply"`
	Error        *string            `xml:"error"`
	BadRequest   *struct{}          `xml:"bad_request"`
	Authorized   *struct{}          `xml:"authorized"`
	Unauthorized *struct{}          `xml:"unauthorized"`
	Nonce        *string            `xml:"nonce"`
	Results      []boincReplyResult `xml:"results>result"`
}

type (
	boincReplyResult struct {
		State      int                         `xml:"state"`
		ActiveTask *boincReplyResultActiveTask `xml:"active_task"`
	}
	boincReplyResultActiveTask struct {
		ActiveTaskState int `xml:"active_task_state"`
		SchedulerState  int `xml:"scheduler_state"`
	}
)

func (r *boincReplyResult) state() string {
	if v, ok := resultStateMap[r.State]; ok {
		return v
	}
	return "unknown"
}

func (r *boincReplyResult) activeTaskState() string {
	if r.ActiveTask == nil {
		return "no_active_task"
	}
	if v, ok := activeTaskStateMap[r.ActiveTask.ActiveTaskState]; ok {
		return v
	}
	return "unknown"
}

func (r *boincReplyResult) schedulerState() string {
	if r.ActiveTask == nil {
		return "no_scheduler"
	}
	if v, ok := schedulerStateMap[r.ActiveTask.SchedulerState]; ok {
		return v
	}
	return "unknown"
}

var resultStateMap = map[int]string{
	// https://github.com/BOINC/boinc/blob/a3b79635d87423c972125efa318e4e880ad698dd/html/inc/common_defs.inc#L75
	0: "new",
	1: "files_downloading",
	2: "files_downloaded",
	3: "compute_error",
	4: "files_uploading",
	5: "files_uploaded",
	6: "aborted",
	7: "upload_failed",
}

var activeTaskStateMap = map[int]string{
	// https://github.com/BOINC/boinc/blob/a3b79635d87423c972125efa318e4e880ad698dd/lib/common_defs.h#L227
	0: "uninitialized",
	1: "executing",
	//2:  "exited",
	//3:  "was_signaled",
	//4:  "exit_unknown",
	5: "abort_pending",
	//6:  "aborted",
	//7:  "couldnt_start",
	8:  "quit_pending",
	9:  "suspended",
	10: "copy_pending",
}

var schedulerStateMap = map[int]string{
	// https://github.com/BOINC/boinc/blob/a3b79635d87423c972125efa318e4e880ad698dd/lib/common_defs.h#L56
	0: "uninitialized",
	1: "preempted",
	2: "scheduled",
}
