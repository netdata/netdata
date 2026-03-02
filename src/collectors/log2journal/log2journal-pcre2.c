// SPDX-License-Identifier: GPL-3.0-or-later

#include "log2journal.h"

#define PCRE2_ERROR_LINE_MAX 1024
#define PCRE2_KEY_MAX 1024

struct pcre2_state {
    LOG_JOB *jb;

    const char *line;
    uint32_t pos;
    uint32_t key_start;

    pcre2_code *re;
    pcre2_match_data *match_data;

    char key[PCRE2_KEY_MAX];
    char msg[PCRE2_ERROR_LINE_MAX];
};

static inline void copy_and_convert_key(PCRE2_STATE *pcre2, const char *key) {
    char *d = &pcre2->key[pcre2->key_start];
    size_t remaining = sizeof(pcre2->key) - pcre2->key_start;

    while(remaining >= 2 && *key) {
        *d = journal_key_characters_map[(unsigned) (*key)];
        remaining--;
        key++;
        d++;
    }

    *d = '\0';
}

static inline void jb_traverse_pcre2_named_groups_and_send_keys(PCRE2_STATE *pcre2, pcre2_code *re, pcre2_match_data *match_data, char *line) {
    PCRE2_SIZE *ovector = pcre2_get_ovector_pointer(match_data);
    uint32_t names_count;
    pcre2_pattern_info(re, PCRE2_INFO_NAMECOUNT, &names_count);

    if (names_count > 0) {
        PCRE2_SPTR name_table;
        pcre2_pattern_info(re, PCRE2_INFO_NAMETABLE, &name_table);
        uint32_t name_entry_size;
        pcre2_pattern_info(re, PCRE2_INFO_NAMEENTRYSIZE, &name_entry_size);

        const unsigned char *table_ptr = name_table;
        for (uint32_t i = 0; i < names_count; i++) {
            int n = (table_ptr[0] << 8) | table_ptr[1];
            const char *group_name = (const char *)(table_ptr + 2);

            PCRE2_SIZE start_offset = ovector[2 * n];
            PCRE2_SIZE end_offset = ovector[2 * n + 1];
            PCRE2_SIZE group_length = end_offset - start_offset;

            copy_and_convert_key(pcre2, group_name);
            log_job_send_extracted_key_value(pcre2->jb, pcre2->key, line + start_offset, group_length);

            table_ptr += name_entry_size;
        }
    }
}

void pcre2_get_error_in_buffer(char *msg, size_t msg_len, int rc, int pos) {
    int l;

    if(pos >= 0)
        l = snprintf(msg, msg_len, "PCRE2 error %d at pos %d on: ", rc, pos);
    else
        l = snprintf(msg, msg_len, "PCRE2 error %d on: ", rc);

    pcre2_get_error_message(rc, (PCRE2_UCHAR *)&msg[l], msg_len - l);
}

static void pcre2_error_message(PCRE2_STATE *pcre2, int rc, int pos) {
    pcre2_get_error_in_buffer(pcre2->msg, sizeof(pcre2->msg), rc, pos);
}

bool pcre2_has_error(PCRE2_STATE *pcre2) {
    return !pcre2->re || pcre2->msg[0];
}

PCRE2_STATE *pcre2_parser_create(LOG_JOB *jb) {
    PCRE2_STATE *pcre2 = mallocz(sizeof(PCRE2_STATE));
    memset(pcre2, 0, sizeof(PCRE2_STATE));
    pcre2->jb = jb;

    if(jb->prefix)
        pcre2->key_start = copy_to_buffer(pcre2->key, sizeof(pcre2->key), pcre2->jb->prefix, strlen(pcre2->jb->prefix));

    int rc;
    PCRE2_SIZE pos;
    pcre2->re = pcre2_compile((PCRE2_SPTR)jb->pattern, PCRE2_ZERO_TERMINATED, 0, &rc, &pos, NULL);
    if (!pcre2->re) {
        pcre2_error_message(pcre2, rc, pos);
        return pcre2;
    }

    pcre2->match_data = pcre2_match_data_create_from_pattern(pcre2->re, NULL);

    return pcre2;
}

void pcre2_parser_destroy(PCRE2_STATE *pcre2) {
    if(pcre2) {
        if(pcre2->re)
            pcre2_code_free(pcre2->re);

        if(pcre2->match_data)
            pcre2_match_data_free(pcre2->match_data);

        freez(pcre2);
    }
}

const char *pcre2_parser_error(PCRE2_STATE *pcre2) {
    return pcre2->msg;
}

bool pcre2_parse_document(PCRE2_STATE *pcre2, const char *txt, size_t len) {
    pcre2->line = txt;
    pcre2->pos = 0;
    pcre2->msg[0] = '\0';

    if(!len)
        len = strlen(txt);

    int rc = pcre2_match(pcre2->re, (PCRE2_SPTR)pcre2->line, len, 0, 0, pcre2->match_data, NULL);
    if(rc < 0) {
        pcre2_error_message(pcre2, rc, -1);
        return false;
    }

    jb_traverse_pcre2_named_groups_and_send_keys(pcre2, pcre2->re, pcre2->match_data, (char *)pcre2->line);

    return true;
}

void pcre2_test(void) {
    LOG_JOB jb = { .prefix = "NIGNX_" };
    PCRE2_STATE *pcre2 = pcre2_parser_create(&jb);

    pcre2_parse_document(pcre2, "{\"value\":\"\\u\\u039A\\u03B1\\u03BB\\u03B7\\u03BC\\u03AD\\u03C1\\u03B1\"}", 0);

    pcre2_parser_destroy(pcre2);
}
