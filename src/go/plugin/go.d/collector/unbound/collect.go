// SPDX-License-Identifier: GPL-3.0-or-later

package unbound

import (
	"fmt"
	"strconv"
	"strings"
)

// https://github.com/NLnetLabs/unbound/blob/master/daemon/remote.c (do_stats: print_stats, print_thread_stats, print_mem, print_uptime, print_ext)
// https://github.com/NLnetLabs/unbound/blob/master/libunbound/unbound.h (structs: ub_server_stats, ub_shm_stat_info)
// https://docs.datadoghq.com/integrations/unbound/#metrics (stats description)
// https://docs.menandmice.com/display/MM/Unbound+request-list+demystified (request lists explanation)

func (c *Collector) collect() (map[string]int64, error) {
	stats, err := c.scrapeUnboundStats()
	if err != nil {
		return nil, err
	}

	mx := c.collectStats(stats)
	c.updateCharts()
	return mx, nil
}

func (c *Collector) scrapeUnboundStats() ([]entry, error) {
	var output []string
	var command = "UBCT1 stats"
	if c.Cumulative {
		command = "UBCT1 stats_noreset"
	}

	if err := c.client.Connect(); err != nil {
		return nil, fmt.Errorf("failed to connect: %v", err)
	}
	defer func() { _ = c.client.Disconnect() }()

	err := c.client.Command(command+"\n", func(bytes []byte) (bool, error) {
		output = append(output, string(bytes))
		return true, nil
	})
	if err != nil {
		return nil, fmt.Errorf("send command '%s': %w", command, err)
	}

	switch len(output) {
	case 0:
		return nil, fmt.Errorf("command '%s': empty resopnse", command)
	case 1:
		// 	in case of error the first line of the response is: error <descriptive text possible> \n
		return nil, fmt.Errorf("command '%s': '%s'", command, output[0])
	}
	return parseStatsOutput(output)
}

func (c *Collector) collectStats(stats []entry) map[string]int64 {
	if c.Cumulative {
		return c.collectCumulativeStats(stats)
	}
	return c.collectNonCumulativeStats(stats)
}

func (c *Collector) collectCumulativeStats(stats []entry) map[string]int64 {
	mul := float64(1000)
	// following stats change only on cachemiss event in cumulative mode
	// - *.requestlist.avg,
	// - *.recursion.time.avg
	// - *.recursion.time.median
	v := findEntry("total.num.cachemiss", stats)
	if v == c.prevCacheMiss {
		// so we need to reset them if there is no such event
		mul = 0
	}
	c.prevCacheMiss = v
	return c.processStats(stats, mul)
}

func (c *Collector) collectNonCumulativeStats(stats []entry) map[string]int64 {
	mul := float64(1000)
	mx := c.processStats(stats, mul)

	// see 'static int print_ext(RES* ssl, struct ub_stats_info* s)' in
	// https://github.com/NLnetLabs/unbound/blob/master/daemon/remote.c
	// - zero value queries type not included
	// - zero value queries class not included
	// - zero value queries opcode not included
	// - only 0-6 rcodes answers always included, other zero value rcodes not included
	for k := range c.cache.queryType {
		if _, ok := c.curCache.queryType[k]; !ok {
			mx["num.query.type."+k] = 0
		}
	}
	for k := range c.cache.queryClass {
		if _, ok := c.curCache.queryClass[k]; !ok {
			mx["num.query.class."+k] = 0
		}
	}
	for k := range c.cache.queryOpCode {
		if _, ok := c.curCache.queryOpCode[k]; !ok {
			mx["num.query.opcode."+k] = 0
		}
	}
	for k := range c.cache.answerRCode {
		if _, ok := c.curCache.answerRCode[k]; !ok {
			mx["num.answer.rcode."+k] = 0
		}
	}
	return mx
}

func (c *Collector) processStats(stats []entry, mul float64) map[string]int64 {
	c.curCache.clear()
	mx := make(map[string]int64, len(stats))
	for _, e := range stats {
		switch {
		// 	*.requestlist.avg, *.recursion.time.avg, *.recursion.time.median
		case e.hasSuffix(".avg"), e.hasSuffix(".median"):
			e.value *= mul
		case e.hasPrefix("thread") && e.hasSuffix("num.queries"):
			v := extractThread(e.key)
			c.curCache.threads[v] = true
		case e.hasPrefix("num.query.type"):
			v := extractQueryType(e.key)
			c.curCache.queryType[v] = true
		case e.hasPrefix("num.query.class"):
			v := extractQueryClass(e.key)
			c.curCache.queryClass[v] = true
		case e.hasPrefix("num.query.opcode"):
			v := extractQueryOpCode(e.key)
			c.curCache.queryOpCode[v] = true
		case e.hasPrefix("num.answer.rcode"):
			v := extractAnswerRCode(e.key)
			c.curCache.answerRCode[v] = true
		}
		mx[e.key] = int64(e.value)
	}
	return mx
}

func extractThread(key string) string      { idx := strings.IndexByte(key, '.'); return key[:idx] }
func extractQueryType(key string) string   { i := len("num.query.type."); return key[i:] }
func extractQueryClass(key string) string  { i := len("num.query.class."); return key[i:] }
func extractQueryOpCode(key string) string { i := len("num.query.opcode."); return key[i:] }
func extractAnswerRCode(key string) string { i := len("num.answer.rcode."); return key[i:] }

type entry struct {
	key   string
	value float64
}

func (e entry) hasPrefix(prefix string) bool { return strings.HasPrefix(e.key, prefix) }
func (e entry) hasSuffix(suffix string) bool { return strings.HasSuffix(e.key, suffix) }

func findEntry(key string, entries []entry) float64 {
	for _, e := range entries {
		if e.key == key {
			return e.value
		}
	}
	return -1
}

func parseStatsOutput(output []string) ([]entry, error) {
	var es []entry
	for _, v := range output {
		e, err := parseStatsLine(v)
		if err != nil {
			return nil, err
		}
		if e.hasPrefix("histogram") {
			continue
		}
		es = append(es, e)
	}
	return es, nil
}

func parseStatsLine(line string) (entry, error) {
	// 'stats' output is a list of [key]=[value] lines.
	parts := strings.Split(line, "=")
	if len(parts) != 2 {
		return entry{}, fmt.Errorf("bad line syntax: %s", line)
	}
	f, err := strconv.ParseFloat(parts[1], 64)
	return entry{key: parts[0], value: f}, err
}

func newCollectCache() collectCache {
	return collectCache{
		threads:     make(map[string]bool),
		queryType:   make(map[string]bool),
		queryClass:  make(map[string]bool),
		queryOpCode: make(map[string]bool),
		answerRCode: make(map[string]bool),
	}
}

type collectCache struct {
	threads     map[string]bool
	queryType   map[string]bool
	queryClass  map[string]bool
	queryOpCode map[string]bool
	answerRCode map[string]bool
}

func (c *collectCache) clear() {
	*c = newCollectCache()
}
