// SPDX-License-Identifier: GPL-3.0-or-later

#include "apps-cgroups-path.h"

#ifdef OS_LINUX

static bool cgroup_controller_token_matches(const char *controllers, size_t len, const char *needle)
{
    size_t needle_len = strlen(needle);
    const char *p = controllers;
    const char *end = controllers + len;

    while (p < end) {
        const char *comma = memchr(p, ',', (size_t)(end - p));
        const char *token_end = comma ? comma : end;
        size_t token_len = (size_t)(token_end - p);

        if (token_len == needle_len && memcmp(p, needle, needle_len) == 0)
            return true;

        if (!comma)
            break;

        p = comma + 1;
    }

    return false;
}

static unsigned cgroup_v1_precedence(const char *controllers, size_t len)
{
    if (cgroup_controller_token_matches(controllers, len, "cpuacct"))
        return 1;

    if (cgroup_controller_token_matches(controllers, len, "blkio"))
        return 2;

    if (cgroup_controller_token_matches(controllers, len, "memory"))
        return 3;

    if (len == strlen("name=systemd") && memcmp(controllers, "name=systemd", len) == 0)
        return 4;

    return 5;
}

static bool cgroup_copy_path(const char *path, size_t len, char *dst, size_t dst_size)
{
    if (!path || len == 0 || path[0] != '/' || !dst || dst_size == 0 || len >= dst_size)
        return false;

    memcpy(dst, path, len);
    dst[len] = '\0';
    return true;
}

bool apps_cgroup_parse_proc_pid_cgroup_content(const char *content, char *dst, size_t dst_size)
{
    if (!content || !dst || dst_size == 0)
        return false;

    dst[0] = '\0';

    const char *best_path = NULL;
    size_t best_path_len = 0;
    unsigned best_score = UINT_MAX;

    const char *line = content;
    while (*line) {
        const char *line_end = strchr(line, '\n');
        if (!line_end)
            line_end = line + strlen(line);

        const char *next_line = (*line_end == '\n') ? line_end + 1 : line_end;
        const char *field_end = line_end;
        if (field_end > line && field_end[-1] == '\r')
            field_end--;

        const char *first_colon = memchr(line, ':', (size_t)(field_end - line));
        if (!first_colon) {
            line = next_line;
            continue;
        }

        const char *second_colon = memchr(first_colon + 1, ':', (size_t)(field_end - first_colon - 1));
        if (!second_colon) {
            line = next_line;
            continue;
        }

        const char *path = second_colon + 1;
        size_t path_len = (size_t)(field_end - path);
        if (path_len == 0 || path[0] != '/') {
            line = next_line;
            continue;
        }

        bool is_v2 = (first_colon == line + 1 && line[0] == '0' && second_colon == first_colon + 1);
        if (is_v2)
            return cgroup_copy_path(path, path_len, dst, dst_size);

        const char *controllers = first_colon + 1;
        size_t controllers_len = (size_t)(second_colon - controllers);
        unsigned score = cgroup_v1_precedence(controllers, controllers_len);

        if (score < best_score) {
            best_score = score;
            best_path = path;
            best_path_len = path_len;
        }

        line = next_line;
    }

    return cgroup_copy_path(best_path, best_path_len, dst, dst_size);
}

#endif /* OS_LINUX */
