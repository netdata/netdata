// SPDX-License-Identifier: GPL-3.0-or-later

#include "cgroup-name-labels.h"

#define CGROUP_NETDATA_CLOUD_LABEL_PREFIX "netdata.cloud/"
#define CGROUP_K8S_NETDATA_CLOUD_LABEL_PREFIX "k8s_netdata.cloud/"
#define CGROUP_RENAME_LABEL "cgroup.name="
#define CGROUP_IGNORE_LABEL "ignore="

static char *cgroup_next_label_pair(char **labels) {
    if (!labels || !*labels)
        return NULL;

    char *s = *labels;
    while (*s == ',' || isspace((uint8_t)*s))
        s++;

    if (!*s) {
        *labels = NULL;
        return NULL;
    }

    char *pair = s;
    char quote = '\0';
    bool escaped = false;

    for (; *s; s++) {
        if (quote) {
            if (escaped) {
                escaped = false;
                continue;
            }

            if (*s == '\\') {
                escaped = true;
                continue;
            }

            if (*s == quote)
                quote = '\0';

            continue;
        }

        if (*s == '\'' || *s == '"') {
            quote = *s;
            continue;
        }

        if (*s == ',') {
            *s++ = '\0';
            *labels = s;
            return pair;
        }
    }

    *labels = NULL;
    return pair;
}

bool cgroup_name_line_is_complete(const char *data) {
    return data && strchr(data, '\n');
}

char *cgroup_parse_name_and_labels(RRDLABELS *labels, char *data, bool *ignored) {
    rrdlabels_unmark_all(labels);

    // the first word, up to the first space is the name
    char *name = strsep_skip_consecutive_separators(&data, " ");

    bool ign = false;

    // the rest are key=value pairs separated by comma
    char *pair;
    while((pair = cgroup_next_label_pair(&data))) {

        bool kubernetes_prefixed = false;
        char *key = NULL;
        if(strncmp(pair, CGROUP_NETDATA_CLOUD_LABEL_PREFIX, sizeof(CGROUP_NETDATA_CLOUD_LABEL_PREFIX) - 1) == 0)
            key = &pair[sizeof(CGROUP_NETDATA_CLOUD_LABEL_PREFIX) - 1];
        else if(strncmp(pair, CGROUP_K8S_NETDATA_CLOUD_LABEL_PREFIX,
                        sizeof(CGROUP_K8S_NETDATA_CLOUD_LABEL_PREFIX) - 1) == 0) {
            key = &pair[sizeof(CGROUP_K8S_NETDATA_CLOUD_LABEL_PREFIX) - 1];
            kubernetes_prefixed = true;
        }

        if(key) {
            // a netdata.cloud label
            if(strncmp(key, CGROUP_RENAME_LABEL, sizeof(CGROUP_RENAME_LABEL) - 1) == 0) {
                char *n = &key[sizeof(CGROUP_RENAME_LABEL) - 1];
                size_t len = strlen(n);
                if(len > 0 && n[0] == '"' && n[len - 1] == '"') {
                    n[len - 1] = '\0';
                    n++;
                }
                if(*n) name = n;

                // no need to add this label
            }
            else if(strncmp(key, CGROUP_IGNORE_LABEL, sizeof(CGROUP_IGNORE_LABEL) - 1) == 0) {
                char *v = &key[sizeof(CGROUP_IGNORE_LABEL) - 1];
                if(strcasecmp(v, "\"true\"") == 0 || strcasecmp(v, "\"yes\"") == 0)
                    ign = true;
                else
                    ign = false;

                // no need to add this label
            }
            else if(kubernetes_prefixed)
                // Kubernetes annotations other than controls remain labels.
                rrdlabels_add_pair(labels, pair, RRDLABEL_SRC_AUTO | RRDLABEL_SRC_K8S);
        }
        else
            // add the label as-is
            rrdlabels_add_pair(labels, pair, RRDLABEL_SRC_AUTO | RRDLABEL_SRC_K8S);
    }

    rrdlabels_remove_all_unmarked(labels);

    if(ignored)
        *ignored = ign;

    return name;
}
