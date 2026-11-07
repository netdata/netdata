// SPDX-License-Identifier: GPL-3.0-or-later

#include "../apps_os_linux.c"
#include <sys/prctl.h>

bool debug_enabled = false;
bool enable_guest_charts = false;
bool show_guest_time = false;
proc_state proc_state_count[PROC_STATUS_END];

void update_pid_comm(struct pid_stat *p, const char *comm) {
    string_freez(p->comm_orig);
    p->comm_orig = string_strdupz(comm);
}

#define CHECK(condition) do { if(!(condition)) { \
    fprintf(stderr, "%s:%d: %s\n", __func__, __LINE__, #condition); return false; \
} } while(0)

static void stat_fixture(char *buf, size_t size, const char *comm, size_t count) {
    size_t len = snprintf(buf, size, "123 (%s) S", comm);
    for(size_t field = 3; field < count; field++)
        len += snprintf(buf + len, size - len, " %zu", field);
    snprintf(buf + len, size - len, "\n");
}

static bool test_names(void) {
    const char *names[] = {"normal", "next-server (v", "bad) name", "a(b)c", "(", ")", "", "a\nb\tc", "))(("};
    for(size_t i = 0; i < sizeof(names) / sizeof(names[0]); i++) {
        char buf[4096], *fields[44];
        stat_fixture(buf, sizeof(buf), names[i], 44);
        CHECK(parse_proc_pid_stat(buf, fields, 44));
        CHECK(strcmp(fields[0], "123") == 0);
        CHECK(strcmp(fields[1], names[i]) == 0);
        CHECK(strcmp(fields[2], "S") == 0);
        for(size_t f = 3; f < 44; f++)
            CHECK(str2uint64_t(fields[f], NULL) == f);
    }
    return true;
}

static bool test_invalid_records(void) {
    const char *invalid[] = {"", "123 no delimiters\n", "123 (name S 1\n", "123 name) S 1\n",
                             "123 (name) SS 1\n", "123 (name) S 1\n"};
    for(size_t i = 0; i < sizeof(invalid) / sizeof(invalid[0]); i++) {
        char buf[4096], *fields[44];
        snprintf(buf, sizeof(buf), "%s", invalid[i]);
        CHECK(!parse_proc_pid_stat(buf, fields, 22));
    }
    char buf[4096], *fields[44];
    stat_fixture(buf, sizeof(buf), "name", 22);
    CHECK(!parse_proc_pid_stat(buf, fields, 44));
    stat_fixture(buf, sizeof(buf), "name", 44);
    buf[strlen(buf) - 1] = '\0';
    CHECK(!parse_proc_pid_stat(buf, fields, 44));
    stat_fixture(buf, sizeof(buf), "name", 44);
    char *number = strstr(buf, " 13 ");
    CHECK(number);
    number[1] = 'x';
    CHECK(!parse_proc_pid_stat(buf, fields, 44));
    return true;
}

static bool test_reader(void) {
    FILE *fp = tmpfile();
    CHECK(fp);
    char path[64];
    snprintf(path, sizeof(path), "/proc/self/fd/%d", fileno(fp));
    struct pid_stat p = {.pid = 123, .stat_filename = path, .stat_collected_usec = 2 * USEC_PER_SEC,
                         .last_stat_collected_usec = USEC_PER_SEC};
    char buf[4096];
    stat_fixture(buf, sizeof(buf), "next-server (v", 44);
    CHECK(fputs(buf, fp) >= 0 && fflush(fp) == 0);
    enable_guest_charts = true;
    int saved_open_flags = procfile_open_flags;
    procfile_open_flags = O_RDONLY | O_NOFOLLOW;
    CHECK(!apps_os_read_pid_stat_linux(&p, NULL));
    CHECK(!p.comm_orig && !p.starttime);
    procfile_open_flags = saved_open_flags;
    CHECK(apps_os_read_pid_stat_linux(&p, NULL));
    CHECK(p.ppid == 3 && p.state == 'S' && p.starttime == 21 && p.values[PDF_THREADS] == 19);
    CHECK(strcmp(string2str(p.comm_orig), "next-server (v") == 0);
    const PID_FIELD counters[] = {PDF_MINFLT, PDF_CMINFLT, PDF_MAJFLT, PDF_CMAJFLT,
                                 PDF_UTIME, PDF_STIME, PDF_CUTIME, PDF_CSTIME, PDF_GTIME, PDF_CGTIME};
    const uint64_t expected[] = {9, 10, 11, 12, 13, 14, 15, 16, 42, 43};
    for(size_t i = 0; i < sizeof(counters) / sizeof(counters[0]); i++)
        CHECK(p.raw[counters[i]] == expected[i]);

    struct pid_stat before = p;
    rewind(fp);
    CHECK(ftruncate(fileno(fp), 0) == 0);
    CHECK(fputs("123 (broken) S 1\n", fp) >= 0 && fflush(fp) == 0);
    CHECK(!apps_os_read_pid_stat_linux(&p, NULL));
    CHECK(memcmp(&p, &before, sizeof(p)) == 0);
    string_freez(p.comm_orig);
    CHECK(fclose(fp) == 0);
    enable_guest_charts = false;
    return true;
}

static bool test_live_kernel_names(void) {
    char original[16];
    CHECK(prctl(PR_GET_NAME, original) == 0);
    const char *names[] = {"normal", "next-server (v", "bad) name", "a\nb\tc", ""};
    struct pid_stat p = {.pid = getpid(), .stat_filename = "/proc/self/stat",
                         .stat_collected_usec = 2 * USEC_PER_SEC, .last_stat_collected_usec = USEC_PER_SEC};
    for(size_t i = 0; i < sizeof(names) / sizeof(names[0]); i++) {
        CHECK(prctl(PR_SET_NAME, names[i]) == 0);
        CHECK(apps_os_read_pid_stat_linux(&p, NULL));
        CHECK(p.ppid == getppid());
        CHECK(strcmp(string2str(p.comm_orig), names[i]) == 0);
        CHECK(p.values[PDF_THREADS] >= 1 && p.starttime > 0);
    }
    CHECK(prctl(PR_SET_NAME, original) == 0);
    string_freez(p.comm_orig);
    return true;
}

int main(void) {
    if(!test_names() || !test_invalid_records() || !test_reader() || !test_live_kernel_names())
        return 1;
    puts("apps stat tests passed");
    return 0;
}
