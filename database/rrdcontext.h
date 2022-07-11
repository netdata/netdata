// SPDX-License-Identifier: GPL-3.0-or-later

#ifndef NETDATA_RRDCONTEXT_H
#define NETDATA_RRDCONTEXT_H 1

// ----------------------------------------------------------------------------
// RRDMETRIC

typedef struct rrdmetric_acquired RRDMETRIC_ACQUIRED;


// ----------------------------------------------------------------------------
// RRDINSTANCE

typedef struct rrdinstances_dictionary RRDINSTANCES;
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

extern void rrdhost_create_rrdinstances(RRDHOST *host);
extern void rrdhost_destroy_rrdinstances(RRDHOST *host);

extern void rrdhost_create_rrdcontexts(RRDHOST *host);
extern void rrdhost_destroy_rrdcontexts(RRDHOST *host);


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

#endif // NETDATA_RRDCONTEXT_H

