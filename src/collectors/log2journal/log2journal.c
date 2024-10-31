// SPDX-License-Identifier: GPL-3.0-or-later

#include "log2journal.h"
#include "libnetdata/required_dummies.h"

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

static inline HASHED_KEY *get_key_from_hashtable(LOG_JOB *jb, HASHED_KEY *k) {
    if(k->flags & HK_HASHTABLE_ALLOCATED)
        return k;

    if(!k->hashtable_ptr) {
        HASHED_KEY *ht_key;
        SIMPLE_HASHTABLE_SLOT_KEY *slot = simple_hashtable_get_slot_KEY(&jb->hashtable, k->hash, NULL, true);
        if((ht_key = SIMPLE_HASHTABLE_SLOT_DATA(slot))) {
            if(!(ht_key->flags & HK_COLLISION_CHECKED)) {
                ht_key->flags |= HK_COLLISION_CHECKED;

                if(strcmp(ht_key->key, k->key) != 0)
                    l2j_log(
                        "Hashtable collision detected on key '%s' (hash %lx) and '%s' (hash %lx). "
                        "Please file a bug report.",
                        ht_key->key,
                        (unsigned long)ht_key->hash,
                        k->key,
                        (unsigned long)k->hash);
            }
        }
        else {
            ht_key = callocz(1, sizeof(HASHED_KEY));
            ht_key->key = strdupz(k->key);
            ht_key->len = k->len;
            ht_key->hash = k->hash;
            ht_key->flags = HK_HASHTABLE_ALLOCATED;

            simple_hashtable_set_slot_KEY(&jb->hashtable, slot, ht_key->hash, ht_key);
        }

        k->hashtable_ptr = ht_key;
    }

    return k->hashtable_ptr;
}

static inline HASHED_KEY *get_key_from_hashtable_with_char_ptr(LOG_JOB *jb, const char *key) {
    HASHED_KEY find = {
        .flags = HK_NONE,
        .key = key,
        .len = strlen(key),
    };
    find.hash = XXH3_64bits(key, find.len);

    return get_key_from_hashtable(jb, &find);
}

// ----------------------------------------------------------------------------

static inline void validate_key(LOG_JOB *jb __maybe_unused, HASHED_KEY *k) {
    if(k->len > JOURNAL_MAX_KEY_LEN)
        l2j_log(
            "WARNING: key '%s' has length %zu, which is more than %zu, the max systemd-journal allows",
            k->key,
            (size_t)k->len,
            (size_t)JOURNAL_MAX_KEY_LEN);

    for(size_t i = 0; i < k->len ;i++) {
        char c = k->key[i];

        if((c < 'A' || c > 'Z') && !isdigit(c) && c != '_') {
            l2j_log("WARNING: key '%s' contains characters that are not allowed by systemd-journal.", k->key);
            break;
        }
    }

    if(isdigit(k->key[0]))
        l2j_log("WARNING: key '%s' starts with a digit and may not be accepted by systemd-journal.", k->key);

    if(k->key[0] == '_')
        l2j_log(
            "WARNING: key '%s' starts with an underscore, which makes it a systemd-journal trusted field. "
            "Such fields are accepted by systemd-journal-remote, but not by systemd-journald.",
            k->key);
}

// ----------------------------------------------------------------------------

static inline size_t replace_evaluate_to_buffer(LOG_JOB *jb, HASHED_KEY *k __maybe_unused, REPLACE_PATTERN *rp, char *dst, size_t dst_size) {
    size_t remaining = dst_size;
    char *copy_to = dst;

    for(REPLACE_NODE *node = rp->nodes; node != NULL && remaining > 1; node = node->next) {
        if(node->is_variable) {
            if(hashed_keys_match(&node->name, &jb->line.key)) {
                size_t copied = copy_to_buffer(copy_to, remaining, jb->line.trimmed, jb->line.trimmed_len);
                copy_to += copied;
                remaining -= copied;
            }
            else {
                HASHED_KEY *ktmp = get_key_from_hashtable_with_char_ptr(jb, node->name.key);
                if(ktmp->value.len) {
                    size_t copied = copy_to_buffer(copy_to, remaining, ktmp->value.txt, ktmp->value.len);
                    copy_to += copied;
                    remaining -= copied;
                }
            }
        }
        else {
            size_t copied = copy_to_buffer(copy_to, remaining, node->name.key, node->name.len);
            copy_to += copied;
            remaining -= copied;
        }
    }

    return copy_to - dst;
}

static inline void replace_evaluate(LOG_JOB *jb, HASHED_KEY *k, REPLACE_PATTERN *rp) {
    HASHED_KEY *ht_key = get_key_from_hashtable(jb, k);

    // set it to empty value
    k->value.len = 0;

    for(REPLACE_NODE *node = rp->nodes; node != NULL; node = node->next) {
        if(node->is_variable) {
            if(hashed_keys_match(&node->name, &jb->line.key))
                txt_l2j_append(&ht_key->value, jb->line.trimmed, jb->line.trimmed_len);

            else {
                HASHED_KEY *ktmp = get_key_from_hashtable_with_char_ptr(jb, node->name.key);
                if(ktmp->value.len)
                    txt_l2j_append(&ht_key->value, ktmp->value.txt, ktmp->value.len);
            }
        }
        else
            txt_l2j_append(&ht_key->value, node->name.key, node->name.len);
    }
}

static inline void replace_evaluate_from_pcre2(LOG_JOB *jb, HASHED_KEY *k, REPLACE_PATTERN *rp, SEARCH_PATTERN *sp) {
    assert(k->flags & HK_HASHTABLE_ALLOCATED);

    // set the temporary TEXT to zero length
    jb->rewrites.tmp.len = 0;

    PCRE2_SIZE *ovector = pcre2_get_ovector_pointer(sp->match_data);

    // Iterate through the linked list of replacement nodes
    for(REPLACE_NODE *node = rp->nodes; node != NULL; node = node->next) {
        if(node->is_variable) {
            int group_number = pcre2_substring_number_from_name(
                    sp->re, (PCRE2_SPTR) node->name.key);

            if(group_number >= 0) {
                PCRE2_SIZE start_offset = ovector[2 * group_number];
                PCRE2_SIZE end_offset = ovector[2 * group_number + 1];
                PCRE2_SIZE length = end_offset - start_offset;

                txt_l2j_append(&jb->rewrites.tmp, k->value.txt + start_offset, length);
            }
            else {
                if(hashed_keys_match(&node->name, &jb->line.key))
                    txt_l2j_append(&jb->rewrites.tmp, jb->line.trimmed, jb->line.trimmed_len);

                else {
                    HASHED_KEY *ktmp = get_key_from_hashtable_with_char_ptr(jb, node->name.key);
                    if(ktmp->value.len)
                        txt_l2j_append(&jb->rewrites.tmp, ktmp->value.txt, ktmp->value.len);
                }
            }
        }
        else {
            txt_l2j_append(&jb->rewrites.tmp, node->name.key, node->name.len);
        }
    }

    // swap the values of the temporary TEXT and the key value
    TXT_L2J tmp = k->value;
    k->value = jb->rewrites.tmp;
    jb->rewrites.tmp = tmp;
}

static inline bool rewrite_conditions_satisfied(LOG_JOB *jb, HASHED_KEY *k, REWRITE *rw) {
    assert(k->flags & HK_HASHTABLE_ALLOCATED);

    if(rw->flags & RW_MATCH_PCRE2) {
        return search_pattern_matches(&rw->match_pcre2, k->value.txt, k->value.len);
    }
    else if(rw->flags & RW_MATCH_NON_EMPTY) {
        char buffer[2]; // we don't need a big buffer - we just check if anything is written
        if(replace_evaluate_to_buffer(jb, k, &rw->match_non_empty, buffer, sizeof(buffer)))
            // it copied something
            return true;
        else
            // it copied nothing
            return false;
    }
    else
        // no conditions
        return true;
}

// ----------------------------------------------------------------------------

static inline HASHED_KEY *rename_key(LOG_JOB *jb, HASHED_KEY *k) {
    if(!(k->flags & HK_RENAMES_CHECKED) || k->flags & HK_HAS_RENAMES) {
        k->flags |= HK_RENAMES_CHECKED;

        for(size_t i = 0; i < jb->renames.used; i++) {
            RENAME *rn = &jb->renames.array[i];

            if(hashed_keys_match(&rn->old_key, k)) {
                k->flags |= HK_HAS_RENAMES;

                return get_key_from_hashtable(jb, &rn->new_key);
            }
        }
    }

    return k;
}

// ----------------------------------------------------------------------------

static inline void send_key_value_constant(LOG_JOB *jb __maybe_unused, HASHED_KEY *key, const char *value, size_t len) {
    HASHED_KEY *ht_key = get_key_from_hashtable(jb, key);

    txt_l2j_set(&ht_key->value, value, len);
    ht_key->flags |= HK_VALUE_FROM_LOG;

    //    fprintf(stderr, "SET %s=%.*s\n", ht_key->key, (int)ht_key->value.len, ht_key->value.txt);
}

static inline void send_key_value_error(LOG_JOB *jb, HASHED_KEY *key, const char *format, ...) PRINTFLIKE(3, 4);
static inline void send_key_value_error(LOG_JOB *jb, HASHED_KEY *key, const char *format, ...) {
    HASHED_KEY *ht_key = get_key_from_hashtable(jb, key);

    printf("%s=", ht_key->key);
    va_list args;
    va_start(args, format);
    vprintf(format, args);
    va_end(args);
    printf("\n");
}

inline void log_job_send_extracted_key_value(LOG_JOB *jb, const char *key, const char *value, size_t len) {
    HASHED_KEY *ht_key = get_key_from_hashtable_with_char_ptr(jb, key);
    HASHED_KEY *nk = rename_key(jb, ht_key);
    txt_l2j_set(&nk->value, value, len);
    ht_key->flags |= HK_VALUE_FROM_LOG;

//    fprintf(stderr, "SET %s=%.*s\n", ht_key->key, (int)ht_key->value.len, ht_key->value.txt);
}

static inline void log_job_process_rewrites(LOG_JOB *jb) {
    for(size_t i = 0; i < jb->rewrites.used ;i++) {
        REWRITE *rw = &jb->rewrites.array[i];

        HASHED_KEY *k = get_key_from_hashtable(jb, &rw->key);

        if(!(rw->flags & RW_INJECT) && !(k->flags & HK_VALUE_FROM_LOG) && !k->value.len)
            continue;

        if(!(k->flags & HK_VALUE_REWRITTEN) && rewrite_conditions_satisfied(jb, k, rw)) {
            if(rw->flags & RW_MATCH_PCRE2)
                replace_evaluate_from_pcre2(jb, k, &rw->value, &rw->match_pcre2);
            else
                replace_evaluate(jb, k, &rw->value);

            if(!(rw->flags & RW_DONT_STOP))
                k->flags |= HK_VALUE_REWRITTEN;

//            fprintf(stderr, "REWRITE %s=%.*s\n", k->key, (int)k->value.len, k->value.txt);
        }
    }
}

static inline void send_all_fields(LOG_JOB *jb) {
    SIMPLE_HASHTABLE_SORTED_FOREACH_READ_ONLY(&jb->hashtable, kptr, HASHED_KEY, _KEY) {
        HASHED_KEY *k = SIMPLE_HASHTABLE_SORTED_FOREACH_READ_ONLY_VALUE(kptr);

        if(k->value.len) {
            // the key exists and has some value

            if(!(k->flags & HK_FILTERED)) {
                k->flags |= HK_FILTERED;

                bool included = jb->filter.include.re ? search_pattern_matches(&jb->filter.include, k->key, k->len) : true;
                bool excluded = jb->filter.exclude.re ? search_pattern_matches(&jb->filter.exclude, k->key, k->len) : false;

                if(included && !excluded)
                    k->flags |= HK_FILTERED_INCLUDED;
                else
                    k->flags &= ~HK_FILTERED_INCLUDED;

                // log some error if the key does not comply to journal standards
                validate_key(jb, k);
            }

            if(k->flags & HK_FILTERED_INCLUDED)
                printf("%s=%.*s\n", k->key, (int)k->value.len, k->value.txt);

            // reset it for the next round
            k->value.txt[0] = '\0';
            k->value.len = 0;
        }

        k->flags &= ~(HK_VALUE_REWRITTEN | HK_VALUE_FROM_LOG);
    }
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

        INJECTION *inj = &jb->injections.keys[j];

        replace_evaluate(jb, &inj->key, &inj->value);
    }
}

// ----------------------------------------------------------------------------
// filename injection

static inline void jb_inject_filename(LOG_JOB *jb) {
    if (jb->filename.key.key && jb->filename.current.len)
        send_key_value_constant(jb, &jb->filename.key, jb->filename.current.txt, jb->filename.current.len);
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
            txt_l2j_set(&jb->filename.current, start, end - start);
            return true;
        }
    }

    jb->filename.last_line_was_empty = false;
    return false;
}

static inline bool jb_send_unmatched_line(LOG_JOB *jb, const char *line) {
    if (!jb->unmatched.key.key)
        return false;

    // we are sending errors to systemd-journal
    send_key_value_error(jb, &jb->unmatched.key, "Parsing error on: %s", line);

    for (size_t j = 0; j < jb->unmatched.injections.used; j++) {
        INJECTION *inj = &jb->unmatched.injections.keys[j];

        replace_evaluate(jb, &inj->key, &inj->value);
    }

    return true;
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

    PCRE2_STATE *pcre2 = NULL;
    LOG_JSON_STATE *json = NULL;
    LOGFMT_STATE *logfmt = NULL;

    if(strcmp(jb->pattern, "json") == 0) {
        json = json_parser_create(jb);
        // never fails
    }
    else if(strcmp(jb->pattern, "logfmt") == 0) {
        logfmt = logfmt_parser_create(jb);
        // never fails
    }
    else if(strcmp(jb->pattern, "none") != 0) {
        pcre2 = pcre2_parser_create(jb);
        if(pcre2_has_error(pcre2)) {
            l2j_log("%s", pcre2_parser_error(pcre2));
            pcre2_parser_destroy(pcre2);
            return 1;
        }
    }

    jb->line.buffer = mallocz(MAX_LINE_LENGTH + 1);
    jb->line.size = MAX_LINE_LENGTH + 1;
    jb->line.trimmed_len = 0;
    jb->line.trimmed = jb->line.buffer;

    while ((jb->line.trimmed = get_next_line(jb, (char *)jb->line.buffer, jb->line.size, &jb->line.trimmed_len))) {
        const char *line = jb->line.trimmed;
        size_t len = jb->line.trimmed_len;

        if(jb_switched_filename(jb, line, len))
            continue;

        bool line_is_matched = true;

        if(json)
            line_is_matched = json_parse_document(json, line);
        else if(logfmt)
            line_is_matched = logfmt_parse_document(logfmt, line);
        else if(pcre2)
            line_is_matched = pcre2_parse_document(pcre2, line, len);

        if(!line_is_matched) {
            if(json)
                l2j_log("%s", json_parser_error(json));
            else if(logfmt)
                l2j_log("%s", logfmt_parser_error(logfmt));
            else if(pcre2)
                l2j_log("%s", pcre2_parser_error(pcre2));

            if(!jb_send_unmatched_line(jb, line))
                // just logging to stderr, not sending unmatched lines
                continue;
        }

        jb_inject_filename(jb);
        jb_finalize_injections(jb, line_is_matched);

        log_job_process_rewrites(jb);
        send_all_fields(jb);
        printf("\n");
        fflush(stdout);
    }

    if(json)
        json_parser_destroy(json);

    else if(logfmt)
        logfmt_parser_destroy(logfmt);

    else if(pcre2)
        pcre2_parser_destroy(pcre2);

    freez((void *)jb->line.buffer);

    return 0;
}

// ----------------------------------------------------------------------------

int main(int argc, char *argv[]) {
    LOG_JOB log_job;

    log_job_init(&log_job);

    if(!log_job_command_line_parse_parameters(&log_job, argc, argv))
        exit(1);

    if(log_job.show_config)
        log_job_configuration_to_yaml(&log_job);

    int ret = log_job_run(&log_job);

    log_job_cleanup(&log_job);
    return ret;
}
