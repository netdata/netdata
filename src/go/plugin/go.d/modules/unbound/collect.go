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

func (u *Unbound) collect() (map[string]int64, error) {
	stats, err := u.scrapeUnboundStats()
	if err != nil {
		return nil, err
	}

	mx := u.collectStats(stats)
	u.updateCharts()
	return mx, nil
}

func (u *Unbound) scrapeUnboundStats() ([]entry, error) {
	var output []string
	var command = "UBCT1 stats"
	if u.Cumulative {
		command = "UBCT1 stats_noreset"
	}

	if err := u.client.Connect(); err != nil {
		return nil, fmt.Errorf("failed to connect: %v", err)
	}
	defer func() { _ = u.client.Disconnect() }()

	err := u.client.Command(command+"\n", func(bytes []byte) bool {
		output = append(output, string(bytes))
		return true
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

func (u *Unbound) collectStats(stats []entry) map[string]int64 {
	if u.Cumulative {
		return u.collectCumulativeStats(stats)
	}
	return u.collectNonCumulativeStats(stats)
}

func (u *Unbound) collectCumulativeStats(stats []entry) map[string]int64 {
	mul := float64(1000)
	// following stats change only on cachemiss event in cumulative mode
	// - *.requestlist.avg,
	// - *.recursion.time.avg
	// - *.recursion.time.median
	v := findEntry("total.num.cachemiss", stats)
	if v == u.prevCacheMiss {
		// so we need to reset them if there is no such event
		mul = 0
	}
	u.prevCacheMiss = v
	return u.processStats(stats, mul)
}

func (u *Unbound) collectNonCumulativeStats(stats []entry) map[string]int64 {
	mul := float64(1000)
	mx := u.processStats(stats, mul)

	// see 'static int print_ext(RES* ssl, struct ub_stats_info* s)' in
	// https://github.com/NLnetLabs/unbound/blob/master/daemon/remote.c
	// - zero value queries type not included
	// - zero value queries class not included
	// - zero value queries opcode not included
	// - only 0-6 rcodes answers always included, other zero value rcodes not included
	for k := range u.cache.queryType {
		if _, ok := u.curCache.queryType[k]; !ok {
			mx["num.query.type."+k] = 0
		}
	}
	for k := range u.cache.queryClass {
		if _, ok := u.curCache.queryClass[k]; !ok {
			mx["num.query.class."+k] = 0
		}
	}
	for k := range u.cache.queryOpCode {
		if _, ok := u.curCache.queryOpCode[k]; !ok {
			mx["num.query.opcode."+k] = 0
		}
	}
	for k := range u.cache.answerRCode {
		if _, ok := u.curCache.answerRCode[k]; !ok {
			mx["num.answer.rcode."+k] = 0
		}
	}
	return mx
}

func (u *Unbound) processStats(stats []entry, mul float64) map[string]int64 {
	u.curCache.clear()
	mx := make(map[string]int64, len(stats))
	for _, e := range stats {
		switch {
		// 	*.requestlist.avg, *.recursion.time.avg, *.recursion.time.median
		case e.hasSuffix(".avg"), e.hasSuffix(".median"):
			e.value *= mul
		case e.hasPrefix("thread") && e.hasSuffix("num.queries"):
			v := extractThread(e.key)
			u.curCache.threads[v] = true
		case e.hasPrefix("num.query.type"):
			v := extractQueryType(e.key)
			u.curCache.queryType[v] = true
		case e.hasPrefix("num.query.class"):
			v := extractQueryClass(e.key)
			u.curCache.queryClass[v] = true
		case e.hasPrefix("num.query.opcode"):
			v := extractQueryOpCode(e.key)
			u.curCache.queryOpCode[v] = true
		case e.hasPrefix("num.answer.rcode"):
			v := extractAnswerRCode(e.key)
			u.curCache.answerRCode[v] = true
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
