// SPDX-License-Identifier: GPL-3.0-or-later

#ifndef NETDATA_STORAGE_POINT_H
#define NETDATA_STORAGE_POINT_H

#include "storage_number/storage_number.h"

typedef struct storage_point {
    NETDATA_DOUBLE min;     // when count > 1, this is the minimum among them
    NETDATA_DOUBLE max;     // when count > 1, this is the maximum among them
    NETDATA_DOUBLE sum;     // the point sum - divided by count gives the average

    // end_time - start_time = point duration
    time_t start_time_s;    // the time the point starts
    time_t end_time_s;      // the time the point ends

    uint32_t count;         // the number of original points aggregated
    uint32_t anomaly_count; // the number of original points found anomalous

    SN_FLAGS flags;         // flags stored with the point
    uint32_t gap_count;     // the number of missing original points
} STORAGE_POINT;

#define storage_point_unset(x)                     do { \
    (x).min = (x).max = (x).sum = NAN;                  \
    (x).count = 0;                                      \
    (x).anomaly_count = 0;                              \
    (x).flags = SN_FLAG_NONE;                           \
    (x).gap_count = 0;                                  \
    (x).start_time_s = 0;                               \
    (x).end_time_s = 0;                                 \
    } while(0)

#define storage_point_empty(x, start_s, end_s)     do { \
    (x).min = (x).max = (x).sum = NAN;                  \
    (x).count = 0;                                      \
    (x).anomaly_count = 0;                              \
    (x).flags = SN_FLAG_NONE;                           \
    (x).gap_count = 1;                                  \
    (x).start_time_s = start_s;                         \
    (x).end_time_s = end_s;                             \
    } while(0)

#define storage_point_empty_slots(x, start_s, end_s, slots) do { \
    uint32_t _storage_point_slots = (uint32_t)(slots);         \
    storage_point_empty(x, start_s, end_s);                    \
    (x).gap_count = _storage_point_slots ?                     \
        _storage_point_slots : 1;                              \
} while(0)

#define STORAGE_POINT_UNSET (STORAGE_POINT){ .min = NAN, .max = NAN, .sum = NAN, .start_time_s = 0, .end_time_s = 0, .count = 0, .anomaly_count = 0, .flags = SN_FLAG_NONE, .gap_count = 0 }

#define storage_point_has_value(x) ((x).count != 0)
#define storage_point_is_unset(x) (!(x).count && !(x).gap_count)
#define storage_point_is_gap(x) (!(x).count && (x).gap_count)
#define storage_point_is_complete(x) ((x).count && !(x).gap_count)
#define storage_point_is_partial(x) ((x).count && (x).gap_count)
#define storage_point_slots(x) ((uint64_t)(x).count + (uint64_t)(x).gap_count)
#define storage_point_is_zero(x) (!storage_point_has_value(x) || \
    (netdata_double_is_zero((x).min) && netdata_double_is_zero((x).max) && \
     netdata_double_is_zero((x).sum) && (x).anomaly_count == 0))

#define storage_point_merge_values(dst, src) do {       \
        if((src).min < (dst).min)                       \
            (dst).min = (src).min;                      \
        if((src).max > (dst).max)                       \
            (dst).max = (src).max;                      \
} while(0)

#define storage_point_add_values(dst, src) do {         \
        (dst).min += (src).min;                         \
        (dst).max += (src).max;                         \
} while(0)

#define storage_point_accumulate_to(dst, src, values) do { \
        if(storage_point_is_unset(dst))                 \
            (dst) = (src);                              \
                                                        \
        else if(!storage_point_is_unset(src)) {         \
                                                        \
            if((src).start_time_s < (dst).start_time_s) \
                (dst).start_time_s = (src).start_time_s;\
                                                        \
            if((src).end_time_s > (dst).end_time_s)     \
                (dst).end_time_s = (src).end_time_s;    \
                                                        \
            if(storage_point_has_value(src)) {          \
                if(storage_point_has_value(dst)) {      \
                    values(dst, src);                   \
                    (dst).sum += (src).sum;             \
                    (dst).count += (src).count;         \
                    (dst).anomaly_count +=              \
                        (src).anomaly_count;             \
                }                                       \
                else {                                  \
                    (dst).min = (src).min;              \
                    (dst).max = (src).max;              \
                    (dst).sum = (src).sum;              \
                    (dst).count = (src).count;          \
                    (dst).anomaly_count =               \
                        (src).anomaly_count;             \
                }                                       \
                (dst).flags |=                          \
                    (src).flags & SN_FLAG_RESET;         \
            }                                           \
            (dst).gap_count += (src).gap_count;         \
        }                                               \
} while(0)

#define storage_point_merge_to(dst, src) \
    storage_point_accumulate_to(dst, src, storage_point_merge_values)

#define storage_point_add_to(dst, src) \
    storage_point_accumulate_to(dst, src, storage_point_add_values)

#define storage_point_make_positive(sp) do {            \
        if(storage_point_has_value(sp)) {               \
                                                        \
            if(unlikely(signbit((sp).sum)))             \
                (sp).sum = -(sp).sum;                   \
                                                        \
            if(unlikely(signbit((sp).min)))             \
                (sp).min = -(sp).min;                   \
                                                        \
            if(unlikely(signbit((sp).max)))             \
                (sp).max = -(sp).max;                   \
                                                        \
            if(unlikely((sp).min > (sp).max)) {         \
                NETDATA_DOUBLE t = (sp).min;            \
                (sp).min = (sp).max;                    \
                (sp).max = t;                           \
            }                                           \
        }                                               \
} while(0)

#define storage_point_anomaly_rate(sp) \
    (NETDATA_DOUBLE)(storage_point_has_value(sp) ? (NETDATA_DOUBLE)((sp).anomaly_count) * 100.0 / (NETDATA_DOUBLE)((sp).count) : 0.0)

#define storage_point_average_value(sp) \
    (storage_point_is_gap(sp) ? NAN : \
     ((sp).count ? (sp).sum / (NETDATA_DOUBLE)((sp).count) : 0.0))


#endif //NETDATA_STORAGE_POINT_H
