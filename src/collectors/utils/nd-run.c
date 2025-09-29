#include <unistd.h>
#include <errno.h>
#include <sys/types.h>
#include <grp.h>
#include <pwd.h>
#include <stdio.h>
#include <stdlib.h>
#include <string.h>

#include "config.h"

#ifdef HAVE_CAPABILITY
#include <sys/capability.h>
#endif

#define FALLBACK_USER "nobody"

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

static void fatal(const char *msg) {
    perror(msg);
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

static void set_env_var(const char *name, const char *value) {
    // Set an environment variable if the specified value is not a NULL pointer.
    char buf[64];

    if (value == NULL) {
        return;
    }

    if (setenv(name, value, 1) != 0) {
        snprintf(buf, 64, "setenv %s", name);
        perror(buf);
    }
}

static void clean_environment(struct passwd *pw) {
    // Explicitly scrub the environment, only passing on a few things
    // we know are needed to make things work correctly.

    // First, save copies of the environment variables we want to keep.
    // We must copy them before clearing the environment, as getenv()
    // returns pointers into the environment block which will be invalidated.
    char *saved_path = NULL;
    char *saved_tz = NULL;
    char *saved_tzdir = NULL;
    char *saved_tmpdir = NULL;
    char *saved_pwd = NULL;

    const char *tmp;
    if ((tmp = getenv("PATH")) != NULL) {
        saved_path = strdup(tmp);
        if (!saved_path) fatal("strdup PATH");
    }
    if ((tmp = getenv("TZ")) != NULL) {
        saved_tz = strdup(tmp);
        if (!saved_tz) fatal("strdup TZ");
    }
    if ((tmp = getenv("TZDIR")) != NULL) {
        saved_tzdir = strdup(tmp);
        if (!saved_tzdir) fatal("strdup TZDIR");
    }
    if ((tmp = getenv("TMPDIR")) != NULL) {
        saved_tmpdir = strdup(tmp);
        if (!saved_tmpdir) fatal("strdup TMPDIR");
    }
    if ((tmp = getenv("PWD")) != NULL) {
        saved_pwd = strdup(tmp);
        if (!saved_pwd) fatal("strdup PWD");
    }

    // Now clear the environment
    #ifdef HAVE_CLEARENV
    clearenv();
    #else
    extern char **environ;
    environ = NULL;
    #endif

    // Set the new environment with our saved values
    set_env_var("USER", pw->pw_name);
    set_env_var("LOGNAME", pw->pw_name);
    set_env_var("HOME", pw->pw_dir);
    set_env_var("SHELL", "/bin/sh"); // Ignore user default shell
    set_env_var("LC_ALL", "C"); // Force C locale
    set_env_var("PATH", saved_path);
    set_env_var("PWD", saved_pwd);
    set_env_var("TZ", saved_tz);
    set_env_var("TZDIR", saved_tzdir);
    set_env_var("TMPDIR", (saved_tmpdir == NULL) ? "/tmp" : saved_tmpdir); // Use a sane default for TMPDIR if it wasn't set.

    // Free the saved copies
    free(saved_path);
    free(saved_tz);
    free(saved_tzdir);
    free(saved_tmpdir);
    free(saved_pwd);
}

int main(int argc, char *argv[]) {
    if (argc < 2) {
        show_help();
        return EXIT_FAILURE;
    }

    uid_t euid = geteuid();

    struct passwd *pw = getpwnam(NETDATA_USER);
    if (!pw) {
        fprintf(stderr, "User '%s' not found, falling back to '%s'\n",
                NETDATA_USER, FALLBACK_USER);
        pw = getpwnam(FALLBACK_USER);
        if (!pw) {
            fprintf(stderr, "Fallback user '%s' not found either\n", FALLBACK_USER);
            return EXIT_FAILURE;
        }
    }

    if (euid != pw->pw_uid) {
        fprintf(stderr, "Attempting to run as user: %s (UID=%d, GID=%d)\n", pw->pw_name, (int)pw->pw_uid, (int)pw->pw_gid);

        // Set supplementary groups for this user (must be done before dropping privs)
        if (initgroups(pw->pw_name, pw->pw_gid) != 0) {
            if (euid == 0) {
                perror("initgroups");

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

    clean_environment(pw);

    // Exec the requested command (replaces the current process on success)
    execvp(argv[1], &argv[1]);
    fatal("execvp"); // Only reached on error
}
