// SPDX-License-Identifier: GPL-3.0-or-later

#include "log2journal.h"

static inline void send_duplications_for_key(LOG_JOB *jb, HASHED_KEY *k, const char *value, size_t value_len);

// ----------------------------------------------------------------------------

const char journal_key_characters_map[256] = {
        // control characters
        [0] = '\0', [1] = '_', [2] = '_', [3] = '_', [4] = '_', [5] = '_', [6] = '_', [7] = '_',
        [8] = '_', [9] = '_', [10] = '_', [11] = '_', [12] = '_', [13] = '_', [14] = '_', [15] = '_',
        [16] = '_', [17] = '_', [18] = '_', [19] = '_', [20] = '_', [21] = '_', [22] = '_', [23] = '_',
        [24] = '_', [25] = '_', [26] = '_', [27] = '_', [28] = '_', [29] = '_', [30] = '_', [31] = '_',

        // symbols
        [' '] = '_', ['!'] = '_', ['"'] = '_', ['#'] = '_', ['$'] = '_', ['%'] = '_', ['&'] = '_', ['\''] = '_',
        ['('] = '_', [')'] = '_', ['*'] = '_', ['+'] = '_', [','] = '_', ['-'] = '_', ['.'] = '_', ['/'] = '_',

        // numbers
        ['0'] = '0', ['1'] = '1', ['2'] = '2', ['3'] = '3', ['4'] = '4', ['5'] = '5', ['6'] = '6', ['7'] = '7',
        ['8'] = '8', ['9'] = '9',

        // symbols
        [':'] = '_', [';'] = '_', ['<'] = '_', ['='] = '_', ['>'] = '_', ['?'] = '_', ['@'] = '_',

        // capitals
        ['A'] = 'A', ['B'] = 'B', ['C'] = 'C', ['D'] = 'D', ['E'] = 'E', ['F'] = 'F', ['G'] = 'G', ['H'] = 'H',
        ['I'] = 'I', ['J'] = 'J', ['K'] = 'K', ['L'] = 'L', ['M'] = 'M', ['N'] = 'N', ['O'] = 'O', ['P'] = 'P',
        ['Q'] = 'Q', ['R'] = 'R', ['S'] = 'S', ['T'] = 'T', ['U'] = 'U', ['V'] = 'V', ['W'] = 'W', ['X'] = 'X',
        ['Y'] = 'Y', ['Z'] = 'Z',

        // symbols
        ['['] = '_', ['\\'] = '_', [']'] = '_', ['^'] = '_', ['_'] = '_', ['`'] = '_',

        // lower to upper
        ['a'] = 'A', ['b'] = 'B', ['c'] = 'C', ['d'] = 'D', ['e'] = 'E', ['f'] = 'F', ['g'] = 'G', ['h'] = 'H',
        ['i'] = 'I', ['j'] = 'J', ['k'] = 'K', ['l'] = 'L', ['m'] = 'M', ['n'] = 'N', ['o'] = 'O', ['p'] = 'P',
        ['q'] = 'Q', ['r'] = 'R', ['s'] = 'S', ['t'] = 'T', ['u'] = 'U', ['v'] = 'V', ['w'] = 'W', ['x'] = 'X',
        ['y'] = 'Y', ['z'] = 'Z',

        // symbols
        ['{'] = '_', ['|'] = '_', ['}'] = '_', ['~'] = '_', [127] = '_', // Delete (DEL)

        // Extended ASCII characters (128-255) set to underscore
        [128] = '_', [129] = '_', [130] = '_', [131] = '_', [132] = '_', [133] = '_', [134] = '_', [135] = '_',
        [136] = '_', [137] = '_', [138] = '_', [139] = '_', [140] = '_', [141] = '_', [142] = '_', [143] = '_',
        [144] = '_', [145] = '_', [146] = '_', [147] = '_', [148] = '_', [149] = '_', [150] = '_', [151] = '_',
        [152] = '_', [153] = '_', [154] = '_', [155] = '_', [156] = '_', [157] = '_', [158] = '_', [159] = '_',
        [160] = '_', [161] = '_', [162] = '_', [163] = '_', [164] = '_', [165] = '_', [166] = '_', [167] = '_',
        [168] = '_', [169] = '_', [170] = '_', [171] = '_', [172] = '_', [173] = '_', [174] = '_', [175] = '_',
        [176] = '_', [177] = '_', [178] = '_', [179] = '_', [180] = '_', [181] = '_', [182] = '_', [183] = '_',
        [184] = '_', [185] = '_', [186] = '_', [187] = '_', [188] = '_', [189] = '_', [190] = '_', [191] = '_',
        [192] = '_', [193] = '_', [194] = '_', [195] = '_', [196] = '_', [197] = '_', [198] = '_', [199] = '_',
        [200] = '_', [201] = '_', [202] = '_', [203] = '_', [204] = '_', [205] = '_', [206] = '_', [207] = '_',
        [208] = '_', [209] = '_', [210] = '_', [211] = '_', [212] = '_', [213] = '_', [214] = '_', [215] = '_',
        [216] = '_', [217] = '_', [218] = '_', [219] = '_', [220] = '_', [221] = '_', [222] = '_', [223] = '_',
        [224] = '_', [225] = '_', [226] = '_', [227] = '_', [228] = '_', [229] = '_', [230] = '_', [231] = '_',
        [232] = '_', [233] = '_', [234] = '_', [235] = '_', [236] = '_', [237] = '_', [238] = '_', [239] = '_',
        [240] = '_', [241] = '_', [242] = '_', [243] = '_', [244] = '_', [245] = '_', [246] = '_', [247] = '_',
        [248] = '_', [249] = '_', [250] = '_', [251] = '_', [252] = '_', [253] = '_', [254] = '_', [255] = '_',
};

// ----------------------------------------------------------------------------

static char *rewrite_value(LOG_JOB *jb, HASHED_KEY *k, const char *value, size_t value_len) {
    static __thread char rewritten_value[JOURNAL_MAX_VALUE_LEN + 1];

    if(!(k->flags & HK_REWRITES_CHECKED) || k->flags & HK_HAS_REWRITES) {
        k->flags |= HK_REWRITES_CHECKED;

        for(size_t i = 0; i < jb->rewrites.used; i++) {
            REWRITE *rw = &jb->rewrites.array[i];

            if(rw->key.hash == k->hash && strcmp(rw->key.key, k->key) == 0) {
                if(!search_pattern_matches(&rw->search, value, value_len))
                    continue; // No match found, skip to next rewrite rule

                PCRE2_SIZE *ovector = pcre2_get_ovector_pointer(rw->search.match_data);

                char *buffer = rewritten_value;
                size_t buffer_remaining = sizeof(rewritten_value);

                // Iterate through the linked list of replacement nodes
                for(REPLACE_NODE *node = rw->replace.nodes; node != NULL; node = node->next) {
                    if(node->is_variable) {
                        int group_number = pcre2_substring_number_from_name(rw->search.re, (PCRE2_SPTR) node->name.key);
                        if(group_number >= 0) {
                            PCRE2_SIZE start_offset = ovector[2 * group_number];
                            PCRE2_SIZE end_offset = ovector[2 * group_number + 1];
                            PCRE2_SIZE length = end_offset - start_offset;

                            size_t copied = copy_to_buffer(buffer, buffer_remaining, value + start_offset, length);
                            buffer += copied;
                            buffer_remaining -= copied;
                        }
                        else {
                            // TODO: lookup in key names to get their values

                            if(!node->logged_error) {
                                log2stderr("WARNING: variable '${%s}' in rewrite rule cannot be resolved.",
                                           node->name.key);
                                node->logged_error = true;
                            }
                        }
                    }
                    else {
                        size_t copied = copy_to_buffer(buffer, buffer_remaining, node->name.key, node->name.len);
                        buffer += copied;
                        buffer_remaining -= copied;
                    }
                }

                k->flags |= HK_HAS_REWRITES;
                return rewritten_value;
            }
        }
    }

    return NULL;
}

static inline HASHED_KEY *rename_key(LOG_JOB *jb, HASHED_KEY *k) {
    if(!(k->flags & HK_RENAMES_CHECKED) || k->flags & HK_HAS_RENAMES) {
        k->flags |= HK_RENAMES_CHECKED;

        for(size_t i = 0; i < jb->renames.used; i++) {
            RENAME *rn = &jb->renames.array[i];

            if(rn->old_key.hash == k->hash && strcmp(rn->old_key.key, k->key) == 0) {
                k->flags |= HK_HAS_RENAMES;
                return &rn->new_key;
            }
        }
    }

    return k;
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

static inline void send_key_value_and_rewrite(LOG_JOB *jb, HASHED_KEY *k, const char *value, size_t len) {
    char *rewritten = rewrite_value(jb, k, value, len);
    if(!rewritten)
        printf("%s=%.*s\n", k->key, (int)len, value);
    else
        printf("%s=%s\n", k->key, rewritten);
}

static inline HASHED_KEY *get_key_from_hashtable(LOG_JOB *jb, const char *key) {
    size_t key_len = strlen(key);
    XXH64_hash_t key_hash = XXH3_64bits(key, key_len);

    HASHED_KEY *k;
    SIMPLE_HASHTABLE_SLOT *slot = simple_hashtable_get_slot(&jb->hashtable, key_hash, true);
    if(slot->data) {
        k = slot->data;

        if(!(k->flags & HK_COLLISION_CHECKED)) {
            k->flags |= HK_COLLISION_CHECKED;

            if(strcmp(k->key, key) != 0)
                log2stderr("Hashtable collision detected on key '%s' (hash %lx) and '%s' (hash %lx). "
                           "Please report this to Netdata.",
                        k->key, (unsigned long)k->hash, key, (unsigned long)key_hash);
        }
    }
    else {
        k = mallocz(sizeof(HASHED_KEY));
        k->key = strdupz(key);
        k->len = key_len;
        k->hash = key_hash;
        k->flags = HK_ALLOCATED;

        slot->hash = k->hash;
        slot->data = k;
        jb->hashtable.used++;
    }

    return k;
}

inline void log_job_send_extracted_key_value(LOG_JOB *jb, const char *key, const char *value, size_t len) {
    HASHED_KEY *k = get_key_from_hashtable(jb, key);

    if(!(k->flags & HK_FILTERED)) {
        k->flags |= HK_FILTERED;

        bool included = jb->filter.include.re ? search_pattern_matches(&jb->filter.include, k->key, k->len) : true;
        bool excluded = jb->filter.exclude.re ? search_pattern_matches(&jb->filter.exclude, k->key, k->len) : false;

        if(included && !excluded)
            k->flags |= HK_FILTERED_INCLUDED;
        else
            k->flags &= ~HK_FILTERED_INCLUDED;
    }

    if(k->flags & HK_FILTERED_INCLUDED) {
        // process renames (changing the key)
        HASHED_KEY *nk = rename_key(jb, k);

        // process rewrites (changing the value)
        // and send it to output
        send_key_value_and_rewrite(jb, nk, value, len);
    }

    // process the duplications (using the original key)
    // and send them to output
    send_duplications_for_key(jb, k, value, len);
}

static inline void send_key_value_constant(LOG_JOB *jb __maybe_unused, const char *key, const char *value) {
    printf("%s=%s\n", key, value);
}

// ----------------------------------------------------------------------------
// injection of constant fields

static void select_which_injections_should_be_injected_on_unmatched(LOG_JOB *jb) {
    // mark all injections to be added to unmatched logs
    for(size_t i = 0; i < jb->injections.used ; i++)
        jb->injections.keys[i].on_unmatched = true;

    if(jb->injections.used && jb->unmatched.injections.used) {
        // we have both injections and injections on unmatched

        // we find all the injections that are also configured as injections on unmatched,
        // and we disable them, so that the output will not have the same key twice

        for(size_t i = 0; i < jb->injections.used ;i++) {
            for(size_t u = 0; u < jb->unmatched.injections.used ; u++) {
                if(strcmp(jb->injections.keys[i].key.key, jb->unmatched.injections.keys[u].key.key) == 0)
                    jb->injections.keys[i].on_unmatched = false;
            }
        }
    }
}


static inline void jb_finalize_injections(LOG_JOB *jb, bool line_is_matched) {
    for (size_t j = 0; j < jb->injections.used; j++) {
        if(!line_is_matched && !jb->injections.keys[j].on_unmatched)
            continue;

        send_key_value_constant(jb, jb->injections.keys[j].key.key, jb->injections.keys[j].value.txt);
    }
}

static inline void log_job_duplications_reset(LOG_JOB *jb) {
    for(size_t d = 0; d < jb->dups.used ; d++) {
        DUPLICATION *kd = &jb->dups.array[d];
        kd->exposed = false;

        for(size_t g = 0; g < kd->used ; g++) {
            if(kd->values[g].txt)
                kd->values[g].txt[0] = '\0';
        }
    }
}

// ----------------------------------------------------------------------------
// duplications

static inline void send_duplications_for_key(LOG_JOB *jb, HASHED_KEY *k, const char *value, size_t value_len) {
    // IMPORTANT:
    // The 'value' may not be NULL terminated and have more data that the value we need

    if(!(k->flags & HK_DUPS_CHECKED) || k->flags & HK_HAS_DUPS) {
        k->flags |= HK_DUPS_CHECKED;

        for(size_t d = 0; d < jb->dups.used; d++) {
            DUPLICATION *kd = &jb->dups.array[d];

            if(kd->exposed || kd->used == 0)
                continue;

            if(kd->used == 1) {
                // just one key to be duplicated
                if(kd->keys[0].hash == k->hash && strcmp(kd->keys[0].key, k->key) == 0) {
                    k->flags |= HK_HAS_DUPS;
                    send_key_value_and_rewrite(jb, &kd->target, value, value_len);
                    kd->exposed = true;
                }
            }
            else {
                // multiple keys to be duplicated
                for(size_t g = 0; g < kd->used; g++) {
                    if(kd->keys[g].hash == k->hash && strcmp(kd->keys[g].key, k->key) == 0) {
                        k->flags |= HK_HAS_DUPS;
                        txt_replace(&kd->values[g], value, value_len);
                    }
                }
            }
        }
    }
}

static inline void jb_send_remaining_duplications(LOG_JOB *jb) {
    static __thread char buffer[JOURNAL_MAX_VALUE_LEN + 1];

    // IMPORTANT:
    // all duplications are exposed, even the ones we haven't found their keys in the source,
    // so that the output always has the same fields for matched entries.

    for(size_t d = 0; d < jb->dups.used ; d++) {
        DUPLICATION *kd = &jb->dups.array[d];

        if(kd->exposed || kd->used == 0)
            continue;

        buffer[0] = '\0';
        size_t remaining = sizeof(buffer);
        char *s = buffer;

        for(size_t g = 0; g < kd->used ; g++) {
            if(remaining < 2) {
                log2stderr("Warning: duplicated key '%s' cannot fit the values.", kd->target.key);
                break;
            }

            if(g > 0) {
                *s++ = ',';
                *s = '\0';
                remaining--;
            }

            char *value = (kd->values[g].txt && kd->values[g].txt[0]) ? kd->values[g].txt : "[unavailable]";
            size_t len = strlen(value);
            size_t copied = copy_to_buffer(s, remaining, value, len);
            remaining -= copied;
            s += copied;

            if(copied != len) {
                log2stderr("Warning: duplicated key '%s' will have truncated value", jb->dups.array[d].target.key);
                break;
            }
        }
        send_key_value_and_rewrite(jb, &kd->target, buffer, s - buffer);
    }
}

// ----------------------------------------------------------------------------
// filename injection

static inline void jb_inject_filename(LOG_JOB *jb) {
    if (jb->filename.key && jb->filename.current[0])
        send_key_value_constant(jb, jb->filename.key, jb->filename.current);
}

static inline bool jb_switched_filename(LOG_JOB *jb, const char *line, size_t len) {
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

static void simple_hashtable_cleanup_allocated(SIMPLE_HASHTABLE *ht) {
    for(size_t i = 0; i < ht->used ;i++) {
        HASHED_KEY *k = ht->hashtable[i].data;
        if(k && k->flags & HK_ALLOCATED) {
            hashed_key_cleanup(k);
            freez(k);
            ht->hashtable[i].data = NULL;
            ht->hashtable[i].hash = 0;
        }
    }
}

// ----------------------------------------------------------------------------
// running a job

static char *get_next_line(LOG_JOB *jb __maybe_unused, char *buffer, size_t size, size_t *line_length) {
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

int log_job_run(LOG_JOB *jb) {
    select_which_injections_should_be_injected_on_unmatched(jb);

    simple_hashtable_init(&jb->hashtable, 32);

    PCRE2_STATE *pcre2 = NULL;
    LOG_JSON_STATE *json = NULL;
    LOGFMT_STATE *logfmt = NULL;

    if(strcmp(jb->pattern, "json") == 0) {
        json = json_parser_create(jb);
    }
    else if(strcmp(jb->pattern, "logfmt") == 0) {
        logfmt = logfmt_parser_create(jb);
    }
    else {
        pcre2 = pcre2_parser_create(jb);
        if(pcre2_has_error(pcre2)) {
            log2stderr("%s", pcre2_parser_error(pcre2));
            pcre2_parser_destroy(pcre2);
            return 1;
        }
    }

    char buffer[MAX_LINE_LENGTH];
    char *line;
    size_t len;

    while ((line = get_next_line(jb, buffer, sizeof(buffer), &len))) {
        if(jb_switched_filename(jb, line, len))
            continue;

        log_job_duplications_reset(jb);

        bool line_is_matched;

        if(json)
            line_is_matched = json_parse_document(json, line);
        else if(logfmt)
            line_is_matched = logfmt_parse_document(logfmt, line);
        else
            line_is_matched = pcre2_parse_document(pcre2, line, len);

        if(!line_is_matched) {
            if(json)
                log2stderr("%s", json_parser_error(json));
            else if(logfmt)
                log2stderr("%s", logfmt_parser_error(logfmt));
            else
                log2stderr("%s", pcre2_parser_error(pcre2));

            if (jb->unmatched.key) {
                // we are sending errors to systemd-journal
                send_key_value_error(jb->unmatched.key, "Parsing error on: %s", line);

                for (size_t j = 0; j < jb->unmatched.injections.used; j++)
                    send_key_value_constant(jb, jb->unmatched.injections.keys[j].key.key,
                            jb->unmatched.injections.keys[j].value.txt);
            }
            else {
                // we are just logging errors to stderr
                continue;
            }
        }
        else {
            // print all non-exposed duplications
            jb_send_remaining_duplications(jb);
        }

        jb_inject_filename(jb);
        jb_finalize_injections(jb, line_is_matched);

        printf("\n");
        fflush(stdout);
    }

    if(json)
        json_parser_destroy(json);

    else if(logfmt)
        logfmt_parser_destroy(logfmt);

    else if(pcre2)
        pcre2_parser_destroy(pcre2);

    simple_hashtable_cleanup_allocated(&jb->hashtable);
    simple_hashtable_free(&jb->hashtable);

    return 0;
}

// ----------------------------------------------------------------------------

int main(int argc, char *argv[]) {
    LOG_JOB log_job = { 0 };

    if(!log_job_command_line_parse_parameters(&log_job, argc, argv))
        exit(1);

    if(log_job.show_config)
        log_job_configuration_to_yaml(&log_job);

    int ret = log_job_run(&log_job);

    nd_log_cleanup(&log_job);
    return ret;
}
