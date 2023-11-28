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
#define OVECCOUNT (MAX_OUTPUT_KEYS * 3)    // should be a multiple of 3
#define MAX_LINE_LENGTH (1024 * 1024)
#define MAX_KEY_DUPS (MAX_OUTPUT_KEYS / 2)
#define MAX_INJECTIONS (MAX_OUTPUT_KEYS / 2)
#define MAX_REWRITES (MAX_OUTPUT_KEYS / 2)
#define MAX_RENAMES (MAX_OUTPUT_KEYS / 2)
#define MAX_KEY_DUPS_KEYS 20

#define MAX_KEY_LEN 64              // according to systemd-journald
#define MAX_VALUE_LEN (48 * 1024)   // according to systemd-journald

#define LOG2JOURNAL_CONFIG_PATH LIBCONFIG_DIR "/log2journal.d"

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

typedef struct txt {
    char *s;
    size_t size;
} TEXT;

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

        if(txt->s)
            freez(txt->s);

        txt->s = strndupz(s, len);
        txt->size = len + 1;
    }
}

// ----------------------------------------------------------------------------

typedef struct key_value {
    char key[MAX_KEY_LEN + 1];
    TEXT value;
    bool on_unmatched;
} KEY_VALUE;

static inline void key_value_replace(KEY_VALUE *kv, const char *key, size_t key_len, const char *value, size_t value_len) {
    copy_to_buffer(kv->key, sizeof(kv->key), key, key_len);
    txt_replace(&kv->value, value, value_len);
}

// ----------------------------------------------------------------------------

struct key_dup {
    XXH64_hash_t hash;
    char *target;
    char *keys[MAX_KEY_DUPS_KEYS];
    TEXT values[MAX_KEY_DUPS_KEYS];
    size_t used;
    bool exposed;
};

struct key_rename {
    XXH64_hash_t new_hash;
    XXH64_hash_t old_hash;
    char *new_key;
    char *old_key;
};

struct replacement_node {
    bool is_variable;
    const char *s;
    size_t len;
    struct replacement_node *next;
};

struct key_rewrite {
    XXH64_hash_t hash;
    char *key;
    char *search_pattern;
    char *replace_pattern;
    pcre2_code *re;
    pcre2_match_data *match_data;
    struct replacement_node *nodes;
};

struct log_job {
    bool show_config;

    const char *pattern;
    const char *prefix;

    struct {
        const char *key;
        char current[FILENAME_MAX + 1];
        bool last_line_was_empty;
    } filename;

    struct {
        KEY_VALUE keys[MAX_INJECTIONS];
        size_t used;
    } injections;

    struct {
        const char *key;
        struct {
            KEY_VALUE keys[MAX_INJECTIONS];
            size_t used;
        } injections;
    } unmatched;

    struct {
        struct key_dup array[MAX_KEY_DUPS];
        size_t used;
    } dups;

    struct {
        struct key_rewrite array[MAX_REWRITES];
        size_t used;
    } rewrites;

    struct {
        struct key_rename array[MAX_RENAMES];
        size_t used;
    } renames;
};

void jb_send_key_value_and_rewrite(struct log_job *jb, const char *key, XXH64_hash_t hash, const char *value, size_t len);
void jb_send_duplications_for_key(struct log_job *jb, const char *key, XXH64_hash_t hash, const char *value, size_t value_len);
void jb_send_extracted_key_value(struct log_job *jb, const char *key, const char *value, size_t len);

struct key_dup *log_job_add_duplication_to_job(struct log_job *jb, const char *target, size_t target_len);
bool log_job_add_key_to_duplication(struct key_dup *kd, const char *key, size_t key_len);
bool log_job_add_filename_key(struct log_job *jb, const char *key, size_t key_len);
bool log_job_add_key_prefix(struct log_job *jb, const char *prefix, size_t prefix_len);
bool log_job_add_injection(struct log_job *jb, const char *key, size_t key_len, const char *value, size_t value_len, bool unmatched);
bool log_job_add_rewrite(struct log_job *jb, const char *key, const char *search_pattern, const char *replace_pattern);
bool log_job_add_rename(struct log_job *jb, const char *new_key, size_t new_key_len, const char *old_key, size_t old_key_len);

// entry point to parse command line parameters
bool parse_log2journal_parameters(struct log_job *jb, int argc, char **argv);

void log2journal_command_line_help(const char *name);

// free all resources consumed by the log job
void nd_log_destroy(struct log_job *jb);

#ifdef HAVE_LIBYAML
bool yaml_parse_file(const char *config_file_path, struct log_job *jb);
bool yaml_parse_config(const char *config_name, struct log_job *jb);
#endif

void log_job_to_yaml(struct log_job *jb);

typedef struct log_json_state LOG_JSON_STATE;
LOG_JSON_STATE *json_parser_create(struct log_job *jb);
void json_parser_destroy(LOG_JSON_STATE *js);
const char *json_parser_error(LOG_JSON_STATE *js);
bool json_parse_document(LOG_JSON_STATE *js, const char *txt);
void json_test(void);

typedef struct logfmt_state LOGFMT_STATE;
LOGFMT_STATE *logfmt_parser_create(struct log_job *jb);
void logfmt_parser_destroy(LOGFMT_STATE *lfs);
const char *logfmt_parser_error(LOGFMT_STATE *lfs);
bool logfmt_parse_document(LOGFMT_STATE *js, const char *txt);
void logfmt_test(void);

// ----------------------------------------------------------------------------
// PCRE2 patters handling

static inline pcre2_code *jb_compile_pcre2_pattern(const char *pattern) {
    int error_number;
    PCRE2_SIZE error_offset;
    PCRE2_SPTR pattern_ptr = (PCRE2_SPTR)pattern;

    pcre2_code *re = pcre2_compile(pattern_ptr, PCRE2_ZERO_TERMINATED, 0, &error_number, &error_offset, NULL);
    if (re == NULL) {
        PCRE2_UCHAR errbuf[1024];
        pcre2_get_error_message(error_number, errbuf, sizeof(errbuf));
        log2stderr("PCRE2 compilation failed at offset %d: %s", (int)error_offset, errbuf);
        log2stderr("Check for common regex syntax errors or unsupported PCRE2 patterns.");
        return NULL;
    }

    return re;
}

static inline bool jb_pcre2_match(pcre2_code *re, pcre2_match_data *match_data, char *line, size_t len, bool log) {
    int rc = pcre2_match(re, (PCRE2_SPTR)line, len, 0, 0, match_data, NULL);
    if(rc < 0) {
        PCRE2_UCHAR errbuf[1024];
        pcre2_get_error_message(rc, errbuf, sizeof(errbuf));

        if(log)
            log2stderr("PCRE2 error %d: %s on: %s", rc, errbuf, line);

        return false;
    }

    return true;
}

#endif //NETDATA_LOG2JOURNAL_H
