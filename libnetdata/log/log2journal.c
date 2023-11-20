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

struct key_dup {
    char *target;
    char *keys[MAX_KEY_DUPS_KEYS];
    char *values[MAX_KEY_DUPS_KEYS];
    size_t values_lengths[MAX_KEY_DUPS_KEYS];
    size_t used;
    bool exposed;
};

char *strdup_len(const char *src, size_t len) {
    char *s = malloc(len + 1);
    memcpy(s, src, len);
    s[len] = '\0';
    return s;
}

int main(int argc, char *argv[]) {
    char *filename_key = NULL;
    char *pattern = NULL;
    char *unmatched_key = NULL;
    struct key_dup dups[MAX_KEY_DUPS] = { 0 };
    size_t key_dup_used = 0;
    const char *injections[MAX_INJECTIONS] = { 0 };
    bool injections_on_unmatched[MAX_INJECTIONS] = { 0 };
    size_t injections_used = 0;
    const char *injections_unmatched[MAX_INJECTIONS] = { 0 };
    size_t injections_unmatched_used = 0;

    // Parse command line arguments
    for (int i = 1; i < argc; i++) {
        char *arg = argv[i];
        if (strcmp(arg, "--help") == 0 || strcmp(arg, "-h") == 0) {
            display_help(argv[0]);
            exit(0);
        }
        else if (strncmp(arg, "--filename-key=", 15) == 0)
            filename_key = arg + 15;
        else if (strncmp(arg, "--unmatched-key=", 16) == 0)
            unmatched_key = arg + 16;
        else if(strncmp(arg, "--duplicate=", 12) == 0) {
            const char *first_key = arg + 12;
            const char *comma = strchr(first_key, '=');
            if(!comma) {
                fprintf(stderr, "Error: --duplicate=TARGET=KEY1,... is missing the equal sign.\n");
                return 1;
            }
            const char *next_key = comma + 1;

            if (key_dup_used >= MAX_KEY_DUPS) {
                fprintf(stderr, "Error: too many duplications. You can duplicate up to %d keys\n", MAX_KEY_DUPS);
                return 1;
            }

            size_t first_key_len = comma - first_key;
            dups[key_dup_used].target = strdup_len(first_key, first_key_len);
            dups[key_dup_used].used = 0;

            while(next_key) {
                if(dups[key_dup_used].used >= MAX_KEY_DUPS_KEYS) {
                    fprintf(stderr, "Error: too many keys in duplication of target '%s'.\n", dups[key_dup_used].target);
                    return 1;
                }

                first_key = next_key;
                comma = strchr(first_key, ',');

                if(comma) {
                    first_key_len = comma - first_key;
                    dups[key_dup_used].keys[dups[key_dup_used].used++] = strdup_len(first_key, first_key_len);
                    next_key = comma + 1;
                }
                else {
                    dups[key_dup_used].keys[dups[key_dup_used].used++] = strdup(first_key);
                    next_key = NULL;
                }
            }

            key_dup_used++;
        }
        else if(strncmp(arg, "--inject=", 9) == 0) {
            if(injections_used >= MAX_INJECTIONS) {
                fprintf(stderr, "Error: too many injections. You can inject up to %d lines\n", MAX_INJECTIONS);
                return 1;
            }
            injections[injections_used++] = strdup(arg + 9);
        }
        else if(strncmp(arg, "--inject-unmatched=", 19) == 0) {
            if(injections_unmatched_used >= MAX_INJECTIONS) {
                fprintf(stderr, "Error: too many unmatched injections. You can inject up to %d lines\n", MAX_INJECTIONS);
                return 1;
            }
            injections_unmatched[injections_unmatched_used++] = strdup(arg + 19);
        }
        else {
            // Assume it's the pattern if not recognized as a parameter
            if (!pattern) {
                pattern = arg;
            } else {
                fprintf(stderr, "Error: Multiple patterns detected. Specify only one pattern.\n");
                return 1;
            }
        }
    }

    // Check if pattern is set and exactly one pattern is specified
    if (!pattern) {
        fprintf(stderr, "Error: Pattern not specified.\n");
        display_help(argv[0]);
        return 1;
    }

    // mark all injections to be added to unmatched logs
    for(size_t i = 0; i < injections_used ; i++)
        injections_on_unmatched[i] = true;

    if(injections_used && injections_unmatched_used) {
        for(size_t i = 0; i < injections_used ;i++) {
            const char *equal = strchr(injections[i], '=');
            if(!equal)
                continue;

            size_t len = equal - injections[i] + 1;
            for(size_t u = 0; u < injections_unmatched_used ;u++) {
                equal = strchr(injections_unmatched[u], '=');
                if(!equal)
                    continue;

                size_t len2 = equal - injections_unmatched[u] + 1;
                if(len == len2 && strncmp(injections[i], injections_unmatched[u], len) == 0)
                    injections_on_unmatched[i] = false;
            }
        }
    }

    pcre2_code *re;
    PCRE2_SPTR pattern_ptr = (PCRE2_SPTR)pattern;
    int errornumber;
    PCRE2_SIZE erroffset;
    pcre2_match_data *match_data;
    PCRE2_UCHAR errbuf[1024];

    // Compile the regular expression
    re = pcre2_compile(pattern_ptr, PCRE2_ZERO_TERMINATED, 0, &errornumber, &erroffset, NULL);
    if (re == NULL) {
        pcre2_get_error_message(errornumber, errbuf, sizeof(errbuf));
        fprintf(stderr, "PCRE2 compilation failed at offset %d: %s\n", (int)erroffset, errbuf);
        fprintf(stderr, "Check for common regex syntax errors or unsupported PCRE2 patterns.\n");
        return 1;
    }

    // Create a match data block
    match_data = pcre2_match_data_create_from_pattern(re, NULL);

    char current_filename[FILENAME_MAX + 1] = "";
    char buffer[MAX_LINE_LENGTH];
    bool last_line_was_newline = true;

    while (fgets(buffer, MAX_LINE_LENGTH, stdin) != NULL) {
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

        // Check if it's an empty line and skip it
        if (!len) {
            last_line_was_newline = true;
            continue;
        }

        // Check if it's a log file change line
        if (last_line_was_newline && line[0] == '=' && strncmp(line, "==> ", 4) == 0) {
            char *filename_start = line + 4;
            char *filename_end = strstr(line, " <==");
            while (*filename_start == ' ') filename_start++;
            if (*filename_start != '\n' && *filename_start != '\0') {
                if (filename_end)
                    *filename_end = '\0'; // Terminate the filename
                snprintf(current_filename, sizeof(current_filename) - 1, "%s", filename_start);
                current_filename[sizeof(current_filename) - 1] = '\0';
            }

            continue;
        }

        last_line_was_newline = false;

        // Regular log line, process it as usual
        int rc = pcre2_match(re, (PCRE2_SPTR)line, len, 0, 0, match_data, NULL);

        // Check for match
        bool line_is_matched;
        if (rc < 0) {
            line_is_matched = false;

            pcre2_get_error_message(rc, errbuf, sizeof(errbuf));
            fprintf(stderr, "PCRE2 error %d: %s on line: %s\n", rc, errbuf, line);

            if (unmatched_key) {
                printf("%s=PCRE2 error %d: %s on line: %s\n", unmatched_key, rc, errbuf, line);

                for (size_t j = 0; j < injections_unmatched_used; j++)
                    printf("%s\n", injections_unmatched[j]);
            }
            else
                continue;
        }
        else {
            line_is_matched = true;

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
                    for (size_t d = 0; d < key_dup_used; d++) {
                        if(dups[d].exposed || dups[d].used == 0) continue;

                        if(dups[d].used == 1) {
                            // just one key to be duplicated
                            if(strcmp(dups[d].keys[0], group_name) == 0) {
                                printf("%s=%.*s\n", dups[d].target, (int) group_length, line + start_offset);
                                dups[d].exposed = true;
                            }
                        }
                        else {
                            // multiple keys to be duplicated
                            for(size_t g = 0; g < dups[d].used ;g++) {
                                struct key_dup *kd = &dups[d];
                                if(strcmp(kd->keys[g], group_name) == 0) {
                                    if(kd->values_lengths[g] >= group_length) {
                                        // the existing value allocation, fits our value

                                        memcpy(kd->values[g], line + start_offset, group_length);
                                        kd->values[g][group_length] = '\0';
                                    }
                                    else {
                                        // no existing value allocation, or too small for our value

                                        if(kd->values[g])
                                            free(kd->values[g]);

                                        kd->values[g] = strdup_len(line + start_offset, group_length);
                                        kd->values_lengths[g] = group_length;
                                    }
                                }
                            }
                        }
                    }

                    tabptr += name_entry_size;
                }

                // print all non-exposed duplications
                for(size_t d = 0; d < key_dup_used ; d++) {
                    if(dups[d].exposed || dups[d].used == 0)
                        continue;

                    printf("%s=", dups[d].target);
                    for(size_t g = 0; g < dups[d].used ;g++) {
                        char *comma = (g > 0) ? "," : "";
                        char *value = (dups[d].values[g] && dups[d].values[g][0]) ? dups[d].values[g] : "[unavailable]";
                        printf("%s%s", comma, value);

                        // clear it for the next iteration
                        if(dups[d].values[g])
                            dups[d].values[g][0] = '\0';
                    }
                    printf("\n");
                }
            }
        }

        for (size_t j = 0; j < injections_used; j++) {
            if(!line_is_matched && !injections_on_unmatched[j])
                continue;

            printf("%s\n", injections[j]);
        }

        if (filename_key && current_filename[0]) {
            printf("%s=%s\n", filename_key, current_filename);
        }

        printf("\n");
        fflush(stdout);

        // print all non-exposed duplications
        for(size_t d = 0; d < key_dup_used ; d++) {
            dups[d].exposed = false;
        }
    }

    // Release memory used for the compiled regular expression and match data
    pcre2_match_data_free(match_data);
    pcre2_code_free(re);

    return 0;
}
