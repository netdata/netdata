// SPDX-License-Identifier: GPL-3.0-or-later

package main

import (
	"container/heap"
	"fmt"
	"net/netip"
	"sort"
)

type asnValue struct {
	asn uint32
	org string
}

type geoValue struct {
	country string
	state   string
	city    string
	lat     float64
	lon     float64
	hasLoc  bool
}

type intervalPriority struct {
	sourceIndex int
	rowIndex    int
}

type intervalRecord[T comparable] struct {
	start    netip.Addr
	end      netip.Addr
	value    T
	priority intervalPriority
}

type intervalEvent struct {
	addr netip.Addr
	kind intervalEventKind
	id   int
}

type intervalEventKind uint8

const (
	intervalEventRemove intervalEventKind = iota
	intervalEventAdd
)

type intervalHeap[T comparable] struct {
	ids     []int
	records []intervalRecord[T]
}

func (h intervalHeap[T]) Len() int { return len(h.ids) }

func (h intervalHeap[T]) Less(i, j int) bool {
	return betterPriority(
		h.records[h.ids[i]].priority,
		h.records[h.ids[j]].priority,
	)
}

func (h intervalHeap[T]) Swap(i, j int) {
	h.ids[i], h.ids[j] = h.ids[j], h.ids[i]
}

func (h *intervalHeap[T]) Push(x any) {
	h.ids = append(h.ids, x.(int))
}

func (h *intervalHeap[T]) Pop() any {
	last := len(h.ids) - 1
	id := h.ids[last]
	h.ids = h.ids[:last]
	return id
}

func betterPriority(a, b intervalPriority) bool {
	if a.sourceIndex != b.sourceIndex {
		return a.sourceIndex < b.sourceIndex
	}
	return a.rowIndex > b.rowIndex
}

func mergeAsnSources(sources [][]asnRange) ([]asnRange, error) {
	records := make([]intervalRecord[asnValue], 0)
	for sourceIndex, ranges := range sources {
		for rowIndex, rec := range ranges {
			if err := rec.validate(); err != nil {
				return nil, fmt.Errorf("asn source %d row %d: %w", sourceIndex, rowIndex, err)
			}
			records = append(records, intervalRecord[asnValue]{
				start: rec.start,
				end:   rec.end,
				value: asnValue{
					asn: rec.asn,
					org: rec.org,
				},
				priority: intervalPriority{
					sourceIndex: sourceIndex,
					rowIndex:    rowIndex,
				},
			})
		}
	}

	merged, err := mergeIntervals(records)
	if err != nil {
		return nil, err
	}

	out := make([]asnRange, 0, len(merged))
	for _, rec := range merged {
		out = append(out, asnRange{
			start: rec.start,
			end:   rec.end,
			asn:   rec.value.asn,
			org:   rec.value.org,
		})
	}
	return out, nil
}

func mergeGeoSources(sources [][]geoRange) ([]geoRange, error) {
	records := make([]intervalRecord[geoValue], 0)
	for sourceIndex, ranges := range sources {
		for rowIndex, rec := range ranges {
			if err := rec.validate(); err != nil {
				return nil, fmt.Errorf("geo source %d row %d: %w", sourceIndex, rowIndex, err)
			}
			records = append(records, intervalRecord[geoValue]{
				start: rec.start,
				end:   rec.end,
				value: geoValue{
					country: rec.country,
					state:   rec.state,
					city:    rec.city,
					lat:     rec.latitude,
					lon:     rec.longitude,
					hasLoc:  rec.hasLocation,
				},
				priority: intervalPriority{
					sourceIndex: sourceIndex,
					rowIndex:    rowIndex,
				},
			})
		}
	}

	merged, err := mergeIntervals(records)
	if err != nil {
		return nil, err
	}

	out := make([]geoRange, 0, len(merged))
	for _, rec := range merged {
		out = append(out, geoRange{
			start:       rec.start,
			end:         rec.end,
			country:     rec.value.country,
			state:       rec.value.state,
			city:        rec.value.city,
			latitude:    rec.value.lat,
			longitude:   rec.value.lon,
			hasLocation: rec.value.hasLoc,
		})
	}
	return out, nil
}

func mergeIntervals[T comparable](records []intervalRecord[T]) ([]intervalRecord[T], error) {
	var v4 []intervalRecord[T]
	var v6 []intervalRecord[T]

	for _, rec := range records {
		if !rec.start.IsValid() || !rec.end.IsValid() {
			return nil, fmt.Errorf("invalid interval %v-%v", rec.start, rec.end)
		}
		if rec.start.BitLen() != rec.end.BitLen() {
			return nil, fmt.Errorf("mixed address family range: %s-%s", rec.start, rec.end)
		}
		switch rec.start.BitLen() {
		case 32:
			v4 = append(v4, rec)
		case 128:
			v6 = append(v6, rec)
		default:
			return nil, fmt.Errorf("unsupported address family bitlen %d", rec.start.BitLen())
		}
	}

	out := make([]intervalRecord[T], 0, len(records))
	familyOut, err := mergeIntervalsForBitLen(v4, 32)
	if err != nil {
		return nil, err
	}
	out = append(out, familyOut...)

	familyOut, err = mergeIntervalsForBitLen(v6, 128)
	if err != nil {
		return nil, err
	}
	out = append(out, familyOut...)

	return out, nil
}

func mergeIntervalsForBitLen[T comparable](
	records []intervalRecord[T],
	bitLen int,
) ([]intervalRecord[T], error) {
	if len(records) == 0 {
		return nil, nil
	}

	events := make([]intervalEvent, 0, len(records)*2)
	for id, rec := range records {
		events = append(events, intervalEvent{
			addr: rec.start,
			kind: intervalEventAdd,
			id:   id,
		})
		if next := rec.end.Next(); next.IsValid() {
			events = append(events, intervalEvent{
				addr: next,
				kind: intervalEventRemove,
				id:   id,
			})
		}
	}

	sort.Slice(events, func(i, j int) bool {
		if records[events[i].id].start.BitLen() != records[events[j].id].start.BitLen() {
			return records[events[i].id].start.BitLen() < records[events[j].id].start.BitLen()
		}
		if cmp := compareAddrs(events[i].addr, events[j].addr); cmp != 0 {
			return cmp < 0
		}
		return events[i].kind < events[j].kind
	})

	active := make(map[int]struct{}, len(records))
	candidates := &intervalHeap[T]{records: records}
	heap.Init(candidates)

	maxAddr := maxAddrForBitLen(bitLen)
	out := make([]intervalRecord[T], 0, len(records))

	for i := 0; i < len(events); {
		addr := events[i].addr
		for i < len(events) && compareAddrs(events[i].addr, addr) == 0 {
			switch events[i].kind {
			case intervalEventRemove:
				delete(active, events[i].id)
			case intervalEventAdd:
				active[events[i].id] = struct{}{}
				heap.Push(candidates, events[i].id)
			}
			i++
		}

		bestID, ok := topActiveRecord(candidates, active)
		if !ok {
			continue
		}

		end := maxAddr
		if i < len(events) {
			end = events[i].addr.Prev()
		}
		if compareAddrs(addr, end) > 0 {
			continue
		}

		best := records[bestID]
		next := intervalRecord[T]{
			start:    addr,
			end:      end,
			value:    best.value,
			priority: best.priority,
		}
		if len(out) > 0 &&
			out[len(out)-1].value == next.value &&
			out[len(out)-1].end.Next() == next.start {
			out[len(out)-1].end = next.end
			continue
		}
		out = append(out, next)
	}

	return out, nil
}

func topActiveRecord[T comparable](
	candidates *intervalHeap[T],
	active map[int]struct{},
) (int, bool) {
	for candidates.Len() > 0 {
		id := candidates.ids[0]
		if _, ok := active[id]; ok {
			return id, true
		}
		heap.Pop(candidates)
	}
	return 0, false
}

func maxAddrForBitLen(bitLen int) netip.Addr {
	switch bitLen {
	case 32:
		return netip.AddrFrom4([4]byte{0xff, 0xff, 0xff, 0xff})
	case 128:
		return netip.AddrFrom16([16]byte{
			0xff, 0xff, 0xff, 0xff,
			0xff, 0xff, 0xff, 0xff,
			0xff, 0xff, 0xff, 0xff,
			0xff, 0xff, 0xff, 0xff,
		})
	default:
		return netip.Addr{}
	}
}
