// SPDX-License-Identifier: GPL-3.0-or-later

#ifndef NETDATA_LOG2JOURNAL_H
#define NETDATA_LOG2JOURNAL_H

// only for PACKAGE_VERSION
#include "config.h"

#include <stdio.h>
#include <stdlib.h>
#include <dirent.h>
#include <string.h>
#include <stdbool.h>
#include <string.h>
#include <ctype.h>
#include <stdarg.h>

// ----------------------------------------------------------------------------
// logging

// enable the compiler to check for printf like errors on our log2stderr() function
static inline void log2stderr(const char *format, ...) __attribute__ ((format(__printf__, 1, 2)));
static inline void log2stderr(const char *format, ...) {
    va_list args;
    va_start(args, format);
    vfprintf(stderr, format, args);
    va_end(args);
    fprintf(stderr, "\n");
}

// ----------------------------------------------------------------------------
// allocation functions abstraction

static inline void *mallocz(size_t size) {
    void *ptr = malloc(size);
    if (!ptr) {
        log2stderr("Fatal Error: Memory allocation failed. Requested size: %zu bytes.", size);
        exit(EXIT_FAILURE);
    }
    return ptr;
}

static inline void *callocz(size_t elements, size_t size) {
    void *ptr = mallocz(elements * size);
    memset(ptr, 0, elements * size);
    return ptr;
}

static inline char *strdupz(const char *s) {
    char *ptr = strdup(s);
    if (!ptr) {
        log2stderr("Fatal Error: Memory allocation failed in strdup.");
        exit(EXIT_FAILURE);
    }
    return ptr;
}

static inline char *strndupz(const char *s, size_t n) {
    char *ptr = strndup(s, n);
    if (!ptr) {
        log2stderr("Fatal Error: Memory allocation failed in strndup. Requested size: %zu bytes.", n);
        exit(EXIT_FAILURE);
    }
    return ptr;
}

static inline void freez(void *ptr) {
    if (ptr)
        free(ptr);
}

// ----------------------------------------------------------------------------

#define XXH_INLINE_ALL
#include "../../libnetdata/xxhash.h"
#include "../../libnetdata/simple_hashtable.h"

#define PCRE2_CODE_UNIT_WIDTH 8
#include <pcre2.h>

#ifdef HAVE_LIBYAML
#include <yaml.h>
#endif

#define MAX_OUTPUT_KEYS 1024
#define MAX_LINE_LENGTH (1024 * 1024)
#define MAX_KEY_DUPS (MAX_OUTPUT_KEYS / 2)
#define MAX_INJECTIONS (MAX_OUTPUT_KEYS / 2)
#define MAX_REWRITES (MAX_OUTPUT_KEYS / 2)
#define MAX_RENAMES (MAX_OUTPUT_KEYS / 2)
#define MAX_KEY_DUPS_KEYS 20

#define JOURNAL_MAX_KEY_LEN 64              // according to systemd-journald
#define JOURNAL_MAX_VALUE_LEN (48 * 1024)   // according to systemd-journald

#define LOG2JOURNAL_CONFIG_PATH LIBCONFIG_DIR "/log2journal.d"

// ----------------------------------------------------------------------------
// character conversion for journal keys

extern const char journal_key_characters_map[256];

// ----------------------------------------------------------------------------
// copy to buffer, while ensuring there is no buffer overflow

static inline size_t copy_to_buffer(char *dst, size_t dst_size, const char *src, size_t src_len) {
    if(dst_size < 2) {
        if(dst_size == 1)
            *dst = '\0';

        return 0;
    }

    if(src_len <= dst_size - 1) {
        memcpy(dst, src, src_len);
        dst[src_len] = '\0';
        return src_len;
    }
    else {
        memcpy(dst, src, dst_size - 1);
        dst[dst_size - 1] = '\0';
        return dst_size - 1;
    }
}

// ----------------------------------------------------------------------------
// A dynamically sized, reusable text buffer,
// allowing us to be fast (no allocations during iterations) while having the
// smallest possible allocations.

typedef struct txt {
    char *txt;
    size_t size;
} TEXT;

static inline void txt_cleanup(TEXT *t) {
    if(!t)
        return;

    if(t->txt)
        freez(t->txt);

    t->txt = NULL;
    t->size = 0;
}

static inline void txt_replace(TEXT *t, const char *s, size_t len) {
    if(!s || !*s || len == 0) {
        s = "";
        len = 0;
    }

    if(len + 1 <= t->size) {
        // the existing value allocation, fits our value

        memcpy(t->txt, s, len);
        t->txt[len] = '\0';
    }
    else {
        // no existing value allocation, or too small for our value
        // cleanup and increase the buffer

        txt_cleanup(t);

        t->txt = strndupz(s, len);
        t->size = len + 1;
    }
}

// ----------------------------------------------------------------------------

typedef enum __attribute__((__packed__)) {
    HK_NONE              = 0,
    HK_ALLOCATED         = (1 << 0),
    HK_FILTERED          = (1 << 1),
    HK_FILTERED_INCLUDED = (1 << 2),
    HK_COLLISION_CHECKED = (1 << 3),
    HK_RENAMES_CHECKED   = (1 << 4),
    HK_HAS_RENAMES       = (1 << 5),
    HK_DUPS_CHECKED      = (1 << 6),
    HK_HAS_DUPS          = (1 << 7),
    HK_REWRITES_CHECKED  = (1 << 8),
    HK_HAS_REWRITES      = (1 << 9),
} HASHED_KEY_FLAGS;

typedef struct hashed_key {
    const char *key;
    uint32_t len;
    HASHED_KEY_FLAGS flags;
    XXH64_hash_t hash;
} HASHED_KEY;

static inline void hashed_key_cleanup(HASHED_KEY *k) {
    if(k->key) {
        freez((void *)k->key);
        k->key = NULL;
    }
}

static inline void hashed_key_set(HASHED_KEY *k, const char *name) {
    hashed_key_cleanup(k);

    k->key = strdupz(name);
    k->len = strlen(k->key);
    k->hash = XXH3_64bits(k->key, k->len);
    k->flags = HK_NONE;
}

static inline void hashed_key_len_set(HASHED_KEY *k, const char *name, size_t len) {
    hashed_key_cleanup(k);

    k->key = strndupz(name, len);
    k->len = len;
    k->hash = XXH3_64bits(k->key, k->len);
    k->flags = HK_NONE;
}

// ----------------------------------------------------------------------------

typedef struct search_pattern {
    const char *pattern;
    pcre2_code *re;
    pcre2_match_data *match_data;
    TEXT error;
} SEARCH_PATTERN;

void search_pattern_cleanup(SEARCH_PATTERN *sp);
bool search_pattern_set(SEARCH_PATTERN *sp, const char *search_pattern, size_t search_pattern_len);

static inline bool search_pattern_matches(SEARCH_PATTERN *sp, const char *value, size_t value_len) {
    return pcre2_match(sp->re, (PCRE2_SPTR)value, value_len, 0, 0, sp->match_data, NULL) >= 0;
}

// ----------------------------------------------------------------------------

typedef struct replacement_node {
    HASHED_KEY name;
    bool is_variable;
    bool logged_error;

    struct replacement_node *next;
} REPLACE_NODE;

void replace_node_free(REPLACE_NODE *rpn);

typedef struct replace_pattern {
    const char *pattern;
    REPLACE_NODE *nodes;
} REPLACE_PATTERN;

void replace_pattern_cleanup(REPLACE_PATTERN *rp);
bool replace_pattern_set(REPLACE_PATTERN *rp, const char *pattern);

// ----------------------------------------------------------------------------

typedef struct injection {
    bool on_unmatched;
    TEXT value;
    HASHED_KEY key;
} INJECTION;

void injection_cleanup(INJECTION *inj);

// ----------------------------------------------------------------------------

typedef struct duplication {
    HASHED_KEY target;
    uint32_t used;
    bool exposed;
    HASHED_KEY keys[MAX_KEY_DUPS_KEYS];
    TEXT values[MAX_KEY_DUPS_KEYS];
} DUPLICATION;

void duplication_cleanup(DUPLICATION *dp);

// ----------------------------------------------------------------------------

typedef struct key_rename {
    HASHED_KEY new_key;
    HASHED_KEY old_key;
} RENAME;

void rename_cleanup(RENAME *rn);

// ----------------------------------------------------------------------------

typedef struct key_rewrite {
    HASHED_KEY key;
    SEARCH_PATTERN search;
    REPLACE_PATTERN replace;
} REWRITE;

void rewrite_cleanup(REWRITE *rw);

// ----------------------------------------------------------------------------
// A job configuration and runtime structures

typedef struct log_job {
    bool show_config;

    const char *pattern;
    const char *prefix;

    SIMPLE_HASHTABLE hashtable;

    struct {
        SEARCH_PATTERN include;
        SEARCH_PATTERN exclude;
    } filter;

    struct {
        bool last_line_was_empty;
        const char *key;
        char current[FILENAME_MAX + 1];
    } filename;

    struct {
        uint32_t used;
        INJECTION keys[MAX_INJECTIONS];
    } injections;

    struct {
        const char *key;
        struct {
            uint32_t used;
            INJECTION keys[MAX_INJECTIONS];
        } injections;
    } unmatched;

    struct {
        uint32_t used;
        DUPLICATION array[MAX_KEY_DUPS];
    } dups;

    struct {
        uint32_t used;
        REWRITE array[MAX_REWRITES];
    } rewrites;

    struct {
        uint32_t used;
        RENAME array[MAX_RENAMES];
    } renames;
} LOG_JOB;

// free all resources consumed by the log job
void nd_log_cleanup(LOG_JOB *jb);

// ----------------------------------------------------------------------------

// the entry point to send key value pairs to the output
// this implements the pipeline of processing renames, rewrites and duplications
void log_job_send_extracted_key_value(LOG_JOB *jb, const char *key, const char *value, size_t len);

// ----------------------------------------------------------------------------
// configuration related

// management of configuration to set settings
DUPLICATION *log_job_duplication_add(LOG_JOB *jb, const char *target, size_t target_len);
bool log_job_duplication_key_add(DUPLICATION *kd, const char *key, size_t key_len);
bool log_job_filename_key_set(LOG_JOB *jb, const char *key, size_t key_len);
bool log_job_key_prefix_set(LOG_JOB *jb, const char *prefix, size_t prefix_len);
bool log_job_pattern_set(LOG_JOB *jb, const char *pattern, size_t pattern_len);
bool log_job_injection_add(LOG_JOB *jb, const char *key, size_t key_len, const char *value, size_t value_len, bool unmatched);
bool log_job_rewrite_add(LOG_JOB *jb, const char *key, const char *search_pattern, const char *replace_pattern);
bool log_job_rename_add(LOG_JOB *jb, const char *new_key, size_t new_key_len, const char *old_key, size_t old_key_len);
bool log_job_include_pattern_set(LOG_JOB *jb, const char *pattern, size_t pattern_len);
bool log_job_exclude_pattern_set(LOG_JOB *jb, const char *pattern, size_t pattern_len);

// entry point to parse command line parameters
bool log_job_command_line_parse_parameters(LOG_JOB *jb, int argc, char **argv);
void log_job_command_line_help(const char *name);

// ----------------------------------------------------------------------------
// YAML configuration related

#ifdef HAVE_LIBYAML
bool yaml_parse_file(const char *config_file_path, LOG_JOB *jb);
bool yaml_parse_config(const char *config_name, LOG_JOB *jb);
#endif

void log_job_configuration_to_yaml(LOG_JOB *jb);

// ----------------------------------------------------------------------------
// JSON parser

typedef struct log_json_state LOG_JSON_STATE;
LOG_JSON_STATE *json_parser_create(LOG_JOB *jb);
void json_parser_destroy(LOG_JSON_STATE *js);
const char *json_parser_error(LOG_JSON_STATE *js);
bool json_parse_document(LOG_JSON_STATE *js, const char *txt);
void json_test(void);

// ----------------------------------------------------------------------------
// logfmt parser

typedef struct logfmt_state LOGFMT_STATE;
LOGFMT_STATE *logfmt_parser_create(LOG_JOB *jb);
void logfmt_parser_destroy(LOGFMT_STATE *lfs);
const char *logfmt_parser_error(LOGFMT_STATE *lfs);
bool logfmt_parse_document(LOGFMT_STATE *js, const char *txt);
void logfmt_test(void);

// ----------------------------------------------------------------------------
// pcre2 parser

typedef struct pcre2_state PCRE2_STATE;
PCRE2_STATE *pcre2_parser_create(LOG_JOB *jb);
void pcre2_parser_destroy(PCRE2_STATE *pcre2);
const char *pcre2_parser_error(PCRE2_STATE *pcre2);
bool pcre2_parse_document(PCRE2_STATE *pcre2, const char *txt, size_t len);
bool pcre2_has_error(PCRE2_STATE *pcre2);
void pcre2_test(void);

void pcre2_get_error_in_buffer(char *msg, size_t msg_len, int rc, int pos);

#endif //NETDATA_LOG2JOURNAL_H
