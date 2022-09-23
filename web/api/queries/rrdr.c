// SPDX-License-Identifier: GPL-3.0-or-later

#include "rrdr.h"

/*
static void rrdr_dump(RRDR *r)
{
    long c, i;
    RRDDIM *d;

    fprintf(stderr, "\nCHART %s (%s)\n", r->st->id, r->st->name);

    for(c = 0, d = r->st->dimensions; d ;c++, d = d->next) {
        fprintf(stderr, "DIMENSION %s (%s), %s%s%s%s\n"
                , d->id
                , d->name
                , (r->od[c] & RRDR_EMPTY)?"EMPTY ":""
                , (r->od[c] & RRDR_RESET)?"RESET ":""
                , (r->od[c] & RRDR_DIMENSION_HIDDEN)?"HIDDEN ":""
                , (r->od[c] & RRDR_DIMENSION_NONZERO)?"NONZERO ":""
                );
    }

    if(r->rows <= 0) {
        fprintf(stderr, "RRDR does not have any values in it.\n");
        return;
    }

    fprintf(stderr, "RRDR includes %d values in it:\n", r->rows);

    // for each line in the array
    for(i = 0; i < r->rows ;i++) {
        NETDATA_DOUBLE *cn = &r->v[ i * r->d ];
        RRDR_DIMENSION_FLAGS *co = &r->o[ i * r->d ];

        // print the id and the timestamp of the line
        fprintf(stderr, "%ld %ld ", i + 1, r->t[i]);

        // for each dimension
        for(c = 0, d = r->st->dimensions; d ;c++, d = d->next) {
            if(unlikely(r->od[c] & RRDR_DIMENSION_HIDDEN)) continue;
            if(unlikely(!(r->od[c] & RRDR_DIMENSION_NONZERO))) continue;

            if(co[c] & RRDR_EMPTY)
                fprintf(stderr, "null ");
            else
                fprintf(stderr, NETDATA_DOUBLE_FORMAT " %s%s%s%s "
                    , cn[c]
                    , (co[c] & RRDR_EMPTY)?"E":" "
                    , (co[c] & RRDR_RESET)?"R":" "
                    , (co[c] & RRDR_DIMENSION_HIDDEN)?"H":" "
                    , (co[c] & RRDR_DIMENSION_NONZERO)?"N":" "
                    );
        }

        fprintf(stderr, "\n");
    }
}
*/

inline void rrdr_free(ONEWAYALLOC *owa, RRDR *r) {
    if(unlikely(!r)) return;

    if(likely(r->st_locked_by_rrdr_create))
        rrdset_unlock(r->st);

    onewayalloc_freez(owa, r->t);
    onewayalloc_freez(owa, r->v);
    onewayalloc_freez(owa, r->o);
    onewayalloc_freez(owa, r->od);
    onewayalloc_freez(owa, r->ar);
    onewayalloc_freez(owa, r);
}

RRDR *rrdr_create_for_x_dimensions(ONEWAYALLOC *owa, int dimensions, long points) {
    RRDR *r = onewayalloc_callocz(owa, 1, sizeof(RRDR));
    r->internal.owa = owa;

    r->d = dimensions;
    r->n = points;

    r->t = onewayalloc_callocz(owa, points, sizeof(time_t));
    r->v = onewayalloc_mallocz(owa, points * dimensions * sizeof(NETDATA_DOUBLE));
    r->o = onewayalloc_mallocz(owa, points * dimensions * sizeof(RRDR_VALUE_FLAGS));
    r->ar = onewayalloc_mallocz(owa, points * dimensions * sizeof(NETDATA_DOUBLE));
    r->od = onewayalloc_mallocz(owa, dimensions * sizeof(RRDR_DIMENSION_FLAGS));

    r->group = 1;
    r->update_every = 1;

    return r;
}

RRDR *rrdr_create(ONEWAYALLOC *owa, struct rrdset *st, long n, struct context_param *context_param_list) {
    if (unlikely(!st)) return NULL;

    bool st_locked_by_rrdr_create = false;
    if (!context_param_list || !(context_param_list->flags & CONTEXT_FLAGS_ARCHIVE)) {
        rrdset_rdlock(st);
        st_locked_by_rrdr_create = true;
    }

    // count the number of dimensions
    int dimensions = 0;
    RRDDIM *temp_rd =  context_param_list ? context_param_list->rd : NULL;
    RRDDIM *rd;
    if (temp_rd) {
        RRDDIM *t = temp_rd;
        while (t) {
            dimensions++;
            t = t->next;
        }
    } else
        dimensions = rrdset_number_of_dimensions(st);

    // create the rrdr
    RRDR *r = rrdr_create_for_x_dimensions(owa, dimensions, n);
    r->st = st;
    r->st_locked_by_rrdr_create = st_locked_by_rrdr_create;

    // set the hidden flag on hidden dimensions
    int c;
    for (c = 0, rd = temp_rd ? temp_rd : st->dimensions; rd; c++, rd = rd->next) {
        if (unlikely(rrddim_option_check(rd, RRDDIM_OPTION_HIDDEN)))
            r->od[c] = RRDR_DIMENSION_HIDDEN;
        else
            r->od[c] = RRDR_DIMENSION_DEFAULT;
    }

    return r;
}
