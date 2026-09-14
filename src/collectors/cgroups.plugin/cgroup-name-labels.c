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

CGROUP_NAME_READ_RESULT cgroup_name_read_response(int fd, char *buffer, size_t size, int timeout_ms) {
    if (fd < 0 || !buffer || size < 2)
        return CGROUP_NAME_READ_ERROR;

    usec_t deadline_ut = 0;
    if (timeout_ms >= 0) {
        usec_t now_ut = now_monotonic_usec();
        if (now_ut)
            deadline_ut = now_ut + (usec_t)timeout_ms * USEC_PER_MS;
    }

    size_t used = 0;
    for (;;) {
        int poll_wait_ms = timeout_ms;
        if (deadline_ut) {
            usec_t now_ut = now_monotonic_usec();
            if (now_ut >= deadline_ut)
                return CGROUP_NAME_READ_TIMEOUT;
            poll_wait_ms = (int)((deadline_ut - now_ut + USEC_PER_MS - 1) / USEC_PER_MS);
        }

        struct pollfd pfd = { .fd = fd, .events = POLLIN };
        int pr = poll(&pfd, 1, poll_wait_ms);
        if (pr < 0) {
            if (errno == EINTR)
                continue;
            return CGROUP_NAME_READ_ERROR;
        }
        if (pr == 0)
            return CGROUP_NAME_READ_TIMEOUT;
        if (pfd.revents & (POLLERR | POLLNVAL))
            return CGROUP_NAME_READ_ERROR;

        ssize_t bytes = read(fd, buffer + used, size - used - 1);
        if (bytes < 0) {
            if (errno == EINTR || errno == EAGAIN)
                continue;
            return CGROUP_NAME_READ_ERROR;
        }
        if (bytes == 0) {
            buffer[used] = '\0';
            return used == 0 ? CGROUP_NAME_READ_EMPTY : CGROUP_NAME_READ_INVALID;
        }

        used += (size_t)bytes;
        buffer[used] = '\0';
        char *newline = memchr(buffer + used - (size_t)bytes, '\n', (size_t)bytes);
        if (newline)
            return newline == buffer + used - 1 ? CGROUP_NAME_READ_COMPLETE : CGROUP_NAME_READ_INVALID;
        if (used == size - 1)
            return CGROUP_NAME_READ_INVALID;
    }
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
