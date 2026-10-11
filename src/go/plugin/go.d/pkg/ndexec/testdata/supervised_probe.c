// SPDX-License-Identifier: GPL-3.0-or-later
#define _GNU_SOURCE 1
#include <dirent.h>
#include <errno.h>
#include <signal.h>
#include <stdio.h>
#include <stdlib.h>
#include <string.h>
#include <sys/prctl.h>
#include <unistd.h>
#include <time.h>

static void delay_ms(long milliseconds) {
    struct timespec remaining = {
        .tv_sec = milliseconds / 1000,
        .tv_nsec = (milliseconds % 1000) * 1000000L,
    };
    while (nanosleep(&remaining, &remaining) != 0) {
        if (errno != EINTR)
            _exit(14);
    }
}

extern char **environ;
int main(int argc, char **argv) {
    alarm(20); // Independent bound if supervision is broken.
    char mode[32] = "silent", cwd[256];
    FILE *config = fopen(PROBE_MODE_FILE, "r");
    if (!config || fscanf(config, "%31s", mode) != 1) return 2;
    fclose(config);
    int extra = 0;
    DIR *fds = opendir("/proc/self/fd");
    if (!fds || !getcwd(cwd, sizeof(cwd))) return 3;
    struct dirent *entry;
    while ((entry = readdir(fds))) {
        int fd = atoi(entry->d_name);
        if (fd > 2 && fd != dirfd(fds)) extra++;
    }
    closedir(fds);
    int env = 0;
    for (char **p = environ; *p; p++) env++;
    if (argc != 3 || strcmp(argv[0], "/usr/bin/tegrastats") || strcmp(argv[1], "--interval") || strcmp(argv[2], "1000")) return 4;
    pid_t leaf = 0;
    if (!strcmp(mode, "group") || !strcmp(mode, "escaped") || !strcmp(mode, "adopted")) {
        int ready[2];
        if (pipe(ready)) return 5;
        pid_t child = fork();
        if (child < 0) return 6;
        if (!child) {
            close(ready[0]);
            if (!strcmp(mode, "adopted")) {
                pid_t grandchild = fork();
                if (grandchild < 0) _exit(7);
                if (grandchild) _exit(0);
            }
            if (!strcmp(mode, "escaped") && setsid() < 0) _exit(8);
            leaf = getpid();
            if (write(ready[1], &leaf, sizeof(leaf)) != sizeof(leaf)) _exit(9);
            close(ready[1]);
            if (!strcmp(mode, "escaped")) { delay_ms(2000); _exit(0); }
            alarm(20);
            for (;;) pause();
        }
        close(ready[1]);
        if (read(ready[0], &leaf, sizeof(leaf)) != sizeof(leaf)) return 10;
        close(ready[0]);
    }
    printf("%d %d %d %d %d %d %s %d\n", getpid(), getppid(), getuid(), getgid(), extra, env, cwd, leaf);
    fflush(stdout);
    if (!strcmp(mode, "loss")) {
        delay_ms(200);
        if (kill(getppid(), SIGKILL)) return 11;
    }
    if (!strcmp(mode, "signal")) {
        delay_ms(200);
        if (kill(getppid(), SIGTERM)) return 13;
    }
    if (!strcmp(mode, "exit")) return 0;
    if (!strcmp(mode, "blocked")) {
        char data[4096]; memset(data, 'x', sizeof(data));
        for (;;) if (write(STDOUT_FILENO, data, sizeof(data)) < 0) return 12;
    }
    if (!strcmp(mode, "spoof")) { puts("NDSUP1 cleaned 0"); fflush(stdout); }
    for (;;) pause();
}
