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

extern void rrdhost_load_rrdcontext_data(RRDHOST *host);
extern void rrdhost_create_rrdcontexts(RRDHOST *host);
extern void rrdhost_destroy_rrdcontexts(RRDHOST *host);

extern void rrdcontext_host_child_connected(RRDHOST *host);
extern void rrdcontext_host_child_disconnected(RRDHOST *host);

typedef enum {
    RRDCONTEXT_OPTION_NONE               = 0,
    RRDCONTEXT_OPTION_SHOW_METRICS       = (1 << 0),
    RRDCONTEXT_OPTION_SHOW_INSTANCES     = (1 << 1),
    RRDCONTEXT_OPTION_SHOW_LABELS        = (1 << 2),
    RRDCONTEXT_OPTION_SHOW_QUEUED        = (1 << 3),
    RRDCONTEXT_OPTION_SHOW_FLAGS         = (1 << 4),
    RRDCONTEXT_OPTION_SHOW_DELETED       = (1 << 5),
    RRDCONTEXT_OPTION_DEEPSCAN           = (1 << 6),
    RRDCONTEXT_OPTION_SKIP_ID            = (1 << 31), // internal use
} RRDCONTEXT_TO_JSON_OPTIONS;

#define RRDCONTEXT_OPTIONS_ALL (RRDCONTEXT_OPTION_SHOW_METRICS|RRDCONTEXT_OPTION_SHOW_INSTANCES|RRDCONTEXT_OPTION_SHOW_LABELS|RRDCONTEXT_OPTION_SHOW_QUEUED|RRDCONTEXT_OPTION_SHOW_FLAGS|RRDCONTEXT_OPTION_SHOW_DELETED)

extern int rrdcontext_to_json(RRDHOST *host, BUFFER *wb, RRDCONTEXT_TO_JSON_OPTIONS options, const char *context);
extern int rrdcontexts_to_json(RRDHOST *host, BUFFER *wb, RRDCONTEXT_TO_JSON_OPTIONS options);

// ----------------------------------------------------------------------------
// public API for rrddims

extern void rrdcontext_updated_rrddim(RRDDIM *rd);
extern void rrdcontext_removed_rrddim(RRDDIM *rd);
extern void rrdcontext_updated_rrddim_algorithm(RRDDIM *rd);
extern void rrdcontext_updated_rrddim_multiplier(RRDDIM *rd);
extern void rrdcontext_updated_rrddim_divisor(RRDDIM *rd);
extern void rrdcontext_updated_rrddim_flags(RRDDIM *rd);
extern void rrdcontext_collected_rrddim(RRDDIM *rd);

// ----------------------------------------------------------------------------
// public API for rrdsets

extern void rrdcontext_updated_rrdset(RRDSET *st);
extern void rrdcontext_removed_rrdset(RRDSET *st);
extern void rrdcontext_updated_rrdset_name(RRDSET *st);
extern void rrdcontext_updated_rrdset_flags(RRDSET *st);
extern void rrdcontext_collected_rrdset(RRDSET *st);

// ----------------------------------------------------------------------------
// public API for ACLK

extern void rrdcontext_hub_checkpoint_command(void *cmd);
extern void rrdcontext_hub_stop_streaming_command(void *cmd);


// ----------------------------------------------------------------------------
// public API for threads

extern int rrdcontext_enabled;

extern void rrdcontext_db_rotation(void);
extern void *rrdcontext_main(void *);

#endif // NETDATA_RRDCONTEXT_H

