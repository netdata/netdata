// SPDX-License-Identifier: GPL-3.0-or-later

#ifndef NETDATA_RRDLABELS_H
#define NETDATA_RRDLABELS_H

#include "rrd.h"

typedef enum __attribute__ ((__packed__)) {
    RRDLABEL_SRC_AUTO       = (1 << 0), // set when Netdata found the label by some automation
    RRDLABEL_SRC_CONFIG     = (1 << 1), // set when the user configured the label
    RRDLABEL_SRC_K8S        = (1 << 2), // set when this label is found from k8s (RRDLABEL_SRC_AUTO should also be set)
    RRDLABEL_SRC_ACLK       = (1 << 3), // set when this label is found from ACLK (RRDLABEL_SRC_AUTO should also be set)

    // more sources can be added here

    RRDLABEL_FLAG_PERMANENT = (1 << 29), // set when this label should never be removed (can be overwritten though)
    RRDLABEL_FLAG_OLD       = (1 << 30), // marks for rrdlabels internal use - they are not exposed outside rrdlabels
    RRDLABEL_FLAG_NEW       = (1 << 31)  // marks for rrdlabels internal use - they are not exposed outside rrdlabels
} RRDLABEL_SRC;

#define RRDLABEL_FLAG_INTERNAL (RRDLABEL_FLAG_OLD | RRDLABEL_FLAG_NEW | RRDLABEL_FLAG_PERMANENT)

size_t text_sanitize(unsigned char *dst, const unsigned char *src, size_t dst_size, unsigned char *char_map, bool utf, const char *empty, size_t *multibyte_length);

DICTIONARY *rrdlabels_create(void);
void rrdlabels_destroy(DICTIONARY *labels_dict);
void rrdlabels_add(DICTIONARY *dict, const char *name, const char *value, RRDLABEL_SRC ls);
void rrdlabels_add_pair(DICTIONARY *dict, const char *string, RRDLABEL_SRC ls);
void rrdlabels_value_to_buffer_array_item_or_null(DICTIONARY *labels, BUFFER *wb, const char *key);
void rrdlabels_get_value_strdup_or_null(DICTIONARY *labels, char **value, const char *key);
void rrdlabels_get_value_strcpyz(DICTIONARY *labels, char *dst, size_t dst_len, const char *key);
STRING *rrdlabels_get_value_string_dup(DICTIONARY *labels, const char *key);
STRING *rrdlabels_get_value_to_buffer_or_unset(DICTIONARY *labels, BUFFER *wb, const char *key, const char *unset);
void rrdlabels_flush(DICTIONARY *labels_dict);

void rrdlabels_unmark_all(DICTIONARY *labels);
void rrdlabels_remove_all_unmarked(DICTIONARY *labels);

int rrdlabels_walkthrough_read(DICTIONARY *labels, int (*callback)(const char *name, const char *value, RRDLABEL_SRC ls, void *data), void *data);
int rrdlabels_sorted_walkthrough_read(DICTIONARY *labels, int (*callback)(const char *name, const char *value, RRDLABEL_SRC ls, void *data), void *data);

void rrdlabels_log_to_buffer(DICTIONARY *labels, BUFFER *wb);
bool rrdlabels_match_simple_pattern(DICTIONARY *labels, const char *simple_pattern_txt);

bool rrdlabels_match_simple_pattern_parsed(DICTIONARY *labels, SIMPLE_PATTERN *pattern, char equal, size_t *searches);
int rrdlabels_to_buffer(DICTIONARY *labels, BUFFER *wb, const char *before_each, const char *equal, const char *quote, const char *between_them,
                        bool (*filter_callback)(const char *name, const char *value, RRDLABEL_SRC ls, void *data), void *filter_data,
                        void (*name_sanitizer)(char *dst, const char *src, size_t dst_size),
                        void (*value_sanitizer)(char *dst, const char *src, size_t dst_size));
void rrdlabels_to_buffer_json_members(DICTIONARY *labels, BUFFER *wb);

void rrdlabels_migrate_to_these(DICTIONARY *dst, DICTIONARY *src);
void rrdlabels_copy(DICTIONARY *dst, DICTIONARY *src);

int rrdlabels_unittest(void);

// unfortunately this break when defined in exporting_engine.h
bool exporting_labels_filter_callback(const char *name, const char *value, RRDLABEL_SRC ls, void *data);

#endif /* NETDATA_RRDLABELS_H */
