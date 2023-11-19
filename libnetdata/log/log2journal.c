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
    printf("       Use this to log unmatched entries to stdout instead of stderr.\n");
    printf("       Use --inject-unmatched to inject additional fields to unmatched lines.\n");
    printf("\n");
    printf("  --duplicate=OLD,NEW\n");
    printf("       Duplicate a field with OLD key as NEW key, retaining the same value.\n");
    printf("       Useful for further processing. Up to %d duplications allowed.\n", MAX_KEY_DUPS);
    printf("\n");
    printf("  --inject=LINE\n");
    printf("       Inject constant fields into successfully parsed log entries.\n");
    printf("       Up to %d fields can be injected.\n", MAX_INJECTIONS);
    printf("\n");
    printf("  --inject-unmatched=LINE\n");
    printf("       Inject lines into the output for each unmatched log entry.\n");
    printf("       Up to %d such lines can be injected.\n", MAX_INJECTIONS);
    printf("\n");
    printf("  -h, --help\n");
    printf("       Display this help and exit.\n");
    printf("\n");
    printf("  PATTERN\n");
    printf("       PATTERN should be a valid PCRE2 regular expression.\n");
    printf("       Regular expressions without named groups are ignored.\n");
    printf("\n");
    printf("The maximum line length accepted is %d characters\n", MAX_LINE_LENGTH);
    printf("The maximum number of fields in the PCRE2 pattern is %d\n", OVECCOUNT / 3);
    printf("\n");
}

struct key_dup {
    char *old_key;
    char *new_key;
};

int main(int argc, char *argv[]) {
    char *filename_key = NULL;
    char *pattern = NULL;
    char *unmatched_key = NULL;
    struct key_dup dups[MAX_KEY_DUPS];
    size_t key_dup_used = 0;
    const char *injections[MAX_INJECTIONS];
    size_t injections_used = 0;
    const char *injections_unmatched[MAX_INJECTIONS];
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
            const char *old_key = arg + 12;
            const char *comma = strchr(old_key, ',');
            if(!comma) {
                fprintf(stderr, "Error: --duplicate=KEY1,KEY2 is missing the comma.\n");
                return 1;
            }
            const char *new_key = comma + 1;

            if (key_dup_used >= MAX_KEY_DUPS) {
                fprintf(stderr, "Error: too many duplications. You can duplicate up to %d keys\n", MAX_KEY_DUPS);
                return 1;
            }

            size_t old_key_len = comma - old_key;
            dups[key_dup_used].old_key = malloc(old_key_len + 1);
            memcpy(dups[key_dup_used].old_key, old_key, old_key_len);
            dups[key_dup_used].old_key[old_key_len] = '\0';

            dups[key_dup_used].new_key = strdup(new_key);
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
            if (filename_start && *filename_start != '\n' && *filename_start != '\0') {
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
        if (rc < 0) {
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

                    for (size_t d = 0; d < key_dup_used; d++) {
                        if (strcmp(dups[d].old_key, group_name) == 0) {
                            printf("%s=%.*s\n", dups[d].new_key, (int)group_length, line + start_offset);
                        }
                    }

                    tabptr += name_entry_size;
                }
            }
        }

        for (size_t j = 0; j < injections_used; j++) {
            printf("%s\n", injections[j]);
        }

        if (filename_key && current_filename[0]) {
            printf("%s=%s\n", filename_key, current_filename);
        }

        printf("\n");
        fflush(stdout);
    }

    // Release memory used for the compiled regular expression and match data
    pcre2_match_data_free(match_data);
    pcre2_code_free(re);

    return 0;
}
