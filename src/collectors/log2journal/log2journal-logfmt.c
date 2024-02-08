// SPDX-License-Identifier: GPL-3.0-or-later

#include "log2journal.h"

#define LOGFMT_ERROR_LINE_MAX 1024
#define LOGFMT_KEY_MAX 1024

struct logfmt_state {
    LOG_JOB *jb;

    const char *line;
    uint32_t pos;
    uint32_t key_start;

    char key[LOGFMT_KEY_MAX];
    char msg[LOGFMT_ERROR_LINE_MAX];
};

#define logfmt_current_pos(lfs) &(lfs)->line[(lfs)->pos]
#define logfmt_consume_char(lfs) ++(lfs)->pos

static inline void logfmt_process_key_value(LOGFMT_STATE *lfs, const char *value, size_t len) {
    log_job_send_extracted_key_value(lfs->jb, lfs->key, value, len);
}

static inline void logfmt_skip_spaces(LOGFMT_STATE *lfs) {
    const char *s = logfmt_current_pos(lfs);
    const char *start = s;

    while(isspace(*s)) s++;

    lfs->pos += s - start;
}

static inline void copy_newline(LOGFMT_STATE *lfs __maybe_unused, char **d, size_t *remaining) {
    if(*remaining > 3) {
        *(*d)++ = '\\';
        *(*d)++ = 'n';
        (*remaining) -= 2;
    }
}

static inline void copy_tab(LOGFMT_STATE *lfs __maybe_unused, char **d, size_t *remaining) {
    if(*remaining > 3) {
        *(*d)++ = '\\';
        *(*d)++ = 't';
        (*remaining) -= 2;
    }
}

static inline bool logftm_parse_value(LOGFMT_STATE *lfs) {
    static __thread char value[JOURNAL_MAX_VALUE_LEN];

    char quote = '\0';
    const char *s = logfmt_current_pos(lfs);
    if(*s == '\"' || *s == '\'') {
        quote = *s;
        logfmt_consume_char(lfs);
    }

    value[0] = '\0';
    char *d = value;
    s = logfmt_current_pos(lfs);
    size_t remaining = sizeof(value);

    char end_char = (char)(quote == '\0' ? ' ' : quote);
    while (*s && *s != end_char) {
        char c;

        if (*s == '\\') {
            s++;

            switch (*s) {
                case 'n':
                    copy_newline(lfs, &d, &remaining);
                    s++;
                    continue;

                case 't':
                    copy_tab(lfs, &d, &remaining);
                    s++;
                    continue;

                case 'f':
                case 'b':
                case 'r':
                    c = ' ';
                    s++;
                    break;

                default:
                    c = *s++;
                    break;
            }
        }
        else
            c = *s++;

        if(remaining < 2) {
            snprintf(lfs->msg, sizeof(lfs->msg),
                     "LOGFMT PARSER: truncated string value at position %u", lfs->pos);
            return false;
        }
        else {
            *d++ = c;
            remaining--;
        }
    }
    *d = '\0';
    lfs->pos += s - logfmt_current_pos(lfs);

    s = logfmt_current_pos(lfs);

    if(quote != '\0') {
        if (*s != quote) {
            snprintf(lfs->msg, sizeof(lfs->msg),
                     "LOGFMT PARSER: missing quote at position %u: '%s'",
                     lfs->pos, s);
            return false;
        }
        else
            logfmt_consume_char(lfs);
    }

    if(d > value)
        logfmt_process_key_value(lfs, value, d - value);

    return true;
}

static inline bool logfmt_parse_key(LOGFMT_STATE *lfs) {
    logfmt_skip_spaces(lfs);

    char *d = &lfs->key[lfs->key_start];

    size_t remaining = sizeof(lfs->key) - (d - lfs->key);

    const char *s = logfmt_current_pos(lfs);
    char last_c = '\0';
    while(*s && *s != '=') {
        char c;

        if (*s == '\\')
            s++;

        c = journal_key_characters_map[(unsigned char)*s++];

        if(c == '_' && last_c == '_')
            continue;
        else {
            if(remaining < 2) {
                snprintf(lfs->msg, sizeof(lfs->msg),
                         "LOGFMT PARSER: key buffer full - keys are too long, at position %u", lfs->pos);
                return false;
            }
            *d++ = c;
            remaining--;
        }

        last_c = c;
    }
    *d = '\0';
    lfs->pos += s - logfmt_current_pos(lfs);

    s = logfmt_current_pos(lfs);
    if(*s != '=') {
        snprintf(lfs->msg, sizeof(lfs->msg),
                 "LOGFMT PARSER: key is missing the equal sign, at position %u", lfs->pos);
        return false;
    }

    logfmt_consume_char(lfs);

    return true;
}

LOGFMT_STATE *logfmt_parser_create(LOG_JOB *jb) {
    LOGFMT_STATE *lfs = mallocz(sizeof(LOGFMT_STATE));
    memset(lfs, 0, sizeof(LOGFMT_STATE));
    lfs->jb = jb;

    if(jb->prefix)
        lfs->key_start = copy_to_buffer(lfs->key, sizeof(lfs->key), lfs->jb->prefix, strlen(lfs->jb->prefix));

    return lfs;
}

void logfmt_parser_destroy(LOGFMT_STATE *lfs) {
    if(lfs)
        freez(lfs);
}

const char *logfmt_parser_error(LOGFMT_STATE *lfs) {
    return lfs->msg;
}

bool logfmt_parse_document(LOGFMT_STATE *lfs, const char *txt) {
    lfs->line = txt;
    lfs->pos = 0;
    lfs->msg[0] = '\0';

    const char *s;
    do {
        if(!logfmt_parse_key(lfs))
            return false;

        if(!logftm_parse_value(lfs))
            return false;

        logfmt_skip_spaces(lfs);

        s = logfmt_current_pos(lfs);
    } while(*s);

    return true;
}


void logfmt_test(void) {
    LOG_JOB jb = { .prefix = "NIGNX_" };
    LOGFMT_STATE *logfmt = logfmt_parser_create(&jb);

    logfmt_parse_document(logfmt, "x=1 y=2 z=\"3 \\ 4\" 5  ");

    logfmt_parser_destroy(logfmt);
}
