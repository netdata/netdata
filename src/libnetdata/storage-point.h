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
} STORAGE_POINT;

#define storage_point_unset(x)                     do { \
    (x).min = (x).max = (x).sum = NAN;                  \
    (x).count = 0;                                      \
    (x).anomaly_count = 0;                              \
    (x).flags = SN_FLAG_NONE;                           \
    (x).start_time_s = 0;                               \
    (x).end_time_s = 0;                                 \
    } while(0)

#define storage_point_empty(x, start_s, end_s)     do { \
    (x).min = (x).max = (x).sum = NAN;                  \
    (x).count = 1;                                      \
    (x).anomaly_count = 0;                              \
    (x).flags = SN_FLAG_NONE;                           \
    (x).start_time_s = start_s;                         \
    (x).end_time_s = end_s;                             \
    } while(0)

#define STORAGE_POINT_UNSET (STORAGE_POINT){ .min = NAN, .max = NAN, .sum = NAN, .count = 0, .anomaly_count = 0, .flags = SN_FLAG_NONE, .start_time_s = 0, .end_time_s = 0 }

#define storage_point_is_unset(x) (!(x).count)
#define storage_point_is_gap(x) (!netdata_double_isnumber((x).sum))
#define storage_point_is_zero(x) (!(x).count || (netdata_double_is_zero((x).min) && netdata_double_is_zero((x).max) && netdata_double_is_zero((x).sum) && (x).anomaly_count == 0))

#define storage_point_merge_to(dst, src) do {           \
        if(storage_point_is_unset(dst))                 \
            (dst) = (src);                              \
                                                        \
        else if(!storage_point_is_unset(src) &&         \
                !storage_point_is_gap(src)) {           \
                                                        \
            if((src).start_time_s < (dst).start_time_s) \
                (dst).start_time_s = (src).start_time_s;\
                                                        \
            if((src).end_time_s > (dst).end_time_s)     \
                (dst).end_time_s = (src).end_time_s;    \
                                                        \
            if((src).min < (dst).min)                   \
                (dst).min = (src).min;                  \
                                                        \
            if((src).max > (dst).max)                   \
                (dst).max = (src).max;                  \
                                                        \
            (dst).sum += (src).sum;                     \
                                                        \
            (dst).count += (src).count;                 \
            (dst).anomaly_count += (src).anomaly_count; \
                                                        \
            (dst).flags |= (src).flags & SN_FLAG_RESET; \
        }                                               \
} while(0)

#define storage_point_add_to(dst, src) do {             \
        if(storage_point_is_unset(dst))                 \
            (dst) = (src);                              \
                                                        \
        else if(!storage_point_is_unset(src) &&         \
                !storage_point_is_gap(src)) {           \
                                                        \
            if((src).start_time_s < (dst).start_time_s) \
                (dst).start_time_s = (src).start_time_s;\
                                                        \
            if((src).end_time_s > (dst).end_time_s)     \
                (dst).end_time_s = (src).end_time_s;    \
                                                        \
            (dst).min += (src).min;                     \
            (dst).max += (src).max;                     \
            (dst).sum += (src).sum;                     \
                                                        \
            (dst).count += (src).count;                 \
            (dst).anomaly_count += (src).anomaly_count; \
                                                        \
            (dst).flags |= (src).flags & SN_FLAG_RESET; \
        }                                               \
} while(0)

#define storage_point_make_positive(sp) do {            \
        if(!storage_point_is_unset(sp) &&               \
           !storage_point_is_gap(sp)) {                 \
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
    (NETDATA_DOUBLE)(storage_point_is_unset(sp) ? 0.0 : (NETDATA_DOUBLE)((sp).anomaly_count) * 100.0 / (NETDATA_DOUBLE)((sp).count))

#define storage_point_average_value(sp) \
    ((sp).count ? (sp).sum / (NETDATA_DOUBLE)((sp).count) : 0.0)


#endif //NETDATA_STORAGE_POINT_H
