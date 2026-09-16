// SPDX-License-Identifier: GPL-3.0-or-later
#include "config.h"

#include <errno.h>
#include <grp.h>
#include <pwd.h>
#include <signal.h>
#include <stdio.h>
#include <stdlib.h>
#include <string.h>
#include <unistd.h>
#ifdef __linux__
#include <linux/capability.h>
#include <sys/prctl.h>
#include <sys/syscall.h>
#endif

extern char **environ;

static void check(int result) {
    if (result == -1) {
        perror("probe");
        exit(1);
    }
}

int main(int argc, char **argv) {
    if (argc < 2)
        return 2;

    if (!strcmp(argv[1], "env")) {
        for (char **entry = environ; *entry; entry++)
            fwrite(*entry, 1, strlen(*entry) + 1, stdout);
    } else if (!strcmp(argv[1], "argv")) {
        for (int i = 2; i < argc; i++)
            fwrite(argv[i], 1, strlen(argv[i]) + 1, stdout);
    } else if (!strcmp(argv[1], "identity")) {
        printf("%lu %lu %lu %lu\n", (unsigned long)getuid(), (unsigned long)geteuid(),
               (unsigned long)getgid(), (unsigned long)getegid());
        int count = getgroups(0, NULL);
        check(count);
        gid_t *groups = calloc((size_t)count + 1, sizeof(*groups));
        if (!groups)
            return 1;
        check(getgroups(count, groups));
        for (int i = 0; i < count; i++)
            printf("%lu ", (unsigned long)groups[i]);
        printf("\n");
        free(groups);
        // A dropped root or saved ID must not be recoverable by the child.
        int regain_uid = setuid(0);
        int regain_gid = setgid(0);
        printf("%d %d\n", regain_uid, regain_gid);
#ifdef __linux__
        struct __user_cap_header_struct header = { .version = _LINUX_CAPABILITY_VERSION_3, .pid = 0 };
        struct __user_cap_data_struct caps[2] = {{0}, {0}};
        check(syscall(SYS_capget, &header, caps));
        printf("caps_empty=%d\n", !(caps[0].effective | caps[0].permitted | caps[0].inheritable |
                                    caps[1].effective | caps[1].permitted | caps[1].inheritable));
        printf("ambient=%d\n", prctl(PR_CAP_AMBIENT, PR_CAP_AMBIENT_IS_SET, CAP_NET_BIND_SERVICE, 0, 0));
#endif
    } else if (!strcmp(argv[1], "pid")) {
        printf("%lu\n", (unsigned long)getpid());
    } else if (!strcmp(argv[1], "exit") && argc == 3) {
        return atoi(argv[2]);
    } else if (!strcmp(argv[1], "signal")) {
        sigset_t mask;
        check(sigprocmask(SIG_SETMASK, NULL, &mask));
        struct sigaction action;
        check(sigaction(SIGPIPE, NULL, &action));
        if (action.sa_handler != SIG_DFL || sigismember(&mask, SIGPIPE))
            return 3;
        raise(SIGPIPE);
        return 4;
    } else if (!strcmp(argv[1], "launch-signals") && argc > 2) {
        struct sigaction action = { .sa_handler = SIG_IGN };
        sigemptyset(&action.sa_mask);
        check(sigaction(SIGPIPE, &action, NULL));
        sigset_t mask;
        sigemptyset(&mask);
        sigaddset(&mask, SIGPIPE);
        check(sigprocmask(SIG_BLOCK, &mask, NULL));
        execv(argv[2], &argv[2]);
        return 5;
    } else if (!strcmp(argv[1], "launch-duplicates") && argc > 2) {
        // Only synthetic entries; bypass libc setters to retain duplicate keys.
        char *env[] = {
            "USER=first", "USER=second", "LOGNAME=first", "LOGNAME=second",
            "HOME=first", "HOME=second", "SHELL=first", "SHELL=second",
            "LC_ALL=first", "LC_ALL=second", "TMPDIR=", "ND_AUTH=synthetic",
            "USER_SUFFIX=keep", "HOM=keep", "HOMELESS=keep", NULL
        };
        execve(argv[2], &argv[2], env);
        return 5;
#ifdef __linux__
    } else if (!strcmp(argv[1], "read-file") && argc == 3) {
        FILE *file = fopen(argv[2], "rb");
        if (!file)
            return 1;
        int ch;
        while ((ch = fgetc(file)) != EOF)
            putchar(ch);
        int failed = ferror(file);
        if (fclose(file) != 0)
            failed = 1;
        if (failed)
            return 1;
    } else if (!strcmp(argv[1], "launch-checked-file") && argc > 4) {
        // This executable is a synthetic setuid/file-capability parent. Verify
        // its authority before exec without exposing the fixture bytes.
        FILE *file = fopen(argv[2], "rb");
        if (!file)
            return 1;
        int ch = fgetc(file);
        int failed = ch == EOF || ferror(file);
        if (fclose(file) != 0)
            failed = 1;
        if (failed)
            return 1;
        struct __user_cap_header_struct header = { .version = _LINUX_CAPABILITY_VERSION_3, .pid = 0 };
        struct __user_cap_data_struct caps[2] = {{0}, {0}};
        check(syscall(SYS_capget, &header, caps));
        fprintf(stderr, "parent_uid=%lu parent_euid=%lu parent_cap_eff=%x%08x parent_readable=1\n",
                (unsigned long)getuid(), (unsigned long)geteuid(),
                caps[1].effective, caps[0].effective);
        if (fflush(stderr) != 0)
            return 1;
        execv(argv[3], &argv[3]);
        return 5;
    } else if (!strcmp(argv[1], "launch-real-root") && argc > 2) {
        struct passwd *pw = getpwnam(NETDATA_USER);
        if (!pw || pw->pw_uid == 0)
            return 2;
        check(initgroups(pw->pw_name, pw->pw_gid));
        check(setresgid(0, pw->pw_gid, 0));
        // Real root can regain effective root unless nd-run normalizes all IDs.
        check(setresuid(0, pw->pw_uid, 0));
        execv(argv[2], &argv[2]);
        return 5;
    } else if ((!strcmp(argv[1], "launch-caps") || !strcmp(argv[1], "launch-file-caps")) && argc > 2) {
        struct passwd *pw = getpwnam(NETDATA_USER);
        if (!pw || pw->pw_uid == 0)
            return 2;
        check(prctl(PR_SET_KEEPCAPS, 1, 0, 0, 0));
        check(initgroups(pw->pw_name, pw->pw_gid));
        check(setgid(pw->pw_gid));
        check(setuid(pw->pw_uid));
        struct __user_cap_header_struct header = { .version = _LINUX_CAPABILITY_VERSION_3, .pid = 0 };
        struct __user_cap_data_struct caps[2] = {{0}, {0}};
        int capability = !strcmp(argv[1], "launch-file-caps") ? CAP_DAC_OVERRIDE : CAP_NET_BIND_SERVICE;
        caps[0].effective = caps[0].permitted = caps[0].inheritable = 1U << capability;
        check(syscall(SYS_capset, &header, caps));
        check(prctl(PR_CAP_AMBIENT, PR_CAP_AMBIENT_RAISE, capability, 0, 0));
        execv(argv[2], &argv[2]);
        return 5;
#endif
    } else {
        return 2;
    }
    return ferror(stdout) ? 1 : 0;
}
