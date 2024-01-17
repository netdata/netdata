// SPDX-License-Identifier: GPL-3.0-or-later

#ifndef NETDATA_RRDLABELS_H
#define NETDATA_RRDLABELS_H

#include "rrd.h"

typedef enum __attribute__ ((__packed__)) rrdlabel_source {
    RRDLABEL_SRC_AUTO       = (1 << 0), // set when Netdata found the label by some automation
    RRDLABEL_SRC_CONFIG     = (1 << 1), // set when the user configured the label
    RRDLABEL_SRC_K8S        = (1 << 2), // set when this label is found from k8s (RRDLABEL_SRC_AUTO should also be set)
    RRDLABEL_SRC_ACLK       = (1 << 3), // set when this label is found from ACLK (RRDLABEL_SRC_AUTO should also be set)

    // more sources can be added here

    RRDLABEL_FLAG_DONT_DELETE   = (1 << 29), // set when this label should never be removed (can be overwritten though)
    RRDLABEL_FLAG_OLD           = (1 << 30), // marks for rrdlabels internal use - they are not exposed outside rrdlabels
    RRDLABEL_FLAG_NEW           = (1 << 31)  // marks for rrdlabels internal use - they are not exposed outside rrdlabels
} RRDLABEL_SRC;

#define RRDLABEL_FLAG_INTERNAL (RRDLABEL_FLAG_OLD | RRDLABEL_FLAG_NEW | RRDLABEL_FLAG_DONT_DELETE)

size_t text_sanitize(unsigned char *dst, const unsigned char *src, size_t dst_size, const unsigned char *char_map, bool utf, const char *empty, size_t *multibyte_length);

RRDLABELS *rrdlabels_create(void);
void rrdlabels_destroy(RRDLABELS *labels_dict);
void rrdlabels_add(RRDLABELS *labels, const char *name, const char *value, RRDLABEL_SRC ls);
void rrdlabels_add_pair(RRDLABELS *labels, const char *string, RRDLABEL_SRC ls);
void rrdlabels_value_to_buffer_array_item_or_null(RRDLABELS *labels, BUFFER *wb, const char *key);
void rrdlabels_get_value_strdup_or_null(RRDLABELS *labels, char **value, const char *key);
void rrdlabels_get_value_to_buffer_or_unset(RRDLABELS *labels, BUFFER *wb, const char *key, const char *unset);
bool rrdlabels_exist(RRDLABELS *labels, const char *key);
size_t rrdlabels_entries(RRDLABELS *labels __maybe_unused);
size_t rrdlabels_version(RRDLABELS *labels __maybe_unused);
void rrdlabels_get_value_strcpyz(RRDLABELS *labels, char *dst, size_t dst_len, const char *key);

void rrdlabels_unmark_all(RRDLABELS *labels);
void rrdlabels_remove_all_unmarked(RRDLABELS *labels);

int rrdlabels_walkthrough_read(RRDLABELS *labels, int (*callback)(const char *name, const char *value, RRDLABEL_SRC ls, void *data), void *data);
void rrdlabels_log_to_buffer(RRDLABELS *labels, BUFFER *wb);
bool rrdlabels_match_simple_pattern(RRDLABELS *labels, const char *simple_pattern_txt);

bool rrdlabels_match_simple_pattern_parsed(RRDLABELS *labels, SIMPLE_PATTERN *pattern, char equal, size_t *searches);
int rrdlabels_to_buffer(RRDLABELS *labels, BUFFER *wb, const char *before_each, const char *equal, const char *quote, const char *between_them,
                        bool (*filter_callback)(const char *name, const char *value, RRDLABEL_SRC ls, void *data), void *filter_data,
                        void (*name_sanitizer)(char *dst, const char *src, size_t dst_size),
                        void (*value_sanitizer)(char *dst, const char *src, size_t dst_size));
void rrdlabels_to_buffer_json_members(RRDLABELS *labels, BUFFER *wb);

void rrdlabels_migrate_to_these(RRDLABELS *dst, RRDLABELS *src);
void rrdlabels_copy(RRDLABELS *dst, RRDLABELS *src);
size_t rrdlabels_common_count(RRDLABELS *labels1, RRDLABELS *labels2);

int rrdlabels_unittest(void);

// unfortunately this break when defined in exporting_engine.h
bool exporting_labels_filter_callback(const char *name, const char *value, RRDLABEL_SRC ls, void *data);

#endif /* NETDATA_RRDLABELS_H */
