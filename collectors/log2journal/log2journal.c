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

#ifdef HAVE_LIBYAML
#include <yaml.h>
#endif

#define MAX_OUTPUT_KEYS 1024
#define OVECCOUNT (MAX_OUTPUT_KEYS * 3)    // should be a multiple of 3
#define MAX_LINE_LENGTH (1024 * 1024)
#define MAX_KEY_DUPS (MAX_OUTPUT_KEYS / 2)
#define MAX_INJECTIONS (MAX_OUTPUT_KEYS / 2)
#define MAX_REWRITES (MAX_OUTPUT_KEYS / 2)
#define MAX_KEY_DUPS_KEYS 20

#define MAX_KEY_LEN 64              // according to systemd-journald
#define MAX_VALUE_LEN (48 * 1024)   // according to systemd-journald

struct key_rewrite;
static pcre2_code *jb_compile_pcre2_pattern(const char *pattern);
static bool parse_replacement_pattern(struct key_rewrite *rw);

#define YAML_CONFIG_NGINX_COMBINED \
    "# Netdata log2journal Configuration Template\n" \
    "# The following parses nginx log files using the combined format.\n" \
    "\n" \
    "# The PCRE2 pattern to match log entries and give names to the fields.\n" \
    "# The journal will have these names, so follow their rules. You can\n" \
    "# initiate an extended PCRE2 pattern by starting the pattern with (?x)\n" \
    "pattern: |\n" \
    "  (?x)                                   # Enable PCRE2 extended mode\n" \
    "  ^\n" \
    "  (?<NGINX_REMOTE_ADDR>[^ ]+) \\s - \\s    # NGINX_REMOTE_ADDR\n" \
    "  (?<NGINX_REMOTE_USER>[^ ]+) \\s         # NGINX_REMOTE_USER\n" \
    "  \\[\n" \
    "    (?<NGINX_TIME_LOCAL>[^\\]]+)          # NGINX_TIME_LOCAL\n" \
    "  \\]\n" \
    "  \\s+ \"\n" \
    "  (?<MESSAGE>\n" \
    "    (?<NGINX_METHOD>[A-Z]+) \\s+          # NGINX_METHOD\n" \
    "    (?<NGINX_URL>[^ ]+) \\s+\n" \
    "    HTTP/(?<NGINX_HTTP_VERSION>[^\"]+)\n" \
    "  )\n" \
    "  \" \\s+\n" \
    "  (?<NGINX_STATUS>\\d+) \\s+               # NGINX_STATUS\n" \
    "  (?<NGINX_BODY_BYTES_SENT>\\d+) \\s+      # NGINX_BODY_BYTES_SENT\n" \
    "  \"(?<NGINX_HTTP_REFERER>[^\"]*)\" \\s+     # NGINX_HTTP_REFERER\n" \
    "  \"(?<NGINX_HTTP_USER_AGENT>[^\"]*)\"      # NGINX_HTTP_USER_AGENT\n" \
    "\n" \
    "# When log2journal can detect the filename of each log entry (tail gives it\n" \
    "# only when it tails multiple files), this key will be used to send the\n" \
    "# filename to the journals.\n" \
    "filename:\n" \
    "  key: NGINX_LOG_FILENAME\n" \
    "\n" \
    "# Duplicate fields under a different name. You can duplicate multiple fields\n" \
    "# to a new one and then use rewrite rules to change its value.\n" \
    "duplicate:\n" \
    "\n" \
    "  # we insert the field PRIORITY as a copy of NGINX_STATUS.\n" \
    "  - key: PRIORITY\n" \
    "    values_of:\n" \
    "    - NGINX_STATUS\n" \
    "\n" \
    "  # we inject the field NGINX_STATUS_FAMILY as a copy of NGINX_STATUS.\n" \
    "  - key: NGINX_STATUS_FAMILY\n" \
    "    values_of: \n" \
    "    - NGINX_STATUS\n" \
    "\n" \
    "# Inject constant fields into the journal logs.\n" \
    "inject:\n" \
    "  - key: SYSLOG_IDENTIFIER\n" \
    "    value: \"nginx-log\"\n" \
    "\n" \
    "# Rewrite the value of fields (including the duplicated ones).\n" \
    "# The search pattern can have named groups, and the replace pattern can use\n" \
    "# them as ${name}.\n" \
    "rewrite:\n" \
    "  # PRIORTY is a duplicate of NGINX_STATUS\n" \
    "  # Valid PRIORITIES: 0=emerg, 1=alert, 2=crit, 3=error, 4=warn, 5=notice, 6=info, 7=debug\n" \
    "  - key: \"PRIORITY\"\n" \
    "    search: \"^[123]\"\n" \
    "    replace: 6\n" \
    "\n" \
    "  - key: \"PRIORITY\"\n" \
    "    search: \"^4\"\n" \
    "    replace: 5\n" \
    "\n" \
    "  - key: \"PRIORITY\"\n" \
    "    search: \"^5\"\n" \
    "    replace: 3\n" \
    "\n" \
    "  - key: \"PRIORITY\"\n" \
    "    search: \".*\"\n" \
    "    replace: 4\n" \
    "  \n" \
    "  # NGINX_STATUS_FAMILY is a duplicate of NGINX_STATUS\n" \
    "  - key: \"NGINX_STATUS_FAMILY\"\n" \
    "    search: \"^(?<first_digit>[1-5])\"\n" \
    "    replace: \"${first_digit}xx\"\n" \
    "\n" \
    "  - key: \"NGINX_STATUS_FAMILY\"\n" \
    "    search: \".*\"\n" \
    "    replace: \"UNKNOWN\"\n" \
    "\n" \
    "# Control what to do when input logs do not match the main PCRE2 pattern.\n" \
    "unmatched:\n" \
    "  # The journal key to log the PCRE2 error message to.\n" \
    "  # Set this to MESSAGE, so you to see the error in the log.\n" \
    "  key: MESSAGE\n" \
    "  \n" \
    "  # Inject static fields to the unmatched entries.\n" \
    "  # Set PRIORITY=1 (alert) to help you spot unmatched entries in the logs.\n" \
    "  inject:\n" \
    "   - key: PRIORITY\n" \
    "     value: 1\n" \
    "\n"

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
    printf("  --file /path/to/file.yaml\n");
    printf("       Read yaml configuration file for instructions.\n");
    printf("\n");
    printf("  --config CONFIG_NAME\n");
    printf("       Run with the internal configuration named CONFIG_NAME\n");
    printf("       Available internal configs: nginx-combined\n");
    printf("\n");
    printf("  --show-config\n");
    printf("       Show the configuration in yaml format before starting the job.\n");
    printf("       This is also an easy way to convert command line parameters to yaml.\n");
    printf("\n");
    printf("  --filename-key KEY\n");
    printf("       Add a field with KEY as the key and the current filename as value.\n");
    printf("       Automatically detects filenames when piped after 'tail -F',\n");
    printf("       and tail matches multiple filenames.\n");
    printf("       To inject the filename when tailing a single file, use --inject.\n");
    printf("\n");
    printf("  --unmatched-key KEY\n");
    printf("       Include unmatched log entries in the output with KEY as the field name.\n");
    printf("       Use this to include unmatched entries to the output stream.\n");
    printf("       Usually it should be set to --unmatched-key=MESSAGE so that the\n");
    printf("       unmatched entry will appear as the log message in the journals.\n");
    printf("       Use --inject-unmatched to inject additional fields to unmatched lines.\n");
    printf("\n");
    printf("  --duplicate TARGET=KEY1[,KEY2[,KEY3[,...]]\n");
    printf("       Create a new key called TARGET, duplicating the values of the keys\n");
    printf("       given. Useful for further processing. When multiple keys are given,\n");
    printf("       their values are separated by comma.\n");
    printf("       Up to %d duplications can be given on the command line, and up to\n", MAX_KEY_DUPS);
    printf("       %d keys per duplication command are allowed.\n", MAX_KEY_DUPS_KEYS);
    printf("\n");
    printf("  --inject LINE\n");
    printf("       Inject constant fields to the output (both matched and unmatched logs).\n");
    printf("       --inject entries are added to unmatched lines too, when their key is\n");
    printf("       not used in --inject-unmatched (--inject-unmatched override --inject).\n");
    printf("       Up to %d fields can be injected.\n", MAX_INJECTIONS);
    printf("\n");
    printf("  --inject-unmatched LINE\n");
    printf("       Inject lines into the output for each unmatched log entry.\n");
    printf("       Usually, --inject-unmatched=PRIORITY=3 is needed to mark the unmatched\n");
    printf("       lines as errors, so that they can easily be spotted in the journals.\n");
    printf("       Up to %d such lines can be injected.\n", MAX_INJECTIONS);
    printf("\n");
    printf("  --rewrite KEY=/SearchPattern/ReplacePattern\n");
    printf("       Apply a rewrite rule to the values of a specific key.\n");
    printf("       The first character after KEY= is the separator, which should also\n");
    printf("       be used between the search pattern and the replacement pattern.\n");
    printf("       The search pattern is a PCRE2 regular expression, and the replacement\n");
    printf("       pattern supports literals and named capture groups from the search pattern.\n");
    printf("       Example:\n");
    printf("              --rewrite DATE=/^(?<year>\\d{4})-(?<month>\\d{2})-(?<day>\\d{2})$/\n");
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
    printf("The program accepts all parameters as both --option=value and --option value.\n");
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
    printf("Example YAML file:\n\n"
           "--------------------------------------------------------------------------------\n"
           "%s"
           "--------------------------------------------------------------------------------\n"
           "\n",
           YAML_CONFIG_NGINX_COMBINED);
}

// ----------------------------------------------------------------------------
// logging

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
// allocation functions abstraction

void *mallocz(size_t size) {
    void *ptr = malloc(size);
    if (!ptr) {
        log2stderr("Fatal Error: Memory allocation failed. Requested size: %zu bytes.", size);
        exit(EXIT_FAILURE);
    }
    return ptr;
}

char *strdupz(const char *s) {
    char *ptr = strdup(s);
    if (!ptr) {
        log2stderr("Fatal Error: Memory allocation failed in strdup.");
        exit(EXIT_FAILURE);
    }
    return ptr;
}

char *strndupz(const char *s, size_t n) {
    char *ptr = strndup(s, n);
    if (!ptr) {
        log2stderr("Fatal Error: Memory allocation failed in strndup. Requested size: %zu bytes.", n);
        exit(EXIT_FAILURE);
    }
    return ptr;
}

void freez(void *ptr) {
    if (ptr)
        free(ptr);
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
    bool show_config;

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

static bool log_job_add_filename_key(struct log_job *jb, const char *key, size_t key_len) {
    if(!key || !*key) {
        log2stderr("filename key cannot be empty.");
        return false;
    }

    if(jb->filename.key)
        freez((char*)jb->filename.key);

    jb->filename.key = strndupz(key, key_len);

    return true;
}

static bool log_job_add_injection(struct log_job *jb, const char *key, size_t key_len, const char *value, size_t value_len, bool unmatched) {
    if (unmatched) {
        if (jb->unmatched.injections.used >= MAX_INJECTIONS) {
            log2stderr("Error: too many unmatched injections. You can inject up to %d lines.", MAX_INJECTIONS);
            return false;
        }
    }
    else {
        if (jb->injections.used >= MAX_INJECTIONS) {
            log2stderr("Error: too many injections. You can inject up to %d lines.", MAX_INJECTIONS);
            return false;
        }
    }

    if (unmatched) {
        key_value_replace(&jb->unmatched.injections.keys[jb->unmatched.injections.used++],
                          key, key_len,
                          value, value_len);
    } else {
        key_value_replace(&jb->injections.keys[jb->injections.used++],
                          key, key_len,
                          value, value_len);
    }

    return true;
}

static bool log_job_add_rewrite(struct log_job *jb, const char *key, const char *search_pattern, const char *replace_pattern) {
    pcre2_code *re = jb_compile_pcre2_pattern(search_pattern);
    if (!re) {
        return false;
    }

    struct key_rewrite *rw = &jb->rewrites.array[jb->rewrites.used++];
    rw->key = strdupz(key);
    rw->hash = XXH3_64bits(rw->key, strlen(rw->key));
    rw->search_pattern = strdupz(search_pattern);
    rw->replace_pattern = strdupz(replace_pattern);
    rw->re = re;
    rw->match_data = pcre2_match_data_create_from_pattern(rw->re, NULL);

    // Parse the replacement pattern and create the linked list
    if (!parse_replacement_pattern(rw)) {
        pcre2_match_data_free(rw->match_data);
        pcre2_code_free(rw->re);
        freez(rw->key);
        freez(rw->search_pattern);
        freez(rw->replace_pattern);
        jb->rewrites.used--;
        return false;
    }

    return true;
}

void jb_cleanup(struct log_job *jb) {
    for(size_t i = 0; i < jb->injections.used ;i++) {
        if(jb->injections.keys[i].value.s)
            freez(jb->injections.keys[i].value.s);
    }

    for(size_t i = 0; i < jb->unmatched.injections.used ;i++) {
        if(jb->unmatched.injections.keys[i].value.s)
            freez(jb->unmatched.injections.keys[i].value.s);
    }

    for(size_t i = 0; i < jb->dups.used ;i++) {
        struct key_dup *kd = &jb->dups.array[i];

        if(kd->target)
            freez(kd->target);

        for(size_t j = 0; j < kd->used ; j++) {
            if (kd->keys[j])
                freez(kd->keys[j]);

            if (kd->values[j].s)
                freez(kd->values[j].s);
        }
    }

    for(size_t i = 0; i < jb->rewrites.used; i++) {
        struct key_rewrite *rw = &jb->rewrites.array[i];

        if (rw->key)
            freez(rw->key);

        if (rw->search_pattern)
            freez(rw->search_pattern);

        if (rw->replace_pattern)
            freez(rw->replace_pattern);

        if(rw->match_data)
            pcre2_match_data_free(rw->match_data);

        if (rw->re)
            pcre2_code_free(rw->re);

        // Cleanup for replacement nodes linked list
        struct replacement_node *current = rw->nodes;
        while (current != NULL) {
            struct replacement_node *next = current->next;

            if (current->s)
                freez((void *)current->s);

            freez(current);
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

static struct key_dup *add_duplicate_target_to_job(struct log_job *jb, const char *target, size_t target_len) {
    if (jb->dups.used >= MAX_KEY_DUPS) {
        log2stderr("Error: Too many duplicates defined. Maximum allowed is %d.", MAX_KEY_DUPS);
        return NULL;
    }

    struct key_dup *kd = &jb->dups.array[jb->dups.used++];
    kd->target = strndupz(target, target_len);
    kd->hash = XXH3_64bits(kd->target, target_len);
    kd->used = 0;
    kd->exposed = false;

    // Initialize values array
    for (size_t i = 0; i < MAX_KEY_DUPS_KEYS; i++) {
        kd->values[i].s = NULL;
        kd->values[i].size = 0;
    }

    return kd;
}

static bool add_key_to_duplicate(struct key_dup *kd, const char *key, size_t key_len) {
    if (kd->used >= MAX_KEY_DUPS_KEYS) {
        log2stderr("Error: Too many keys in duplication of target '%s'.", kd->target);
        return false;
    }

    kd->keys[kd->used++] = strndupz(key, key_len);
    return true;
}

// ----------------------------------------------------------------------------
// yaml configuration file

#ifdef HAVE_LIBYAML


// ----------------------------------------------------------------------------
// yaml library functions

static const char *yaml_event_name(yaml_event_type_t type) {
    switch (type) {
        case YAML_NO_EVENT:
            return "YAML_NO_EVENT";

        case YAML_SCALAR_EVENT:
            return "YAML_SCALAR_EVENT";

        case YAML_ALIAS_EVENT:
            return "YAML_ALIAS_EVENT";

        case YAML_MAPPING_START_EVENT:
            return "YAML_MAPPING_START_EVENT";

        case YAML_MAPPING_END_EVENT:
            return "YAML_MAPPING_END_EVENT";

        case YAML_SEQUENCE_START_EVENT:
            return "YAML_SEQUENCE_START_EVENT";

        case YAML_SEQUENCE_END_EVENT:
            return "YAML_SEQUENCE_END_EVENT";

        case YAML_STREAM_START_EVENT:
            return "YAML_STREAM_START_EVENT";

        case YAML_STREAM_END_EVENT:
            return "YAML_STREAM_END_EVENT";

        case YAML_DOCUMENT_START_EVENT:
            return "YAML_DOCUMENT_START_EVENT";

        case YAML_DOCUMENT_END_EVENT:
            return "YAML_DOCUMENT_END_EVENT";

        default:
            return "UNKNOWN";
    }
}

#define yaml_error(parser, event, fmt, args...) yaml_error_with_trace(parser, event, __LINE__, __FUNCTION__, __FILE__, fmt, ##args)
static void yaml_error_with_trace(yaml_parser_t *parser, yaml_event_t *event, size_t line, const char *function, const char *file, const char *format, ...) __attribute__ ((format(__printf__, 6, 7)));
static void yaml_error_with_trace(yaml_parser_t *parser, yaml_event_t *event, size_t line, const char *function, const char *file, const char *format, ...) {
    char buf[1024] = ""; // Initialize buf to an empty string
    const char *type = "";

    if(event) {
        type = yaml_event_name(event->type);

        switch (event->type) {
            case YAML_SCALAR_EVENT:
                copy_to_buffer(buf, sizeof(buf), (char *)event->data.scalar.value, event->data.scalar.length);
                break;

            case YAML_ALIAS_EVENT:
                snprintf(buf, sizeof(buf), "%s", event->data.alias.anchor);
                break;

            default:
                break;
        }
    }

    fprintf(stderr, "YAML %zu@%s, %s(): (line %d, column %d, %s%s%s): ",
            line, file, function,
            (int)(parser->mark.line + 1), (int)(parser->mark.column + 1),
            type, buf[0]? ", near ": "", buf);

    va_list args;
    va_start(args, format);
    vfprintf(stderr, format, args);
    va_end(args);
    fprintf(stderr, "\n");
}

#define yaml_parse(parser, event) yaml_parse_with_trace(parser, event, __LINE__, __FUNCTION__, __FILE__)
static bool yaml_parse_with_trace(yaml_parser_t *parser, yaml_event_t *event, size_t line, const char *function, const char *file) {
    if (!yaml_parser_parse(parser, event)) {
        yaml_error(parser, NULL, "YAML parser error %d", parser->error);
        return false;
    }

//    fprintf(stderr, ">>> %s >>> %.*s\n",
//            yaml_event_name(event->type),
//            event->type == YAML_SCALAR_EVENT ? event->data.scalar.length : 0,
//            event->type == YAML_SCALAR_EVENT ? (char *)event->data.scalar.value : "");

    return true;
}

#define yaml_parse_expect_event(parser, type) yaml_parse_expect_event_with_trace(parser, type, __LINE__, __FUNCTION__, __FILE__)
static bool yaml_parse_expect_event_with_trace(yaml_parser_t *parser, yaml_event_type_t type, size_t line, const char *function, const char *file) {
    yaml_event_t event;
    if (!yaml_parse(parser, &event))
        return false;

    bool ret = true;
    if(event.type != type) {
        yaml_error_with_trace(parser, &event, line, function, file, "unexpected event - expecting: %s", yaml_event_name(type));
        ret = false;
    }
//    else
//        fprintf(stderr, "OK (%zu@%s, %s()\n", line, file, function);

    yaml_event_delete(&event);
    return ret;
}

#define yaml_scalar_matches(event, s, len) yaml_scalar_matches_with_trace(event, s, len, __LINE__, __FUNCTION__, __FILE__)
static bool yaml_scalar_matches_with_trace(yaml_event_t *event, const char *s, size_t len, size_t line __maybe_unused, const char *function __maybe_unused, const char *file __maybe_unused) {
    if(event->type != YAML_SCALAR_EVENT)
        return false;

    if(len != event->data.scalar.length)
        return false;
//    else
//        fprintf(stderr, "OK (%zu@%s, %s()\n", line, file, function);

    return strcmp((char *)event->data.scalar.value, s) == 0;
}

// ----------------------------------------------------------------------------

static struct key_dup *yaml_parse_duplicate_key(struct log_job *jb, yaml_parser_t *parser) {
    yaml_event_t event;

    if (!yaml_parse(parser, &event))
        return false;

    struct key_dup *kd = NULL;
    if(event.type == YAML_SCALAR_EVENT) {
        kd = add_duplicate_target_to_job(jb, (char *)event.data.scalar.value, event.data.scalar.length);
    }
    else
        yaml_error(parser, &event, "duplicate key must be a scalar.");

    yaml_event_delete(&event);
    return kd;
}

static size_t yaml_parse_duplicate_from(struct log_job *jb, yaml_parser_t *parser, struct key_dup *kd) {
    size_t errors = 0;
    yaml_event_t event;

    if (!yaml_parse(parser, &event))
        return 1;

    bool ret = true;
    if(event.type == YAML_SCALAR_EVENT)
        ret = add_key_to_duplicate(kd, (char *)event.data.scalar.value, event.data.scalar.length);

    else if(event.type == YAML_SEQUENCE_START_EVENT) {
        bool finished = false;
        while(!errors && !finished) {
            yaml_event_t sub_event;
            if (!yaml_parse(parser, &sub_event))
                return errors++;
            else {
                if (sub_event.type == YAML_SCALAR_EVENT)
                    add_key_to_duplicate(kd, (char *)sub_event.data.scalar.value, sub_event.data.scalar.length);

                else if (sub_event.type == YAML_SEQUENCE_END_EVENT)
                    finished = true;

                yaml_event_delete(&sub_event);
            }
        }
    }
    else
        yaml_error(parser, &event, "not expected event type");

    yaml_event_delete(&event);
    return errors;
}

static size_t yaml_parse_filename_injection(yaml_parser_t *parser, struct log_job *jb) {
    yaml_event_t event;
    size_t errors = 0;

    if(!yaml_parse_expect_event(parser, YAML_MAPPING_START_EVENT))
        return 1;

    if (!yaml_parse(parser, &event))
        return 1;

    if (yaml_scalar_matches(&event, "key", strlen("key"))) {
        yaml_event_t sub_event;
        if (!yaml_parse(parser, &sub_event))
            errors++;

        else {
            if (event.type == YAML_SCALAR_EVENT) {
                if(!log_job_add_filename_key(jb, (char *)sub_event.data.scalar.value, sub_event.data.scalar.length))
                    errors++;
            }

            else {
                yaml_error(parser, &sub_event, "expected the filename as %s", yaml_event_name(YAML_SCALAR_EVENT));
                errors++;
            }

            yaml_event_delete(&sub_event);
        }
    }

    if(!yaml_parse_expect_event(parser, YAML_MAPPING_END_EVENT))
        errors++;

    yaml_event_delete(&event);
    return errors;
}

static size_t yaml_parse_duplicates_injection(yaml_parser_t *parser, struct log_job *jb) {
    if (!yaml_parse_expect_event(parser, YAML_SEQUENCE_START_EVENT))
        return 1;

    struct key_dup *kd = NULL;

    // Expecting a key-value pair for each duplicate
    bool finished;
    size_t errors = 0;
    while (!errors && !finished) {
        yaml_event_t event;
        if (!yaml_parse(parser, &event)) {
            errors++;
            break;
        }

        if(event.type == YAML_MAPPING_START_EVENT) {
            ;
        }
        if (event.type == YAML_SEQUENCE_END_EVENT) {
            finished = true;
        }
        else if(event.type == YAML_SCALAR_EVENT) {
            if (yaml_scalar_matches(&event, "key", strlen("key"))) {
                kd = yaml_parse_duplicate_key(jb, parser);
                if (!kd)
                    errors++;
                else {
                    while (!errors && kd) {
                        yaml_event_t sub_event;
                        if (!yaml_parse(parser, &sub_event)) {
                            errors++;
                            break;
                        }

                        if (sub_event.type == YAML_MAPPING_END_EVENT) {
                            kd = NULL;
                        } else if (sub_event.type == YAML_SCALAR_EVENT) {
                            if (yaml_scalar_matches(&sub_event, "values_of", strlen("values_of"))) {
                                if (!kd) {
                                    yaml_error(parser, &sub_event, "Found 'values_of' but the 'key' is not set.");
                                    errors++;
                                } else
                                    errors += yaml_parse_duplicate_from(jb, parser, kd);
                            } else {
                                yaml_error(parser, &sub_event, "unknown scalar");
                                errors++;
                            }
                        } else {
                            yaml_error(parser, &sub_event, "unexpected event type");
                            errors++;
                        }

                        // Delete the event after processing
                        yaml_event_delete(&event);
                    }
                }
            } else {
                yaml_error(parser, &event, "unknown scalar");
                errors++;
            }
        }

        yaml_event_delete(&event);
    }

    return errors;
}

static bool yaml_parse_constant_field_injection(yaml_parser_t *parser, struct log_job *jb, bool unmatched) {
    yaml_event_t event;
    if (!yaml_parse(parser, &event) || event.type != YAML_SCALAR_EVENT) {
        yaml_error(parser, &event, "Expected scalar for constant field injection key");
        yaml_event_delete(&event);
        return false;
    }

    char *key = strndupz((char *)event.data.scalar.value, event.data.scalar.length);
    char *value = NULL;
    bool ret = false;

    yaml_event_delete(&event);

    if (!yaml_parse(parser, &event) || event.type != YAML_SCALAR_EVENT) {
        yaml_error(parser, &event, "Expected scalar for constant field injection value");
        goto cleanup;
    }

    if(!yaml_scalar_matches(&event, "value", strlen("value"))) {
        yaml_error(parser, &event, "Expected scalar 'value'");
        goto cleanup;
    }

    if (!yaml_parse(parser, &event) || event.type != YAML_SCALAR_EVENT) {
        yaml_error(parser, &event, "Expected scalar for constant field injection value");
        goto cleanup;
    }

    value = strndupz((char *)event.data.scalar.value, event.data.scalar.length);

    if(!log_job_add_injection(jb, key, strlen(key), value, strlen(value), unmatched))
        ret = false;
    else
        ret = true;

    ret = true;

cleanup:
    yaml_event_delete(&event);
    freez(key);
    freez(value);
    return !ret ? 1 : 0;
}

static bool yaml_parse_injection_mapping(yaml_parser_t *parser, struct log_job *jb, bool unmatched) {
    yaml_event_t event;
    size_t errors = 0;
    bool finished = false;

    while (!errors && !finished) {
        if (!yaml_parse(parser, &event)) {
            errors++;
            continue;
        }

        switch (event.type) {
            case YAML_SCALAR_EVENT:
                if (yaml_scalar_matches(&event, "key", strlen("key"))) {
                    errors += yaml_parse_constant_field_injection(parser, jb, unmatched);
                } else {
                    yaml_error(parser, &event, "Unexpected scalar in injection mapping");
                    errors++;
                }
                break;

            case YAML_MAPPING_END_EVENT:
                finished = true;
                break;

            default:
                yaml_error(parser, &event, "Unexpected event in injection mapping");
                errors++;
                break;
        }

        yaml_event_delete(&event);
    }

    return errors == 0;
}

static size_t yaml_parse_injections(yaml_parser_t *parser, struct log_job *jb, bool unmatched) {
    yaml_event_t event;
    size_t errors = 0;
    bool finished = false;

    if (!yaml_parse_expect_event(parser, YAML_SEQUENCE_START_EVENT))
        return 1;

    while (!errors && !finished) {
        if (!yaml_parse(parser, &event)) {
            errors++;
            continue;
        }

        switch (event.type) {
            case YAML_MAPPING_START_EVENT:
                if (!yaml_parse_injection_mapping(parser, jb, unmatched))
                    errors++;
                break;

            case YAML_SEQUENCE_END_EVENT:
                finished = true;
                break;

            default:
                yaml_error(parser, &event, "Unexpected event in injections sequence");
                errors++;
                break;
        }

        yaml_event_delete(&event);
    }

    return errors;
}

static size_t yaml_parse_unmatched(yaml_parser_t *parser, struct log_job *jb) {
    size_t errors = 0;
    bool finished = false;

    if (!yaml_parse_expect_event(parser, YAML_MAPPING_START_EVENT))
        return 1;

    while (!errors && !finished) {
        yaml_event_t event;
        if (!yaml_parse(parser, &event)) {
            errors++;
            continue;
        }

        switch (event.type) {
            case YAML_SCALAR_EVENT:
                if (yaml_scalar_matches(&event, "key", strlen("key"))) {
                    yaml_event_t sub_event;
                    if (!yaml_parse(parser, &sub_event)) {
                        errors++;
                    } else {
                        if (sub_event.type == YAML_SCALAR_EVENT) {
                            jb->unmatched.key = strndupz((char *)sub_event.data.scalar.value, sub_event.data.scalar.length);
                        } else {
                            yaml_error(parser, &sub_event, "expected a scalar value for 'key'");
                            errors++;
                        }
                        yaml_event_delete(&sub_event);
                    }
                } else if (yaml_scalar_matches(&event, "inject", strlen("inject"))) {
                    errors += yaml_parse_injections(parser, jb, true);
                } else {
                    yaml_error(parser, &event, "Unexpected scalar in unmatched section");
                    errors++;
                }
                break;

            case YAML_MAPPING_END_EVENT:
                finished = true;
                break;

            default:
                yaml_error(parser, &event, "Unexpected event in unmatched section");
                errors++;
                break;
        }

        yaml_event_delete(&event);
    }

    return errors;
}

static size_t yaml_parse_rewrites(yaml_parser_t *parser, struct log_job *jb) {
    size_t errors = 0;

    if (!yaml_parse_expect_event(parser, YAML_SEQUENCE_START_EVENT))
        return 1;

    bool finished = false;
    while (!errors && !finished) {
        yaml_event_t event;
        if (!yaml_parse(parser, &event)) {
            errors++;
            continue;
        }

        switch (event.type) {
            case YAML_MAPPING_START_EVENT:
            {
                struct key_rewrite rw = {0};

                bool mapping_finished = false;
                while (!errors && !mapping_finished) {
                    yaml_event_t sub_event;
                    if (!yaml_parse(parser, &sub_event)) {
                        errors++;
                        continue;
                    }

                    switch (sub_event.type) {
                        case YAML_SCALAR_EVENT:
                            if (yaml_scalar_matches(&sub_event, "key", strlen("key"))) {
                                if (!yaml_parse(parser, &sub_event) || sub_event.type != YAML_SCALAR_EVENT) {
                                    yaml_error(parser, &sub_event, "Expected scalar for rewrite key");
                                    errors++;
                                } else {
                                    rw.key = strndupz((char *)sub_event.data.scalar.value, sub_event.data.scalar.length);
                                    yaml_event_delete(&sub_event);
                                }
                            } else if (yaml_scalar_matches(&sub_event, "search", strlen("search"))) {
                                if (!yaml_parse(parser, &sub_event) || sub_event.type != YAML_SCALAR_EVENT) {
                                    yaml_error(parser, &sub_event, "Expected scalar for rewrite search pattern");
                                    errors++;
                                } else {
                                    rw.search_pattern = strndupz((char *)sub_event.data.scalar.value, sub_event.data.scalar.length);
                                    yaml_event_delete(&sub_event);
                                }
                            } else if (yaml_scalar_matches(&sub_event, "replace", strlen("replace"))) {
                                if (!yaml_parse(parser, &sub_event) || sub_event.type != YAML_SCALAR_EVENT) {
                                    yaml_error(parser, &sub_event, "Expected scalar for rewrite replace pattern");
                                    errors++;
                                } else {
                                    rw.replace_pattern = strndupz((char *)sub_event.data.scalar.value, sub_event.data.scalar.length);
                                    yaml_event_delete(&sub_event);
                                }
                            } else {
                                yaml_error(parser, &sub_event, "Unexpected scalar in rewrite mapping");
                                errors++;
                            }
                            break;

                        case YAML_MAPPING_END_EVENT:
                            if(rw.key && rw.search_pattern && rw.replace_pattern) {
                                if (!log_job_add_rewrite(jb, rw.key, rw.search_pattern, rw.replace_pattern))
                                    errors++;
                            }
                            freez(rw.key);
                            freez(rw.search_pattern);
                            freez(rw.replace_pattern);
                            memset(&rw, 0, sizeof(rw));

                            mapping_finished = true;
                            break;

                        default:
                            yaml_error(parser, &sub_event, "Unexpected event in rewrite mapping");
                            errors++;
                            break;
                    }

                    yaml_event_delete(&sub_event);
                }
            }
                break;

            case YAML_SEQUENCE_END_EVENT:
                finished = true;
                break;

            default:
                yaml_error(parser, &event, "Unexpected event in rewrites sequence");
                errors++;
                break;
        }

        yaml_event_delete(&event);
    }

    return errors;
}

static size_t yaml_parse_pattern(yaml_parser_t *parser, struct log_job *jb) {
    yaml_event_t event;
    size_t errors = 0;

    if (!yaml_parse(parser, &event))
        return 1;

    if(event.type == YAML_SCALAR_EVENT)
        jb->pattern = strndupz((char *)event.data.scalar.value, event.data.scalar.length);
    else {
        yaml_error(parser, &event, "unexpected event type");
        errors++;
    }

    yaml_event_delete(&event);
    return errors;
}

static size_t yaml_parse_initialized(yaml_parser_t *parser, struct log_job *jb) {
    size_t errors = 0;

    if(!yaml_parse_expect_event(parser, YAML_STREAM_START_EVENT)) {
        errors++;
        goto cleanup;
    }

    if(!yaml_parse_expect_event(parser, YAML_DOCUMENT_START_EVENT)) {
        errors++;
        goto cleanup;
    }

    if(!yaml_parse_expect_event(parser, YAML_MAPPING_START_EVENT)) {
        errors++;
        goto cleanup;
    }

    bool finished = false;
    while (!errors && !finished) {
        yaml_event_t event;
        if(!yaml_parse(parser, &event)) {
            errors++;
            continue;
        }

        switch(event.type) {
            default:
                yaml_error(parser, &event, "unexpected type");
                errors++;
                break;

            case YAML_MAPPING_END_EVENT:
                finished = true;
                break;

            case YAML_SCALAR_EVENT:
                if (yaml_scalar_matches(&event, "pattern", strlen("pattern")))
                    errors += yaml_parse_pattern(parser, jb);

                else if (yaml_scalar_matches(&event, "filename", strlen("filename")))
                    errors += yaml_parse_filename_injection(parser, jb);

                else if (yaml_scalar_matches(&event, "duplicate", strlen("duplicate")))
                    errors += yaml_parse_duplicates_injection(parser, jb);

                else if (yaml_scalar_matches(&event, "inject", strlen("inject")))
                    errors += yaml_parse_injections(parser, jb, false);

                else if (yaml_scalar_matches(&event, "unmatched", strlen("unmatched")))
                    errors += yaml_parse_unmatched(parser, jb);

                else if (yaml_scalar_matches(&event, "rewrite", strlen("rewrite")))
                    errors += yaml_parse_rewrites(parser, jb);

                else {
                    yaml_error(parser, &event, "unexpected scalar");
                    errors++;
                }
                break;
        }

        yaml_event_delete(&event);
    }

    if(!yaml_parse_expect_event(parser, YAML_DOCUMENT_END_EVENT)) {
        errors++;
        goto cleanup;
    }

    if(!yaml_parse_expect_event(parser, YAML_STREAM_END_EVENT)) {
        errors++;
        goto cleanup;
    }

cleanup:
    return errors;
}

static bool yaml_parse_file(const char *config_file_path, struct log_job *jb) {
    if(!config_file_path || !*config_file_path) {
        log2stderr("yaml configuration filename cannot be empty.");
        return false;
    }

    FILE *fp = fopen(config_file_path, "r");
    if (!fp) {
        log2stderr("Error opening config file: %s", config_file_path);
        return false;
    }

    yaml_parser_t parser;
    yaml_parser_initialize(&parser);
    yaml_parser_set_input_file(&parser, fp);

    size_t errors = yaml_parse_initialized(&parser, jb);

    yaml_parser_delete(&parser);
    fclose(fp);
    return errors == 0;
}

static bool yaml_parse_config(const char *config_name, struct log_job *jb) {

    const char *config = NULL;

    if(strcmp(config_name, "nginx-combined") == 0)
        config = YAML_CONFIG_NGINX_COMBINED;
    else {
        log2stderr("Unknown configuration: '%s'", config_name);
        return false;
    }

    yaml_parser_t parser;
    yaml_parser_initialize(&parser);
    yaml_parser_set_input_string(&parser, (const unsigned char *)config, strlen(config));

    size_t errors = yaml_parse_initialized(&parser, jb);

    yaml_parser_delete(&parser);
    return errors == 0;
}

#endif


// ----------------------------------------------------------------------------
// command line params

struct replacement_node *add_replacement_node(struct replacement_node **head, bool is_variable, const char *text) {
    struct replacement_node *new_node = mallocz(sizeof(struct replacement_node));
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

static bool parse_replacement_pattern(struct key_rewrite *rw) {
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
            char *variable_name = strndupz(current + 2, name_length);
            if (!variable_name) {
                log2stderr("Error: Memory allocation failed for variable name.");
                return false;
            }

            struct replacement_node *node = add_replacement_node(&(rw->nodes), true, variable_name);
            if (!node) {
                freez(variable_name);
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
            char *text = strndupz(start, text_length);
            if (!text) {
                log2stderr("Error: Memory allocation failed for literal text.");
                return false;
            }

            struct replacement_node *node = add_replacement_node(&(rw->nodes), false, text);
            if (!node) {
                freez(text);
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
    char *key = strndupz(param, equal_sign - param);
    char *search_pattern = strndupz(equal_sign + 2, second_separator - (equal_sign + 2));
    char *replace_pattern = strdupz(second_separator + 1);

    bool ret = log_job_add_rewrite(jb, key, search_pattern, replace_pattern);

    freez(key);
    freez(search_pattern);
    freez(replace_pattern);

    return ret;
}

static bool parse_inject(struct log_job *jb, const char *value, bool unmatched) {
    const char *equal = strchr(value, '=');
    if (!equal) {
        log2stderr("Error: injection '%s' does not have an equal sign.", value);
        return false;
    }

    const char *key = value;
    const char *val = equal + 1;
    log_job_add_injection(jb, key, equal - key, val, strlen(val), unmatched);

    return true;
}

static bool parse_duplicate(struct log_job *jb, const char *value) {
    const char *target = value;
    const char *equal_sign = strchr(value, '=');
    if (!equal_sign || equal_sign == target) {
        log2stderr("Error: Invalid duplicate format, '=' not found or at the start in %s", value);
        return false;
    }

    size_t target_len = equal_sign - target;
    struct key_dup *kd = add_duplicate_target_to_job(jb, target, target_len);
    if(!kd) return false;

    const char *key = equal_sign + 1;
    while (key) {
        if (kd->used >= MAX_KEY_DUPS_KEYS) {
            log2stderr("Error: too many keys in duplication of target '%s'.", kd->target);
            return false;
        }

        const char *comma = strchr(key, ',');
        size_t key_len;
        if (comma) {
            key_len = comma - key;
            add_key_to_duplicate(kd, key, key_len);
            key = comma + 1;
        }
        else {
            add_key_to_duplicate(kd, key, strlen(key));
            break;  // No more keys
        }
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
        else if (strcmp(arg, "--show-config") == 0) {
            jb->show_config = true;
        }
        else {
            char buffer[1024];
            char *param = NULL;
            char *value = NULL;

            char *equal_sign = strchr(arg, '=');
            if (equal_sign) {
                copy_to_buffer(buffer, sizeof(buffer), arg, equal_sign - arg);
                param = buffer;
                value = equal_sign + 1;
            }
            else {
                param = arg;
                if (i + 1 < argc) {
                    value = argv[++i];
                }
                else {
                    if (!jb->pattern) {
                        jb->pattern = arg;
                        continue;
                    } else {
                        log2stderr("Error: Multiple patterns detected. Specify only one pattern. The first is '%s', the second is '%s'", jb->pattern, arg);
                        return false;
                    }
                }
            }

            if (strcmp(param, "--filename-key") == 0) {
                if(!log_job_add_filename_key(jb, value, value ? strlen(value) : 0))
                    return false;
            }
#ifdef HAVE_LIBYAML
            else if (strcmp(param, "-f") == 0 || strcmp(param, "--file") == 0) {
                if (!yaml_parse_file(value, jb))
                    return false;
            }
            else if (strcmp(param, "--config") == 0) {
                if (!yaml_parse_config(value, jb))
                    return false;
            }
#endif
            else if (strcmp(param, "--unmatched-key") == 0)
                jb->unmatched.key = value;
            else if (strcmp(param, "--duplicate") == 0) {
                if (!parse_duplicate(jb, value))
                    return false;
            }
            else if (strcmp(param, "--inject") == 0) {
                if (!parse_inject(jb, value, false))
                    return false;
            }
            else if (strcmp(param, "--inject-unmatched") == 0) {
                if (!parse_inject(jb, value, true))
                    return false;
            }
            else if (strcmp(param, "--rewrite") == 0) {
                if (!parse_rewrite(jb, value))
                    return false;
            }
            else {
                if (!jb->pattern) {
                    jb->pattern = arg;
                    continue;
                } else {
                    log2stderr("Error: Multiple patterns detected. Specify only one pattern. The first is '%s', the second is '%s'", jb->pattern, arg);
                    return false;
                }
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

static void yaml_print_multiline_value(const char *s, size_t depth) {
    if (!s)
        s = "";

    do {
        const char* next = strchr(s, '\n');
        if(next) next++;

        size_t len = next ? (size_t)(next - s) : strlen(s);
        char buf[len + 1];
        strncpy(buf, s, len);
        buf[len] = '\0';

        fprintf(stderr, "%.*s%s%s",
                (int)(depth * 2), "                    ",
                buf, next ? "" : "\n");

        s = next;
    } while(s && *s);
}

static bool needs_quotes_in_yaml(const char *str) {
    // Lookup table for special YAML characters
    static bool special_chars[256] = { false };
    static bool table_initialized = false;

    if (!table_initialized) {
        // Initialize the lookup table
        const char *special_chars_str = ":{}[],&*!|>'\"%@`^";
        for (const char *c = special_chars_str; *c; ++c) {
            special_chars[(unsigned char)*c] = true;
        }
        table_initialized = true;
    }

    while (*str) {
        if (special_chars[(unsigned char)*str]) {
            return true;
        }
        str++;
    }
    return false;
}

static void yaml_print_node(const char *key, const char *value, size_t depth, bool dash) {
    if(depth > 10) depth = 10;
    const char *quote = "\"";

    const char *second_line = NULL;
    if(value && strchr(value, '\n')) {
        second_line = value;
        value = "|";
        quote = "";
    }
    else if(!value || !needs_quotes_in_yaml(value))
        quote = "";

    fprintf(stderr, "%.*s%s%s%s%s%s%s\n",
            (int)(depth * 2), "                    ", dash ? "- ": "",
            key ? key : "", key ? ": " : "",
            quote, value ? value : "", quote);

    if(second_line) {
        yaml_print_multiline_value(second_line, depth + 1);
    }
}

static void log_job_to_yaml(struct log_job *jb) {
    if(jb->pattern)
        yaml_print_node("pattern", jb->pattern, 0, false);

    if(jb->filename.key) {
        fprintf(stderr, "\n");
        yaml_print_node("filename", NULL, 0, false);
        yaml_print_node("key", jb->filename.key, 1, false);
    }

    if(jb->dups.used) {
        fprintf(stderr, "\n");
        yaml_print_node("duplicate", NULL, 0, false);
        for(size_t i = 0; i < jb->dups.used ;i++) {
            struct key_dup *kd = &jb->dups.array[i];
            yaml_print_node("key", kd->target, 1, true);
            yaml_print_node("values_of", NULL, 2, false);

            for(size_t k = 0; k < kd->used ;k++)
                yaml_print_node(NULL, kd->keys[k], 3, true);
        }
    }

    if(jb->injections.used) {
        fprintf(stderr, "\n");
        yaml_print_node("inject", NULL, 0, false);

        for (size_t i = 0; i < jb->injections.used; i++) {
            yaml_print_node("key", jb->injections.keys[i].key, 1, true);
            yaml_print_node("value", jb->injections.keys[i].value.s, 2, false);
        }
    }

    if(jb->rewrites.used) {
        fprintf(stderr, "\n");
        yaml_print_node("rewrite", NULL, 0, false);

        for(size_t i = 0; i < jb->rewrites.used ;i++) {
            yaml_print_node("key", jb->rewrites.array[i].key, 1, true);
            yaml_print_node("search", jb->rewrites.array[i].search_pattern, 2, false);
            yaml_print_node("replace", jb->rewrites.array[i].replace_pattern, 2, false);
        }
    }

    if(jb->unmatched.key || jb->unmatched.injections.used) {
        fprintf(stderr, "\n");
        yaml_print_node("unmatched", NULL, 0, false);

        if(jb->unmatched.key)
            yaml_print_node("key", jb->unmatched.key, 1, false);

        if(jb->unmatched.injections.used) {
            fprintf(stderr, "\n");
            yaml_print_node("inject", NULL, 1, false);

            for (size_t i = 0; i < jb->unmatched.injections.used; i++) {
                yaml_print_node("key", jb->unmatched.injections.keys[i].key, 2, true);
                yaml_print_node("value", jb->unmatched.injections.keys[i].value.s, 3, false);
            }
        }
    }
}

struct log_job log_job = { 0 };
int main(int argc, char *argv[]) {
    struct log_job *jb = &log_job;

    if(!parse_parameters(jb, argc, argv))
        exit(1);

    if(jb->show_config)
        log_job_to_yaml(jb);

    jb_select_which_injections_should_be_injected_on_unmatched(jb);

    pcre2_code *re = jb_compile_pcre2_pattern(jb->pattern);
    if(!re)
        return 1;

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
                // we are sending errors to systemd-journal
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
