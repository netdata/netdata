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

#define XXH_INLINE_ALL
#include "../../libnetdata/xxhash.h"

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
    char *s;
    size_t size;
} TEXT;

static inline void txt_cleanup(TEXT *txt) {
    if(!txt)
        return;

    if(txt->s)
        freez(txt->s);

    txt->s = NULL;
    txt->size = 0;
}

static inline void txt_replace(TEXT *txt, const char *s, size_t len) {
    if(!s || !*s || len == 0) {
        s = "";
        len = 0;
    }

    if(len + 1 <= txt->size) {
        // the existing value allocation, fits our value

        memcpy(txt->s, s, len);
        txt->s[len] = '\0';
    }
    else {
        // no existing value allocation, or too small for our value
        // cleanup and increase the buffer

        txt_cleanup(txt);

        txt->s = strndupz(s, len);
        txt->size = len + 1;
    }
}

// ----------------------------------------------------------------------------

typedef struct injection {
    bool on_unmatched;
    TEXT value;
    char key[JOURNAL_MAX_KEY_LEN + 1];
} INJECTION;

// ----------------------------------------------------------------------------

typedef struct duplication {
    XXH64_hash_t hash;
    char *target;
    uint32_t used;
    bool exposed;
    char *keys[MAX_KEY_DUPS_KEYS];
    TEXT values[MAX_KEY_DUPS_KEYS];
} DUPLICATION;

// ----------------------------------------------------------------------------

typedef struct key_rename {
    XXH64_hash_t new_hash;
    XXH64_hash_t old_hash;
    char *new_key;
    char *old_key;
} RENAME;

// ----------------------------------------------------------------------------

typedef struct replacement_node {
    const char *s;
    uint32_t len;
    bool is_variable;

    struct replacement_node *next;
} REWRITE_REPLACEMENT_NODE;

typedef struct key_rewrite {
    XXH64_hash_t hash;
    char *key;
    char *search_pattern;
    char *replace_pattern;
    pcre2_code *re;
    pcre2_match_data *match_data;
    REWRITE_REPLACEMENT_NODE *nodes;
} REWRITE;

// ----------------------------------------------------------------------------
// A job configuration and runtime structures

typedef struct log_job {
    bool show_config;

    const char *pattern;
    const char *prefix;

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
DUPLICATION *log_job_add_duplication_to_job(LOG_JOB *jb, const char *target, size_t target_len);
bool log_job_add_key_to_duplication(DUPLICATION *kd, const char *key, size_t key_len);
bool log_job_add_filename_key(LOG_JOB *jb, const char *key, size_t key_len);
bool log_job_add_key_prefix(LOG_JOB *jb, const char *prefix, size_t prefix_len);
bool log_job_add_injection(LOG_JOB *jb, const char *key, size_t key_len, const char *value, size_t value_len, bool unmatched);
bool log_job_add_rewrite(LOG_JOB *jb, const char *key, const char *search_pattern, const char *replace_pattern);
bool log_job_add_rename(LOG_JOB *jb, const char *new_key, size_t new_key_len, const char *old_key, size_t old_key_len);

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

#endif //NETDATA_LOG2JOURNAL_H
