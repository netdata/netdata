// SPDX-License-Identifier: GPL-3.0-or-later

#include "log2journal.h"

static void config_dir_print_available(void) {
    const char *path = LOG2JOURNAL_CONFIG_PATH;
    DIR *dir;
    struct dirent *entry;

    dir = opendir(path);

    if (dir == NULL) {
        log2stderr("       >>> Cannot open directory:\n       %s", path);
        return;
    }

    size_t column_width = 80;
    size_t current_columns = 7; // Start with 7 spaces for the first line

    while ((entry = readdir(dir))) {
        if (entry->d_type == DT_REG) { // Check if it's a regular file
            const char *file_name = entry->d_name;
            size_t len = strlen(file_name);
            if (len >= 5 && strcmp(file_name + len - 5, ".yaml") == 0) {
                // Remove the ".yaml" extension
                len -= 5;
                if (current_columns == 7) {
                    printf("       "); // Print 7 spaces at the beginning of a new line
                }
                if (current_columns + len + 1 > column_width) {
                    // Start a new line if the current line is full
                    printf("\n       "); // Print newline and 7 spaces
                    current_columns = 7;
                }
                printf("%.*s ", (int)len, file_name); // Print the filename without extension
                current_columns += len + 1; // Add filename length and a space
            }
        }
    }

    closedir(dir);
    printf("\n"); // Add a newline at the end
}

void log_job_command_line_help(const char *name) {
    printf("\n");
    printf("Netdata log2journal " PACKAGE_VERSION "\n");
    printf("\n");
    printf("Convert logs to systemd Journal Export Format.\n");
    printf("\n");
    printf(" - JSON logs: extracts all JSON fields.\n");
    printf(" - logfmt logs: extracts all logfmt fields.\n");
    printf(" - free-form logs: uses PCRE2 patterns to extracts fields.\n");
    printf("\n");
    printf("Usage: %s [OPTIONS] PATTERN|json\n", name);
    printf("\n");
    printf("Options:\n");
    printf("\n");
#ifdef HAVE_LIBYAML
    printf("  --file /path/to/file.yaml or -f /path/to/file.yaml\n");
    printf("       Read yaml configuration file for instructions.\n");
    printf("\n");
    printf("  --config CONFIG_NAME\n");
    printf("       Run with the internal configuration named CONFIG_NAME.\n");
    printf("       Available internal configs:\n");
    printf("\n");
    config_dir_print_available();
    printf("\n");
#else
    printf("  IMPORTANT:\n");
    printf("  YAML configuration parsing is not compiled in this binary.\n");
    printf("\n");
#endif
    printf("  --show-config\n");
    printf("       Show the configuration in YAML format before starting the job.\n");
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
    printf("\n");
    printf("       Up to %d duplications can be given on the command line, and up to\n", MAX_KEY_DUPS);
    printf("       %d keys per duplication command are allowed.\n", MAX_KEY_DUPS_KEYS);
    printf("\n");
    printf("  --inject LINE\n");
    printf("       Inject constant fields to the output (both matched and unmatched logs).\n");
    printf("       --inject entries are added to unmatched lines too, when their key is\n");
    printf("       not used in --inject-unmatched (--inject-unmatched override --inject).\n");
    printf("\n");
    printf("       Up to %d fields can be injected.\n", MAX_INJECTIONS);
    printf("\n");
    printf("  --inject-unmatched LINE\n");
    printf("       Inject lines into the output for each unmatched log entry.\n");
    printf("       Usually, --inject-unmatched=PRIORITY=3 is needed to mark the unmatched\n");
    printf("       lines as errors, so that they can easily be spotted in the journals.\n");
    printf("\n");
    printf("       Up to %d such lines can be injected.\n", MAX_INJECTIONS);
    printf("\n");
    printf("  --rewrite KEY=/SearchPattern/ReplacePattern\n");
    printf("       Apply a rewrite rule to the values of a specific key.\n");
    printf("       The first character after KEY= is the separator, which should also\n");
    printf("       be used between the search pattern and the replacement pattern.\n");
    printf("       The search pattern is a PCRE2 regular expression, and the replacement\n");
    printf("       pattern supports literals and named capture groups from the search pattern.\n");
    printf("\n");
    printf("       Example:\n");
    printf("              --rewrite DATE=/^(?<year>\\d{4})-(?<month>\\d{2})-(?<day>\\d{2})$/\n");
    printf("                             ${day}/${month}/${year}\n");
    printf("       This will rewrite dates in the format YYYY-MM-DD to DD/MM/YYYY.\n");
    printf("\n");
    printf("       Only one rewrite rule is applied per key; the sequence of rewrites stops\n");
    printf("       for the key once a rule matches it. This allows providing a sequence of\n");
    printf("       independent rewriting rules for the same key, matching the different\n");
    printf("       values the key may get, and also provide a catch-all rewrite rule at the\n");
    printf("       end, for setting the key value if no other rule matched it.\n");
    printf("\n");
    printf("       Duplication of keys with the values of multiple other keys, combined with\n");
    printf("       multiple value rewriting rules, allows creating complex rules for adding\n");
    printf("       new keys, based on the values of existing keys.\n");
    printf("\n");
    printf("       Up to %d rewriting rules are allowed.\n", MAX_REWRITES);
    printf("\n");
    printf("  --include PATTERN\n");
    printf("       Include only keys matching the PCRE2 PATTERN.\n");
    printf("       Useful when parsing JSON of logfmt logs, to include only the keys given.\n");
    printf("       The keys are matched after the PREFIX has been added to them.\n");
    printf("\n");
    printf("  --exclude PATTERN\n");
    printf("       Exclude the keys matching the PCRE2 PATTERN.\n");
    printf("       Useful when parsing JSON of logfmt logs, to exclude some of the keys given.\n");
    printf("       The keys are matched after the PREFIX has been added to them.\n");
    printf("\n");
    printf("       When both include and exclude patterns are set and both match a key,\n");
    printf("       exclude wins and the key will not be added, like a pipeline, we first\n");
    printf("       include it and then exclude it.\n");
    printf("\n");
    printf("  --prefix PREFIX\n");
    printf("       Prefix all fields with PREFIX. The PREFIX is added before processing\n");
    printf("       duplications, renames and rewrites, so that the extracted keys have to\n");
    printf("       be matched with the PREFIX in them.\n");
    printf("       PREFIX is assumed to be systemd-journal friendly.\n");
    printf("\n");
    printf("  --rename NEW=OLD\n");
    printf("       Rename fields, before rewriting their values.\n");
    printf("\n");
    printf("       Up to %d renaming rules are allowed.\n", MAX_RENAMES);
    printf("\n");
    printf("  -h, or --help\n");
    printf("       Display this help and exit.\n");
    printf("\n");
    printf("  PATTERN\n");
    printf("       PATTERN should be a valid PCRE2 regular expression.\n");
    printf("       RE2 regular expressions (like the ones usually used in Go applications),\n");
    printf("       are usually valid PCRE2 patterns too.\n");
    printf("       Sub-expressions without named groups are evaluated, but their matches are\n");
    printf("       not added to the output.\n");
    printf("\n");
    printf("  JSON mode\n");
    printf("       JSON mode is enabled when the pattern is set to: json\n");
    printf("       Field names are extracted from the JSON logs and are converted to the\n");
    printf("       format expected by Journal Export Format (all caps, only _ is allowed).\n");
    printf("\n");
    printf("  logfmt mode\n");
    printf("       logfmt mode is enabled when the pattern is set to: logfmt\n");
    printf("       Field names are extracted from the logfmt logs and are converted to the\n");
    printf("       format expected by Journal Export Format (all caps, only _ is allowed).\n");
    printf("\n");
    printf("\n");
    printf("The program accepts all parameters as both --option=value and --option value.\n");
    printf("\n");
    printf("The maximum log line length accepted is %d characters.\n", MAX_LINE_LENGTH);
    printf("\n");
    printf("PIPELINE AND SEQUENCE OF PROCESSING\n");
    printf("\n");
    printf("This is a simple diagram of the pipeline taking place:\n");
    printf("                                                                 \n");
    printf("          +---------------------------------------------------+  \n");
    printf("          |                       INPUT                       |  \n");
    printf("          |             read one log line at a time           |  \n");
    printf("          +---------------------------------------------------+  \n");
    printf("                           v                          v          \n");
    printf("          +---------------------------------+         |          \n");
    printf("          |   EXTRACT FIELDS AND VALUES     |         |          \n");
    printf("          |  JSON, logfmt, or pattern based |         |          \n");
    printf("          |    (apply optional PREFIX)      |         |          \n");
    printf("          +---------------------------------+         |          \n");
    printf("                  v                 v                 |          \n");
    printf("          +---------------+  +--------------+         |          \n");
    printf("          |   DUPLICATE   |  |    FILTER    |         |          \n");
    printf("          |               |  | filter keys  |         |          \n");
    printf("          |  create new   |  +--------------+         |          \n");
    printf("          |  fields by    |         v                 |          \n");
    printf("          |  duplicating  |  +--------------+         |          \n");
    printf("          |  other fields |  |    RENAME    |         |          \n");
    printf("          |  and their    |  |    change    |         |          \n");
    printf("          |  values       |  | field names  |         |          \n");
    printf("          +---------------+  +--------------+         |          \n");
    printf("                  v                 v                 v          \n");
    printf("          +---------------------------------+  +--------------+  \n");
    printf("          |        REWRITE PIPELINES        |  |    INJECT    |  \n");
    printf("          |  altering the values of fields  |  |   constants  |  \n");
    printf("          +---------------------------------+  +--------------+  \n");
    printf("                          v                           v          \n");
    printf("          +---------------------------------------------------+  \n");
    printf("          |                       OUTPUT                      |  \n");
    printf("          |           generate Journal Export Format          |  \n");
    printf("          +---------------------------------------------------+  \n");
    printf("                                                                 \n");
    printf("IMPORTANT:\n");
    printf(" - Extraction of keys includes formatting them according to journal rules.\n");
    printf(" - Duplication rules use the original extracted field names, after they have\n");
    printf("   been prefixed (when a PREFIX is set) and before they are renamed.\n");
    printf(" - Rewriting is always the last stage, so the final field names are matched.\n");
    printf("\n");
    printf("--------------------------------------------------------------------------------\n");
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
