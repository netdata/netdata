// SPDX-License-Identifier: GPL-3.0-or-later

#include "rrdlabels-aggregated.h"

// Internal structure for aggregated labels
// Uses JudyL for efficiency: key = STRING* (with reference), value = Pvoid_t to values JudyL
struct rrdlabels_aggregated {
    Pvoid_t keys_judy;  // JudyL: key=STRING* of label key, value=Pvoid_t to values JudyL
};

// Create a new aggregated labels structure
RRDLABELS_AGGREGATED *rrdlabels_aggregated_create(void) {
    RRDLABELS_AGGREGATED *agg = callocz(1, sizeof(RRDLABELS_AGGREGATED));
    agg->keys_judy = (Pvoid_t) NULL;
    return agg;
}

// Destroy aggregated labels structure and free all memory
void rrdlabels_aggregated_destroy(RRDLABELS_AGGREGATED *agg) {
    if (!agg) return;

    // Free all nested JudyL arrays and their STRING references
    Pvoid_t *PValue;
    Word_t key_index = 0;
    bool first_then_next = true;

    while ((PValue = JudyLFirstThenNext(agg->keys_judy, &key_index, &first_then_next))) {
        STRING *key_string = (STRING *)key_index;
        Pvoid_t values_judy = *PValue;

        // Free all value STRING references in the nested JudyL
        Word_t value_index = 0;
        bool value_first_then_next = true;
        Pvoid_t *PValueInner;
        
        while ((PValueInner = JudyLFirstThenNext(values_judy, &value_index, &value_first_then_next))) {
            STRING *value_string = (STRING *)value_index;
            string_freez(value_string);
        }

        // Free the values JudyL
        JudyLFreeArray(&values_judy, PJE0);
        
        // Free the key STRING reference
        string_freez(key_string);
    }

    // Free the main JudyL
    JudyLFreeArray(&agg->keys_judy, PJE0);
    
    freez(agg);
}

// Callback function for adding labels
static int rrdlabels_aggregated_add_callback(const char *name, const char *value, RRDLABEL_SRC ls, void *data) {
    (void)ls; // unused

    struct { RRDLABELS_AGGREGATED *agg; } *callback_data = data;
    RRDLABELS_AGGREGATED *agg = callback_data->agg;

    // Create STRING references with dup for safety
    STRING *key_string = string_strdupz(name);
    STRING *value_string = string_strdupz(value);

    // Find or create the values JudyL for this key
    Pvoid_t *PValue = JudyLIns(&agg->keys_judy, (Word_t)key_string, PJE0);
    if (!PValue || PValue == PJERR) {
        string_freez(key_string);
        string_freez(value_string);
        return -1; // Error
    }

    Pvoid_t values_judy;
    if (!*PValue) {
        // New key - create new values JudyL
        values_judy = (Pvoid_t) NULL;
        *PValue = values_judy;
    } else {
        // Existing key - free the duplicate key string since we don't need it
        string_freez(key_string);
        values_judy = *PValue;
    }

    // Add the value to the values JudyL
    Pvoid_t *PValueInner = JudyLIns(&values_judy, (Word_t)value_string, PJE0);
    if (!PValueInner || PValueInner == PJERR) {
        string_freez(value_string);
        return -1; // Error
    }

    if (*PValueInner) {
        // Value already exists - free the duplicate
        string_freez(value_string);
    } else {
        // New value - store it
        *PValueInner = (Pvoid_t)1; // Just mark as present
    }

    // Update the main JudyL with the potentially modified values_judy
    *PValue = values_judy;

    return 0; // Continue
}

// Add all labels from an RRDLABELS instance to the aggregated structure
void rrdlabels_aggregated_add_from_rrdlabels(RRDLABELS_AGGREGATED *agg, RRDLABELS *labels) {
    if (!agg || !labels) return;

    // Use the rrdlabels iteration macros from rrdlabels.c
    // We need access to the internal structure, so we'll use walkthrough instead
    
    // Helper structure for the callback
    struct {
        RRDLABELS_AGGREGATED *agg;
    } callback_data = { .agg = agg };

    // Use the existing walkthrough function
    rrdlabels_walkthrough_read(labels, rrdlabels_aggregated_add_callback, &callback_data);
}

// Add a single label key-value pair to the aggregated structure
void rrdlabels_aggregated_add_label(RRDLABELS_AGGREGATED *agg, const char *key, const char *value) {
    if (!agg || !key || !value) return;
    
    // This is essentially the same logic as the callback, but exposed as a public function
    rrdlabels_aggregated_add_callback(key, value, RRDLABEL_SRC_AUTO, &(struct { RRDLABELS_AGGREGATED *agg; }){ .agg = agg });
}

// Output aggregated labels as JSON object with keys and their value arrays
void rrdlabels_aggregated_to_buffer_json(RRDLABELS_AGGREGATED *agg, BUFFER *wb, const char *key, size_t cardinality_limit) {
    if (!agg || !wb) return;

    buffer_json_member_add_object(wb, key);

    // Iterate through all keys
    Pvoid_t *PValue;
    Word_t key_index = 0;
    bool first_then_next = true;

    while ((PValue = JudyLFirstThenNext(agg->keys_judy, &key_index, &first_then_next))) {
        STRING *key_string = (STRING *)key_index;
        Pvoid_t values_judy = *PValue;

        // Add key and its values array
        buffer_json_member_add_array(wb, string2str(key_string));

        // Count total values for this key
        Word_t total_values = JudyLCount(values_judy, 0, -1, PJE0);
        
        // Iterate through all values for this key
        Word_t value_index = 0;
        bool value_first_then_next = true;
        Pvoid_t *PValueInner;
        size_t count = 0;
        
        while ((PValueInner = JudyLFirstThenNext(values_judy, &value_index, &value_first_then_next))) {
            if(cardinality_limit && count >= cardinality_limit - 1 && total_values > cardinality_limit) {
                // Add remaining count message
                char msg[100];
                snprintf(msg, sizeof(msg), "... %zu values more", total_values - count);
                buffer_json_add_array_item_string(wb, msg);
                break;
            }
            STRING *value_string = (STRING *)value_index;
            buffer_json_add_array_item_string(wb, string2str(value_string));
            count++;
        }

        buffer_json_array_close(wb);
    }

    buffer_json_object_close(wb);
}

// Merge all labels from source aggregated structure into destination
void rrdlabels_aggregated_merge(RRDLABELS_AGGREGATED *dst, RRDLABELS_AGGREGATED *src) {
    if (!dst || !src) return;
    
    // Iterate through all keys in source
    Pvoid_t *PValue;
    Word_t key_index = 0;
    bool first_then_next = true;
    
    while ((PValue = JudyLFirstThenNext(src->keys_judy, &key_index, &first_then_next))) {
        STRING *src_key_string = (STRING *)key_index;
        Pvoid_t src_values_judy = *PValue;
        
        // Get or create the destination values JudyL for this key
        STRING *dst_key_string = string_dup(src_key_string); // Create a new reference
        Pvoid_t *PDstValue = JudyLIns(&dst->keys_judy, (Word_t)dst_key_string, PJE0);
        
        if (!PDstValue || PDstValue == PJERR) {
            string_freez(dst_key_string);
            continue; // Skip on error
        }
        
        Pvoid_t dst_values_judy;
        if (!*PDstValue) {
            // New key in destination
            dst_values_judy = (Pvoid_t) NULL;
            *PDstValue = dst_values_judy;
        } else {
            // Key already exists in destination - free the duplicate key
            string_freez(dst_key_string);
            dst_values_judy = *PDstValue;
        }
        
        // Now merge all values from source to destination
        Word_t value_index = 0;
        bool value_first_then_next = true;
        Pvoid_t *PValueInner;
        
        while ((PValueInner = JudyLFirstThenNext(src_values_judy, &value_index, &value_first_then_next))) {
            STRING *src_value_string = (STRING *)value_index;
            STRING *dst_value_string = string_dup(src_value_string); // Create a new reference
            
            // Insert into destination values
            Pvoid_t *PDstValueInner = JudyLIns(&dst_values_judy, (Word_t)dst_value_string, PJE0);
            
            if (!PDstValueInner || PDstValueInner == PJERR) {
                string_freez(dst_value_string);
                continue; // Skip on error
            }
            
            if (*PDstValueInner) {
                // Value already exists - free the duplicate
                string_freez(dst_value_string);
            } else {
                // New value - mark as present
                *PDstValueInner = (Pvoid_t)1;
            }
        }
        
        // Update the destination JudyL with the potentially modified values_judy
        *PDstValue = dst_values_judy;
    }
}