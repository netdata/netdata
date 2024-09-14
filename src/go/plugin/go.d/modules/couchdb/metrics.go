// SPDX-License-Identifier: GPL-3.0-or-later

package couchdb

// https://docs.couchdb.org/en/stable/api/index.html

type cdbMetrics struct {
	// https://docs.couchdb.org/en/stable/api/server/common.html#active-tasks
	ActiveTasks []cdbActiveTask
	// https://docs.couchdb.org/en/stable/api/server/common.html#node-node-name-stats
	NodeStats *cdbNodeStats
	// https://docs.couchdb.org/en/stable/api/server/common.html#node-node-name-system
	NodeSystem *cdbNodeSystem
	// https://docs.couchdb.org/en/stable/api/database/common.html
	DBStats []cdbDBStats
}

func (m cdbMetrics) empty() bool {
	switch {
	case m.hasActiveTasks(), m.hasNodeStats(), m.hasNodeSystem(), m.hasDBStats():
		return false
	}
	return true
}

func (m cdbMetrics) hasActiveTasks() bool { return m.ActiveTasks != nil }
func (m cdbMetrics) hasNodeStats() bool   { return m.NodeStats != nil }
func (m cdbMetrics) hasNodeSystem() bool  { return m.NodeSystem != nil }
func (m cdbMetrics) hasDBStats() bool     { return m.DBStats != nil }

type cdbActiveTask struct {
	Type string `json:"type"`
}

type cdbNodeStats struct {
	CouchDB struct {
		DatabaseReads struct {
			Value float64 `stm:"" json:"value"`
		} `stm:"database_reads" json:"database_reads"`
		DatabaseWrites struct {
			Value float64 `stm:"" json:"value"`
		} `stm:"database_writes" json:"database_writes"`
		HTTPd struct {
			ViewReads struct {
				Value float64 `stm:"" json:"value"`
			} `stm:"view_reads" json:"view_reads"`
		} `stm:"httpd" json:"httpd"`
		HTTPdRequestMethods struct {
			Copy struct {
				Value float64 `stm:"" json:"value"`
			} `stm:"COPY" json:"COPY"`
			Delete struct {
				Value float64 `stm:"" json:"value"`
			} `stm:"DELETE" json:"DELETE"`
			Get struct {
				Value float64 `stm:"" json:"value"`
			} `stm:"GET" json:"GET"`
			Head struct {
				Value float64 `stm:"" json:"value"`
			} `stm:"HEAD" json:"HEAD"`
			Options struct {
				Value float64 `stm:"" json:"value"`
			} `stm:"OPTIONS" json:"OPTIONS"`
			Post struct {
				Value float64 `stm:"" json:"value"`
			} `stm:"POST" json:"POST"`
			Put struct {
				Value float64 `stm:"" json:"value"`
			} `stm:"PUT" json:"PUT"`
		} `stm:"httpd_request_methods" json:"httpd_request_methods"`
		HTTPdStatusCodes struct {
			Code200 struct {
				Value float64 `stm:"" json:"value"`
			} `stm:"200" json:"200"`
			Code201 struct {
				Value float64 `stm:"" json:"value"`
			} `stm:"201" json:"201"`
			Code202 struct {
				Value float64 `stm:"" json:"value"`
			} `stm:"202" json:"202"`
			Code204 struct {
				Value float64 `stm:"" json:"value"`
			} `stm:"204" json:"204"`
			Code206 struct {
				Value float64 `stm:"" json:"value"`
			} `stm:"206" json:"206"`
			Code301 struct {
				Value float64 `stm:"" json:"value"`
			} `stm:"301" json:"301"`
			Code302 struct {
				Value float64 `stm:"" json:"value"`
			} `stm:"302" json:"302"`
			Code304 struct {
				Value float64 `stm:"" json:"value"`
			} `stm:"304" json:"304"`
			Code400 struct {
				Value float64 `stm:"" json:"value"`
			} `stm:"400" json:"400"`
			Code401 struct {
				Value float64 `stm:"" json:"value"`
			} `stm:"401" json:"401"`
			Code403 struct {
				Value float64 `stm:"" json:"value"`
			} `stm:"403" json:"403"`
			Code404 struct {
				Value float64 `stm:"" json:"value"`
			} `stm:"404" json:"404"`
			Code405 struct {
				Value float64 `stm:"" json:"value"`
			} `stm:"405" json:"405"`
			Code406 struct {
				Value float64 `stm:"" json:"value"`
			} `stm:"406" json:"406"`
			Code409 struct {
				Value float64 `stm:"" json:"value"`
			} `stm:"409" json:"409"`
			Code412 struct {
				Value float64 `stm:"" json:"value"`
			} `stm:"412" json:"412"`
			Code413 struct {
				Value float64 `stm:"" json:"value"`
			} `stm:"413" json:"413"`
			Code414 struct {
				Value float64 `stm:"" json:"value"`
			} `stm:"414" json:"414"`
			Code415 struct {
				Value float64 `stm:"" json:"value"`
			} `stm:"415" json:"415"`
			Code416 struct {
				Value float64 `stm:"" json:"value"`
			} `stm:"416" json:"416"`
			Code417 struct {
				Value float64 `stm:"" json:"value"`
			} `stm:"417" json:"417"`
			Code500 struct {
				Value float64 `stm:"" json:"value"`
			} `stm:"500" json:"500"`
			Code501 struct {
				Value float64 `stm:"" json:"value"`
			} `stm:"501" json:"501"`
			Code503 struct {
				Value float64 `stm:"" json:"value"`
			} `stm:"503" json:"503"`
		} `stm:"httpd_status_codes" json:"httpd_status_codes"`
		OpenOSFiles struct {
			Value float64 `stm:"" json:"value"`
		} `stm:"open_os_files" json:"open_os_files"`
	} `stm:"couchdb"  json:"couchdb"`
	CouchReplicator struct {
		Jobs struct {
			Running struct {
				Value float64 `stm:"" json:"value"`
			} `stm:"running" json:"running"`
			Pending struct {
				Value float64 `stm:"" json:"value"`
			} `stm:"pending" json:"pending"`
			Crashed struct {
				Value float64 `stm:"" json:"value"`
			} `stm:"crashed" json:"crashed"`
		} `stm:"jobs" json:"jobs"`
	} `stm:"couch_replicator" json:"couch_replicator"`
}

type cdbNodeSystem struct {
	Memory struct {
		Other     float64 `stm:"other" json:"other"`
		Atom      float64 `stm:"atom" json:"atom"`
		AtomUsed  float64 `stm:"atom_used" json:"atom_used"`
		Processes float64 `stm:"processes" json:"processes"`
		Binary    float64 `stm:"binary" json:"binary"`
		Code      float64 `stm:"code" json:"code"`
		Ets       float64 `stm:"ets" json:"ets"`
	} `stm:"memory" json:"memory"`

	RunQueue                float64 `stm:"run_queue" json:"run_queue"`
	EtsTableCount           float64 `stm:"ets_table_count" json:"ets_table_count"`
	ContextSwitches         float64 `stm:"context_switches" json:"context_switches"`
	Reductions              float64 `stm:"reductions" json:"reductions"`
	IOInput                 float64 `stm:"io_input" json:"io_input"`
	IOOutput                float64 `stm:"io_output" json:"io_output"`
	OSProcCount             float64 `stm:"os_proc_count" json:"os_proc_count"`
	ProcessCount            float64 `stm:"process_count" json:"process_count"`
	InternalReplicationJobs float64 `stm:"internal_replication_jobs" json:"internal_replication_jobs"`

	MessageQueues map[string]any `json:"message_queues"`
}

type cdbDBStats struct {
	Key   string
	Error string
	Info  struct {
		Sizes struct {
			File     float64 `stm:"file" json:"file"`
			External float64 `stm:"external" json:"external"`
			Active   float64 `stm:"active" json:"active"`
		} `stm:"db_sizes" json:"sizes"`
		DocDelCount float64 `stm:"db_doc_del_counts" json:"doc_del_count"`
		DocCount    float64 `stm:"db_doc_counts" json:"doc_count"`
	}
}
