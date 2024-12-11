// SPDX-License-Identifier: GPL-3.0-or-later

package couchdb

import (
	"github.com/netdata/netdata/go/plugins/plugin/go.d/agent/module"
)

type (
	Charts = module.Charts
	Dims   = module.Dims
	Vars   = module.Vars
)

var dbActivityCharts = Charts{
	{
		ID:    "activity",
		Title: "Overall Activity",
		Units: "requests/s",
		Fam:   "dbactivity",
		Ctx:   "couchdb.activity",
		Type:  module.Stacked,
		Dims: Dims{
			{ID: "couchdb_database_reads", Name: "DB reads", Algo: module.Incremental},
			{ID: "couchdb_database_writes", Name: "DB writes", Algo: module.Incremental},
			{ID: "couchdb_httpd_view_reads", Name: "View reads", Algo: module.Incremental},
		},
	},
}

var httpTrafficBreakdownCharts = Charts{
	{
		ID:    "request_methods",
		Title: "HTTP request methods",
		Units: "requests/s",
		Fam:   "httptraffic",
		Ctx:   "couchdb.request_methods",
		Type:  module.Stacked,
		Dims: Dims{
			{ID: "couchdb_httpd_request_methods_COPY", Name: "COPY", Algo: module.Incremental},
			{ID: "couchdb_httpd_request_methods_DELETE", Name: "DELETE", Algo: module.Incremental},
			{ID: "couchdb_httpd_request_methods_GET", Name: "GET", Algo: module.Incremental},
			{ID: "couchdb_httpd_request_methods_HEAD", Name: "HEAD", Algo: module.Incremental},
			{ID: "couchdb_httpd_request_methods_OPTIONS", Name: "OPTIONS", Algo: module.Incremental},
			{ID: "couchdb_httpd_request_methods_POST", Name: "POST", Algo: module.Incremental},
			{ID: "couchdb_httpd_request_methods_PUT", Name: "PUT", Algo: module.Incremental},
		},
	},
	{
		ID:    "response_codes",
		Title: "HTTP response status codes",
		Units: "responses/s",
		Fam:   "httptraffic",
		Ctx:   "couchdb.response_codes",
		Type:  module.Stacked,
		Dims: Dims{
			{ID: "couchdb_httpd_status_codes_200", Name: "200 OK", Algo: module.Incremental},
			{ID: "couchdb_httpd_status_codes_201", Name: "201 Created", Algo: module.Incremental},
			{ID: "couchdb_httpd_status_codes_202", Name: "202 Accepted", Algo: module.Incremental},
			{ID: "couchdb_httpd_status_codes_204", Name: "204 No Content", Algo: module.Incremental},
			{ID: "couchdb_httpd_status_codes_206", Name: "206 Partial Content", Algo: module.Incremental},
			{ID: "couchdb_httpd_status_codes_301", Name: "301 Moved Permanently", Algo: module.Incremental},
			{ID: "couchdb_httpd_status_codes_302", Name: "302 Found", Algo: module.Incremental},
			{ID: "couchdb_httpd_status_codes_304", Name: "304 Not Modified", Algo: module.Incremental},
			{ID: "couchdb_httpd_status_codes_400", Name: "400 Bad Request", Algo: module.Incremental},
			{ID: "couchdb_httpd_status_codes_401", Name: "401 Unauthorized", Algo: module.Incremental},
			{ID: "couchdb_httpd_status_codes_403", Name: "403 Forbidden", Algo: module.Incremental},
			{ID: "couchdb_httpd_status_codes_404", Name: "404 Not Found", Algo: module.Incremental},
			{ID: "couchdb_httpd_status_codes_406", Name: "406 Not Acceptable", Algo: module.Incremental},
			{ID: "couchdb_httpd_status_codes_409", Name: "409 Conflict", Algo: module.Incremental},
			{ID: "couchdb_httpd_status_codes_412", Name: "412 Precondition Failed", Algo: module.Incremental},
			{ID: "couchdb_httpd_status_codes_413", Name: "413 Request Entity Too Long", Algo: module.Incremental},
			{ID: "couchdb_httpd_status_codes_414", Name: "414 Request URI Too Long", Algo: module.Incremental},
			{ID: "couchdb_httpd_status_codes_415", Name: "415 Unsupported Media Type", Algo: module.Incremental},
			{ID: "couchdb_httpd_status_codes_416", Name: "416 Requested Range Not Satisfiable", Algo: module.Incremental},
			{ID: "couchdb_httpd_status_codes_417", Name: "417 Expectation Failed", Algo: module.Incremental},
			{ID: "couchdb_httpd_status_codes_500", Name: "500 Internal Server Error", Algo: module.Incremental},
			{ID: "couchdb_httpd_status_codes_501", Name: "501 Not Implemented", Algo: module.Incremental},
			{ID: "couchdb_httpd_status_codes_503", Name: "503 Service Unavailable", Algo: module.Incremental},
		},
	},
	{
		ID:    "response_code_classes",
		Title: "HTTP response status code classes",
		Units: "responses/s",
		Fam:   "httptraffic",
		Ctx:   "couchdb.response_code_classes",
		Type:  module.Stacked,
		Dims: Dims{
			{ID: "couchdb_httpd_status_codes_2xx", Name: "2xx Success", Algo: module.Incremental},
			{ID: "couchdb_httpd_status_codes_3xx", Name: "3xx Redirection", Algo: module.Incremental},
			{ID: "couchdb_httpd_status_codes_4xx", Name: "4xx Client error", Algo: module.Incremental},
			{ID: "couchdb_httpd_status_codes_5xx", Name: "5xx Server error", Algo: module.Incremental},
		},
	},
}

var serverOperationsCharts = Charts{
	{
		ID:    "active_tasks",
		Title: "Active task breakdown",
		Units: "tasks",
		Fam:   "ops",
		Ctx:   "couchdb.active_tasks",
		Type:  module.Stacked,
		Dims: Dims{
			{ID: "active_tasks_indexer", Name: "Indexer"},
			{ID: "active_tasks_database_compaction", Name: "DB Compaction"},
			{ID: "active_tasks_replication", Name: "Replication"},
			{ID: "active_tasks_view_compaction", Name: "View Compaction"},
		},
	},
	{
		ID:    "replicator_jobs",
		Title: "Replicator job breakdown",
		Units: "jobs",
		Fam:   "ops",
		Ctx:   "couchdb.replicator_jobs",
		Type:  module.Stacked,
		Dims: Dims{
			{ID: "couch_replicator_jobs_running", Name: "Running"},
			{ID: "couch_replicator_jobs_pending", Name: "Pending"},
			{ID: "couch_replicator_jobs_crashed", Name: "Crashed"},
			{ID: "internal_replication_jobs", Name: "Internal replication jobs"},
		},
	},
	{
		ID:    "open_files",
		Title: "Open files",
		Units: "files",
		Fam:   "ops",
		Ctx:   "couchdb.open_files",
		Dims: Dims{
			{ID: "couchdb_open_os_files", Name: "# files"},
		},
	},
}

var erlangStatisticsCharts = Charts{
	{
		ID:    "erlang_memory",
		Title: "Erlang VM memory usage",
		Units: "B",
		Fam:   "erlang",
		Ctx:   "couchdb.erlang_vm_memory",
		Type:  module.Stacked,
		Dims: Dims{
			{ID: "memory_atom", Name: "atom"},
			{ID: "memory_binary", Name: "binaries"},
			{ID: "memory_code", Name: "code"},
			{ID: "memory_ets", Name: "ets"},
			{ID: "memory_processes", Name: "procs"},
			{ID: "memory_other", Name: "other"},
		},
	},
	{
		ID:    "erlang_proc_counts",
		Title: "Process counts",
		Units: "processes",
		Fam:   "erlang",
		Ctx:   "couchdb.proccounts",
		Dims: Dims{
			{ID: "os_proc_count", Name: "OS procs"},
			{ID: "process_count", Name: "erl procs"},
		},
	},
	{
		ID:    "erlang_peak_msg_queue",
		Title: "Peak message queue size",
		Units: "messages",
		Fam:   "erlang",
		Ctx:   "couchdb.peakmsgqueue",
		Dims: Dims{
			{ID: "peak_msg_queue", Name: "peak size"},
		},
	},
	{
		ID:    "erlang_reductions",
		Title: "Erlang reductions",
		Units: "reductions",
		Fam:   "erlang",
		Ctx:   "couchdb.reductions",
		Type:  module.Stacked,
		Dims: Dims{
			{ID: "reductions", Name: "reductions", Algo: module.Incremental},
		},
	},
}

var (
	dbSpecificCharts = Charts{
		{
			ID:    "db_sizes_file",
			Title: "Database sizes (file)",
			Units: "KiB",
			Fam:   "perdbstats",
			Ctx:   "couchdb.db_sizes_file",
		},
		{
			ID:    "db_sizes_external",
			Title: "Database sizes (external)",
			Units: "KiB",
			Fam:   "perdbstats",
			Ctx:   "couchdb.db_sizes_external",
		},
		{
			ID:    "db_sizes_active",
			Title: "Database sizes (active)",
			Units: "KiB",
			Fam:   "perdbstats",
			Ctx:   "couchdb.db_sizes_active",
		},
		{
			ID:    "db_doc_counts",
			Title: "Database # of docs",
			Units: "docs",
			Fam:   "perdbstats",
			Ctx:   "couchdb.db_doc_count",
		},
		{
			ID:    "db_doc_del_counts",
			Title: "Database # of deleted docs",
			Units: "docs",
			Fam:   "perdbstats",
			Ctx:   "couchdb.db_doc_del_count",
		},
	}
)
