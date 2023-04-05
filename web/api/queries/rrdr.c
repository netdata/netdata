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

    for(size_t d = 0; d < r->d ;d++) {
        string_freez(r->di[d]);
        string_freez(r->dn[d]);
        string_freez(r->du[d]);
    }

    query_target_release(r->internal.release_with_rrdr_qt);

    onewayalloc_freez(owa, r->t);
    onewayalloc_freez(owa, r->v);
    onewayalloc_freez(owa, r->vh);
    onewayalloc_freez(owa, r->o);
    onewayalloc_freez(owa, r->od);
    onewayalloc_freez(owa, r->di);
    onewayalloc_freez(owa, r->dn);
    onewayalloc_freez(owa, r->du);
    onewayalloc_freez(owa, r->dp);
    onewayalloc_freez(owa, r->dview);
    onewayalloc_freez(owa, r->dqp);
    onewayalloc_freez(owa, r->ar);
    onewayalloc_freez(owa, r->gbc);
    onewayalloc_freez(owa, r->dgbc);
    onewayalloc_freez(owa, r->dgbs);

    if(r->dl) {
        for(size_t d = 0; d < r->d ;d++)
            dictionary_destroy(r->dl[d]);

        onewayalloc_freez(owa, r->dl);
    }

    dictionary_destroy(r->label_keys);

    if(r->group_by.r) {
        // prevent accidental infinite recursion
        r->group_by.r->group_by.r = NULL;

        // do not release qt twice
        r->group_by.r->internal.qt = NULL;

        rrdr_free(owa, r->group_by.r);
    }

    onewayalloc_freez(owa, r);
}

RRDR *rrdr_create(ONEWAYALLOC *owa, QUERY_TARGET *qt, size_t dimensions, size_t points) {
    if(unlikely(!qt))
        return NULL;

    // create the rrdr
    RRDR *r = onewayalloc_callocz(owa, 1, sizeof(RRDR));
    r->internal.owa = owa;
    r->internal.qt = qt;

    r->view.before = qt->window.before;
    r->view.after = qt->window.after;
    r->time_grouping.points_wanted = points;
    r->d = (int)dimensions;
    r->n = (int)points;

    if(points && dimensions) {
        r->v = onewayalloc_mallocz(owa, points * dimensions * sizeof(NETDATA_DOUBLE));
        r->o = onewayalloc_mallocz(owa, points * dimensions * sizeof(RRDR_VALUE_FLAGS));
        r->ar = onewayalloc_mallocz(owa, points * dimensions * sizeof(NETDATA_DOUBLE));
    }

    if(points) {
        r->t = onewayalloc_callocz(owa, points, sizeof(time_t));
    }

    if(dimensions) {
        r->od = onewayalloc_mallocz(owa, dimensions * sizeof(RRDR_DIMENSION_FLAGS));
        r->di = onewayalloc_callocz(owa, dimensions, sizeof(STRING *));
        r->dn = onewayalloc_callocz(owa, dimensions, sizeof(STRING *));
        r->du = onewayalloc_callocz(owa, dimensions, sizeof(STRING *));
    }

    r->view.group = 1;
    r->view.update_every = 1;

    return r;
}
