<!--
custom_edit_url: "https://github.com/netdata/netdata/edit/master/src/libnetdata/clocks/README.md"
title: "Clocks"
sidebar_label: "Clocks"
learn_status: "Published"
learn_topic_type: "References"
learn_rel_path: "Developers/libnetdata"
-->

# Clocks

The libnetdata clock utilities provide consistent wall-clock timestamps, monotonic elapsed-time measurements, periodic
scheduling, sleeps, and overflow-safe time arithmetic for Netdata C code. Include `libnetdata/libnetdata.h` to use these
APIs; it exports the public clock headers with the rest of libnetdata.

## Choose the clock for the job

| Clock | Public entry points | Use it for |
|---|---|---|
| Realtime | `now_realtime_sec()`, `now_realtime_msec()`, `now_realtime_usec()`, `now_realtime_timeval()` | Wall-clock timestamps that must correspond to system time. The value can jump when the system clock changes. |
| Monotonic | `now_monotonic_sec()`, `now_monotonic_usec()`, `now_monotonic_timeval()` | Elapsed time, deadlines, and intervals that must not follow wall-clock changes. |
| High-precision monotonic | `now_monotonic_high_precision_sec()`, `now_monotonic_high_precision_usec()`, `now_monotonic_high_precision_timeval()` | Short measurements that should use the operating system's `CLOCK_MONOTONIC` source directly. |
| Boottime | `now_boottime_sec()`, `now_boottime_usec()`, `now_boottime_timeval()` | Uptime-style intervals that include suspended time where the operating system supports it. |

The platform initialization selects supported clock sources and fallbacks. Call the named helpers instead of the lower-level
`now_sec()`, `now_usec()`, and `now_timeval()` functions.

## Measure intervals

The [`clocks.h`](https://github.com/netdata/netdata/blob/master/src/libnetdata/clocks/clocks.h) interface defines nanosecond,
microsecond, and millisecond types and conversion constants. It also
provides these interval helpers:

- `clocks_usec_delta_or_zero()` returns an ordered unsigned delta and clamps a backward sample to zero.
- `clocks_usec_delta_or_zero_with_rebase()` also moves a valid baseline backward when the clock sample moves backward.
- `timeval_usec()` and `timeval_msec()` convert a `struct timeval` to one scalar value.
- `dt_usec()` returns the absolute difference between two `struct timeval` values; `dt_usec_signed()` preserves the direction.

The named `now_*_timeval()` functions return `0` on success and `-1` on error. The scalar `now_*_sec()` and
`now_*_usec()` functions return `0` on error.

## Schedule periodic work and sleep

Initialize a `heartbeat_t` with `heartbeat_init()`, then call `heartbeat_next()` in a worker loop. The call waits for the next
aligned tick and returns the realtime microseconds since the previous tick; its first call returns zero. Use
`heartbeat_statistics()` to read aggregate alignment statistics.

Use `sleep_usec()` for a relative microsecond sleep. Its implementation handles interruptions and uses monotonic elapsed time
to keep retries within the requested sleep budget.

## Use safe `time_t` arithmetic

[`time_t_arithmetic.h`](https://github.com/netdata/netdata/blob/master/src/libnetdata/clocks/time_t_arithmetic.h) provides
inline helpers for arithmetic at the limits of signed `time_t`:

- `nd_time_t_add_saturating()` clamps an unrepresentable sum to the nearest `time_t` limit.
- `nd_time_t_add_compare()` compares a mathematical sum with a `time_t` without first narrowing the sum.
- `nd_time_t_elapsed_saturating()` clamps backward elapsed time to zero and overflow to the maximum `time_t`.
- `nd_duration_to_uint32_saturating()` clamps a signed duration to the range of `uint32_t`.

The clock and time-arithmetic edge cases are covered by
[`clocks-unittest.c`](https://github.com/netdata/netdata/blob/master/src/libnetdata/clocks/clocks-unittest.c).
