// SPDX-License-Identifier: GPL-3.0-or-later

package bgp

type (
	frrRPKICacheServersReply struct {
		Servers []frrRPKICacheServer `json:"servers"`
	}

	frrRPKICacheServer struct {
		Mode       string `json:"mode"`
		Host       string `json:"host"`
		Port       string `json:"port"`
		Preference int64  `json:"preference"`
	}

	frrRPKICacheConnectionsReply struct {
		Error          string                   `json:"error"`
		ConnectedGroup int64                    `json:"connectedGroup"`
		Connections    []frrRPKICacheConnection `json:"connections"`
	}

	frrRPKICacheConnection struct {
		Mode       string `json:"mode"`
		Host       string `json:"host"`
		Port       string `json:"port"`
		Preference int64  `json:"preference"`
		State      string `json:"state"`
	}

	frrRPKIPrefixCountReply struct {
		IPv4PrefixCount int64 `json:"ipv4PrefixCount"`
		IPv6PrefixCount int64 `json:"ipv6PrefixCount"`
	}
)
