// SPDX-License-Identifier: GPL-3.0-or-later

package bgp

import (
	"encoding/json"
	"fmt"
	"sort"
	"strconv"
	"strings"
)

func (c *Collector) collectFRRRPKICaches(client frrClientAPI, scrape *scrapeMetrics) []rpkiCacheStats {
	serversData, err := client.RPKICacheServers()
	if err != nil {
		if isUnsupportedProbeError(err) {
			c.Debugf("collect FRR RPKI cache servers: %v", err)
			return nil
		}
		scrape.noteQueryError(err, false)
		c.Debugf("collect FRR RPKI cache servers: %v", err)
		return nil
	}

	connectionsData, err := client.RPKICacheConnections()
	if err != nil {
		if isUnsupportedProbeError(err) {
			c.Debugf("collect FRR RPKI cache connections: %v", err)
			return nil
		}
		scrape.noteQueryError(err, false)
		c.Debugf("collect FRR RPKI cache connections: %v", err)
		return nil
	}

	caches, err := buildFRRRPKICaches(serversData, connectionsData)
	if err != nil {
		scrape.noteParseError(false)
		c.Debugf("parse FRR RPKI cache state: %v", err)
		return nil
	}

	return caches
}

func buildFRRRPKICaches(serversData, connectionsData []byte) ([]rpkiCacheStats, error) {
	servers, err := parseFRRRPKICacheServers(serversData)
	if err != nil {
		return nil, err
	}

	connections, err := parseFRRRPKICacheConnections(connectionsData)
	if err != nil {
		return nil, err
	}

	connected := make(map[string]frrRPKICacheConnection, len(connections.Connections))
	for _, conn := range connections.Connections {
		connected[frrRPKICacheKey(conn.Mode, conn.Host, conn.Port, conn.Preference)] = conn
	}

	caches := make([]rpkiCacheStats, 0, len(servers.Servers))
	for _, server := range servers.Servers {
		key := frrRPKICacheKey(server.Mode, server.Host, server.Port, server.Preference)
		name := frrRPKICacheName(server.Mode, server.Host, server.Port, server.Preference)
		conn, ok := connected[key]

		stateText := "disconnected"
		up := false
		if ok {
			stateText = strings.TrimSpace(conn.State)
			up = strings.EqualFold(stateText, "connected")
			if stateText == "" {
				if up {
					stateText = "connected"
				} else {
					stateText = "disconnected"
				}
			}
		}

		caches = append(caches, rpkiCacheStats{
			ID:        idPart(name),
			Backend:   backendFRR,
			Name:      name,
			StateText: stateText,
			Up:        up,
		})
	}

	sort.Slice(caches, func(i, j int) bool {
		return caches[i].Name < caches[j].Name
	})

	return caches, nil
}

func parseFRRRPKICacheServers(data []byte) (frrRPKICacheServersReply, error) {
	var reply frrRPKICacheServersReply
	if err := json.Unmarshal(data, &reply); err != nil {
		return reply, fmt.Errorf("unmarshal FRR RPKI cache servers: %w", err)
	}
	return reply, nil
}

func parseFRRRPKICacheConnections(data []byte) (frrRPKICacheConnectionsReply, error) {
	var reply frrRPKICacheConnectionsReply
	if err := json.Unmarshal(data, &reply); err != nil {
		return reply, fmt.Errorf("unmarshal FRR RPKI cache connections: %w", err)
	}
	return reply, nil
}

func frrRPKICacheName(mode, host, port string, preference int64) string {
	mode = strings.TrimSpace(strings.ToLower(mode))
	host = strings.TrimSpace(host)
	port = strings.TrimSpace(port)
	return fmt.Sprintf("%s %s:%s pref %d", mode, host, port, preference)
}

func frrRPKICacheKey(mode, host, port string, preference int64) string {
	return makeCompositeID(mode, host, port, strconv.FormatInt(preference, 10))
}
