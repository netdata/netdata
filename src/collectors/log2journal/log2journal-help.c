// SPDX-License-Identifier: GPL-3.0-or-later

#include "log2journal.h"

static void config_dir_print_available(void) {
    const char *path = LOG2JOURNAL_CONFIG_PATH;
    DIR *dir;
    struct dirent *entry;

    dir = opendir(path);

    if (dir == NULL) {
        l2j_log("       >>> Cannot open directory:\n       %s", path);
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
    printf("Netdata log2journal " NETDATA_VERSION "\n");
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
    printf("  --config CONFIG_NAME or -c CONFIG_NAME\n");
    printf("       Run with the internal YAML configuration named CONFIG_NAME.\n");
    printf("       Available internal YAML configs:\n");
    printf("\n");
    config_dir_print_available();
    printf("\n");
#else
    printf("  IMPORTANT:\n");
    printf("  YAML configuration parsing is not compiled in this binary.\n");
    printf("\n");
#endif
    printf("--------------------------------------------------------------------------------\n");
    printf("  INPUT PROCESSING\n");
    printf("\n");
    printf("  PATTERN\n");
    printf("       PATTERN should be a valid PCRE2 regular expression.\n");
    printf("       RE2 regular expressions (like the ones usually used in Go applications),\n");
    printf("       are usually valid PCRE2 patterns too.\n");
    printf("       Sub-expressions without named groups are evaluated, but their matches are\n");
    printf("       not added to the output.\n");
    printf("\n");
    printf("     - JSON mode\n");
    printf("       JSON mode is enabled when the pattern is set to: json\n");
    printf("       Field names are extracted from the JSON logs and are converted to the\n");
    printf("       format expected by Journal Export Format (all caps, only _ is allowed).\n");
    printf("\n");
    printf("     - logfmt mode\n");
    printf("       logfmt mode is enabled when the pattern is set to: logfmt\n");
    printf("       Field names are extracted from the logfmt logs and are converted to the\n");
    printf("       format expected by Journal Export Format (all caps, only _ is allowed).\n");
    printf("\n");
    printf("       All keys extracted from the input, are transliterated to match Journal\n");
    printf("       semantics (capital A-Z, digits 0-9, underscore).\n");
    printf("\n");
    printf("       In a YAML file:\n");
    printf("       ```yaml\n");
    printf("       pattern: 'PCRE2 pattern | json | logfmt'\n");
    printf("       ```\n");
    printf("\n");
    printf("--------------------------------------------------------------------------------\n");
    printf("  GLOBALS\n");
    printf("\n");
    printf("  --prefix PREFIX\n");
    printf("       Prefix all fields with PREFIX. The PREFIX is added before any other\n");
    printf("       processing, so that the extracted keys have to be matched with the PREFIX in\n");
    printf("       them. PREFIX is NOT transliterated and it is assumed to be systemd-journal\n");
    printf("       friendly.\n");
    printf("\n");
    printf("       In a YAML file:\n");
    printf("       ```yaml\n");
    printf("       prefix: 'PREFIX_' # prepend all keys with this prefix.\n");
    printf("       ```\n");
    printf("\n");
    printf("  --filename-key KEY\n");
    printf("       Add a field with KEY as the key and the current filename as value.\n");
    printf("       Automatically detects filenames when piped after 'tail -F',\n");
    printf("       and tail matches multiple filenames.\n");
    printf("       To inject the filename when tailing a single file, use --inject.\n");
    printf("\n");
    printf("       In a YAML file:\n");
    printf("       ```yaml\n");
    printf("       filename:\n");
    printf("         key: KEY\n");
    printf("       ```\n");
    printf("\n");
    printf("--------------------------------------------------------------------------------\n");
    printf("  RENAMING OF KEYS\n");
    printf("\n");
    printf("  --rename NEW=OLD\n");
    printf("       Rename fields. OLD has been transliterated and PREFIX has been added.\n");
    printf("       NEW is assumed to be systemd journal friendly.\n");
    printf("\n");
    printf("       Up to %d renaming rules are allowed.\n", MAX_RENAMES);
    printf("\n");
    printf("       In a YAML file:\n");
    printf("       ```yaml\n");
    printf("       rename:\n");
    printf("         - new_key: KEY1\n");
    printf("           old_key: KEY2 # transliterated with PREFIX added\n");
    printf("         - new_key: KEY3\n");
    printf("           old_key: KEY4 # transliterated with PREFIX added\n");
    printf("         # add as many as required\n");
    printf("       ```\n");
    printf("\n");
    printf("--------------------------------------------------------------------------------\n");
    printf("  INJECTING NEW KEYS\n");
    printf("\n");
    printf("  --inject KEY=VALUE\n");
    printf("       Inject constant fields to the output (both matched and unmatched logs).\n");
    printf("       --inject entries are added to unmatched lines too, when their key is\n");
    printf("       not used in --inject-unmatched (--inject-unmatched override --inject).\n");
    printf("       VALUE can use variable like ${OTHER_KEY} to be replaced with the values\n");
    printf("       of other keys available.\n");
    printf("\n");
    printf("       Up to %d fields can be injected.\n", MAX_INJECTIONS);
    printf("\n");
    printf("       In a YAML file:\n");
    printf("       ```yaml\n");
    printf("       inject:\n");
    printf("         - key: KEY1\n");
    printf("           value: 'VALUE1'\n");
    printf("         - key: KEY2\n");
    printf("           value: '${KEY3}${KEY4}' # gets the values of KEY3 and KEY4\n");
    printf("         # add as many as required\n");
    printf("       ```\n");
    printf("\n");
    printf("--------------------------------------------------------------------------------\n");
    printf("  REWRITING KEY VALUES\n");
    printf("\n");
    printf("  --rewrite KEY=/MATCH/REPLACE[/OPTIONS]\n");
    printf("       Apply a rewrite rule to the values of a specific key.\n");
    printf("       The first character after KEY= is the separator, which should also\n");
    printf("       be used between the MATCH, REPLACE and OPTIONS.\n");
    printf("\n");
    printf("       OPTIONS can be a comma separated list of `non-empty`, `dont-stop` and\n");
    printf("       `inject`.\n");
    printf("\n");
    printf("       When `non-empty` is given, MATCH is expected to be a variable\n");
    printf("       substitution using `${KEY1}${KEY2}`. Once the substitution is completed\n");
    printf("       the rule is matching the KEY only if the result is not empty.\n");
    printf("       When `non-empty` is not set, the MATCH string is expected to be a PCRE2\n");
    printf("       regular expression to be checked against the KEY value. This PCRE2\n");
    printf("       pattern may include named groups to extract parts of the KEY's value.\n");
    printf("\n");
    printf("       REPLACE supports variable substitution like `${variable}` against MATCH\n");
    printf("       named groups (when MATCH is a PCRE2 pattern) and `${KEY}` against the\n");
    printf("       keys defined so far.\n");
    printf("\n");
    printf("       Example:\n");
    printf("              --rewrite DATE=/^(?<year>\\d{4})-(?<month>\\d{2})-(?<day>\\d{2})$/\n");
    printf("                             ${day}/${month}/${year}\n");
    printf("       The above will rewrite dates in the format YYYY-MM-DD to DD/MM/YYYY.\n");
    printf("\n");
    printf("       Only one rewrite rule is applied per key; the sequence of rewrites for a\n");
    printf("       given key, stops once a rule matches it. This allows providing a sequence\n");
    printf("       of independent rewriting rules for the same key, matching the different\n");
    printf("       values the key may get, and also provide a catch-all rewrite rule at the\n");
    printf("       end, for setting the key value if no other rule matched it. The rewrite\n");
    printf("       rule can allow processing more rewrite rules when OPTIONS includes\n");
    printf("       the keyword 'dont-stop'.\n");
    printf("\n");
    printf("       Up to %d rewriting rules are allowed.\n", MAX_REWRITES);
    printf("\n");
    printf("       In a YAML file:\n");
    printf("       ```yaml\n");
    printf("       rewrite:\n");
    printf("         # the order if these rules in important - processed top to bottom\n");
    printf("         - key: KEY1\n");
    printf("           match: 'PCRE2 PATTERN WITH NAMED GROUPS'\n");
    printf("           value: 'all match fields and input keys as ${VARIABLE}'\n");
    printf("           inject: BOOLEAN # yes = inject the field, don't just rewrite it\n");
    printf("           stop: BOOLEAN # no = continue processing, don't stop if matched\n");
    printf("         - key: KEY2\n");
    printf("           non_empty: '${KEY3}${KEY4}' # match only if this evaluates to non empty\n");
    printf("           value: 'all input keys as ${VARIABLE}'\n");
    printf("           inject: BOOLEAN # yes = inject the field, don't just rewrite it\n");
    printf("           stop: BOOLEAN # no = continue processing, don't stop if matched\n");
    printf("         # add as many rewrites as required\n");
    printf("       ```\n");
    printf("\n");
    printf("       By default rewrite rules are applied only on fields already defined.\n");
    printf("       This allows shipping YAML files that include more rewrites than are\n");
    printf("       required for a specific input file.\n");
    printf("       Rewrite rules however allow injecting new fields when OPTIONS include\n");
    printf("       the keyword `inject` or in YAML `inject: yes` is given.\n");
    printf("\n");
    printf("       MATCH on the command line can be empty to define an unconditional rule.\n");
    printf("       Similarly, `match` and `non_empty` can be omitted in the YAML file.");
    printf("\n");
    printf("--------------------------------------------------------------------------------\n");
    printf("  UNMATCHED LINES\n");
    printf("\n");
    printf("  --unmatched-key KEY\n");
    printf("       Include unmatched log entries in the output with KEY as the field name.\n");
    printf("       Use this to include unmatched entries to the output stream.\n");
    printf("       Usually it should be set to --unmatched-key=MESSAGE so that the\n");
    printf("       unmatched entry will appear as the log message in the journals.\n");
    printf("       Use --inject-unmatched to inject additional fields to unmatched lines.\n");
    printf("\n");
    printf("       In a YAML file:\n");
    printf("       ```yaml\n");
    printf("       unmatched:\n");
    printf("         key: MESSAGE  # inject the error log as MESSAGE\n");
    printf("       ```\n");
    printf("\n");
    printf("  --inject-unmatched LINE\n");
    printf("       Inject lines into the output for each unmatched log entry.\n");
    printf("       Usually, --inject-unmatched=PRIORITY=3 is needed to mark the unmatched\n");
    printf("       lines as errors, so that they can easily be spotted in the journals.\n");
    printf("\n");
    printf("       Up to %d such lines can be injected.\n", MAX_INJECTIONS);
    printf("\n");
    printf("       In a YAML file:\n");
    printf("       ```yaml\n");
    printf("       unmatched:\n");
    printf("         key: MESSAGE  # inject the error log as MESSAGE\n");
    printf("         inject::\n");
    printf("           - key: KEY1\n");
    printf("             value: 'VALUE1'\n");
    printf("           # add as many constants as required\n");
    printf("       ```\n");
    printf("\n");
    printf("--------------------------------------------------------------------------------\n");
    printf("  FILTERING\n");
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
    printf("       In a YAML file:\n");
    printf("       ```yaml\n");
    printf("       filter:\n");
    printf("         include: 'PCRE2 PATTERN MATCHING KEY NAMES TO INCLUDE'\n");
    printf("         exclude: 'PCRE2 PATTERN MATCHING KEY NAMES TO EXCLUDE'\n");
    printf("       ```\n");
    printf("\n");
    printf("--------------------------------------------------------------------------------\n");
    printf("  OTHER\n");
    printf("\n");
    printf("  -h, or --help\n");
    printf("       Display this help and exit.\n");
    printf("\n");
    printf("  --show-config\n");
    printf("       Show the configuration in YAML format before starting the job.\n");
    printf("       This is also an easy way to convert command line parameters to yaml.\n");
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
    printf("                          v   v   v   v   v   v                  \n");
    printf("          +---------------------------------------------------+  \n");
    printf("          |             EXTRACT FIELDS AND VALUES             |  \n");
    printf("          |            JSON, logfmt, or pattern based         |  \n");
    printf("          |  (apply optional PREFIX - all keys use capitals)  |  \n");
    printf("          +---------------------------------------------------+  \n");
    printf("                          v   v   v   v   v   v                  \n");
    printf("          +---------------------------------------------------+  \n");
    printf("          |                   RENAME FIELDS                   |  \n");
    printf("          |           change the names of the fields          |  \n");
    printf("          +---------------------------------------------------+  \n");
    printf("                          v   v   v   v   v   v                  \n");
    printf("          +---------------------------------------------------+  \n");
    printf("          |                 INJECT NEW FIELDS                 |  \n");
    printf("          |   constants, or other field values as variables   |  \n");
    printf("          +---------------------------------------------------+  \n");
    printf("                          v   v   v   v   v   v                  \n");
    printf("          +---------------------------------------------------+  \n");
    printf("          |                REWRITE FIELD VALUES               |  \n");
    printf("          |     pipeline multiple rewriting rules to alter    |  \n");
    printf("          |               the values of the fields            |  \n");
    printf("          +---------------------------------------------------+  \n");
    printf("                          v   v   v   v   v   v                  \n");
    printf("          +---------------------------------------------------+  \n");
    printf("          |                   FILTER FIELDS                   |  \n");
    printf("          |  use include and exclude patterns on the field    |  \n");
    printf("          | names, to select which fields are sent to journal |  \n");
    printf("          +---------------------------------------------------+  \n");
    printf("                          v   v   v   v   v   v                  \n");
    printf("          +---------------------------------------------------+  \n");
    printf("          |                       OUTPUT                      |  \n");
    printf("          |           generate Journal Export Format          |  \n");
    printf("          +---------------------------------------------------+  \n");
    printf("                                                                 \n");
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
