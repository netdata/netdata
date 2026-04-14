// SPDX-License-Identifier: GPL-3.0-or-later

#include "query-internal.h"

static int compare_contributions(const void *a, const void *b) {
    const struct { size_t dim_idx; NETDATA_DOUBLE contribution; } *da = a;
    const struct { size_t dim_idx; NETDATA_DOUBLE contribution; } *db = b;

    if (da->contribution > db->contribution) return -1;
    if (da->contribution < db->contribution) return 1;
    return 0;
}

RRDR *rrd2rrdr_cardinality_limit(RRDR *r) {
    QUERY_TARGET *qt = r->internal.qt;
    
    if(!qt || qt->request.cardinality_limit == 0 || r->d <= qt->request.cardinality_limit)
        return r;
        
    ONEWAYALLOC *owa = r->internal.owa;
    
    // Calculate contribution of each dimension using dview statistics (sum of values)
    NETDATA_DOUBLE *contributions = onewayalloc_mallocz(owa, r->d * sizeof(NETDATA_DOUBLE));
    
    // Count queried dimensions and get their contributions from dview
    size_t queried_count = 0;
    for (size_t d = 0; d < r->d; d++) {
        contributions[d] = 0.0;
        
        if (!(r->od[d] & RRDR_DIMENSION_QUERIED))
            continue;
            
        queried_count++;
        
        // Use the sum from dview if available, otherwise fall back to manual calculation
        if(r->dview && !isnan(r->dview[d].sum)) {
            contributions[d] = fabsndd(r->dview[d].sum);
        } else {
            // Fallback: calculate manually from values
            for(size_t i = 0; i < r->rows; i++) {
                size_t idx = i * r->d + d;
                
                if(r->o[idx] & RRDR_VALUE_EMPTY)
                    continue;
                    
                NETDATA_DOUBLE value = r->v[idx];
                if(!isnan(value))
                    contributions[d] += fabsndd(value);
            }
        }
    }
    
    // If we don't need to reduce, return original
    if(queried_count <= qt->request.cardinality_limit) {
        onewayalloc_freez(owa, contributions);
        return r;
    }
    
    // Create array of dimension indices sorted by contribution (descending)
    struct {
        size_t dim_idx;
        NETDATA_DOUBLE contribution;
    } *sorted_dims = onewayalloc_mallocz(owa, queried_count * sizeof(*sorted_dims));
    
    size_t sorted_idx = 0;
    for (size_t d = 0; d < r->d; d++) {
        if (r->od[d] & RRDR_DIMENSION_QUERIED) {
            sorted_dims[sorted_idx].dim_idx = d;
            sorted_dims[sorted_idx].contribution = contributions[d];
            sorted_idx++;
        }
    }
    
    // Sort by contribution (descending)
    qsort(sorted_dims, queried_count, sizeof(*sorted_dims), compare_contributions);
    
    // Create new RRDR with limited dimensions
    size_t new_d = qt->request.cardinality_limit;
    size_t remaining_count = queried_count - (qt->request.cardinality_limit - 1);
    if(remaining_count > 0)
        new_d = qt->request.cardinality_limit; // Keep one slot for "remaining N dimensions"
    else
        new_d = queried_count; // No remaining dimensions needed
        
    RRDR *new_r = rrdr_create(owa, qt, new_d, r->n);
    if (!new_r) {
        internal_error(true, "QUERY: cannot create cardinality limited RRDR");
        onewayalloc_freez(owa, contributions);
        onewayalloc_freez(owa, sorted_dims);
        return r;
    }
    
    // Copy basic metadata from original RRDR
    new_r->view = r->view;
    new_r->time_grouping = r->time_grouping;
    new_r->partial_data_trimming = r->partial_data_trimming;
    new_r->rows = r->rows;
    
    // Copy timestamps
    memcpy(new_r->t, r->t, r->n * sizeof(time_t));
    
    // Setup arrays for new RRDR
    if(new_r->d) {
        new_r->dp = onewayalloc_callocz(owa, new_r->d, sizeof(*new_r->dp));
        new_r->dview = onewayalloc_callocz(owa, new_r->d, sizeof(*new_r->dview));
        
        if(new_r->n) {
            // Initialize all values as empty
            for (size_t i = 0; i < new_r->n; i++) {
                for (size_t d = 0; d < new_r->d; d++) {
                    size_t idx = i * new_r->d + d;
                    new_r->v[idx] = NAN;
                    new_r->ar[idx] = 0.0;
                    new_r->o[idx] = RRDR_VALUE_EMPTY;
                }
            }
        }
    }
    
    // Copy top dimensions
    size_t kept_dimensions = (remaining_count > 0) ? qt->request.cardinality_limit - 1 : queried_count;
    
    for (size_t i = 0; i < kept_dimensions; i++) {
        size_t src_d = sorted_dims[i].dim_idx;
        
        // Copy metadata
        new_r->di[i] = string_dup(r->di[src_d]);
        new_r->dn[i] = string_dup(r->dn[src_d]);
        new_r->od[i] = r->od[src_d];
        new_r->du[i] = string_dup(r->du[src_d]);
        new_r->dp[i] = r->dp[src_d];
        
        // Copy data
        for (size_t row = 0; row < r->rows; row++) {
            size_t src_idx = row * r->d + src_d;
            size_t dst_idx = row * new_r->d + i;
            
            new_r->v[dst_idx] = r->v[src_idx];
            new_r->ar[dst_idx] = r->ar[src_idx];
            new_r->o[dst_idx] = r->o[src_idx];
        }
        
        // Copy dview stats
        if(r->dview)
            new_r->dview[i] = r->dview[src_d];
    }
    
    // Create "remaining N dimensions" if needed
    if (remaining_count > 0) {
        size_t remaining_idx = kept_dimensions;
        
        char remaining_name[256];
        snprintfz(remaining_name, sizeof(remaining_name), "remaining %zu dimension%s", 
                 remaining_count, remaining_count == 1 ? "" : "s");
        
        new_r->di[remaining_idx] = string_strdupz(remaining_name);
        new_r->dn[remaining_idx] = string_strdupz(remaining_name);
        new_r->od[remaining_idx] = RRDR_DIMENSION_QUERIED | RRDR_DIMENSION_NONZERO;
        
        // Use the units from the first remaining dimension
        if(kept_dimensions < queried_count) {
            size_t first_remaining_d = sorted_dims[kept_dimensions].dim_idx;
            new_r->du[remaining_idx] = string_dup(r->du[first_remaining_d]);
            new_r->dp[remaining_idx] = r->dp[first_remaining_d];
        }
        
        // Aggregate remaining dimensions
        NETDATA_DOUBLE sum = 0.0, min = NAN, max = NAN, ars = 0.0;
        size_t count = 0;
        
        for (size_t row = 0; row < r->rows; row++) {
            size_t dst_idx = row * new_r->d + remaining_idx;
            NETDATA_DOUBLE aggregated_value = 0.0;
            NETDATA_DOUBLE aggregated_ar = 0.0;
            RRDR_VALUE_FLAGS aggregated_flags = RRDR_VALUE_NOTHING;
            bool has_values = false;
            
            for (size_t i = kept_dimensions; i < queried_count; i++) {
                size_t src_d = sorted_dims[i].dim_idx;
                size_t src_idx = row * r->d + src_d;
                
                if(!(r->o[src_idx] & RRDR_VALUE_EMPTY)) {
                    NETDATA_DOUBLE value = r->v[src_idx];
                    if(!isnan(value)) {
                        aggregated_value += value;
                        aggregated_ar += r->ar[src_idx];
                        aggregated_flags |= (r->o[src_idx] & (RRDR_VALUE_RESET | RRDR_VALUE_PARTIAL));
                        has_values = true;
                    }
                }
            }
            
            if(has_values) {
                new_r->v[dst_idx] = aggregated_value;
                new_r->ar[dst_idx] = aggregated_ar;
                new_r->o[dst_idx] = aggregated_flags & ~RRDR_VALUE_EMPTY;
                
                // Update statistics for dview
                sum += aggregated_value;
                ars += aggregated_ar;
                if(count == 0) {
                    min = max = aggregated_value;
                } else {
                    if(aggregated_value < min) min = aggregated_value;
                    if(aggregated_value > max) max = aggregated_value;
                }
                count++;
            } else {
                new_r->v[dst_idx] = NAN;
                new_r->ar[dst_idx] = 0.0;
                new_r->o[dst_idx] = RRDR_VALUE_EMPTY;
            }
        }
        
        // Set dview for remaining dimension
        if(new_r->dview) {
            new_r->dview[remaining_idx] = (STORAGE_POINT) {
                .sum = sum,
                .count = count,
                .min = min,
                .max = max,
                .anomaly_count = (size_t)(ars * RRDR_DVIEW_ANOMALY_COUNT_MULTIPLIER / 100.0),
            };
        }
    }
    
    // Cleanup
    onewayalloc_freez(owa, contributions);
    onewayalloc_freez(owa, sorted_dims);
    
    // Free the original RRDR 
    rrdr_free(owa, r);
    
    return new_r;
}
