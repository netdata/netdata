// SPDX-License-Identifier: GPL-3.0-or-later

#ifndef NETDATA_RRDLABELS_AGGREGATED_H
#define NETDATA_RRDLABELS_AGGREGATED_H

#include "libnetdata/libnetdata.h"
#include "rrdlabels.h"

// Aggregated labels structure for collecting unique keys and their values
// across multiple RRDLABELS instances
struct rrdlabels_aggregated;
typedef struct rrdlabels_aggregated RRDLABELS_AGGREGATED;

// Create a new aggregated labels structure
RRDLABELS_AGGREGATED *rrdlabels_aggregated_create(void);

// Destroy aggregated labels structure and free all memory
void rrdlabels_aggregated_destroy(RRDLABELS_AGGREGATED *agg);

// Add all labels from an RRDLABELS instance to the aggregated structure
void rrdlabels_aggregated_add_from_rrdlabels(RRDLABELS_AGGREGATED *agg, RRDLABELS *labels);

// Add a single label key-value pair to the aggregated structure
void rrdlabels_aggregated_add_label(RRDLABELS_AGGREGATED *agg, const char *key, const char *value);

// Output aggregated labels as JSON object with keys and their value arrays
void rrdlabels_aggregated_to_buffer_json(RRDLABELS_AGGREGATED *agg, BUFFER *wb, const char *key, size_t cardinality_limit);

// Merge all labels from source aggregated structure into destination
void rrdlabels_aggregated_merge(RRDLABELS_AGGREGATED *dst, RRDLABELS_AGGREGATED *src);

#endif /* NETDATA_RRDLABELS_AGGREGATED_H */