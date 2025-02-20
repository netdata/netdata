// SPDX-License-Identifier: GPL-3.0-or-later

#ifndef NETDATA_LOG2JOURNAL_H
#define NETDATA_LOG2JOURNAL_H

#include "libnetdata/libnetdata.h"
#include "log2journal-txt.h"
#include "log2journal-hashed-key.h"

// ----------------------------------------------------------------------------
// logging

// enable the compiler to check for printf like errors on our log2stderr() function
static inline void l2j_log(const char *format, ...) PRINTFLIKE(1, 2);
static inline void l2j_log(const char *format, ...) {
    va_list args;
    va_start(args, format);
    vfprintf(stderr, format, args);
    va_end(args);
    fprintf(stderr, "\n");
}

// ----------------------------------------------------------------------------

#define PCRE2_CODE_UNIT_WIDTH 8
#include <pcre2.h>

#ifdef HAVE_LIBYAML
#include <yaml.h>
#endif

// ----------------------------------------------------------------------------
// hashtable for HASHED_KEY

struct hashed_key;
static inline int compare_keys(struct hashed_key *k1, struct hashed_key *k2);
#define SIMPLE_HASHTABLE_SORT_FUNCTION compare_keys
#define SIMPLE_HASHTABLE_VALUE_TYPE HASHED_KEY *
#define SIMPLE_HASHTABLE_NAME _KEY
#include "libnetdata/simple_hashtable/simple_hashtable.h"

// ----------------------------------------------------------------------------

#define MAX_OUTPUT_KEYS 1024
#define MAX_LINE_LENGTH (1024 * 1024)
#define MAX_INJECTIONS (MAX_OUTPUT_KEYS / 2)
#define MAX_REWRITES (MAX_OUTPUT_KEYS / 2)
#define MAX_RENAMES (MAX_OUTPUT_KEYS / 2)

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

typedef struct search_pattern {
    const char *pattern;
    pcre2_code *re;
    pcre2_match_data *match_data;
    TXT_L2J error;
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
    bool has_variables;
} REPLACE_PATTERN;

void replace_pattern_cleanup(REPLACE_PATTERN *rp);
bool replace_pattern_set(REPLACE_PATTERN *rp, const char *pattern);

// ----------------------------------------------------------------------------

typedef struct injection {
    bool on_unmatched;
    HASHED_KEY key;
    REPLACE_PATTERN value;
} INJECTION;

void injection_cleanup(INJECTION *inj);

// ----------------------------------------------------------------------------

typedef struct key_rename {
    HASHED_KEY new_key;
    HASHED_KEY old_key;
} RENAME;

void rename_cleanup(RENAME *rn);

// ----------------------------------------------------------------------------

typedef enum __attribute__((__packed__)) {
    RW_NONE                 = 0,
    RW_MATCH_PCRE2          = (1 << 1), // a rewrite rule
    RW_MATCH_NON_EMPTY      = (1 << 2), // a rewrite rule
    RW_DONT_STOP            = (1 << 3),
    RW_INJECT               = (1 << 4),
} RW_FLAGS;

typedef struct key_rewrite {
    RW_FLAGS flags;
    HASHED_KEY key;
    union {
        SEARCH_PATTERN match_pcre2;
        REPLACE_PATTERN match_non_empty;
    };
    REPLACE_PATTERN value;
} REWRITE;

void rewrite_cleanup(REWRITE *rw);

// ----------------------------------------------------------------------------
// A job configuration and runtime structures

typedef struct log_job {
    bool show_config;

    const char *pattern;
    const char *prefix;

    SIMPLE_HASHTABLE_KEY hashtable;

    struct {
        const char *buffer;
        const char *trimmed;
        size_t trimmed_len;
        size_t size;
        HASHED_KEY key;
    } line;

    struct {
        SEARCH_PATTERN include;
        SEARCH_PATTERN exclude;
    } filter;

    struct {
        bool last_line_was_empty;
        HASHED_KEY key;
        TXT_L2J current;
    } filename;

    struct {
        uint32_t used;
        INJECTION keys[MAX_INJECTIONS];
    } injections;

    struct {
        HASHED_KEY key;
        struct {
            uint32_t used;
            INJECTION keys[MAX_INJECTIONS];
        } injections;
    } unmatched;

    struct {
        uint32_t used;
        REWRITE array[MAX_REWRITES];
        TXT_L2J tmp;
    } rewrites;

    struct {
        uint32_t used;
        RENAME array[MAX_RENAMES];
    } renames;
} LOG_JOB;

// initialize a log job
void log_job_init(LOG_JOB *jb);

// free all resources consumed by the log job
void log_job_cleanup(LOG_JOB *jb);

// ----------------------------------------------------------------------------

// the entry point to send key value pairs to the output
// this implements the pipeline of processing renames, rewrites and duplications
void log_job_send_extracted_key_value(LOG_JOB *jb, const char *key, const char *value, size_t len);

// ----------------------------------------------------------------------------
// configuration related

// management of configuration to set settings
bool log_job_filename_key_set(LOG_JOB *jb, const char *key, size_t key_len);
bool log_job_key_prefix_set(LOG_JOB *jb, const char *prefix, size_t prefix_len);
bool log_job_pattern_set(LOG_JOB *jb, const char *pattern, size_t pattern_len);
bool log_job_injection_add(LOG_JOB *jb, const char *key, size_t key_len, const char *value, size_t value_len, bool unmatched);
bool log_job_rewrite_add(LOG_JOB *jb, const char *key, RW_FLAGS flags, const char *search_pattern, const char *replace_pattern);
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

size_t parse_surrogate(const char *s, char *d, size_t *remaining);

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
