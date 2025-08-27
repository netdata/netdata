#include <unistd.h>
#include <sys/types.h>
#include <grp.h>
#include <pwd.h>
#include <stdio.h>
#include <stdlib.h>

#include "config.h"

#if defined(HAVE_PR_CAP_AMBIENT)
#if defined(NEED_LINUX_PRCTL_H)
#include <linux/prctl.h>
#endif
#include <sys/prctl.h>
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
    #if defined(HAVE_PR_CAP_AMBIENT)
        fprintf(stdout, "\n");
        fprintf(stdout, "Regardless of whether it switched users, all capabilities will be dropped.\n");
    #endif
}

int main(int argc, char *argv[]) {
    if (argc < 2) {
        show_help();
        return EXIT_FAILURE;
    }

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

    uid_t uid = pw->pw_uid;
    gid_t gid = pw->pw_gid;

    printf("Attempting to run as user: %s (UID=%d, GID=%d)\n", pw->pw_name, (int)uid, (int)gid);

    // Set supplementary groups for this user (must be done before dropping privs)
    if (initgroups(pw->pw_name, gid) != 0) {
        // Intentionally silently ignored
    }

    // Drop GID then UID. Prefer setres* when available to also drop saved IDs.
    // Linux/BSD generally provide setresgid/setresuid; macOS does not.
    #if defined(HAVE_SETRESGID)
        if (setresgid(gid, gid, gid) != 0) {
            // Intentionally silently ignored
        }
    #else
        if (setgid(gid) != 0) {
            // Intentionally silently ignored
        }
    #endif

    #if defined(HAVE_SETRESUID)
        if (setresuid(uid, uid, uid) != 0) {
            // Intentionally silently ignored
        }
    #else
        if (setuid(uid) != 0) {
            // Intentionally silently ignored
        }
    #endif

    #if defined(HAVE_PR_CAP_AMBIENT)
        // Attempt to clear ambient capabilities (this isn’t done automatically on execve())
        //
        // Only needed on Linux, as other platforms do not have the concept
        // of ambient capabilities (or even capabilities at all in many cases).
        //
        // Despite modifying capabilities, this is actually an unprivileged
        // operation, so it’s preferable to have it here in case we would be
        // able to switch UID due to capaibilities instead of being permitted
        // to do so as a result of being root in the current user namespace.
        prctl(PR_CAP_AMBIENT, PR_CAP_AMBIENT_CLEAR_ALL);
    #endif

    // Exec the requested command (replaces the current process on success)
    execvp(argv[1], &argv[1]);
    perror("execvp");           // only reached on error
    return EXIT_FAILURE;
}
