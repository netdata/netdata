// SPDX-License-Identifier: GPL-3.0-or-later

#ifndef NETDATA_RRDCONTEXT_H
#define NETDATA_RRDCONTEXT_H 1

// ----------------------------------------------------------------------------
// RRDMETRIC

typedef struct rrdmetric_acquired RRDMETRIC_ACQUIRED;


// ----------------------------------------------------------------------------
// RRDINSTANCE

typedef struct rrdinstance_acquired RRDINSTANCE_ACQUIRED;

// ----------------------------------------------------------------------------
// RRDCONTEXT

typedef struct rrdcontexts_dictionary RRDCONTEXTS;
typedef struct rrdcontext_acquired RRDCONTEXT_ACQUIRED;

// ----------------------------------------------------------------------------

#include "rrd.h"

// ----------------------------------------------------------------------------
// public API for rrdhost

void rrdhost_load_rrdcontext_data(RRDHOST *host);
void rrdhost_create_rrdcontexts(RRDHOST *host);
void rrdhost_destroy_rrdcontexts(RRDHOST *host);

void rrdcontext_host_child_connected(RRDHOST *host);
void rrdcontext_host_child_disconnected(RRDHOST *host);

int rrdcontext_foreach_instance_with_rrdset_in_context(RRDHOST *host, const char *context, int (*callback)(RRDSET *st, void *data), void *data);

typedef enum {
    RRDCONTEXT_OPTION_NONE               = 0,
    RRDCONTEXT_OPTION_SHOW_METRICS       = (1 << 0),
    RRDCONTEXT_OPTION_SHOW_INSTANCES     = (1 << 1),
    RRDCONTEXT_OPTION_SHOW_LABELS        = (1 << 2),
    RRDCONTEXT_OPTION_SHOW_QUEUED        = (1 << 3),
    RRDCONTEXT_OPTION_SHOW_FLAGS         = (1 << 4),
    RRDCONTEXT_OPTION_SHOW_DELETED       = (1 << 5),
    RRDCONTEXT_OPTION_DEEPSCAN           = (1 << 6),
    RRDCONTEXT_OPTION_SHOW_UUIDS         = (1 << 7),
    RRDCONTEXT_OPTION_SHOW_HIDDEN        = (1 << 8),
    RRDCONTEXT_OPTION_SKIP_ID            = (1 << 31), // internal use
} RRDCONTEXT_TO_JSON_OPTIONS;

#define RRDCONTEXT_OPTIONS_ALL (RRDCONTEXT_OPTION_SHOW_METRICS|RRDCONTEXT_OPTION_SHOW_INSTANCES|RRDCONTEXT_OPTION_SHOW_LABELS|RRDCONTEXT_OPTION_SHOW_QUEUED|RRDCONTEXT_OPTION_SHOW_FLAGS|RRDCONTEXT_OPTION_SHOW_DELETED|RRDCONTEXT_OPTION_SHOW_UUIDS|RRDCONTEXT_OPTION_SHOW_HIDDEN)

int rrdcontext_to_json(RRDHOST *host, BUFFER *wb, time_t after, time_t before, RRDCONTEXT_TO_JSON_OPTIONS options, const char *context, SIMPLE_PATTERN *chart_label_key, SIMPLE_PATTERN *chart_labels_filter, SIMPLE_PATTERN *chart_dimensions);
int rrdcontexts_to_json(RRDHOST *host, BUFFER *wb, time_t after, time_t before, RRDCONTEXT_TO_JSON_OPTIONS options, SIMPLE_PATTERN *chart_label_key, SIMPLE_PATTERN *chart_labels_filter, SIMPLE_PATTERN *chart_dimensions);

// ----------------------------------------------------------------------------
// public API for rrddims

void rrdcontext_updated_rrddim(RRDDIM *rd);
void rrdcontext_removed_rrddim(RRDDIM *rd);
void rrdcontext_updated_rrddim_algorithm(RRDDIM *rd);
void rrdcontext_updated_rrddim_multiplier(RRDDIM *rd);
void rrdcontext_updated_rrddim_divisor(RRDDIM *rd);
void rrdcontext_updated_rrddim_flags(RRDDIM *rd);
void rrdcontext_collected_rrddim(RRDDIM *rd);

// ----------------------------------------------------------------------------
// public API for rrdsets

void rrdcontext_updated_rrdset(RRDSET *st);
void rrdcontext_removed_rrdset(RRDSET *st);
void rrdcontext_updated_rrdset_name(RRDSET *st);
void rrdcontext_updated_rrdset_flags(RRDSET *st);
void rrdcontext_collected_rrdset(RRDSET *st);

// ----------------------------------------------------------------------------
// public API for ACLK

void rrdcontext_hub_checkpoint_command(void *cmd);
void rrdcontext_hub_stop_streaming_command(void *cmd);


// ----------------------------------------------------------------------------
// public API for threads

void rrdcontext_db_rotation(void);
void *rrdcontext_main(void *);

#endif // NETDATA_RRDCONTEXT_H

