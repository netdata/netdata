#ifndef JOURNAL_FILE_METRIC_HASH_TABLE_H
#define JOURNAL_FILE_METRIC_HASH_TABLE_H

#ifdef __cplusplus
extern "C" {
#endif

#include "rrddiskprotocol.h"

// target a 0.8 load factor
static inline size_t jf_metric_hash_table_capacity(size_t items) {
    return (items * 100.0) / 80;
}

typedef struct journal_metric_list journal_metric_t;

typedef struct jf_metric_hash_table {
    journal_metric_t *address;
    uint32_t length;
    uint32_t capacity;
} jf_metric_hash_table_t;

static inline jf_metric_hash_table_t jf_metric_hash_table(const struct journal_v2_header *header) {
    journal_metric_t *address = (struct journal_metric_list *)((uint8_t *)header + header->metric_offset);
    uint32_t length = header->metric_count;
    uint32_t capacity = jf_metric_hash_table_capacity(length);

    jf_metric_hash_table_t ht = {
        address,
        length,
        capacity
    };
    return ht;
}

void jf_metric_hash_table_init(jf_metric_hash_table_t *ht);

// Insert a metric into a memory region organized as a linear-probe hash table.
// Empty slots are identified by checking the metric's page offset to zero.
journal_metric_t *jf_metric_hash_table_insert(jf_metric_hash_table_t *ht, const unsigned char *metric_uuid, uint32_t *chain_length);

// Look up a metric by uuid in a memory region organized as a linear-probe hash table.
journal_metric_t* jf_metric_hash_table_lookup(jf_metric_hash_table_t *ht, const unsigned char *metric_uuid);

// Get the next occupied metric slot. Retursn the first occupied slot when page_metrics is NULL.
journal_metric_t* jf_metric_hash_table_next(jf_metric_hash_table_t *ht, journal_metric_t *prev_metric);

#ifdef __cplusplus
}
#endif

#endif /* JOURNAL_FILE_METRIC_HASH_TABLE_H */
