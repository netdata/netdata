// SPDX-License-Identifier: GPL-3.0-or-later

#ifndef NETDATA_WINDOWS_PDH_H
#define NETDATA_WINDOWS_PDH_H

#include "libnetdata/libnetdata.h"

#if defined(OS_WINDOWS)

// Raw counter values through the Performance Data Helper (PDH).
//
// The registry view of Perflib (HKEY_PERFORMANCE_DATA) is a translation of manifest-based (V2) counter sets, and on
// some Windows builds it attaches values to the wrong counter definitions. PDH reads V2 providers through the V2
// consumer API, which is what Performance Monitor and Get-Counter show. Use this reader for objects whose registry
// view is known to be wrong; Perflib stays the default.
//
// A query is not thread-safe: use it from one thread at a time.

typedef struct nd_pdh_query ND_PDH_QUERY;

typedef enum {
    ND_PDH_READ_OK,      // one callback per instance was made
    ND_PDH_READ_NO_DATA, // the counter has no instances in this sample
    ND_PDH_READ_FAILED,  // the counter could not be read in this sample
} ND_PDH_READ_STATUS;

// instance is NULL when the instance name could not be decoded.
// raw is NULL when PDH has no valid value for this instance in this sample.
// raw follows the RAW_DATA conventions of Perflib, so the perflib chart helpers can use it.
// The struct tag of RAW_DATA is used because perflib.h may not be parsed yet when libnetdata.h includes this file.
struct _rawdata;
typedef void (*nd_pdh_instance_cb)(const char *instance, const struct _rawdata *raw, void *data);

// Adds "\object(*)\counter" by English name for every counter, so instances that appear later are read too.
// The counters array is referenced, not copied: it must outlive the query.
// Counters that cannot be added are reported by nd_pdh_query_has_counter().
// Returns NULL when the query cannot be opened or no counter could be added.
ND_PDH_QUERY *nd_pdh_query_create(const char *object, const char *const *counters, size_t counters_count);
void nd_pdh_query_destroy(ND_PDH_QUERY *q);

bool nd_pdh_query_has_counter(ND_PDH_QUERY *q, size_t counter);

// Takes one sample of every counter. Returns false when the sample failed.
bool nd_pdh_query_collect(ND_PDH_QUERY *q);

// Reports the latest sample of one counter, by its index in the array given to nd_pdh_query_create().
ND_PDH_READ_STATUS nd_pdh_query_read(ND_PDH_QUERY *q, size_t counter, nd_pdh_instance_cb cb, void *data);

#endif // OS_WINDOWS
#endif // NETDATA_WINDOWS_PDH_H
