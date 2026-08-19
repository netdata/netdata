// config.h must come first: it defines _GNU_SOURCE, which the system headers
// below need to declare setresuid()/setresgid().
#include "config.h"

#include <unistd.h>
#include <errno.h>
#include <sys/types.h>
#include <grp.h>
#include <pwd.h>
#include <stdio.h>
#include <stdlib.h>
#include <string.h>

#ifdef HAVE_CAPABILITY
#include <sys/capability.h>
#endif

#ifdef __APPLE__
#include <crt_externs.h>
#define environ (*_NSGetEnviron())
#else
extern char **environ;
#endif

#define FALLBACK_USER "nobody"

// USER, LOGNAME, HOME, SHELL, LC_ALL, PATH, PWD, TZ, TZDIR, TMPDIR, NULL
#define MAX_ENV_VARS 16

// The environment we hand to the child. It must be static: environ has to stay
// valid until execvp() replaces the process image.
static char *new_environ[MAX_ENV_VARS];
static size_t new_environ_entries = 0;

void show_help() {
    fprintf(stdout, "\n");
    fprintf(stdout, "nd-run\n");
    fprintf(stdout, "\n");
    fprintf(stdout, "Copyright 2025 Netdata Inc.\n");
    fprintf(stdout, "\n");
    fprintf(stdout, "A helper to run a command as an unprivileged user without any extra privileges\n");
    fprintf(stdout, "\n");
    fprintf(stdout, "Defaults to running the command as '%s', but will fall back to '%s' if '%s' is not found on the system.\n", NETDATA_USER, FALLBACK_USER, NETDATA_USER);
    fprintf(stdout, "\n");
    fprintf(stdout, "If it's not possible to switch users, the command will run as the current user instead.\n");
    #ifdef HAVE_CAPABILITY
        fprintf(stdout, "\n");
        fprintf(stdout, "Regardless of whether it switched users, all capabilities will be dropped.\n");
    #endif
}

static _Noreturn void fatal(const char *msg) {
    perror(msg);
    exit(EXIT_FAILURE);
}

static _Noreturn void fatal_msg(const char *msg) {
    fprintf(stderr, "nd-run: %s\n", msg);
    exit(EXIT_FAILURE);
}

#ifdef HAVE_CAPABILITY
static void clear_caps() {
    // Clear out all capabilities
    //
    // This does not require any special privileges since it is reducing
    // the process’s privileges.
    cap_t caps = cap_init();

    if (caps == NULL) fatal("cap_init");

    if (cap_clear(caps) == -1) {
        cap_free(caps);
        fatal("cap_clear");
    }

    if (cap_set_proc(caps) == -1) {
        cap_free(caps);
        fatal("cap_set_proc");
    }

    cap_free(caps);
}
#endif

static void add_env_var(const char *name, const char *value) {
    // Append "name=value" to the environment we are building for the child.
    // Variables that are not set are skipped.

    if (value == NULL) {
        return;
    }

    if (new_environ_entries + 1 >= MAX_ENV_VARS) {
        fatal_msg("too many environment variables");
    }

    size_t size = strlen(name) + 1 + strlen(value) + 1;
    char *entry = malloc(size);
    if (entry == NULL) {
        fatal("malloc");
    }

    snprintf(entry, size, "%s=%s", name, value);

    new_environ[new_environ_entries++] = entry;
    new_environ[new_environ_entries] = NULL;
}

static void build_environment(struct passwd *pw) {
    // Build a minimal environment for the child from scratch, only passing on
    // a few things we know are needed to make things work correctly.
    //
    // We never modify our own environment: getenv() keeps returning valid
    // pointers into the original environment block until main() replaces
    // environ, right before execvp(). Clearing the environment in place is not
    // portable - clearenv() does not exist everywhere, and setting environ to
    // NULL makes setenv() dereference a NULL environment array on macOS.

    const char *tmpdir = getenv("TMPDIR");

    add_env_var("USER", pw->pw_name);
    add_env_var("LOGNAME", pw->pw_name);
    add_env_var("HOME", pw->pw_dir);
    add_env_var("SHELL", "/bin/sh"); // Ignore user default shell
    add_env_var("LC_ALL", "C"); // Force C locale
    add_env_var("PATH", getenv("PATH"));
    add_env_var("PWD", getenv("PWD"));
    add_env_var("TZ", getenv("TZ"));
    add_env_var("TZDIR", getenv("TZDIR"));
    add_env_var("TMPDIR", (tmpdir == NULL) ? "/tmp" : tmpdir); // Use a sane default for TMPDIR if it wasn't set.
}

int main(int argc, char *argv[]) {
    if (argc < 2) {
        show_help();
        return EXIT_FAILURE;
    }

    uid_t euid = geteuid();

    struct passwd *pw = getpwnam(NETDATA_USER);
    if (!pw) {
        pw = getpwnam(FALLBACK_USER);
        if (!pw) {
            fprintf(stderr, "Fallback user '%s' not found either\n", FALLBACK_USER);
            return EXIT_FAILURE;
        }
    }

    if (euid != pw->pw_uid) {
        // Set supplementary groups for this user (must be done before dropping privs)
        if (initgroups(pw->pw_name, pw->pw_gid) != 0) {
            if (euid == 0) {
                if (setgroups(0, NULL) != 0) {
                    fatal("setgroups");
                }
            } else if (errno != EPERM) {
                fatal("initgroups");
            }
        }

        // Drop GID then UID. Prefer setres* when available to also drop saved IDs.
        // Linux/BSD generally provide setresgid/setresuid; macOS does not.
        #ifdef HAVE_SETRESGID
            if (setresgid(pw->pw_gid, pw->pw_gid, pw->pw_gid) != 0) {
                if (euid == 0 || errno != EPERM) {
                    fatal("setresgid");
                }
            }
        #else
            if (setgid(pw->pw_gid) != 0) {
                if (euid == 0 || errno != EPERM) {
                    fatal("setgid");
                }
            }
        #endif

        #ifdef HAVE_SETRESUID
            if (setresuid(pw->pw_uid, pw->pw_uid, pw->pw_uid) != 0) {
                if (euid == 0 || errno != EPERM) {
                    fatal("setresuid");
                }
            }
        #else
            if (setuid(pw->pw_uid) != 0) {
                if (euid == 0 || errno != EPERM) {
                    fatal("setuid");
                }
            }
        #endif
    }

    #ifdef HAVE_CAPABILITY
        clear_caps();
    #endif

    build_environment(pw);

    // Replace the environment wholesale. From here on we must not call
    // setenv()/putenv()/unsetenv(): libc may try to realloc() or free() an
    // environment block it did not allocate. Reading it is fine - execvp()
    // itself reads PATH from it.
    environ = new_environ;

    // Exec the requested command (replaces the current process on success)
    execvp(argv[1], &argv[1]);

    // Only reached on error. Use the exit codes every exec wrapper uses, so
    // that callers can tell an exec failure apart from the command exiting 1.
    int err = errno;
    perror("execvp");
    return (err == ENOENT) ? 127 : 126;
}
