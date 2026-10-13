// SPDX-License-Identifier: GPL-3.0-or-later
#define _GNU_SOURCE 1
#include "ndsudo-supervisor.h"

#ifdef __linux__
#include "supervisor-pipes.h"
#include <grp.h>
#include <poll.h>
#include <signal.h>
#include <sys/prctl.h>
#include <sys/signalfd.h>
#include <sys/wait.h>
#include <sys/xattr.h>

#define CONTROL_FD 3
#define STATUS_FD 4

static bool frame(const char *text) {
    ssize_t n;
    do { n = write(STATUS_FD, text, strlen(text)); } while (n < 0 && errno == EINTR);
    return n == (ssize_t)strlen(text);
}

static int refuse(const char *reason) {
    char text[80];
    snprintf(text, sizeof(text), "NDSUP1 refused %s\n", reason);
    (void)frame(text);
    return 126;
}

static bool trusted(const struct stat *st) {
    return st->st_uid == 0 && !(st->st_mode & (S_IWGRP | S_IWOTH | S_ISUID | S_ISGID));
}

// Reject symlinks rather than trusting their target or racing path resolution.
// Each directory and the ELF remain pinned by descriptors during traversal/exec.
static int open_payload(bool *missing) {
    const char *parts[] = { "usr", "bin", "tegrastats" };
    struct stat st;
    int dir = open("/", O_RDONLY | O_DIRECTORY | O_CLOEXEC);
    if (dir < 0)
        return -1;
    if (fstat(dir, &st) || !trusted(&st)) {
        close(dir);
        return -1;
    }
    for (size_t i = 0; i < sizeof(parts) / sizeof(parts[0]); i++) {
        bool last = i == 2;
        int fd = openat(dir, parts[i], O_RDONLY | O_CLOEXEC | O_NOFOLLOW | O_NONBLOCK |
                        (last ? 0 : O_DIRECTORY));
        int error = errno;
        close(dir);
        if (fd < 0) {
            *missing = error == ENOENT;
            return -1;
        }
        if (fstat(fd, &st) || !trusted(&st) || (last && (!S_ISREG(st.st_mode) || !(st.st_mode & S_IXUSR)))) {
            close(fd);
            return -1;
        }
        dir = fd;
    }
    unsigned char magic[4];
    // CLOEXEC descriptor execution deliberately excludes interpreter scripts.
    if (pread(dir, magic, sizeof(magic), 0) != sizeof(magic) || memcmp(magic, "\177ELF", 4)) {
        close(dir);
        return -1;
    }
    if (fgetxattr(dir, "security.capability", NULL, 0) >= 0 || (errno != ENODATA && errno != ENOTSUP)) {
        close(dir);
        return -1;
    }
    return dir;
}

static bool close_unrelated(void) {
    DIR *fds = opendir("/proc/self/fd");
    if (!fds)
        return false;
    struct dirent *entry;
    bool ok = true;
    errno = 0;
    while ((entry = readdir(fds))) {
        char *end;
        long fd = strtol(entry->d_name, &end, 10);
        if (*end || fd <= STATUS_FD || fd > INT_MAX || fd == dirfd(fds))
            continue;
        if (close((int)fd)) {
            ok = false;
            break;
        }
        errno = 0;
    }
    if (errno)
        ok = false;
    closedir(fds);
    return ok;
}

static void child_fail(int fd) {
    int error = errno;
    (void)write(fd, &error, sizeof(error));
    _exit(127);
}

int ndsudo_supervise_tegrastats(int argc) {
    if (!supervisor_protocol_pipes(CONTROL_FD, STATUS_FD)) {
        fprintf(stderr, "ndsudo: tegrastats requires private control/status pipes on fd 3/4\n");
        return 126;
    }
    // Status must never block cancellation if a caller stops reading it.
    if (fcntl(STATUS_FD, F_SETFL, O_NONBLOCK) < 0)
        return 126;
    struct sigaction action = { .sa_handler = SIG_IGN };
    sigemptyset(&action.sa_mask);
    if (sigaction(SIGPIPE, &action, NULL))
        return 126;
    if (argc != 2)
        return refuse("arguments");
    if (setgroups(0, NULL) || setresgid(0, 0, 0) || setresuid(0, 0, 0))
        return refuse("privilege");
    if (!close_unrelated() || chdir("/"))
        return refuse("setup");
    bool missing = false;
    int executable = open_payload(&missing);
    if (executable < 0)
        return refuse(missing ? "missing" : "unsafe");

    sigset_t watched;
    sigemptyset(&watched);
    const int signals[] = { SIGCHLD, SIGTERM, SIGINT, SIGHUP, SIGQUIT };
    action.sa_handler = SIG_DFL;
    for (size_t i = 0; i < sizeof(signals) / sizeof(signals[0]); i++) {
        if (sigaction(signals[i], &action, NULL)) {
            close(executable);
            return refuse("setup");
        }
        sigaddset(&watched, signals[i]);
    }
    if (sigprocmask(SIG_BLOCK, &watched, NULL) || prctl(PR_SET_CHILD_SUBREAPER, 1)) {
        close(executable);
        return refuse("setup");
    }
    int signal_fd = signalfd(-1, &watched, SFD_CLOEXEC | SFD_NONBLOCK);
    int exec_pipe[2];
    if (signal_fd < 0 || pipe2(exec_pipe, O_CLOEXEC | O_NONBLOCK)) {
        close(executable);
        if (signal_fd >= 0) close(signal_fd);
        return refuse("setup");
    }
    pid_t parent = getpid();
    pid_t child = fork();
    if (child < 0) {
        close(executable);
        close(signal_fd);
        close(exec_pipe[0]);
        close(exec_pipe[1]);
        return refuse("setup");
    }
    if (child == 0) {
        close(CONTROL_FD);
        close(STATUS_FD);
        close(signal_fd);
        close(exec_pipe[0]);
        sigset_t empty;
        sigemptyset(&empty);
        if (setpgid(0, 0) || prctl(PR_SET_PDEATHSIG, SIGKILL) || getppid() != parent ||
            sigaction(SIGPIPE, &action, NULL) || sigprocmask(SIG_SETMASK, &empty, NULL))
            child_fail(exec_pipe[1]);
        char *argv[] = { "/usr/bin/tegrastats", "--interval", "1000", NULL };
        char *env[] = { NULL };
        fexecve(executable, argv, env);
        child_fail(exec_pipe[1]);
    }
    close(executable);
    close(exec_pipe[1]);
    // Child also establishes the group before exec; an exec race is harmless.
    bool draining = setpgid(child, child) != 0 && errno != EACCES && errno != ESRCH;
    bool disarmed = false, startup_done = false, command_done = false;
    int command_status = 0;
    for (;;) {
        if (!disarmed) {
            siginfo_t info = {0};
            if (waitid(P_PID, child, &info, WEXITED | WNOHANG | WNOWAIT) == 0) {
                if (info.si_pid == child)
                    draining = true;
            } else if (errno != EINTR) {
                // Without the pinned leader identity no further group signal is safe.
                close(exec_pipe[0]);
                close(signal_fd);
                return 126;
            }
        }
        if (!startup_done) {
            int error;
            ssize_t n = read(exec_pipe[0], &error, sizeof(error));
            if (n == 0 || n > 0) {
                bool started = n == 0;
                startup_done = true;
                close(exec_pipe[0]);
                exec_pipe[0] = -1;
                if (!frame(started ? "NDSUP1 started\n" : "NDSUP1 failed\n") || !started)
                    draining = true;
            } else if (errno != EAGAIN && errno != EINTR) {
                draining = true;
            }
        }
        if (draining && !disarmed) {
            // The leader is not reaped until the last group signal is complete.
            // No later iteration may signal this numeric group identity.
            if (kill(-child, SIGKILL) != 0 && errno != ESRCH)
                return 126;
            disarmed = true;
        }
        if (disarmed) {
            bool exhausted = false;
            for (int i = 0; i < 256; i++) {
                int status;
                pid_t reaped = waitpid(-1, &status, WNOHANG | __WALL);
                if (reaped > 0) {
                    if (reaped == child) {
                        command_status = status;
                        command_done = true;
                    }
                    continue;
                }
                if (reaped < 0 && errno == EINTR) { i--; continue; }
                exhausted = reaped < 0 && errno == ECHILD;
                break;
            }
            // An escaped live descendant keeps ECHILD false. We neither hunt it
            // nor misreport a timeout/empty process group as completed cleanup.
            if (exhausted && startup_done && command_done) {
                char text[80];
                snprintf(text, sizeof(text), "NDSUP1 cleaned %d\n", command_status);
                bool written = frame(text);
                close(signal_fd);
                close(CONTROL_FD);
                close(STATUS_FD);
                if (!written) return 126;
                return WIFEXITED(command_status) ? WEXITSTATUS(command_status) : 128 + WTERMSIG(command_status);
            }
        }
        struct pollfd events[] = {
            { .fd = draining ? -1 : CONTROL_FD, .events = POLLIN },
            { .fd = draining ? -1 : STATUS_FD, .events = 0 },
            { .fd = signal_fd, .events = POLLIN },
            { .fd = exec_pipe[0], .events = POLLIN },
        };
        if (poll(events, 4, 1000) < 0 && errno != EINTR)
            draining = true;
        if (events[0].revents || events[1].revents)
            draining = true;
        if (events[2].revents & POLLIN) {
            struct signalfd_siginfo info;
            for (int i = 0; i < 64 && read(signal_fd, &info, sizeof(info)) == sizeof(info); i++) {
                if (info.ssi_signo != SIGCHLD)
                    draining = true;
            }
        }
    }
}
#endif
