// SPDX-License-Identifier: GPL-3.0-or-later

#ifndef COLLECTORS_UTILS_H
#define COLLECTORS_UTILS_H

#include "database/rrd.h"

static inline RRDSET *rrdset_find_active_localhost(const char *id)
{
    RRDSET *st = rrdset_find_by_id(rrdb.localhost, id);
    if (unlikely(st && rrdset_flag_check(st, RRDSET_FLAG_ARCHIVED)))
        return NULL;
    return st;
}

static inline RRDSET *rrdset_find_active_bytype_localhost(const char *type, const char *id)
{
    RRDSET *st = rrdset_find_by_type(rrdb.localhost, type, id);
    if (unlikely(st && rrdset_flag_check(st, RRDSET_FLAG_ARCHIVED)))
        return NULL;
    return st;
}


static inline RRDSET *rrdset_find_active_byname_localhost(const char *name)
{
    RRDSET *st = rrdset_find_by_name(rrdb.localhost, name);
    if (unlikely(st && rrdset_flag_check(st, RRDSET_FLAG_ARCHIVED)))
        return NULL;
    return st;
}

static inline RRDDIM *rrddim_find_active(RRDSET *st, const char *id) {
    RRDDIM *rd = rrddim_find_by_id(st, id);

    if (unlikely(rd && rrddim_flag_check(rd, RRDDIM_FLAG_ARCHIVED)))
        return NULL;

    return rd;
}

#endif /* COLLECTORS_UTILS_H */
