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

    query_target_release(r->internal.qt);
    onewayalloc_freez(owa, r->t);
    onewayalloc_freez(owa, r->v);
    onewayalloc_freez(owa, r->o);
    onewayalloc_freez(owa, r->od);
    onewayalloc_freez(owa, r->ar);
    onewayalloc_freez(owa, r);
}

RRDR *rrdr_create(ONEWAYALLOC *owa, QUERY_TARGET *qt) {
    if(unlikely(!qt || !qt->query.used || !qt->window.points))
        return NULL;

    size_t dimensions = qt->query.used;
    size_t points = qt->window.points;

    // create the rrdr
    RRDR *r = onewayalloc_callocz(owa, 1, sizeof(RRDR));
    r->internal.owa = owa;
    r->internal.qt = qt;

    r->before = qt->window.before;
    r->after = qt->window.after;
    r->internal.points_wanted = qt->window.points;
    r->d = (int)dimensions;
    r->n = (int)points;

    r->t = onewayalloc_callocz(owa, points, sizeof(time_t));
    r->v = onewayalloc_mallocz(owa, points * dimensions * sizeof(NETDATA_DOUBLE));
    r->o = onewayalloc_mallocz(owa, points * dimensions * sizeof(RRDR_VALUE_FLAGS));
    r->ar = onewayalloc_mallocz(owa, points * dimensions * sizeof(NETDATA_DOUBLE));
    r->od = onewayalloc_mallocz(owa, dimensions * sizeof(RRDR_DIMENSION_FLAGS));

    r->group = 1;
    r->update_every = 1;

    return r;
}
