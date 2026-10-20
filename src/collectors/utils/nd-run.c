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
#include <stdbool.h>
#include <stdint.h>

#include "exec-signals.h"
#include "nd-file-reader.h"

#ifdef __linux__
#include <linux/capability.h>
#include <sys/syscall.h>
#endif

#ifdef __APPLE__
#include <crt_externs.h>
#define environ (*_NSGetEnviron())
#else
extern char **environ;
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
    fprintf(stdout, "Usage: nd-run command [args...]\n");
    fprintf(stdout, "       nd-run --preserve-env -- command [args...]\n");
    fprintf(stdout, "\n");
    fprintf(stdout, "The default is a minimal environment. --preserve-env retains inherited application variables\n");
    fprintf(stdout, "for trusted commands, but USER, LOGNAME, HOME, SHELL and LC_ALL remain helper-controlled.\n");
    fprintf(stdout, "TMPDIR is inherited when set, otherwise it defaults to /tmp. Privilege dropping is unchanged.\n");
    fprintf(stdout, "The -- delimiter is required after --preserve-env.\n");
    fprintf(stdout, "\n");
    fprintf(stdout, "Defaults to running the command as '%s', but will fall back to '%s' if '%s' is not found on the system.\n", NETDATA_USER, FALLBACK_USER, NETDATA_USER);
    fprintf(stdout, "\n");
    fprintf(stdout, "If it's not possible to switch users, the command will run as the current user instead.\n");
    #ifdef __linux__
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

static void clear_caps(void) {
#ifdef __linux__
    // File mode does not exec. Clear capabilities here for every mode, even
    // without libcap; clearing inheritable/permitted also clears ambient caps.
    struct __user_cap_header_struct header = { .version = _LINUX_CAPABILITY_VERSION_3, .pid = 0 };
    struct __user_cap_data_struct caps[2] = {{0}, {0}};
    if (syscall(SYS_capset, &header, caps) != 0 || syscall(SYS_capget, &header, caps) != 0)
        fatal("capability reduction");
    if (caps[0].effective || caps[0].permitted || caps[0].inheritable ||
        caps[1].effective || caps[1].permitted || caps[1].inheritable)
        fatal_msg("capabilities remain after privilege reduction");
#endif
}

static int set_all_gids(gid_t gid) {
#ifdef HAVE_SETRESGID
    return setresgid(gid, gid, gid);
#else
    return setregid(gid, gid);
#endif
}

static int set_all_uids(uid_t uid) {
#ifdef HAVE_SETRESUID
    return setresuid(uid, uid, uid);
#else
    return setreuid(uid, uid);
#endif
}

static void drop_privileges(const struct passwd *pw) {
    uid_t euid = geteuid();
    bool switch_user = euid != pw->pw_uid;
    uid_t uid = switch_user ? pw->pw_uid : euid;
    gid_t gid = switch_user ? pw->pw_gid : getegid();

    // Same-user execution retains OS-granted supplementary groups.
    if (switch_user && initgroups(pw->pw_name, pw->pw_gid) != 0) {
        if (euid == 0) {
            if (setgroups(0, NULL) != 0)
                fatal("setgroups");
        } else if (errno != EPERM) {
            fatal("initgroups");
        }
    }

    // Normalize real/effective/saved IDs even for same-user execution and
    // permitted nonroot fallback. Successful target changes need no second pass.
    if (set_all_gids(gid) != 0) {
        if (euid == 0 || errno != EPERM)
            fatal("set group IDs");
        gid = getegid();
        if (set_all_gids(gid) != 0)
            fatal("retain group IDs");
    }
    if (set_all_uids(uid) != 0) {
        if (euid == 0 || errno != EPERM)
            fatal("set user IDs");
        uid = geteuid();
        if (set_all_uids(uid) != 0)
            fatal("retain user IDs");
    }
    if (getuid() != uid || geteuid() != uid || getgid() != gid || getegid() != gid)
        fatal_msg("identity mismatch after privilege reduction");
    clear_caps();
}

static void add_env_var(char **env, size_t *entries, const char *name, const char *value) {
    // Append "name=value" to the environment we are building for the child.
    // Variables that are not set are skipped.

    if (value == NULL) {
        return;
    }

    size_t name_len = strlen(name);
    size_t value_len = strlen(value);
    if (name_len > SIZE_MAX - 2 || value_len > SIZE_MAX - name_len - 2) {
        fatal_msg("environment variable is too large");
    }

    size_t size = name_len + value_len + 2;
    char *entry = malloc(size);
    if (entry == NULL) {
        fatal("malloc");
    }

    snprintf(entry, size, "%s=%s", name, value);

    env[(*entries)++] = entry;
}

static char **build_environment(struct passwd *pw, bool preserve_env) {
    // We never modify our own environment: getenv() keeps returning valid
    // pointers into the original environment block until main() replaces
    // environ, right before execvp(). Clearing the environment in place is not
    // portable - clearenv() does not exist everywhere, and setting environ to
    // NULL makes setenv() dereference a NULL environment array on macOS.

    const struct {
        const char *name;
        const char *value;
    } controlled[] = {
        { "USER", pw->pw_name },
        { "LOGNAME", pw->pw_name },
        { "HOME", pw->pw_dir },
        { "SHELL", "/bin/sh" },
        { "LC_ALL", "C" },
    };
    const size_t controlled_count = sizeof(controlled) / sizeof(controlled[0]);

    // Space for the controlled fields, PATH/PWD/TZ/TZDIR, TMPDIR and NULL,
    // plus every inherited entry in preservation mode. calloc leaves the terminator.
    size_t capacity = controlled_count + 6;
    if (preserve_env) {
        for (char **entry = environ; *entry; entry++) {
            if (capacity >= SIZE_MAX / sizeof(char *))
                fatal_msg("environment is too large");
            capacity++;
        }
    }
    char **env = calloc(capacity, sizeof(*env));
    if (!env)
        fatal("calloc");

    size_t entries = 0;
    for (size_t i = 0; i < controlled_count; i++)
        add_env_var(env, &entries, controlled[i].name, controlled[i].value);

    if (preserve_env) {
        for (char **entry = environ; *entry; entry++) {
            bool reserved = false;
            for (size_t i = 0; i < controlled_count; i++) {
                size_t len = strlen(controlled[i].name);
                if (strncmp(*entry, controlled[i].name, len) == 0 && (*entry)[len] == '=') {
                    reserved = true;
                    break;
                }
            }
            if (!reserved) {
                env[entries] = strdup(*entry);
                if (!env[entries])
                    fatal("strdup");
                entries++;
            }
        }
    } else {
        add_env_var(env, &entries, "PATH", getenv("PATH"));
        add_env_var(env, &entries, "PWD", getenv("PWD"));
        add_env_var(env, &entries, "TZ", getenv("TZ"));
        add_env_var(env, &entries, "TZDIR", getenv("TZDIR"));
    }

    const char *tmpdir = getenv("TMPDIR");
    if (!preserve_env || tmpdir == NULL)
        add_env_var(env, &entries, "TMPDIR", tmpdir == NULL ? "/tmp" : tmpdir);

    return env;
}

int main(int argc, char *argv[]) {
    if (argc < 2) {
        show_help();
        return EXIT_FAILURE;
    }

    bool file_reader = strcmp(argv[1], "--file-reader") == 0;
    struct nd_file_request file_request;
    if (file_reader && !nd_file_reader_parse(argc - 2, argv + 2, &file_request))
        fatal_msg("usage: --file-reader read regular|stream <limit> <path>, or --file-reader stat <path>");

    bool preserve_env = strcmp(argv[1], "--preserve-env") == 0;
    int command = 1;
    if (preserve_env) {
        if (argc < 4 || strcmp(argv[2], "--") != 0)
            fatal_msg("usage: nd-run --preserve-env -- command [args...]");
        command = 3;
    }

    struct passwd *pw = getpwnam(NETDATA_USER);
    if (!pw) {
        pw = getpwnam(FALLBACK_USER);
        if (!pw) {
            fprintf(stderr, "Fallback user '%s' not found either\n", FALLBACK_USER);
            return EXIT_FAILURE;
        }
    }

    drop_privileges(pw);
    if (file_reader && geteuid() == 0)
        fatal_msg("file reader requires an unprivileged identity");

    // SIG_IGN survives exec; the file reader also needs default SIGPIPE handling.
    reset_signal_dispositions();
    if (file_reader)
        return nd_file_reader_run(&file_request);

    char **new_environ = build_environment(pw, preserve_env);

    // Replace the environment wholesale. From here on we must not call
    // setenv()/putenv()/unsetenv(): libc may try to realloc() or free() an
    // environment block it did not allocate. Reading it is fine - execvp()
    // itself reads PATH from it.
    environ = new_environ;

    // Exec the requested command (replaces the current process on success)
    execvp(argv[command], &argv[command]);

    // Only reached on error. Use the exit codes every exec wrapper uses, so
    // that callers can tell an exec failure apart from the command exiting 1.
    int err = errno;
    perror("execvp");
    return (err == ENOENT) ? 127 : 126;
}
