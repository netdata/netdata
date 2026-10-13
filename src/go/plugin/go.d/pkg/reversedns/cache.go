// SPDX-License-Identifier: GPL-3.0-or-later

package reversedns

import (
	"net/netip"
	"time"
)

type cacheSegment uint8

const (
	segmentProbationary cacheSegment = iota
	segmentProtected
)

type cacheEntry struct {
	addr      netip.Addr
	result    Result
	expiresAt time.Time
	segment   cacheSegment

	recencyPrev *cacheEntry
	recencyNext *cacheEntry
	expiryPrev  *cacheEntry
	expiryNext  *cacheEntry
}

type recencyList struct {
	front *cacheEntry
	back  *cacheEntry
	len   int
}

func (l *recencyList) pushFront(entry *cacheEntry) {
	entry.recencyPrev = nil
	entry.recencyNext = l.front
	if l.front != nil {
		l.front.recencyPrev = entry
	} else {
		l.back = entry
	}
	l.front = entry
	l.len++
}

func (l *recencyList) remove(entry *cacheEntry) {
	if entry.recencyPrev != nil {
		entry.recencyPrev.recencyNext = entry.recencyNext
	} else {
		l.front = entry.recencyNext
	}
	if entry.recencyNext != nil {
		entry.recencyNext.recencyPrev = entry.recencyPrev
	} else {
		l.back = entry.recencyPrev
	}
	entry.recencyPrev = nil
	entry.recencyNext = nil
	l.len--
}

func (l *recencyList) moveToFront(entry *cacheEntry) {
	if l.front == entry {
		return
	}
	l.remove(entry)
	l.pushFront(entry)
}

type expiryList struct {
	front *cacheEntry
	back  *cacheEntry
}

func (l *expiryList) pushBack(entry *cacheEntry) {
	entry.expiryPrev = l.back
	entry.expiryNext = nil
	if l.back != nil {
		l.back.expiryNext = entry
	} else {
		l.front = entry
	}
	l.back = entry
}

func (l *expiryList) remove(entry *cacheEntry) {
	if entry.expiryPrev != nil {
		entry.expiryPrev.expiryNext = entry.expiryNext
	} else {
		l.front = entry.expiryNext
	}
	if entry.expiryNext != nil {
		entry.expiryNext.expiryPrev = entry.expiryPrev
	} else {
		l.back = entry.expiryPrev
	}
	entry.expiryPrev = nil
	entry.expiryNext = nil
}

func (r *Resolver) lookupCacheLocked(addr netip.Addr, now time.Time) (Result, bool) {
	r.purgeExpiredLocked(now)
	entry, ok := r.cache[addr]
	if !ok {
		return Result{}, false
	}
	if !now.Before(entry.expiresAt) {
		r.removeCacheEntryLocked(entry)
		return Result{}, false
	}
	r.touchCacheEntryLocked(entry)
	return entry.result, true
}

func (r *Resolver) insertCacheLocked(addr netip.Addr, result Result, now time.Time) {
	if entry := r.cache[addr]; entry != nil {
		r.removeCacheEntryLocked(entry)
	}

	ttl := r.negativeTTL
	if result.State == StatePositive {
		ttl = r.positiveTTL
	}
	entry := &cacheEntry{
		addr:      addr,
		result:    result,
		expiresAt: now.Add(ttl),
		segment:   segmentProbationary,
	}
	r.cache[addr] = entry
	r.probationary.pushFront(entry)
	if result.State == StatePositive {
		r.positiveExpiry.pushBack(entry)
	} else {
		r.negativeExpiry.pushBack(entry)
	}

	for len(r.cache) > r.maxEntries {
		entry := r.probationary.back
		if entry == nil {
			entry = r.protected.back
		}
		if entry == nil {
			break
		}
		r.removeCacheEntryLocked(entry)
	}
}

func (r *Resolver) touchCacheEntryLocked(entry *cacheEntry) {
	if entry.result.State == StateNegative {
		r.probationary.moveToFront(entry)
		return
	}
	if entry.segment == segmentProtected {
		r.protected.moveToFront(entry)
		return
	}
	if r.protectedLimit == 0 {
		r.probationary.moveToFront(entry)
		return
	}

	r.probationary.remove(entry)
	entry.segment = segmentProtected
	r.protected.pushFront(entry)
	if r.protected.len > r.protectedLimit {
		demoted := r.protected.back
		r.protected.remove(demoted)
		demoted.segment = segmentProbationary
		r.probationary.pushFront(demoted)
	}
}

func (r *Resolver) purgeExpiredLocked(now time.Time) {
	r.purgeExpiryListLocked(&r.positiveExpiry, now)
	r.purgeExpiryListLocked(&r.negativeExpiry, now)
}

func (r *Resolver) purgeExpiryListLocked(expiry *expiryList, now time.Time) {
	for expiry.front != nil && !now.Before(expiry.front.expiresAt) {
		r.removeCacheEntryLocked(expiry.front)
	}
}

func (r *Resolver) removeCacheEntryLocked(entry *cacheEntry) {
	if entry.segment == segmentProtected {
		r.protected.remove(entry)
	} else {
		r.probationary.remove(entry)
	}
	if entry.result.State == StatePositive {
		r.positiveExpiry.remove(entry)
	} else {
		r.negativeExpiry.remove(entry)
	}
	delete(r.cache, entry.addr)
}
