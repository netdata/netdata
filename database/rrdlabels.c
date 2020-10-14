// SPDX-License-Identifier: GPL-3.0-or-later

#define NETDATA_RRD_INTERNALS
#include "rrd.h"

char *translate_label_source(LABEL_SOURCE l) {
    switch (l) {
        case LABEL_SOURCE_AUTO:
            return "AUTO";
        case LABEL_SOURCE_NETDATA_CONF:
            return "NETDATA.CONF";
        case LABEL_SOURCE_DOCKER :
            return "DOCKER";
        case LABEL_SOURCE_ENVIRONMENT  :
            return "ENVIRONMENT";
        case LABEL_SOURCE_KUBERNETES :
            return "KUBERNETES";
        default:
            return "Invalid label source";
    }
}

int is_valid_label_value(char *value) {
    while(*value) {
        if(*value == '"' || *value == '\'' || *value == '*' || *value == '!') {
            return 0;
        }

        value++;
    }

    return 1;
}

int is_valid_label_key(char *key) {
    //Prometheus exporter
    if(!strcmp(key, "chart") || !strcmp(key, "family")  || !strcmp(key, "dimension"))
        return 0;

    //Netdata and Prometheus  internal
    if (*key == '_')
        return 0;

    while(*key) {
        if(!(isdigit(*key) || isalpha(*key) || *key == '.' || *key == '_' || *key == '-'))
            return 0;

        key++;
    }

    return 1;
}

void strip_last_symbol(
    char *str,
    char symbol,
    SKIP_ESCAPED_CHARACTERS_OPTION skip_escaped_characters)
{
    char *end = str;

    while (*end && *end != symbol) {
        if (unlikely(skip_escaped_characters && *end == '\\')) {
            end++;
            if (unlikely(!*end))
                break;
        }
        end++;
    }
    if (likely(*end == symbol))
        *end = '\0';
}

char *strip_double_quotes(char *str, SKIP_ESCAPED_CHARACTERS_OPTION skip_escaped_characters)
{
    if (*str == '"') {
        str++;
        strip_last_symbol(str, '"', skip_escaped_characters);
    }

    return str;
}

struct label *create_label(char *key, char *value, LABEL_SOURCE label_source)
{
    size_t key_len = strlen(key), value_len = strlen(value);
    size_t n = sizeof(struct label) + key_len + 1 + value_len + 1;
    struct label *result = callocz(1,n);
    if (result != NULL) {
        char *c = (char *)result;
        c += sizeof(struct label);
        strcpy(c, key);
        result->key = c;
        c += key_len + 1;
        strcpy(c, value);
        result->value = c;
        result->label_source = label_source;
        result->key_hash = simple_hash(result->key);
    }
    return result;
}

void free_label_list(struct label *labels)
{
    while (labels != NULL)
    {
        struct label *current = labels;
        labels = labels->next;
        freez(current);
    }
}

void replace_label_list(RRDHOST *host, struct label *new_labels)
{
    rrdhost_check_rdlock(host);
    netdata_rwlock_wrlock(&host->labels_rwlock);
    struct label *old_labels = host->labels;
    host->labels = new_labels;
    netdata_rwlock_unlock(&host->labels_rwlock);

    free_label_list(old_labels);
}

struct label *add_label_to_list(struct label *l, char *key, char *value, LABEL_SOURCE label_source)
{
    struct label *lab = create_label(key, value, label_source);
    lab->next = l;
    return lab;
}

int label_list_contains(struct label *head, struct label *check)
{
    while (head != NULL)
    {
        if (head->key_hash == check->key_hash && !strcmp(head->key, check->key))
            return 1;
        head = head->next;
    }
    return 0;
}

/* Create a list with entries from both lists.
   If any entry in the low priority list is masked by an entry in the high priorty list then delete it.
*/
struct label *merge_label_lists(struct label *lo_pri, struct label *hi_pri)
{
    struct label *result = hi_pri;
    while (lo_pri != NULL)
    {
        struct label *current = lo_pri;
        lo_pri = lo_pri->next;
        if (!label_list_contains(result, current)) {
            current->next = result;
            result = current;
        }
        else
            freez(current);
    }
    return result;
}
