#include <cstring>
#include <cstdint>

#include "journalfile_metric_hash_table.h"

// Helper class to represent the hash table
class JournalMetricHashTable {
private:
    journal_metric_t *table;
    size_t capacity;

public:
    JournalMetricHashTable(journal_metric_t *table, size_t capacity)
        : table(table), capacity(capacity) {}

    // hash index for uuids
    size_t hash(const unsigned char* uuid) const {
        uint32_t raw_index;
        memcpy(&raw_index, &uuid[0], sizeof(uint32_t));
        return raw_index % capacity;
    }

    // heck if a slot is empty
    bool is_empty(size_t index) const {
        return table[index].page_offset == 0;
    }

    bool uuid_match(size_t index, const unsigned char* uuid) const {
        return memcmp(table[index].uuid, uuid, 16) == 0;
    }

    journal_metric_t *insert(const unsigned char *metric_uuid, uint32_t *chain_length) {
        size_t index = hash(metric_uuid);
        size_t start_index = index;

        do {
            if (is_empty(index) || uuid_match(index, metric_uuid)) {
                // Found an empty slot or the same uuid
                journal_metric_t *metric = &(table[index]);
                return metric;
            }

            // move to next slot
            *chain_length += 1;
            index = (index + 1) % capacity;
        } while (index != start_index);

        // table is full or in a bad state
        fatal("Journal file v2 metrics index is full.");
    }

    journal_metric_t *lookup(const unsigned char* uuid) const {
        size_t index = hash(uuid);
        size_t start_index = index;

        do {
            if (is_empty(index)) {
                return nullptr;
            }

            if (uuid_match(index, uuid)) {
                return &table[index];
            }

            // move to next slot
            index = (index + 1) % capacity;
        } while (index != start_index);

        // searched the entire table without finding the item
        return nullptr;
    }

    journal_metric_t *next(const journal_metric_t *prev_metric) const {
        size_t start_index = 0;

        if (prev_metric)
            start_index = metric_index(prev_metric) + 1;

        for (size_t index = start_index; index != capacity; index++) {
            if (is_empty(index)) {
                continue;
            }

            return &table[index];
        }

        return nullptr;
    }

private:
    size_t metric_index(const journal_metric_t *metric) const {
        internal_fatal((metric < &table[0]) || (metric > &table[capacity]), "metric out of bounds");

        uintptr_t n = metric - &table[0];
        return n / sizeof(journal_metric_t);
    }
};

void jf_metric_hash_table_init(jf_metric_hash_table_t *ht) {
    memset(ht->address, 0, ht->capacity * sizeof(journal_metric_t));
}

journal_metric_t* jf_metric_hash_table_insert(jf_metric_hash_table_t *ht, const unsigned char *uuid, uint32_t *chain_length) {
    JournalMetricHashTable table(ht->address, ht->capacity);
    return table.insert(uuid, chain_length);
}

journal_metric_t* jf_metric_hash_table_lookup(jf_metric_hash_table_t *ht, const unsigned char* uuid) {
    JournalMetricHashTable table(ht->address, ht->capacity);
    return table.lookup(uuid);
}

journal_metric_t* jf_metric_hash_table_next(jf_metric_hash_table_t *ht, journal_metric_t *prev_metric) {
    JournalMetricHashTable table(ht->address, ht->capacity);
    return table.next(prev_metric);
}
