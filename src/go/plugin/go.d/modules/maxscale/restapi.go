// SPDX-License-Identifier: GPL-3.0-or-later

package maxscale

// https://mariadb.com/kb/en/maxscale-24-02rest-api/

type maxscaleGlobalResponse struct {
	Data *struct {
		Attrs struct {
			Params struct {
				Threads int64 `json:"threads"`
				Passive bool  `json:"passive"`
			} `json:"parameters"`
			Uptime int64 `json:"uptime"`
		} `json:"attributes"`
	} `json:"data"`
}

// https://github.com/mariadb-corporation/MaxScale/blob/f72af9927243f59f2b20cc835273dbe6dc158623/server/core/routingworker.cc#L3021
// https://github.com/mariadb-corporation/MaxScale/blob/f72af9927243f59f2b20cc835273dbe6dc158623/maxctrl/lib/show.js#L471
type maxscaleThreadsResponse struct {
	Data []struct {
		ID    string `json:"id"`
		Attrs struct {
			Stats struct {
				State              string `json:"state"`
				Reads              int64  `json:"reads"`
				Writes             int64  `json:"writes"`
				Errors             int64  `json:"errors"`
				Hangups            int64  `json:"hangups"`
				Accepts            int64  `json:"accepts"`
				Sessions           int64  `json:"sessions"`
				Zombies            int64  `json:"zombies"`
				CurrentDescriptors int64  `json:"current_descriptors"`
				TotalDescriptors   int64  `json:"total_descriptors"`
				QCCache            struct {
					Size      int64 `json:"size"`
					Inserts   int64 `json:"inserts"`
					Hits      int64 `json:"hits"`
					Misses    int64 `json:"misses"`
					Evictions int64 `json:"evictions"`
				} `json:"query_classifier_cache"`
			} `json:"stats"`
		} `json:"attributes"`
	} `json:"data"`
}

// // https://github.com/mariadb-corporation/MaxScale/blob/f72af9927243f59f2b20cc835273dbe6dc158623/server/core/routingworker.cc#L3064
var threadStates = []string{
	"Active",
	"Draining",
	"Dormant",
}

type serversResponse struct {
	Data []struct {
		ID    string `json:"id"`
		Type  string `json:"type"`
		Attrs struct {
			Params struct {
				Address string `json:"address"`
				Port    int    `json:"port"`
			} `json:"parameters"`
			State      string `json:"state"`
			Statistics struct {
				Connections int64 `json:"connections"`
			} `json:"statistics"`
		} `json:"attributes"`
	} `json:"data"`
}

// https://github.com/mariadb-corporation/MaxScale/blob/f72af9927243f59f2b20cc835273dbe6dc158623/system-test/maxtest/src/maxscales.cc#L43
var serverStates = []string{
	"Master",
	"Slave",
	"Running",
	"Down",
	"Maintenance",
	"Draining",
	"Drained",
	"Relay Master",
	"Binlog Relay",
	"Synced",
}
