// SPDX-License-Identifier: GPL-3.0-or-later
// Algorithms adapted from Netdata apps.plugin; see README.md for provenance.
#define _DARWIN_C_SOURCE
#define _DEFAULT_SOURCE
#define _POSIX_C_SOURCE 200809L
#include "native.h"
#include <dirent.h>
#include <errno.h>
#include <fcntl.h>
#include <limits.h>
#include <math.h>
#include <stdio.h>
#include <stdlib.h>
#include <string.h>
#include <sys/stat.h>
#include <time.h>
#include <unistd.h>

typedef struct {
    int number, type;
    ino_t inode;
    char *name;
    double check_at;
    unsigned stable;
} fd_entry;
typedef struct {
    apps_row row;
    uint64_t raw[METRIC_COUNT], raw_valid;
    double stamps[METRIC_COUNT], delta[METRIC_COUNT];
    int32_t parent_pid;
    uint64_t parent_start;
    uint32_t groups[3], retired_groups[3];
    int live, present, departed, read_state, accepted;
    fd_entry *fds;
    size_t fd_count;
    double pss, pss_ratio, pss_at, pss_attempt;
    int pss_valid, pss_force;
} pid_entry;
// Retired lifetime and ancestry belong to an incarnation, not its reusable PID slot.
typedef struct {
    int32_t pid, parent_pid;
    uint64_t start, parent_start, generation;
    double debt[5];
} retired_entry;
struct apps_scanner {
    char *root;
    int collect_fds, collect_pss, failed;
    long hz, page;
    double now, last_now, uptime;
    int fixture_time;
    int uptime_valid;
    pid_entry **pids;
    size_t count;
    retired_entry *retired;
    size_t retired_count, retired_capacity;
    apps_snapshot snap;
    apps_group_fd *groups;
    uint64_t cpu_raw[3];
    double cpu_at;
    int cpu_seen;
};

static double monotime(void) {
    struct timespec t;
    clock_gettime(CLOCK_MONOTONIC, &t);
    return t.tv_sec + t.tv_nsec / 1e9;
}
static double sample_time(apps_scanner *s) {
    return s->fixture_time ? s->now : monotime();
}
static void *resize(apps_scanner *s, void *p, size_t count, size_t size) {
    if (size && count > SIZE_MAX / size) {
        s->failed = 1;
        return NULL;
    }
    void *q = realloc(p, count * size);
    if (!q && count)
        s->failed = 1;
    return q;
}
static char *copy(apps_scanner *s, const char *p) {
    char *q = strdup(p);
    if (!q)
        s->failed = 1;
    return q;
}
static char *path(apps_scanner *s, int32_t pid, const char *file) {
    size_t n = strlen(s->root) + strlen(file) + 40;
    char *p = resize(s, NULL, n, 1);
    if (p) {
        if (pid)
            snprintf(p, n, "%s/%d/%s", s->root, pid, file);
        else
            snprintf(p, n, "%s/%s", s->root, file);
    }
    return p;
}
static char *read_file(apps_scanner *s, int32_t pid, const char *file, size_t *length) {
    char *p = path(s, pid, file);
    if (!p) {
        errno = ENOMEM;
        return NULL;
    }
    int fd = open(p, O_RDONLY | O_CLOEXEC | O_NOFOLLOW);
    int error = errno;
    free(p);
    s->snap.file_reads++;
    if (fd < 0) {
        s->snap.read_errors++;
        errno = error;
        return NULL;
    }
    FILE *f = fdopen(fd, "rb");
    if (!f) {
        error = errno;
        close(fd);
        s->snap.read_errors++;
        errno = error;
        return NULL;
    }
    size_t cap = 4096, len = 0;
    char *buf = resize(s, NULL, cap, 1);
    if (!buf) {
        fclose(f);
        errno = ENOMEM;
        return NULL;
    }
    for (;;) {
        size_t n = fread(buf + len, 1, cap - len - 1, f);
        len += n;
        if (ferror(f)) {
            error = errno;
            s->snap.read_errors++;
            free(buf);
            fclose(f);
            errno = error;
            return NULL;
        }
        if (feof(f))
            break;
        if (cap > SIZE_MAX / 2) {
            s->failed = 1;
            free(buf);
            fclose(f);
            errno = ENOMEM;
            return NULL;
        }
        cap *= 2;
        char *q = resize(s, buf, cap, 1);
        if (!q) {
            free(buf);
            fclose(f);
            errno = ENOMEM;
            return NULL;
        }
        buf = q;
    }
    fclose(f);
    buf[len] = 0;
    if (length)
        *length = len;
    return buf;
}
static int number(const char *text, uint64_t *v) {
    if (!text || *text < '0' || *text > '9')
        return 0;
    errno = 0;
    char *end;
    unsigned long long n = strtoull(text, &end, 10);
    if (errno || end == text || (*end && *end != ' ' && *end != '\t' && *end != '\n'))
        return 0;
    *v = n;
    return 1;
}
static int identifier(const char *text, uint64_t *v) {
    if (!number(text, v))
        return 0;
    for (const char *p = text; *p; p++)
        if (*p < '0' || *p > '9')
            return 0;
    return 1;
}
static int field(const char *buf, const char *key, uint64_t *v) {
    size_t n = strlen(key);
    for (const char *p = buf; p && *p;) {
        if (!strncmp(p, key, n) && p[n] == ':') {
            p += n + 1;
            while (*p == ' ' || *p == '\t')
                p++;
            return number(p, v);
        }
        p = strchr(p, '\n');
        if (p)
            p++;
    }
    return 0;
}
static void value(pid_entry *p, int metric, double v) {
    p->row.values[metric] = v;
    p->row.valid |= UINT64_C(1) << metric;
}
static void rate(pid_entry *p, int metric, uint64_t raw, double scale, double stamp) {
    uint64_t bit = UINT64_C(1) << metric;
    double elapsed = stamp - p->stamps[metric];
    if ((p->raw_valid & bit) && elapsed > 0 && raw >= p->raw[metric]) {
        p->delta[metric] = (double)(raw - p->raw[metric]);
        value(p, metric, p->delta[metric] * scale / elapsed);
    }
    p->raw[metric] = raw;
    p->stamps[metric] = stamp;
    p->raw_valid |= bit;
}
static void free_fds(pid_entry *p) {
    for (size_t i = 0; i < p->fd_count; i++)
        free(p->fds[i].name);
    free(p->fds);
    p->fds = NULL;
    p->fd_count = 0;
}
static void free_pid(pid_entry *p) {
    if (!p)
        return;
    free_fds(p);
    free(p->row.comm);
    free(p->row.cmdline);
    free(p);
}
static int compare_pid(const void *a, const void *b) {
    int32_t x = (*(pid_entry *const *)a)->row.pid, y = (*(pid_entry *const *)b)->row.pid;
    return (x > y) - (x < y);
}
static pid_entry *lookup(apps_scanner *s, int32_t pid) {
    size_t lo = 0, hi = s->count;
    while (lo < hi) {
        size_t m = lo + (hi - lo) / 2;
        int32_t x = s->pids[m]->row.pid;
        if (x == pid)
            return s->pids[m];
        if (x < pid)
            lo = m + 1;
        else
            hi = m;
    }
    return NULL;
}
static int parse_stat(char *buf, int32_t pid, apps_row *row, uint64_t fields[53]) {
    uint64_t parsed_pid;
    char *open = strchr(buf, '('), *close = strrchr(buf, ')');
    if (!number(buf, &parsed_pid) || parsed_pid != (uint64_t)pid || !open || !close || close <= open ||
        close[1] != ' ' || !close[2] || close[3] != ' ')
        return 0;
    char *pid_end = buf;
    while (*pid_end >= '0' && *pid_end <= '9')
        pid_end++;
    while (*pid_end == ' ')
        pid_end++;
    if (pid_end != open)
        return 0;
    row->state = close[2];
    char *cursor = NULL, *token;
    int index = 4;
    for (token = strtok_r(close + 4, " \n", &cursor); token; token = strtok_r(NULL, " \n", &cursor)) {
        if (index > 52)
            break;
        // Signed scheduling fields are irrelevant; required counters below are unsigned.
        if (!number(token, &fields[index]))
            fields[index] = UINT64_MAX;
        index++;
    }
    const int required[] = {4, 10, 11, 12, 13, 14, 15, 16, 17, 20, 22, 23, 24, 43, 44};
    for (size_t i = 0; i < sizeof(required) / sizeof(required[0]); i++)
        if (index <= required[i] || fields[required[i]] == UINT64_MAX)
            return 0;
    if (fields[4] > INT32_MAX || fields[20] > INT32_MAX)
        return 0;
    *close = 0;
    row->comm = open + 1;
    row->ppid = (int32_t)fields[4];
    row->start = fields[22];
    return 1;
}
static int fd_type(const char *name) {
    if (name[0] == '/')
        return FD_FILE;
    if (!strncmp(name, "socket:", 7))
        return FD_SOCKET;
    if (!strncmp(name, "pipe:", 5))
        return FD_PIPE;
    if (!strcmp(name, "anon_inode:inotify") || !strcmp(name, "inotify"))
        return FD_INOTIFY;
    if (!strcmp(name, "anon_inode:[eventfd]"))
        return FD_EVENT;
    if (!strcmp(name, "anon_inode:[timerfd]"))
        return FD_TIMER;
    if (!strcmp(name, "anon_inode:[signalfd]"))
        return FD_SIGNAL;
    if (!strcmp(name, "anon_inode:[eventpoll]"))
        return FD_EPOLL;
    return FD_OTHER;
}
static int compare_fd(const void *a, const void *b) {
    int x = ((const fd_entry *)a)->number, y = ((const fd_entry *)b)->number;
    return (x > y) - (x < y);
}
static fd_entry *find_fd(pid_entry *p, int number) {
    fd_entry key = {.number = number};
    return p->fd_count ? bsearch(&key, p->fds, p->fd_count, sizeof(key), compare_fd) : NULL;
}
static void scan_fds(apps_scanner *s, pid_entry *p) {
    char *dirname = path(s, p->row.pid, "fd");
    if (!dirname)
        return;
    DIR *dir = opendir(dirname);
    free(dirname);
    if (!dir) {
        s->snap.read_errors++;
        free_fds(p);
        return;
    }
    fd_entry *next = NULL;
    size_t count = 0, capacity = 0;
    int valid = 1;
    struct dirent *de;
    for (;;) {
        errno = 0;
        de = readdir(dir);
        if (!de) {
            if (errno)
                valid = 0;
            break;
        }
        uint64_t n;
        if (!identifier(de->d_name, &n) || n > INT_MAX)
            continue;
        fd_entry *old = find_fd(p, (int)n);
        fd_entry entry = {.number = (int)n, .inode = de->d_ino};
        if (old && old->name && old->inode == de->d_ino && old->check_at > s->now) {
            entry = *old;
            old->name = NULL;
        } else {
            char file[64];
            snprintf(file, sizeof(file), "fd/%d", (int)n);
            char *filename = path(s, p->row.pid, file);
            if (!filename) {
                valid = 0;
                break;
            }
            size_t cap = 256;
            char *link = NULL;
            for (;;) {
                char *q = resize(s, link, cap, 1);
                if (!q) {
                    free(link);
                    link = NULL;
                    break;
                }
                link = q;
                ssize_t len = readlink(filename, link, cap - 1);
                s->snap.link_reads++;
                if (len < 0) {
                    free(link);
                    link = NULL;
                    s->snap.read_errors++;
                    break;
                }
                if ((size_t)len < cap - 1) {
                    link[len] = 0;
                    break;
                }
                if (cap > SIZE_MAX / 2) {
                    s->failed = 1;
                    free(link);
                    link = NULL;
                    break;
                }
                cap *= 2;
            }
            free(filename);
            if (!link) {
                valid = 0;
                continue;
            }
            entry.name = link;
            entry.type = fd_type(link);
            entry.stable =
                old && old->name && old->inode == entry.inode && !strcmp(old->name, link) ? old->stable + 1 : 0;
            if (entry.stable > 60)
                entry.stable = 60;
            entry.check_at = s->now + entry.stable;
        }
        if (!entry.name) {
            valid = 0;
            break;
        }
        if (count == capacity) {
            capacity = capacity ? capacity * 2 : 32;
            fd_entry *q = resize(s, next, capacity, sizeof(*q));
            if (!q) {
                free(entry.name);
                valid = 0;
                break;
            }
            next = q;
        }
        next[count++] = entry;
        p->row.fds[entry.type]++;
    }
    closedir(dir);
    free_fds(p);
    p->fds = next;
    p->fd_count = count;
    if (count > 1)
        qsort(next, count, sizeof(*next), compare_fd);
    p->row.fd_valid = valid && !s->failed;
    char *limits = read_file(s, p->row.pid, "limits", NULL);
    if (limits) {
        char *line = strstr(limits, "Max open files");
        uint64_t max;
        if (line) {
            line += strlen("Max open files");
            while (*line == ' ' || *line == '\t')
                line++;
            if (number(line, &max) && max && p->row.fd_valid)
                value(p, FD_PERCENT, 100.0 * count / max);
        }
        free(limits);
    }
}

static void scan_optional(apps_scanner *s, pid_entry *p, const char *status, double status_at) {
    const char *keys[] = {"VmSize", "VmRSS", "VmSwap"};
    const int metrics[] = {VMEM, RSS, SWAP};
    uint64_t v;
    for (size_t i = 0; i < 3; i++)
        if (field(status, keys[i], &v))
            value(p, metrics[i], (double)v * 1024);
    uint64_t file, shmem;
    if (field(status, "RssFile", &file) && field(status, "RssShmem", &shmem))
        value(p, SHARED, ((double)file + shmem) * 1024);
    if (field(status, "voluntary_ctxt_switches", &v))
        rate(p, VOLUNTARY, v, 1, status_at);
    if (field(status, "nonvoluntary_ctxt_switches", &v))
        rate(p, INVOLUNTARY, v, 1, status_at);
    double io_at = sample_time(s);
    char *io = read_file(s, p->row.pid, "io", NULL);
    if (io) {
        const char *names[] = {"read_bytes", "write_bytes", "rchar", "wchar", "syscr", "syscw"};
        for (int i = 0; i < 6; i++)
            if (field(io, names[i], &v))
                rate(p, READ_BYTES + i, v, 1, io_at);
        free(io);
    }
    size_t len;
    char *cmd = read_file(s, p->row.pid, "cmdline", &len);
    free(p->row.cmdline);
    p->row.cmdline = NULL;
    // Preserve procfs argument boundaries; Go renders a separate matcher string.
    p->row.cmdline_len = cmd ? len : 0;
    p->row.cmdline = cmd;
    if (s->collect_fds)
        scan_fds(s, p);
}

// Snapshot only accepted lifetime totals; transient read failures never enter here.
static void retire_pid(apps_scanner *s, pid_entry *p) {
    if (p->departed)
        return;
    if (p->accepted) {
        if (s->retired_count == s->retired_capacity) {
            size_t capacity = s->retired_capacity ? s->retired_capacity * 2 : 32;
            retired_entry *next = resize(s, s->retired, capacity, sizeof(*next));
            if (!next)
                return;
            s->retired = next;
            s->retired_capacity = capacity;
        }
        retired_entry *r = &s->retired[s->retired_count++];
        *r = (retired_entry){.pid = p->row.pid,
                             .start = p->row.start,
                             .parent_pid = p->parent_pid,
                             .parent_start = p->parent_start,
                             .generation = s->snap.generation};
        const int own[] = {CPU_USER, CPU_SYSTEM, CPU_GUEST, MINFLT, MAJFLT};
        const int child[] = {CHILD_USER, CHILD_SYSTEM, CHILD_GUEST, CHILD_MINFLT, CHILD_MAJFLT};
        for (int m = 0; m < 5; m++)
            r->debt[m] = (double)p->raw[own[m]] + p->raw[child[m]];
    }
    p->departed = 1;
    free_fds(p);
}

// A vanished required file is an exit only when the process itself is gone.
// Permission, malformed-file and isolated missing-file failures remain gaps.
static void check_disappearance(apps_scanner *s, pid_entry *p, int error) {
    if (error == ESRCH) {
        p->present = 0;
        return;
    }
    if (error != ENOENT)
        return;
    char *directory = path(s, p->row.pid, "");
    if (!directory)
        return;
    struct stat st;
    if (stat(directory, &st) != 0 && (errno == ENOENT || errno == ESRCH))
        p->present = 0;
    free(directory);
}

static void scan_pid(apps_scanner *s, pid_entry *p) {
    uint64_t f[53] = {0};
    apps_row parsed = {0};
    double stat_at = sample_time(s);
    char *buf = read_file(s, p->row.pid, "stat", NULL);
    if (!buf) {
        check_disappearance(s, p, errno);
        return;
    }
    if (!parse_stat(buf, p->row.pid, &parsed, f)) {
        s->snap.read_errors++;
        free(buf);
        return;
    }
    if (p->row.start != parsed.start || p->departed) {
        retire_pid(s, p);
        if (s->failed) {
            free(buf);
            return;
        }
        int32_t pid = p->row.pid;
        uint32_t retired[3];
        for (int axis = 0; axis < 3; axis++)
            retired[axis] = p->groups[axis] ? p->groups[axis] : p->retired_groups[axis];
        free_fds(p);
        free(p->row.comm);
        free(p->row.cmdline);
        memset(p, 0, sizeof(*p));
        p->row.pid = pid;
        p->present = 1;
        memcpy(p->retired_groups, retired, sizeof(retired));
    }
    p->row.start = parsed.start;
    p->row.ppid = parsed.ppid;
    p->row.state = parsed.state;
    free(p->row.comm);
    p->row.comm = copy(s, parsed.comm);
    free(buf);
    double status_at = sample_time(s);
    char *status = read_file(s, p->row.pid, "status", NULL);
    uint64_t uid, gid;
    if (!status) {
        check_disappearance(s, p, errno);
        return;
    }
    // Without identity we cannot safely attribute a process to a user/group.
    if (!field(status, "Uid", &uid) || !field(status, "Gid", &gid) || uid > UINT32_MAX || gid > UINT32_MAX) {
        s->snap.read_errors++;
        free(status);
        return;
    }
    p->row.uid = (uint32_t)uid;
    p->row.gid = (uint32_t)gid;
    value(p, THREADS, f[20]);
    value(p, VMEM, f[23]);
    value(p, RSS, (double)f[24] * s->page);
    if (s->uptime_valid)
        value(p, UPTIME, fmax(0, s->uptime - (double)f[22] / s->hz));
    scan_optional(s, p, status, status_at);
    free(status);
    // A process may exit and reuse its PID between independent procfs reads.
    char *verify = read_file(s, p->row.pid, "stat", NULL);
    int verify_error = verify ? 0 : errno;
    apps_row identity = {0};
    uint64_t ignored[53] = {0};
    int identity_valid = verify && parse_stat(verify, p->row.pid, &identity, ignored);
    if (!identity_valid || identity.start != p->row.start) {
        if (!verify)
            check_disappearance(s, p, verify_error);
        else if (identity_valid && identity.start != p->row.start)
            p->present = 0;
        free(verify);
        p->raw_valid = 0;
        p->pss_valid = 0;
        free_fds(p);
        return;
    }
    free(verify);
    // Reconciliation may subtract only lifetime counters from accepted rows.
    const int indices[] = {14, 15, 43, 16, 17, 44, 10, 12, 11, 13};
    for (int i = 0; i < 10; i++) {
        uint64_t raw = f[indices[i]];
        if (i == CPU_USER)
            raw = raw >= f[43] ? raw - f[43] : 0;
        if (i == CHILD_USER)
            raw = raw >= f[44] ? raw - f[44] : 0;
        rate(p, i, raw, i < 6 ? 100.0 / s->hz : 1.0, stat_at);
    }
    p->live = 1;
    p->accepted = 1;
}

static void global_cpu(apps_scanner *s) {
    double stamp = sample_time(s);
    char *buf = read_file(s, 0, "stat", NULL);
    if (!buf)
        return;
    unsigned long long u, n, sys, idle, wait, irq, soft, steal, guest, gnice;
    int count = sscanf(buf, "cpu %llu %llu %llu %llu %llu %llu %llu %llu %llu %llu", &u, &n, &sys, &idle, &wait, &irq,
                       &soft, &steal, &guest, &gnice);
    s->snap.cpu_count = 0;
    for (char *p = buf; p && *p;) {
        if (!strncmp(p, "cpu", 3) && p[3] >= '0' && p[3] <= '9')
            s->snap.cpu_count++;
        p = strchr(p, '\n');
        if (p)
            p++;
    }
    if (count == 10) {
        uint64_t raw[3] = {u + n, sys, guest + gnice};
        raw[0] = raw[0] >= raw[2] ? raw[0] - raw[2] : 0;
        double dt = stamp - s->cpu_at;
        int valid = s->cpu_seen && dt > 0;
        for (int i = 0; i < 3; i++) {
            if (raw[i] < s->cpu_raw[i])
                valid = 0;
            s->snap.cpu[i] =
                raw[i] >= s->cpu_raw[i] && dt > 0 ? (double)(raw[i] - s->cpu_raw[i]) * 100.0 / s->hz / dt : 0;
            s->cpu_raw[i] = raw[i];
        }
        s->snap.cpu_valid = valid;
        s->cpu_seen = 1;
        s->cpu_at = stamp;
    } else
        s->snap.read_errors++;
    free(buf);
}

static pid_entry *parent(apps_scanner *s, pid_entry *p) {
    pid_entry *pp = lookup(s, p->parent_pid);
    return pp && pp != p && pp->row.start <= p->row.start && pp->row.start == p->parent_start ? pp : NULL;
}
static int compare_retired(const void *a, const void *b) {
    const retired_entry *x = a, *y = b;
    if (x->pid != y->pid)
        return x->pid > y->pid ? 1 : -1;
    return (x->start > y->start) - (x->start < y->start);
}
static retired_entry *find_retired(apps_scanner *s, int32_t pid, uint64_t start) {
    retired_entry key = {.pid = pid, .start = start};
    return s->retired_count ? bsearch(&key, s->retired, s->retired_count, sizeof(key), compare_retired) : NULL;
}
static pid_entry *surviving_ancestor(apps_scanner *s, const retired_entry *r) {
    int32_t pid = r->parent_pid;
    uint64_t start = r->parent_start;
    // Both indexes use incarnation identity; reused parent PIDs cannot redirect debt.
    for (size_t hops = 0; pid && hops < s->count + s->retired_count; hops++) {
        pid_entry *p = lookup(s, pid);
        if (p && p->row.start == start && p->accepted) {
            if (p->live)
                return p;
            // A read gap does not transfer this parent's child lifetime to its ancestors.
            if (p->present)
                return NULL;
            pid = p->parent_pid;
            start = p->parent_start;
            continue;
        }
        retired_entry *ancestor = find_retired(s, pid, start);
        if (!ancestor)
            return NULL;
        pid = ancestor->parent_pid;
        start = ancestor->parent_start;
    }
    return NULL;
}
static int retirement_expired(apps_scanner *s, const retired_entry *r) {
    return s->snap.generation - r->generation > 1;
}
static void prune_retired(apps_scanner *s) {
    // Preserve ancestry needed by younger retirements without extending old debt's grace.
    for (size_t i = 0; i < s->retired_count; i++) {
        retired_entry *r = &s->retired[i];
        if (retirement_expired(s, r))
            continue;
        for (size_t hops = 0; hops < s->retired_count; hops++) {
            retired_entry *ancestor = find_retired(s, r->parent_pid, r->parent_start);
            if (!ancestor || !retirement_expired(s, ancestor))
                break;
            r->parent_pid = ancestor->parent_pid;
            r->parent_start = ancestor->parent_start;
        }
    }
    size_t kept = 0;
    for (size_t i = 0; i < s->retired_count; i++)
        if (!retirement_expired(s, &s->retired[i]))
            s->retired[kept++] = s->retired[i];
    s->retired_count = kept;
    // Release peak churn storage once the retirement window becomes empty.
    if (!kept) {
        free(s->retired);
        s->retired = NULL;
        s->retired_capacity = 0;
    }
}
static void reconcile(apps_scanner *s) {
    const int child[] = {CHILD_USER, CHILD_SYSTEM, CHILD_GUEST, CHILD_MINFLT, CHILD_MAJFLT};
    for (size_t i = 0; i < s->count; i++)
        if (!s->pids[i]->present)
            retire_pid(s, s->pids[i]);
    if (s->retired_count > 1)
        qsort(s->retired, s->retired_count, sizeof(*s->retired), compare_retired);
    for (size_t i = 0; i < s->retired_count; i++) {
        retired_entry *r = &s->retired[i];
        if (retirement_expired(s, r))
            continue;
        pid_entry *pp = surviving_ancestor(s, r);
        if (!pp)
            continue;
        for (int m = 0; m < 5; m++) {
            int k = child[m];
            if (!(pp->row.valid & (UINT64_C(1) << k)))
                continue;
            double used = fmin(r->debt[m], pp->delta[k]);
            r->debt[m] -= used;
            pp->delta[k] -= used;
            double original = pp->row.values[k];
            pp->row.values[k] = (pp->delta[k] + used) > 0 ? original * pp->delta[k] / (pp->delta[k] + used) : 0;
        }
    }
    prune_retired(s);
    // Export reconciled rates unchanged; Go owns live-first host CPU normalization.
}
static int pss_priority(const void *a, const void *b) {
    const pid_entry *x = *(pid_entry *const *)a, *y = *(pid_entry *const *)b;
    if (x->pss_force != y->pss_force)
        return y->pss_force - x->pss_force;
    if (x->pss_attempt != y->pss_attempt)
        return x->pss_attempt < y->pss_attempt ? -1 : 1;
    return compare_pid(a, b);
}
static void sample_pss(apps_scanner *s) {
    if (!s->collect_pss)
        return;
    pid_entry **candidates = resize(s, NULL, s->count, sizeof(*candidates));
    if (!candidates && s->count)
        return;
    size_t count = 0;
    for (size_t i = 0; i < s->count; i++)
        if (s->pids[i]->live)
            candidates[count++] = s->pids[i];
    if (count > 1)
        qsort(candidates, count, sizeof(*candidates), pss_priority);
    double elapsed = s->last_now > 0 ? s->now - s->last_now : 10;
    size_t budget = (size_t)ceil(count * fmin(1, elapsed / 10.0));
    if (!budget && count)
        budget = 1;
    for (size_t i = 0; i < count; i++) {
        pid_entry *p = candidates[i];
        if (i < budget) {
            char *buf = read_file(s, p->row.pid, "smaps_rollup", NULL);
            uint64_t bytes;
            p->pss_attempt = s->now;
            p->pss_force = 0;
            apps_row identity = {0};
            uint64_t fields[53] = {0};
            char *stat = buf ? read_file(s, p->row.pid, "stat", NULL) : NULL;
            int same = stat && parse_stat(stat, p->row.pid, &identity, fields) && identity.start == p->row.start;
            free(stat);
            if (same && buf && field(buf, "Pss", &bytes)) {
                p->pss = (double)bytes * 1024;
                p->pss_ratio = p->row.values[RSS] > 0 ? fmin(1, p->pss / p->row.values[RSS]) : 1;
                p->pss_at = s->now;
                p->pss_valid = 1;
            } else
                p->pss_valid = 0;
            free(buf);
        }
        if (p->pss_valid) {
            value(p, PSS, p->pss);
            value(p, PSS_AGE, s->now - p->pss_at);
            if (p->row.valid & (UINT64_C(1) << RSS))
                value(p, ESTIMATE, p->row.values[RSS] * p->pss_ratio);
        }
    }
    free(candidates);
}
apps_scanner *apps_new(const char *root, int fds, int pss) {
    apps_scanner *s = calloc(1, sizeof(*s));
    if (!s)
        return NULL;
    s->root = strdup(root);
    if (!s->root) {
        free(s);
        return NULL;
    }
    s->collect_fds = fds;
    s->collect_pss = pss;
    s->hz = sysconf(_SC_CLK_TCK);
    s->page = sysconf(_SC_PAGESIZE);
    if (s->hz <= 0 || s->page <= 0) {
        apps_close(s);
        return NULL;
    }
    return s;
}
void apps_close(apps_scanner *s) {
    if (!s)
        return;
    for (size_t i = 0; i < s->count; i++)
        free_pid(s->pids[i]);
    free(s->pids);
    free(s->retired);
    free(s->root);
    free(s->snap.rows);
    free(s->groups);
    free(s);
}
int apps_scan(apps_scanner *s, double now, apps_snapshot **out) {
    if (s->failed)
        return ENOMEM;
    DIR *dir = opendir(s->root);
    if (!dir)
        return errno;
    free(s->snap.rows);
    s->snap.rows = NULL;
    uint64_t generation = s->snap.generation + 1;
    memset(&s->snap, 0, sizeof(s->snap));
    s->snap.generation = generation;
    s->fixture_time = now > 0;
    s->now = now > 0 ? now : monotime();
    for (size_t i = 0; i < s->count; i++) {
        pid_entry *p = s->pids[i];
        p->live = 0;
        p->present = 0;
        p->read_state = 0;
        p->row.valid = 0;
        p->row.fd_valid = 0;
        memset(p->row.values, 0, sizeof(p->row.values));
        memset(p->row.fds, 0, sizeof(p->row.fds));
        memset(p->delta, 0, sizeof(p->delta));
    }
    pid_entry **added = NULL;
    size_t added_count = 0, capacity = 0;
    struct dirent *de;
    int error = 0;
    for (;;) {
        errno = 0;
        de = readdir(dir);
        if (!de) {
            error = errno;
            break;
        }
        uint64_t id;
        if (!identifier(de->d_name, &id) || !id || id > INT32_MAX)
            continue;
        pid_entry *p = lookup(s, (int32_t)id);
        if (p) {
            p->present = 1;
            continue;
        }
        if (added_count == capacity) {
            capacity = capacity ? capacity * 2 : 128;
            pid_entry **q = resize(s, added, capacity, sizeof(*q));
            if (!q)
                break;
            added = q;
        }
        p = calloc(1, sizeof(*p));
        if (!p) {
            s->failed = 1;
            break;
        }
        p->row.pid = (int32_t)id;
        p->present = 1;
        added[added_count++] = p;
    }
    closedir(dir);
    if (added_count) {
        pid_entry **q = resize(s, s->pids, s->count + added_count, sizeof(*q));
        if (q) {
            s->pids = q;
            memcpy(s->pids + s->count, added, added_count * sizeof(*added));
            s->count += added_count;
            qsort(s->pids, s->count, sizeof(*q), compare_pid);
        } else
            for (size_t i = 0; i < added_count; i++)
                free_pid(added[i]);
    }
    free(added);
    if (error || s->failed)
        return error ? error : ENOMEM;
    global_cpu(s);
    char *uptime = read_file(s, 0, "uptime", NULL);
    s->uptime_valid = uptime && sscanf(uptime, "%lf", &s->uptime) == 1 && isfinite(s->uptime) && s->uptime >= 0;
    free(uptime);
    // Read previously known parents before their children, as apps.plugin does.
    // An iterative traversal avoids making the C stack depend on process depth.
    pid_entry **chain = resize(s, NULL, s->count, sizeof(*chain));
    if (!chain && s->count)
        return ENOMEM;
    for (size_t i = 0; i < s->count; i++) {
        pid_entry *p = s->pids[i];
        size_t depth = 0;
        while (p && p->present && !p->read_state) {
            p->read_state = 1;
            chain[depth++] = p;
            p = parent(s, p);
        }
        while (depth) {
            p = chain[--depth];
            scan_pid(s, p);
            p->read_state = 2;
        }
    }
    free(chain);
    for (size_t i = 0; i < s->count; i++) {
        pid_entry *p = s->pids[i];
        if (!p->live)
            continue;
        pid_entry *pp = lookup(s, p->row.ppid);
        p->parent_pid = p->row.ppid;
        p->parent_start = pp && pp->row.start <= p->row.start ? pp->row.start : UINT64_MAX;
    }
    reconcile(s);
    sample_pss(s);
    s->snap.rows = resize(s, NULL, s->count, sizeof(*s->snap.rows));
    if (!s->snap.rows && s->count)
        return ENOMEM;
    for (size_t i = 0; i < s->count; i++)
        if (s->pids[i]->live)
            s->snap.rows[s->snap.count++] = s->pids[i]->row;
    s->last_now = s->now;
    *out = &s->snap;
    return s->failed ? ENOMEM : 0;
}

typedef struct {
    uint32_t group;
    const char *name;
    int type, valid;
} fd_pair;
static int compare_pair(const void *a, const void *b) {
    const fd_pair *x = a, *y = b;
    if (x->group != y->group)
        return x->group > y->group ? 1 : -1;
    if (!x->name || !y->name)
        return (x->name != NULL) - (y->name != NULL);
    return strcmp(x->name, y->name);
}
static int compare_u32(const void *a, const void *b) {
    uint32_t x = *(const uint32_t *)a, y = *(const uint32_t *)b;
    return (x > y) - (x < y);
}
int apps_finalize(apps_scanner *s, uint64_t generation, const apps_assignment *a, size_t count, apps_group_fd **groups,
                  size_t *group_count) {
    if (s->failed)
        return ENOMEM;
    if (!generation || generation != s->snap.generation || count != s->snap.count)
        return EINVAL;
    // Validate the complete assignment set before changing state.
    pid_entry **ordered = resize(s, NULL, count, sizeof(*ordered));
    if (!ordered && count)
        return ENOMEM;
    for (size_t i = 0; i < count; i++) {
        pid_entry *p = lookup(s, a[i].pid);
        if (!p || !p->live || p->row.start != a[i].start) {
            free(ordered);
            return EINVAL;
        }
        ordered[i] = p;
    }
    if (count > 1)
        qsort(ordered, count, sizeof(*ordered), compare_pid);
    for (size_t i = 1; i < count; i++)
        if (ordered[i] == ordered[i - 1]) {
            free(ordered);
            return EINVAL;
        }
    free(ordered);
    uint32_t *changed = resize(s, NULL, (count + s->count) * 6, sizeof(*changed));
    size_t changed_count = 0;
    if (!changed && (count + s->count))
        return ENOMEM;
    size_t pair_count = 0;
    for (size_t i = 0; i < count; i++) {
        pid_entry *p = lookup(s, a[i].pid);
        for (int axis = 0; axis < 3; axis++) {
            if (p->groups[axis] != a[i].groups[axis]) {
                if (p->groups[axis])
                    changed[changed_count++] = p->groups[axis];
                if (a[i].groups[axis])
                    changed[changed_count++] = a[i].groups[axis];
            }
            p->groups[axis] = a[i].groups[axis];
            if (p->groups[axis]) {
                if (p->fd_count > SIZE_MAX - pair_count - 1) {
                    free(changed);
                    return ENOMEM;
                }
                pair_count += p->fd_count + 1;
            }
        }
    }
    // Reused PIDs still invalidate the old incarnation's group PSS estimates.
    for (size_t i = 0; i < s->count; i++)
        for (int axis = 0; axis < 3; axis++)
            if (s->pids[i]->retired_groups[axis]) {
                changed[changed_count++] = s->pids[i]->retired_groups[axis];
                s->pids[i]->retired_groups[axis] = 0;
            }
    for (size_t i = 0; i < s->count; i++)
        if (s->pids[i]->departed)
            for (int axis = 0; axis < 3; axis++)
                if (s->pids[i]->groups[axis]) {
                    changed[changed_count++] = s->pids[i]->groups[axis];
                    s->pids[i]->groups[axis] = 0;
                }
    if (changed_count > 1)
        qsort(changed, changed_count, sizeof(*changed), compare_u32);
    for (size_t i = 0; i < s->count; i++)
        if (s->pids[i]->live)
            for (int axis = 0; axis < 3; axis++)
                if (changed_count &&
                    bsearch(&s->pids[i]->groups[axis], changed, changed_count, sizeof(*changed), compare_u32))
                    s->pids[i]->pss_force = 1;
    free(changed);
    fd_pair *pairs = resize(s, NULL, pair_count, sizeof(*pairs));
    if (!pairs && pair_count)
        return ENOMEM;
    size_t n = 0;
    for (size_t i = 0; i < count; i++) {
        pid_entry *p = lookup(s, a[i].pid);
        for (int axis = 0; axis < 3; axis++)
            if (p->groups[axis]) {
                pairs[n++] = (fd_pair){.group = p->groups[axis], .valid = p->row.fd_valid};
                for (size_t j = 0; j < p->fd_count; j++)
                    pairs[n++] = (fd_pair){.group = p->groups[axis],
                                           .name = p->fds[j].name,
                                           .type = p->fds[j].type,
                                           .valid = p->row.fd_valid};
            }
    }
    if (n > 1)
        qsort(pairs, n, sizeof(*pairs), compare_pair);
    size_t distinct_groups = 0;
    for (size_t i = 0; i < n; i++)
        if (!i || pairs[i].group != pairs[i - 1].group)
            distinct_groups++;
    free(s->groups);
    s->groups = resize(s, NULL, distinct_groups, sizeof(*s->groups));
    if (!s->groups && distinct_groups) {
        free(pairs);
        return ENOMEM;
    }
    size_t used = 0;
    for (size_t i = 0; i < n; i++) {
        if (!used || s->groups[used - 1].id != pairs[i].group) {
            memset(&s->groups[used], 0, sizeof(*s->groups));
            s->groups[used].id = pairs[i].group;
            s->groups[used].valid = 1;
            used++;
        }
        apps_group_fd *g = &s->groups[used - 1];
        if (!pairs[i].valid)
            g->valid = 0;
        if (pairs[i].name && (!i || pairs[i - 1].group != pairs[i].group || !pairs[i - 1].name ||
                              strcmp(pairs[i - 1].name, pairs[i].name)))
            g->counts[pairs[i].type]++;
    }
    free(pairs);
    // Incarnation retirement records own reconciliation; dead PID slots can be released.
    size_t kept = 0;
    for (size_t i = 0; i < s->count; i++) {
        if (s->pids[i]->departed)
            free_pid(s->pids[i]);
        else
            s->pids[kept++] = s->pids[i];
    }
    s->count = kept;
    *groups = s->groups;
    *group_count = used;
    return 0;
}
