// SPDX-License-Identifier: GPL-3.0-or-later

// only for PACKAGE_VERSION
#include "config.h"

#include <stdio.h>
#include <stdlib.h>
#include <string.h>
#include <stdbool.h>
#include <string.h>
#include <ctype.h>
#include <stdarg.h>

#define XXH_INLINE_ALL
#include "../xxhash.h"

#define PCRE2_CODE_UNIT_WIDTH 8
#include <pcre2.h>

#define MAX_OUTPUT_KEYS 1024
#define OVECCOUNT (MAX_OUTPUT_KEYS * 3)    // should be a multiple of 3
#define MAX_LINE_LENGTH (1024 * 1024)
#define MAX_KEY_DUPS (MAX_OUTPUT_KEYS / 2)
#define MAX_INJECTIONS (MAX_OUTPUT_KEYS / 2)
#define MAX_REWRITES (MAX_OUTPUT_KEYS / 2)
#define MAX_KEY_DUPS_KEYS 20

#define MAX_KEY_LEN 64              // according to systemd-journald
#define MAX_VALUE_LEN (48 * 1024)   // according to systemd-journald

void display_help(const char *name) {
    printf("\n");
    printf("Netdata log2journal " PACKAGE_VERSION "\n");
    printf("\n");
    printf("Convert structured log input to systemd Journal Export Format.\n");
    printf("\n");
    printf("Using PCRE2 patterns, extract the fields from structured logs on the standard\n");
    printf("input, and generate output according to systemd Journal Export Format.\n");
    printf("\n");
    printf("Usage: %s [OPTIONS] PATTERN\n", name);
    printf("\n");
    printf("Options:\n");
    printf("\n");
    printf("  --filename-key=KEY\n");
    printf("       Add a field with KEY as the key and the current filename as value.\n");
    printf("       Automatically detects filenames when piped after 'tail -F',\n");
    printf("       and tail matches multiple filenames.\n");
    printf("       To inject the filename when tailing a single file, use --inject.\n");
    printf("\n");
    printf("  --unmatched-key=KEY\n");
    printf("       Include unmatched log entries in the output with KEY as the field name.\n");
    printf("       Use this to include unmatched entries to the output stream.\n");
    printf("       Usually it should be set to --unmatched-key=MESSAGE so that the\n");
    printf("       unmatched entry will appear as the log message in the journals.\n");
    printf("       Use --inject-unmatched to inject additional fields to unmatched lines.\n");
    printf("\n");
    printf("  --duplicate=TARGET=KEY1[,KEY2[,KEY3[,...]]\n");
    printf("       Create a new key called TARGET, duplicating the values of the keys\n");
    printf("       given. Useful for further processing. When multiple keys are given,\n");
    printf("       their values are separated by comma.\n");
    printf("       Up to %d duplications can be given on the command line, and up to\n", MAX_KEY_DUPS);
    printf("       %d keys per duplication command are allowed.\n", MAX_KEY_DUPS_KEYS);
    printf("\n");
    printf("  --inject=LINE\n");
    printf("       Inject constant fields to the output (both matched and unmatched logs).\n");
    printf("       --inject entries are added to unmatched lines too, when their key is\n");
    printf("       not used in --inject-unmatched (--inject-unmatched override --inject).\n");
    printf("       Up to %d fields can be injected.\n", MAX_INJECTIONS);
    printf("\n");
    printf("  --inject-unmatched=LINE\n");
    printf("       Inject lines into the output for each unmatched log entry.\n");
    printf("       Usually, --inject-unmatched=PRIORITY=3 is needed to mark the unmatched\n");
    printf("       lines as errors, so that they can easily be spotted in the journals.\n");
    printf("       Up to %d such lines can be injected.\n", MAX_INJECTIONS);
    printf("\n");
    printf("  --rewrite=KEY=/SearchPattern/ReplacePattern\n");
    printf("       Apply a rewrite rule to the values of a specific key.\n");
    printf("       The first character after KEY= is the separator, which should also\n");
    printf("       be used between the search pattern and the replacement pattern.\n");
    printf("       The search pattern is a PCRE2 regular expression, and the replacement\n");
    printf("       pattern supports literals and named capture groups from the search pattern.\n");
    printf("       Example:\n");
    printf("              --rewrite=DATE=/^(?<year>\\d{4})-(?<month>\\d{2})-(?<day>\\d{2})$/\n");
    printf("                             ${day}/${month}/${year}\n");
    printf("       This will rewrite dates in the format YYYY-MM-DD to DD/MM/YYYY.\n");
    printf("\n");
    printf("       Only one rewrite rule is applied per key; the sequence of rewrites stops\n");
    printf("       for the key once a rule matches it. This allows providing a sequence of\n");
    printf("       independent rewriting rules for the same key, matching the different values\n");
    printf("       the key may get, and also provide a catch-all rewrite rule at the end of the\n");
    printf("       sequence for setting the key value if no other rule matched it.\n");
    printf("\n");
    printf("       The combination of duplicating keys with the values of multiple other keys\n");
    printf("       combined with multiple rewrite rules, allows creating complex rules for\n");
    printf("       rewriting key values.\n");
    printf("\n");
    printf("       Up to %d rewriting rules are allowed.\n", MAX_REWRITES);
    printf("\n");
    printf("  -h, --help\n");
    printf("       Display this help and exit.\n");
    printf("\n");
    printf("  PATTERN\n");
    printf("       PATTERN should be a valid PCRE2 regular expression.\n");
    printf("       RE2 regular expressions (like the ones usually used in Go applications),\n");
    printf("       are usually valid PCRE2 patterns too.\n");
    printf("       Regular expressions without named groups are ignored.\n");
    printf("\n");
    printf("The maximum line length accepted is %d characters.\n", MAX_LINE_LENGTH);
    printf("The maximum number of fields in the PCRE2 pattern is %d.\n", OVECCOUNT / 3);
    printf("\n");
    printf("PIPELINE AND SEQUENCE OF PROCESSING\n");
    printf("\n");
    printf("This is a simple diagram of the pipeline taking place:\n");
    printf("\n");
    printf("           +---------------------------------------------------+\n");
    printf("           |                       INPUT                       |\n");
    printf("           +---------------------------------------------------+\n");
    printf("                            v                          v\n");
    printf("           +---------------------------------+         |\n");
    printf("           |   EXTRACT FIELDS AND VALUES     |         |\n");
    printf("           +---------------------------------+         |\n");
    printf("                  v                  v                 |\n");
    printf("           +---------------+         |                 |\n");
    printf("           |   DUPLICATE   |         |                 |\n");
    printf("           | create fields |         |                 |\n");
    printf("           |  with values  |         |                 |\n");
    printf("           +---------------+         |                 |\n");
    printf("                  v                  v                 v\n");
    printf("           +---------------------------------+  +--------------+\n");
    printf("           |         REWRITE PIPELINES       |  |    INJECT    |\n");
    printf("           |        altering the values      |  |   constants  |\n");
    printf("           +---------------------------------+  +--------------+\n");
    printf("                             v                          v\n");
    printf("           +---------------------------------------------------+\n");
    printf("           |                       OUTPUT                      |\n");
    printf("           +---------------------------------------------------+\n");
    printf("\n");
    printf("JOURNAL FIELDS RULES (enforced by systemd-journald)\n");
    printf("\n");
    printf("     - field names can be up to 64 characters\n");
    printf("     - the only allowed field characters are A-Z, 0-9 and underscore\n");
    printf("     - the first character of fields cannot be a digit\n");
    printf("     - protected journal fields start with underscore:\n");
    printf("       * they are accepted by systemd-journal-remote\n");
    printf("       * they are NOT accepted by a local systemd-journald\n");
    printf("\n");
    printf("     For best results, always include these fields:\n");
    printf("\n");
    printf("      MESSAGE=TEXT\n");
    printf("      The MESSAGE is the body of the log entry.\n");
    printf("      This field is what we usually see in our logs.\n");
    printf("\n");
    printf("      PRIORITY=NUMBER\n");
    printf("      PRIORITY sets the severity of the log entry.\n");
    printf("      0=emerg, 1=alert, 2=crit, 3=err, 4=warn, 5=notice, 6=info, 7=debug\n");
    printf("      - Emergency events (0) are usually broadcast to all terminals.\n");
    printf("      - Emergency, alert, critical, and error (0-3) are usually colored red.\n");
    printf("      - Warning (4) entries are usually colored yellow.\n");
    printf("      - Notice (5) entries are usually bold or have a brighter white color.\n");
    printf("      - Info (6) entries are the default.\n");
    printf("      - Debug (7) entries are usually grayed or dimmed.\n");
    printf("\n");
    printf("      SYSLOG_IDENTIFIER=NAME\n");
    printf("      SYSLOG_IDENTIFIER sets the name of application.\n");
    printf("      Use something descriptive, like: SYSLOG_IDENTIFIER=nginx-logs\n");
    printf("\n");
    printf("You can find the most common fields at 'man systemd.journal-fields'.\n");
    printf("\n");
}

// ----------------------------------------------------------------------------

// enable the compiler to check for printf like errors on our log2stderr() function
static void log2stderr(const char *format, ...) __attribute__ ((format(__printf__, 1, 2)));
static void log2stderr(const char *format, ...) {
    va_list args;
    va_start(args, format);
    vfprintf(stderr, format, args);
    va_end(args);
    fprintf(stderr, "\n");
}

// ----------------------------------------------------------------------------

size_t copy_to_buffer(char *dst, size_t dst_size, const char *src, size_t src_len) {
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

static void txt_replace(TEXT *txt, const char *s, size_t len) {
    if(len + 1 <= txt->size) {
        // the existing value allocation, fits our value

        memcpy(txt->s, s, len);
        txt->s[len] = '\0';
    }
    else {
        // no existing value allocation, or too small for our value

        if(txt->s)
            free(txt->s);

        txt->s = strndup(s, len);
        txt->size = len + 1;
    }
}

// ----------------------------------------------------------------------------

typedef struct key_value {
    char key[MAX_KEY_LEN + 1];
    TEXT value;
    bool on_unmatched;
} KEY_VALUE;

void key_value_replace(KEY_VALUE *kv, const char *key, size_t key_len, const char *value, size_t value_len) {
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
    const char *pattern;

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
};

void jb_cleanup(struct log_job *jb) {
    for(size_t i = 0; i < jb->injections.used ;i++) {
        if(jb->injections.keys[i].value.s)
            free(jb->injections.keys[i].value.s);
    }

    for(size_t i = 0; i < jb->unmatched.injections.used ;i++) {
        if(jb->unmatched.injections.keys[i].value.s)
            free(jb->unmatched.injections.keys[i].value.s);
    }

    for(size_t i = 0; i < jb->dups.used ;i++) {
        struct key_dup *kd = &jb->dups.array[i];

        if(kd->target)
            free(kd->target);

        for(size_t j = 0; j < kd->used ; j++) {
            if (kd->keys[j])
                free(kd->keys[j]);

            if (kd->values[j].s)
                free(kd->values[j].s);
        }
    }

    for(size_t i = 0; i < jb->rewrites.used; i++) {
        struct key_rewrite *rw = &jb->rewrites.array[i];

        if (rw->key)
            free(rw->key);

        if (rw->search_pattern)
            free(rw->search_pattern);

        if (rw->replace_pattern)
            free(rw->replace_pattern);

        if(rw->match_data)
            pcre2_match_data_free(rw->match_data);

        if (rw->re)
            pcre2_code_free(rw->re);

        // Cleanup for replacement nodes linked list
        struct replacement_node *current = rw->nodes;
        while (current != NULL) {
            struct replacement_node *next = current->next;

            if (current->s)
                free((void *)current->s);

            free(current);
            current = next;
        }
    }

    memset(jb, 0, sizeof(*jb));
}

// ----------------------------------------------------------------------------
// PCRE2

static pcre2_code *jb_compile_pcre2_pattern(const char *pattern) {
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

// ----------------------------------------------------------------------------

static char *rewrite_value(struct log_job *jb, const char *key, XXH64_hash_t hash, const char *value, size_t value_len) {
    static __thread char rewritten_value[MAX_VALUE_LEN + 1];

    for (size_t i = 0; i < jb->rewrites.used; i++) {
        struct key_rewrite *rw = &jb->rewrites.array[i];

        if (rw->hash == hash && strcmp(rw->key, key) == 0) {
            if (!jb_pcre2_match(rw->re, rw->match_data, (char *)value, value_len, false)) {
                continue; // No match found, skip to next rewrite rule
            }

            PCRE2_SIZE *ovector = pcre2_get_ovector_pointer(rw->match_data);

            char *buffer = rewritten_value;
            size_t buffer_remaining = sizeof(rewritten_value);

            // Iterate through the linked list of replacement nodes
            for (struct replacement_node *node = rw->nodes; node != NULL; node = node->next) {
                if (node->is_variable) {
                    uint32_t groupnumber = pcre2_substring_number_from_name(rw->re, (PCRE2_SPTR)node->s);
                    PCRE2_SIZE start_offset = ovector[2 * groupnumber];
                    PCRE2_SIZE end_offset = ovector[2 * groupnumber + 1];
                    PCRE2_SIZE length = end_offset - start_offset;

                    size_t copied = copy_to_buffer(buffer, buffer_remaining, value + start_offset, length);
                    buffer += copied;
                    buffer_remaining -= copied;
                }
                else {
                    size_t len = node->len;
                    size_t copied = copy_to_buffer(buffer, buffer_remaining, node->s, len);
                    buffer += copied;
                    buffer_remaining -= copied;
                }
            }

            return rewritten_value;
        }
    }

    return NULL;
}

// ----------------------------------------------------------------------------

static inline void send_key_value_error(const char *key, const char *format, ...) __attribute__ ((format(__printf__, 2, 3)));
static inline void send_key_value_error(const char *key, const char *format, ...) {
    printf("%s=", key);
    va_list args;
    va_start(args, format);
    vprintf(format, args);
    va_end(args);
    printf("\n");
}

static inline void send_key_value_and_rewrite(struct log_job *jb, const char *key, XXH64_hash_t hash, const char *value, size_t len) {
    char *rewritten = rewrite_value(jb, key, hash, value, len);
    if(!rewritten)
        printf("%s=%.*s\n", key, (int)len, value);
    else
        printf("%s=%s\n", key, rewritten);
}

static inline void send_key_value_constant(struct log_job *jb, const char *key, const char *value) {
    printf("%s=%s\n", key, value);
}

// ----------------------------------------------------------------------------
// command line params

struct replacement_node *add_replacement_node(struct replacement_node **head, bool is_variable, const char *text) {
    struct replacement_node *new_node = malloc(sizeof(struct replacement_node));
    if (!new_node)
        return NULL;

    new_node->is_variable = is_variable;
    new_node->s = text;
    new_node->len = strlen(text);
    new_node->next = NULL;

    if (*head == NULL)
        *head = new_node;

    else {
        struct replacement_node *current = *head;

        // append it
        while (current->next != NULL)
            current = current->next;

        current->next = new_node;
    }

    return new_node;
}

bool parse_replacement_pattern(struct key_rewrite *rw) {
    const char *current = rw->replace_pattern;

    while (*current != '\0') {
        if (*current == '$' && *(current + 1) == '{') {
            // Start of a variable
            const char *end = strchr(current, '}');
            if (!end) {
                log2stderr("Error: Missing closing brace in replacement pattern: %s", rw->replace_pattern);
                return false;
            }

            size_t name_length = end - current - 2; // Length of the variable name
            char *variable_name = strndup(current + 2, name_length);
            if (!variable_name) {
                log2stderr("Error: Memory allocation failed for variable name.");
                return false;
            }

            struct replacement_node *node = add_replacement_node(&(rw->nodes), true, variable_name);
            if (!node) {
                free(variable_name);
                log2stderr("Error: Failed to add replacement node for variable.");
                return false;
            }

            current = end + 1; // Move past the variable
        }
        else {
            // Start of literal text
            const char *start = current;
            while (*current != '\0' && !(*current == '$' && *(current + 1) == '{')) {
                current++;
            }

            size_t text_length = current - start;
            char *text = strndup(start, text_length);
            if (!text) {
                log2stderr("Error: Memory allocation failed for literal text.");
                return false;
            }

            struct replacement_node *node = add_replacement_node(&(rw->nodes), false, text);
            if (!node) {
                free(text);
                log2stderr("Error: Failed to add replacement node for text.");
                return false;
            }
        }
    }

    return true;
}

static bool is_symbol(char c) {
    return !isalpha(c) && !isdigit(c) && !iscntrl(c);
}

static bool parse_rewrite(struct log_job *jb, const char *param) {
    // Search for '=' in param
    const char *equal_sign = strchr(param, '=');
    if (!equal_sign || equal_sign == param) {
        log2stderr("Error: Invalid rewrite format, '=' not found or at the start in %s", param);
        return false;
    }

    // Get the next character as the separator
    char separator = *(equal_sign + 1);
    if (!separator || !is_symbol(separator)) {
        log2stderr("Error: rewrite separator not found after '=', or is not one of /\\|-# in: %s", param);
        return false;
    }

    // Find the next occurrence of the separator
    const char *second_separator = strchr(equal_sign + 2, separator);
    if (!second_separator) {
        log2stderr("Error: rewrite second separator not found in: %s", param);
        return false;
    }

    // Check if the search pattern is empty
    if (equal_sign + 1 == second_separator) {
        log2stderr("Error: rewrite search pattern is empty in: %s", param);
        return false;
    }

    // Check if the replacement pattern is empty
    if (*(second_separator + 1) == '\0') {
        log2stderr("Error: rewrite replacement pattern is empty in: %s", param);
        return false;
    }

    // Reserve a slot in rewrites
    if (jb->rewrites.used >= MAX_REWRITES) {
        log2stderr("Error: Exceeded maximum number of rewrite rules, while processing: %s", param);
        return false;
    }

    // Extract key, search pattern, and replacement pattern
    char *key = strndup(param, equal_sign - param);
    char *search_pattern = strndup(equal_sign + 2, second_separator - (equal_sign + 2));
    char *replace_pattern = strdup(second_separator + 1);

    // Create the PCRE2 pattern
    pcre2_code *re = jb_compile_pcre2_pattern(search_pattern);
    if (!re) {
        free(key);
        free(search_pattern);
        free(replace_pattern);
        return false;
    }

    struct key_rewrite *rw = &jb->rewrites.array[jb->rewrites.used++];
    rw->hash = XXH3_64bits(key, strlen(key));
    rw->key = key;
    rw->search_pattern = search_pattern;
    rw->replace_pattern = replace_pattern;
    rw->re = re;
    rw->match_data = pcre2_match_data_create_from_pattern(rw->re, NULL);

    // Parse the replacement pattern and create the linked list
    if (!parse_replacement_pattern(rw)) {
        pcre2_match_data_free(rw->match_data);
        pcre2_code_free(rw->re);
        free(rw->key);
        free(rw->search_pattern);
        free(rw->replace_pattern);
        jb->rewrites.used--;
        return false;
    }

    return true;
}

bool parse_parameters(struct log_job *jb, int argc, char **argv) {
    for (int i = 1; i < argc; i++) {
        char *arg = argv[i];
        if (strcmp(arg, "--help") == 0 || strcmp(arg, "-h") == 0) {
            display_help(argv[0]);
            exit(0);
        }
        else if (strncmp(arg, "--filename-key=", 15) == 0)
            jb->filename.key = arg + 15;
        else if (strncmp(arg, "--unmatched-key=", 16) == 0)
            jb->unmatched.key = arg + 16;
        else if(strncmp(arg, "--duplicate=", 12) == 0) {
            const char *first_key = arg + 12;
            const char *comma = strchr(first_key, '=');
            if(!comma) {
                log2stderr("Error: --duplicate=TARGET=KEY1,... is missing the equal sign.");
                return false;
            }
            const char *next_key = comma + 1;

            if (jb->dups.used >= MAX_KEY_DUPS) {
                log2stderr("Error: too many duplications. You can duplicate up to %d keys.", MAX_KEY_DUPS);
                return false;
            }

            size_t first_key_len = comma - first_key;
            struct key_dup *kd = &jb->dups.array[jb->dups.used++];
            kd->target = strndup(first_key, first_key_len);
            kd->hash = XXH3_64bits(first_key, first_key_len);
            kd->used = 0;

            while(next_key) {
                if(kd->used >= MAX_KEY_DUPS_KEYS) {
                    log2stderr("Error: too many keys in duplication of target '%s'.", kd->target);
                    return false;
                }

                first_key = next_key;
                comma = strchr(first_key, ',');

                if(comma) {
                    first_key_len = comma - first_key;
                    kd->keys[kd->used++] = strndup(first_key, first_key_len);
                    next_key = comma + 1;
                }
                else {
                    kd->keys[kd->used++] = strdup(first_key);
                    next_key = NULL;
                }
            }
        }
        else if(strncmp(arg, "--inject=", 9) == 0) {
            if(jb->injections.used >= MAX_INJECTIONS) {
                log2stderr("Error: too many injections. You can inject up to %d lines.", MAX_INJECTIONS);
                return false;
            }

            const char *key = arg + 9;
            const char *equal = strchr(key, '=');
            if(!equal) {
                log2stderr("Error: injection '%s' does not have an equal sign.", key);
                return false;
            }

            key_value_replace(&jb->injections.keys[jb->injections.used++],
                              key, equal - key,
                              equal + 1, strlen(equal + 1));
        }
        else if(strncmp(arg, "--inject-unmatched=", 19) == 0) {
            if(jb->unmatched.injections.used >= MAX_INJECTIONS) {
                log2stderr("Error: too many unmatched injections. You can inject up to %d lines.", MAX_INJECTIONS);
                return false;
            }
            const char *key = arg + 19;
            const char *equal = strchr(key, '=');
            if(!equal) {
                log2stderr("Error: unmatched injection '%s' does not have an equal sign.", key);
                return false;
            }

            key_value_replace(&jb->unmatched.injections.keys[jb->unmatched.injections.used++],
                              key, equal - key,
                              equal + 1, strlen(equal + 1));
        }
        else if(strncmp(arg, "--rewrite=", 10) == 0) {
            if(!parse_rewrite(jb, arg + 10))
                return false;
        }
        else {
            // Assume it's the pattern if not recognized as a parameter
            if (!jb->pattern) {
                jb->pattern = arg;
            } else {
                log2stderr("Error: Multiple patterns detected. Specify only one pattern. The first is '%s', the second is '%s'", jb->pattern, arg);
                return false;
            }
        }
    }

    // Check if a pattern is set and exactly one pattern is specified
    if (!jb->pattern) {
        log2stderr("Error: Pattern not specified.");
        display_help(argv[0]);
        return false;
    }

    return true;
}

// ----------------------------------------------------------------------------
// injection of constant fields

static void jb_select_which_injections_should_be_injected_on_unmatched(struct log_job *jb) {
    // mark all injections to be added to unmatched logs
    for(size_t i = 0; i < jb->injections.used ; i++)
        jb->injections.keys[i].on_unmatched = true;

    if(jb->injections.used && jb->unmatched.injections.used) {
        // we have both injections and injections on unmatched

        // we find all the injections that are also configured as injections on unmatched,
        // and we disable them, so that the output will not have the same key twice

        for(size_t i = 0; i < jb->injections.used ;i++) {
            for(size_t u = 0; u < jb->unmatched.injections.used ; u++) {
                if(strcmp(jb->injections.keys[i].key, jb->unmatched.injections.keys[u].key) == 0)
                    jb->injections.keys[i].on_unmatched = false;
            }
        }
    }
}


static inline void jb_finalize_injections(struct log_job *jb, bool line_is_matched) {
    for (size_t j = 0; j < jb->injections.used; j++) {
        if(!line_is_matched && !jb->injections.keys[j].on_unmatched)
            continue;

        send_key_value_constant(jb, jb->injections.keys[j].key, jb->injections.keys[j].value.s);
    }
}

static inline void jb_reset_injections(struct log_job *jb) {
    for(size_t d = 0; d < jb->dups.used ; d++) {
        struct key_dup *kd = &jb->dups.array[d];
        kd->exposed = false;

        for(size_t g = 0; g < kd->used ; g++) {
            if(kd->values[g].s)
                kd->values[g].s[0] = '\0';
        }
    }
}

// ----------------------------------------------------------------------------
// duplications

static inline void jb_send_duplications_for_key(struct log_job *jb, const char *key, XXH64_hash_t hash, const char *value, size_t value_len) {
    // IMPORTANT:
    // The 'value' may not be NULL terminated and have more data that the value we need

    for (size_t d = 0; d < jb->dups.used; d++) {
        struct key_dup *kd = &jb->dups.array[d];

        if(kd->exposed || kd->used == 0)
            continue;

        if(kd->used == 1) {
            // just one key to be duplicated
            if(strcmp(kd->keys[0], key) == 0) {
                send_key_value_and_rewrite(jb, kd->target, kd->hash, value, value_len);
                kd->exposed = true;
            }
        }
        else {
            // multiple keys to be duplicated
            for(size_t g = 0; g < kd->used ; g++) {
                if(strcmp(kd->keys[g], key) == 0)
                    txt_replace(&kd->values[g], value, value_len);
            }
        }
    }
}

static inline void jb_send_remaining_duplications(struct log_job *jb) {
    static __thread char buffer[MAX_VALUE_LEN + 1];

    // IMPORTANT:
    // all duplications are exposed, even the ones we haven't found their keys in the source,
    // so that the output always has the same fields for matched entries.

    for(size_t d = 0; d < jb->dups.used ; d++) {
        struct key_dup *kd = &jb->dups.array[d];

        if(kd->exposed || kd->used == 0)
            continue;

        buffer[0] = '\0';
        size_t remaining = sizeof(buffer);
        char *s = buffer;

        for(size_t g = 0; g < kd->used ; g++) {
            if(remaining < 2) {
                log2stderr("Warning: duplicated key '%s' cannot fit the values.", kd->target);
                break;
            }

            if(g > 0) {
                *s++ = ',';
                *s = '\0';
                remaining--;
            }

            char *value = (kd->values[g].s && kd->values[g].s[0]) ? kd->values[g].s : "[unavailable]";
            size_t len = strlen(value);
            size_t copied = copy_to_buffer(s, remaining, value, len);
            remaining -= copied;
            s += copied;

            if(copied != len) {
                log2stderr("Warning: duplicated key '%s' will have truncated value", jb->dups.array[d].target);
                break;
            }
        }
        send_key_value_and_rewrite(jb, kd->target, kd->hash, buffer, s - buffer);
    }
}

// ----------------------------------------------------------------------------
// filename injection

static inline void jb_inject_filename(struct log_job *jb) {
    if (jb->filename.key && jb->filename.current[0])
        send_key_value_constant(jb, jb->filename.key, jb->filename.current);
}

static inline bool jb_switched_filename(struct log_job *jb, const char *line, size_t len) {
    // IMPORTANT:
    // Return TRUE when the caller should skip this line (because it is ours).
    // Unfortunately, we have to consume empty lines too.

    // IMPORTANT:
    // filename may not be NULL terminated and have more data than the filename.

    if (!len) {
        jb->filename.last_line_was_empty = true;
        return true;
    }

    // Check if it's a log file change line
    if (jb->filename.last_line_was_empty && line[0] == '=' && strncmp(line, "==> ", 4) == 0) {
        const char *start = line + 4;
        const char *end = strstr(line, " <==");
        while (*start == ' ') start++;
        if (*start != '\n' && *start != '\0' && end) {
            copy_to_buffer(jb->filename.current, sizeof(jb->filename.current),
                           start, end - start);
            return true;
        }
    }

    jb->filename.last_line_was_empty = false;
    return false;
}

// ----------------------------------------------------------------------------
// input reading

static char *get_next_line(struct log_job *jb, char *buffer, size_t size, size_t *line_length) {
    if(!fgets(buffer, (int)size, stdin)) {
        *line_length = 0;
        return NULL;
    }

    char *line = buffer;
    size_t len = strlen(line);

    // remove trailing newlines and spaces
    while(len > 1 && (line[len - 1] == '\n' || isspace(line[len - 1])))
        line[--len] = '\0';

    // skip leading spaces
    while(isspace(*line)) {
        line++;
        len--;
    }

    *line_length = len;
    return line;
}

// ----------------------------------------------------------------------------

static inline void jb_traverse_pcre2_named_groups_and_send_keys(struct log_job *jb, pcre2_code *re, pcre2_match_data *match_data, char *line) {
    PCRE2_SIZE *ovector = pcre2_get_ovector_pointer(match_data);
    uint32_t namecount;
    pcre2_pattern_info(re, PCRE2_INFO_NAMECOUNT, &namecount);

    if (namecount > 0) {
        PCRE2_SPTR name_table;
        pcre2_pattern_info(re, PCRE2_INFO_NAMETABLE, &name_table);
        uint32_t name_entry_size;
        pcre2_pattern_info(re, PCRE2_INFO_NAMEENTRYSIZE, &name_entry_size);

        const unsigned char *tabptr = name_table;
        for (uint32_t i = 0; i < namecount; i++) {
            int n = (tabptr[0] << 8) | tabptr[1];
            const char *group_name = (const char *)(tabptr + 2);

            PCRE2_SIZE start_offset = ovector[2 * n];
            PCRE2_SIZE end_offset = ovector[2 * n + 1];
            PCRE2_SIZE group_length = end_offset - start_offset;

            XXH64_hash_t hash = XXH3_64bits(group_name, strlen(group_name));

            send_key_value_and_rewrite(jb, group_name, hash, line + start_offset, group_length);

            // process the duplications
            jb_send_duplications_for_key(jb, group_name, hash, line + start_offset, group_length);

            tabptr += name_entry_size;
        }

        // print all non-exposed duplications
        jb_send_remaining_duplications(jb);
    }
}

// ----------------------------------------------------------------------------

struct log_job log_job = { 0 };
int main(int argc, char *argv[]) {
    struct log_job *jb = &log_job;

    if(!parse_parameters(jb, argc, argv))
        exit(1);

    jb_select_which_injections_should_be_injected_on_unmatched(jb);

    pcre2_code *re = jb_compile_pcre2_pattern(jb->pattern);
    pcre2_match_data *match_data = pcre2_match_data_create_from_pattern(re, NULL);

    char buffer[MAX_LINE_LENGTH];
    char *line;
    size_t len;

    while ((line = get_next_line(jb, buffer, sizeof(buffer), &len))) {
        if(jb_switched_filename(jb, line, len))
            continue;

        jb_reset_injections(jb);

        bool line_is_matched;
        if(!jb_pcre2_match(re, match_data, line, len, true)) {
            line_is_matched = false;

            if (jb->unmatched.key) {
                // we are sending errors to Journal
                send_key_value_error(jb->unmatched.key, "PCRE2 error on: %s", line);

                for (size_t j = 0; j < jb->unmatched.injections.used; j++)
                    send_key_value_constant(jb, jb->unmatched.injections.keys[j].key,
                                            jb->unmatched.injections.keys[j].value.s);
            }
            else {
                // we are just logging errors to stderr
                continue;
            }
        }
        else {
            line_is_matched = true;
            jb_traverse_pcre2_named_groups_and_send_keys(jb, re, match_data, line);
        }

        jb_inject_filename(jb);
        jb_finalize_injections(jb, line_is_matched);

        printf("\n");
        fflush(stdout);
    }

    // Release memory used for the compiled regular expression and match data
    pcre2_match_data_free(match_data);
    pcre2_code_free(re);
    jb_cleanup(jb);

    return 0;
}
