// SPDX-License-Identifier: GPL-3.0-or-later

package bind

import (
	"fmt"
	"net/http"

	"github.com/netdata/netdata/go/plugins/pkg/web"
)

type xml3Stats struct {
	Server xml3Server `xml:"server"`
	Views  []xml3View `xml:"views>view"`
}

type xml3Server struct {
	CounterGroups []xml3CounterGroup `xml:"counters"`
}

type xml3CounterGroup struct {
	Type     string `xml:"type,attr"`
	Counters []struct {
		Name  string `xml:"name,attr"`
		Value int64  `xml:",chardata"`
	} `xml:"counter"`
}

type xml3View struct {
	Name          string             `xml:"name,attr"`
	CounterGroups []xml3CounterGroup `xml:"counters"`
}

func newXML3Client(client *http.Client, request web.RequestConfig) *xml3Client {
	return &xml3Client{httpClient: client, request: request}
}

type xml3Client struct {
	httpClient *http.Client
	request    web.RequestConfig
}

func (c xml3Client) serverStats() (*serverStats, error) {
	req, err := web.NewHTTPRequestWithPath(c.request, "/server")
	if err != nil {
		return nil, fmt.Errorf("failed to create HTTP request: %v", err)
	}

	var stats xml3Stats

	if err := web.DoHTTP(c.httpClient).RequestXML(req, &stats); err != nil {
		return nil, err
	}

	return convertXML(stats), nil
}

func convertXML(xmlStats xml3Stats) *serverStats {
	stats := serverStats{
		OpCodes:   make(map[string]int64),
		NSStats:   make(map[string]int64),
		QTypes:    make(map[string]int64),
		SockStats: make(map[string]int64),
		Views:     make(map[string]jsonView),
	}

	var m map[string]int64

	for _, group := range xmlStats.Server.CounterGroups {
		switch group.Type {
		default:
			continue
		case "opcode":
			m = stats.OpCodes
		case "qtype":
			m = stats.QTypes
		case "nsstat":
			m = stats.NSStats
		case "sockstat":
			m = stats.SockStats
		}

		for _, v := range group.Counters {
			m[v.Name] = v.Value
		}
	}

	for _, view := range xmlStats.Views {
		stats.Views[view.Name] = jsonView{
			Resolver: jsonViewResolver{
				Stats:      make(map[string]int64),
				QTypes:     make(map[string]int64),
				CacheStats: make(map[string]int64),
			},
		}
		for _, viewGroup := range view.CounterGroups {
			switch viewGroup.Type {
			default:
				continue
			case "resqtype":
				m = stats.Views[view.Name].Resolver.QTypes
			case "resstats":
				m = stats.Views[view.Name].Resolver.Stats
			case "cachestats":
				m = stats.Views[view.Name].Resolver.CacheStats
			}
			for _, viewCounter := range viewGroup.Counters {
				m[viewCounter.Name] = viewCounter.Value
			}
		}
	}
	return &stats
}
