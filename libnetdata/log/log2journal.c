// SPDX-License-Identifier: GPL-3.0-or-later

// only for PACKAGE_VERSION
#include "config.h"

#include <stdio.h>
#include <stdlib.h>
#include <string.h>
#include <stdbool.h>
#include <string.h>
#include <ctype.h>

#define PCRE2_CODE_UNIT_WIDTH 8
#include <pcre2.h>

#define OVECCOUNT (8192 * 3)    // should be a multiple of 3
#define MAX_LINE_LENGTH (1024 * 1024)
#define MAX_KEY_DUPS 2048
#define MAX_INJECTIONS 2048
#define MAX_KEY_DUPS_KEYS 10

void display_help(const char *name) {
    printf("\n");
    printf("Netdata log2journal " PACKAGE_VERSION "\n");
    printf("\n");
    printf("Convert structured log input to systemd Journal Export Format.\n");
    printf("\n");
    printf("Using PCRE2 patterns, extract the fields from structured logs on the standard\n");
    printf("input, and generate output according to systemd Journal Export Format\n");
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

struct txt {
    char *s;
    size_t len;
};

struct key_dup {
    char *target;
    char *keys[MAX_KEY_DUPS_KEYS];
    struct txt values[MAX_KEY_DUPS_KEYS];
    size_t used;
    bool exposed;
};

char *strdup_len(const char *src, size_t len) {
    char *s = malloc(len + 1);
    memcpy(s, src, len);
    s[len] = '\0';
    return s;
}

struct log_job {
    const char *pattern;

    struct {
        const char *key;
        char current[FILENAME_MAX + 1];
        bool last_line_was_empty;
    } filename;

    struct {
        const char *keys[MAX_INJECTIONS];
        bool on_unmatched_too[MAX_INJECTIONS];
        size_t used;
    } injections;

    struct {
        const char *key;
        struct {
            const char *keys[MAX_INJECTIONS];
            size_t used;
        } injections;
    } unmatched;

    struct {
        struct key_dup keys[MAX_KEY_DUPS];
        size_t used;
    } dups;
};

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
                fprintf(stderr, "Error: --duplicate=TARGET=KEY1,... is missing the equal sign.\n");
                return false;
            }
            const char *next_key = comma + 1;

            if (jb->dups.used >= MAX_KEY_DUPS) {
                fprintf(stderr, "Error: too many duplications. You can duplicate up to %d keys\n", MAX_KEY_DUPS);
                return false;
            }

            size_t first_key_len = comma - first_key;
            jb->dups.keys[jb->dups.used].target = strdup_len(first_key, first_key_len);
            jb->dups.keys[jb->dups.used].used = 0;

            while(next_key) {
                if(jb->dups.keys[jb->dups.used].used >= MAX_KEY_DUPS_KEYS) {
                    fprintf(stderr, "Error: too many keys in duplication of target '%s'.\n", jb->dups.keys[jb->dups.used].target);
                    return false;
                }

                first_key = next_key;
                comma = strchr(first_key, ',');

                if(comma) {
                    first_key_len = comma - first_key;
                    jb->dups.keys[jb->dups.used].keys[jb->dups.keys[jb->dups.used].used++] = strdup_len(first_key, first_key_len);
                    next_key = comma + 1;
                }
                else {
                    jb->dups.keys[jb->dups.used].keys[jb->dups.keys[jb->dups.used].used++] = strdup(first_key);
                    next_key = NULL;
                }
            }

            jb->dups.used++;
        }
        else if(strncmp(arg, "--inject=", 9) == 0) {
            if(jb->injections.used >= MAX_INJECTIONS) {
                fprintf(stderr, "Error: too many injections. You can inject up to %d lines\n", MAX_INJECTIONS);
                return false;
            }
            jb->injections.keys[jb->injections.used++] = strdup(arg + 9);
        }
        else if(strncmp(arg, "--inject-unmatched=", 19) == 0) {
            if(jb->unmatched.injections.used >= MAX_INJECTIONS) {
                fprintf(stderr, "Error: too many unmatched injections. You can inject up to %d lines\n", MAX_INJECTIONS);
                return false;
            }
            jb->unmatched.injections.keys[jb->unmatched.injections.used++] = strdup(arg + 19);
        }
        else {
            // Assume it's the pattern if not recognized as a parameter
            if (!jb->pattern) {
                jb->pattern = arg;
            } else {
                fprintf(stderr, "Error: Multiple patterns detected. Specify only one pattern.\n");
                return false;
            }
        }
    }

    // Check if a pattern is set and exactly one pattern is specified
    if (!jb->pattern) {
        fprintf(stderr, "Error: Pattern not specified.\n");
        display_help(argv[0]);
        return false;
    }

    return true;
}

static void jb_select_which_injections_should_be_injected_on_unmatched(struct log_job *jb) {
    // mark all injections to be added to unmatched logs
    for(size_t i = 0; i < jb->injections.used ; i++)
        jb->injections.on_unmatched_too[i] = true;

    if(jb->injections.used && jb->unmatched.injections.used) {
        // we have both injections and injections on unmatched

        // we find all the injections that are also configured as injections on unmatched,
        // and we disable them, so that the output will not have the same key twice

        for(size_t i = 0; i < jb->injections.used ;i++) {
            const char *equal = strchr(jb->injections.keys[i], '=');
            if(!equal)
                continue;

            size_t len = equal - jb->injections.keys[i] + 1;
            for(size_t u = 0; u < jb->unmatched.injections.used ; u++) {
                equal = strchr(jb->unmatched.injections.keys[u], '=');
                if(!equal)
                    continue;

                size_t len2 = equal - jb->unmatched.injections.keys[u] + 1;
                if(len == len2 && strncmp(jb->injections.keys[i], jb->unmatched.injections.keys[u], len) == 0)
                    jb->injections.on_unmatched_too[i] = false;
            }
        }
    }
}

static inline void jb_send_duplications_for_key(struct log_job *jb, const char *key, const char *value, size_t value_len) {
    // IMPORTANT:
    // The 'value' may not be NULL terminated and have more data that the value we need

    for (size_t d = 0; d < jb->dups.used; d++) {
        if(jb->dups.keys[d].exposed || jb->dups.keys[d].used == 0) continue;

        if(jb->dups.keys[d].used == 1) {
            // just one key to be duplicated
            if(strcmp(jb->dups.keys[d].keys[0], key) == 0) {
                printf("%s=%.*s\n", jb->dups.keys[d].target, (int) value_len, value);
                jb->dups.keys[d].exposed = true;
            }
        }
        else {
            // multiple keys to be duplicated
            for(size_t g = 0; g < jb->dups.keys[d].used ;g++) {
                struct key_dup *kd = &jb->dups.keys[d];
                if(strcmp(kd->keys[g], key) == 0) {
                    if(kd->values[g].len >= value_len) {
                        // the existing value allocation, fits our value

                        memcpy(kd->values[g].s, value, value_len);
                        kd->values[g].s[value_len] = '\0';
                    }
                    else {
                        // no existing value allocation, or too small for our value

                        if(kd->values[g].s)
                            free(kd->values[g].s);

                        kd->values[g].s = strdup_len(value, value_len);
                        kd->values[g].len = value_len;
                    }
                }
            }
        }
    }
}

static inline void jb_send_remaining_duplications(struct log_job *jb) {
    // IMPORTANT:
    // all duplications are exposed, even the ones we haven't found their keys in the source,
    // so that the output always has the same fields for matched entries.

    for(size_t d = 0; d < jb->dups.used ; d++) {
        if(jb->dups.keys[d].exposed || jb->dups.keys[d].used == 0)
            continue;

        printf("%s=", jb->dups.keys[d].target);
        for(size_t g = 0; g < jb->dups.keys[d].used ;g++) {
            char *comma = (g > 0) ? "," : "";
            char *value = (jb->dups.keys[d].values[g].s && jb->dups.keys[d].values[g].s[0]) ? jb->dups.keys[d].values[g].s : "[unavailable]";
            printf("%s%s", comma, value);

            // clear it for the next iteration
            if(jb->dups.keys[d].values[g].s)
                jb->dups.keys[d].values[g].s[0] = '\0';
        }
        printf("\n");
    }
}

static inline void jb_finalize_injections(struct log_job *jb, bool line_is_matched) {
    for (size_t j = 0; j < jb->injections.used; j++) {
        if(!line_is_matched && !jb->injections.on_unmatched_too[j])
            continue;

        printf("%s\n", jb->injections.keys[j]);
    }
}

static inline void jb_reset_injections(struct log_job *jb) {
    for(size_t d = 0; d < jb->dups.used ; d++)
        jb->dups.keys[d].exposed = false;
}

static inline void jb_inject_filename(struct log_job *jb) {
    if (jb->filename.key && jb->filename.current[0])
        printf("%s=%s\n", jb->filename.key, jb->filename.current);
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
            size_t filename_length = end - start;
            if(filename_length >= sizeof(jb->filename.current) - 1)
                filename_length = sizeof(jb->filename.current) - 2;

            snprintf(jb->filename.current, sizeof(jb->filename.current),
                     "%.*s", (int)filename_length, start);

            jb->filename.current[sizeof(jb->filename.current) - 1] = '\0';
            return true;
        }
    }

    jb->filename.last_line_was_empty = false;
    return false;
}

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

static pcre2_code *jb_compile_pcre2_pattern(const char *pattern) {
    int error_number;
    PCRE2_SIZE error_offset;
    PCRE2_SPTR pattern_ptr = (PCRE2_SPTR)pattern;

    pcre2_code *re = pcre2_compile(pattern_ptr, PCRE2_ZERO_TERMINATED, 0, &error_number, &error_offset, NULL);
    if (re == NULL) {
        PCRE2_UCHAR errbuf[1024];
        pcre2_get_error_message(error_number, errbuf, sizeof(errbuf));
        fprintf(stderr, "PCRE2 compilation failed at offset %d: %s\n", (int)error_offset, errbuf);
        fprintf(stderr, "Check for common regex syntax errors or unsupported PCRE2 patterns.\n");
        return NULL;
    }

    return re;
}
static inline bool jb_pcre2_match(pcre2_code *re, pcre2_match_data *match_data, char *line, size_t len) {
    int rc = pcre2_match(re, (PCRE2_SPTR)line, len, 0, 0, match_data, NULL);
    if(rc < 0) {
        PCRE2_UCHAR errbuf[1024];
        pcre2_get_error_message(rc, errbuf, sizeof(errbuf));
        fprintf(stderr, "PCRE2 error %d: %s on: %s\n", rc, errbuf, line);
        return false;
    }

    return true;
}

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

            printf("%s=%.*s\n", group_name, (int)group_length, line + start_offset);

            // process the duplications
            jb_send_duplications_for_key(jb, group_name, line + start_offset, group_length);

            tabptr += name_entry_size;
        }

        // print all non-exposed duplications
        jb_send_remaining_duplications(jb);
    }
}

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
        if(!jb_pcre2_match(re, match_data, line, len)) {
            line_is_matched = false;

            if (jb->unmatched.key) {
                // we are sending errors to Journal
                printf("%s=PCRE2 error on: %s\n", jb->unmatched.key, line);

                for (size_t j = 0; j < jb->unmatched.injections.used; j++)
                    printf("%s\n", jb->unmatched.injections.keys[j]);
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

    return 0;
}
