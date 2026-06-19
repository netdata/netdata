// SPDX-License-Identifier: GPL-3.0-or-later

#ifndef NETDATA_SINGLE_WRITER_H
#define NETDATA_SINGLE_WRITER_H

// Single-writer atomic counters.
//
// For counters that have EXACTLY ONE writer thread and are read by other
// threads (single-writer / multiple-readers). This is the natural shape of
// most per-host, per-chart and per-connection counters in Netdata.
//
// Why not __atomic_fetch_add(): an atomic read-modify-write is *indivisible*,
// so even at __ATOMIC_RELAXED it forces a lock-prefixed / cache-line-exclusive
// operation (x86 `lock xadd`; ARM ldxr/stxr or ldadd). RELAXED removes the
// ordering fence, NOT the atomicity cost. That cost only exists to defend
// against OTHER writers - which, by definition, a single-writer counter has
// none of.
//
// So the writer does a PLAIN read of its own value + add + a RELAXED store:
//  - the writer reading its own counter cannot race (no other writer), so the
//    read needs no atomic - and a plain read lets the compiler keep the running
//    total in a register across increments;
//  - only the STORE carries a cross-thread contract (readers must not see a torn
//    value and the store must not be optimized away), so it is a relaxed store.
// On x86-64/aarch64 this is a plain load + add + plain store: no lock, no fence.
//
// Readers in OTHER threads MUST read with single_writer_atomic_read() (a relaxed
// atomic load): for them there IS a concurrent writer.
//
// MUST NOT be used when more than one thread writes the same counter. For
// genuinely multi-writer counters use __atomic_fetch_add(). On 32-bit platforms
// a 64-bit relaxed store is not a single instruction; this is the same trade-off
// Netdata's other 64-bit atomics already make.

// add n to a single-writer counter (writer thread only)
#define single_writer_atomic_add(ptr, n) do {                           \
        __typeof__(ptr) _swptr = (ptr);                                 \
        __atomic_store_n(_swptr, *_swptr + (n), __ATOMIC_RELAXED);      \
    } while (0)

// set a single-writer counter to v (writer thread only)
#define single_writer_atomic_set(ptr, v) __atomic_store_n((ptr), (v), __ATOMIC_RELAXED)

// read a single-writer counter (any thread - readers MUST use this)
#define single_writer_atomic_read(ptr) __atomic_load_n((ptr), __ATOMIC_RELAXED)

#endif //NETDATA_SINGLE_WRITER_H
