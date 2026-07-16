// SPDX-License-Identifier: GPL-3.0-or-later

#include "spawn_popen.h"

struct popen_instance {
    SPAWN_INSTANCE *si;
    FILE *child_stdin_fp;
    FILE *child_stdout_fp;
};

SPAWN_SERVER *netdata_main_spawn_server = NULL;
static SPINLOCK netdata_main_spawn_server_spinlock = SPINLOCK_INITIALIZER;

bool netdata_main_spawn_server_init(const char *name, int argc, const char **argv) {
    if(netdata_main_spawn_server == NULL) {
        spinlock_lock(&netdata_main_spawn_server_spinlock);
        if(netdata_main_spawn_server == NULL)
            netdata_main_spawn_server = spawn_server_create(SPAWN_SERVER_OPTION_EXEC, name, NULL, argc, argv);
        spinlock_unlock(&netdata_main_spawn_server_spinlock);
    }

    return netdata_main_spawn_server != NULL;
}

void netdata_main_spawn_server_cleanup(void) {
    if(netdata_main_spawn_server) {
        spinlock_lock(&netdata_main_spawn_server_spinlock);
        if(netdata_main_spawn_server) {
            spawn_server_destroy(netdata_main_spawn_server);
            netdata_main_spawn_server = NULL;
        }
        spinlock_unlock(&netdata_main_spawn_server_spinlock);
    }
}

FILE *spawn_popen_stdin(POPEN_INSTANCE *pi) {
    if(!pi->child_stdin_fp)
        pi->child_stdin_fp = fdopen(spawn_server_instance_write_fd(pi->si), "w");

    if(!pi->child_stdin_fp)
        nd_log(NDLS_COLLECTORS, NDLP_ERR,
               "Cannot open FILE on child's stdin on fd %d.",
               spawn_server_instance_write_fd(pi->si));

    return pi->child_stdin_fp;
}

FILE *spawn_popen_stdout(POPEN_INSTANCE *pi) {
    if(!pi->child_stdout_fp)
        pi->child_stdout_fp = fdopen(spawn_server_instance_read_fd(pi->si), "r");

    if(!pi->child_stdout_fp)
        nd_log(NDLS_COLLECTORS, NDLP_ERR,
               "Cannot open FILE on child's stdout on fd %d.",
               spawn_server_instance_read_fd(pi->si));

    return pi->child_stdout_fp;
}

POPEN_INSTANCE *spawn_popen_run_argv(const char **argv) {
    netdata_main_spawn_server_init(NULL, 0, NULL);

    SPAWN_INSTANCE *si = spawn_server_exec(netdata_main_spawn_server, nd_log_collectors_fd(),
        0, argv, NULL, 0, SPAWN_INSTANCE_TYPE_EXEC);

    if(si == NULL) return NULL;

    POPEN_INSTANCE *pi = callocz(1, sizeof(*pi));
    pi->si = si;
    return pi;
}

POPEN_INSTANCE *spawn_popen_run_variadic(const char *cmd, ...) {
    va_list args;
    va_list args_copy;
    int argc = 0;

    // Start processing variadic arguments
    va_start(args, cmd);

    // Make a copy of args to count the number of arguments
    va_copy(args_copy, args);
    while (va_arg(args_copy, char *) != NULL) argc++;
    va_end(args_copy);

    // Allocate memory for argv array (+2 for cmd and NULL terminator)
    const char **argv = callocz(argc + 2, sizeof(*argv));

    // Populate the argv array
    argv[0] = cmd;

    for (int i = 1; i <= argc; i++)
        argv[i] = va_arg(args, const char *);

    argv[argc + 1] = NULL; // NULL-terminate the array

    // End processing variadic arguments
    va_end(args);

    POPEN_INSTANCE *pi = spawn_popen_run_argv(argv);
    freez(argv);

    return pi;
}

POPEN_INSTANCE *spawn_popen_run(const char *cmd) {
    if(!cmd || !*cmd) return NULL;

#if defined(OS_WINDOWS)
    // Windows has no /bin/sh. If the command starts with "exec ", strip it and
    // exec the plugin directly. Plugins on Windows carry a .plugin.exe suffix.
    // Script plugins (e.g. python.d.plugin) are handled by finding their interpreter.
    if(strncmp(cmd, "exec ", 5) == 0) {
        size_t len = strlen(cmd);
        char cmd_copy[len + 1];
        memcpy(cmd_copy, cmd, len + 1);
        char *words[100];
        size_t num_words = quoted_strings_splitter(cmd_copy, words, 100, isspace_map_pluginsd);
        char *exec = get_word(words, num_words, 0);
        char *prog = get_word(words, num_words, 1);
        if (strcmp(exec, "exec") == 0 && prog) {
            if (strendswith(prog, ".plugin.exe") &&
                !strendswith(prog, "charts.d.plugin.exe") &&
                !strendswith(prog, "ioping.plugin.exe")) {
                // Native .exe plugin — exec directly, no shell needed.
                const char *argv[num_words]; // remove exec, add NULL terminator

                size_t dst = 0;
                for (size_t i = 1; i < num_words; i++)
                    argv[dst++] = get_word(words, num_words, i);

                argv[dst] = NULL;
                return spawn_popen_run_argv(argv);
            }

            if (strendswith(prog, ".plugin") && !strendswith(prog, ".plugin.exe")) {
                // Script plugin (e.g. python.d.plugin) — needs a Python interpreter.
                // Parse args first: a -p<interpreter> option works even when Python is
                // absent from PATH, matching the Linux bash wrapper that strips -p before
                // exec'ing Python.  The PATH search (and its early-return guard) is only
                // needed when no explicit interpreter was provided.
                // Guarded by a spinlock: each plugin spawns in its own thread, so two
                // concurrent calls could race on the unsearched→searched transition and
                // one would read python_path before SearchPathA had written it.
                static char python_path[MAX_PATH + 1];
                static bool python_searched = false;
                static SPINLOCK python_lock = SPINLOCK_INITIALIZER;

                // words[0]="exec", words[1]=plugin_path; real args start at [2]
                const char *interp_override = NULL;
                size_t filt_count = 0;
                const char *filt_args[num_words]; // worst case: all args fit

                for (size_t i = 2; i < num_words; i++) {
                    const char *w = get_word(words, num_words, i);
                    if (w && w[0] == '-' && w[1] == 'p' && w[2] != '\0')
                        interp_override = w + 2;
                    else
                        filt_args[filt_count++] = w;
                }

                const char *interp;
                if (interp_override) {
                    interp = interp_override;
                } else {
                    spinlock_lock(&python_lock);
                    if (!python_searched) {
                        if (SearchPathA(NULL, "python3.exe", NULL, MAX_PATH, python_path, NULL) == 0 &&
                            SearchPathA(NULL, "python.exe",  NULL, MAX_PATH, python_path, NULL) == 0)
                            python_path[0] = '\0';
                        python_searched = true;
                    }
                    spinlock_unlock(&python_lock);
                    if (!python_path[0]) {
                        nd_log(NDLS_COLLECTORS, NDLP_WARNING,
                               "SPAWN: skipping '%s' — Python is not installed on this system", prog);
                        return NULL;
                    }
                    interp = python_path;
                }

                // argv: [interpreter, plugin_path, filtered_args..., NULL]
                const char *argv[2 + filt_count + 1];
                argv[0] = interp;
                argv[1] = prog;
                for (size_t i = 0; i < filt_count; i++)
                    argv[2 + i] = filt_args[i];
                argv[2 + filt_count] = NULL;
                return spawn_popen_run_argv(argv);
            }
        }
    }
#endif

    const char *argv[] = {
#if defined(OS_WINDOWS)
        // Windows has no /bin/sh; use sh.exe from PATH (MSYS2, Git Bash, etc.)
        "sh",
#else
        "/bin/sh",
#endif
        "-c",
        cmd,
        NULL
    };
    return spawn_popen_run_argv(argv);
}

static int spawn_popen_status_rc(int status) {
    if(WIFEXITED(status))
        return WEXITSTATUS(status);

    if(WIFSIGNALED(status)) {
        int sig = WTERMSIG(status);
        switch(sig) {
            case SIGTERM:
            case SIGPIPE:
                return 0;

            default:
                return -1;
        }
    }

    return -1;
}

static void spawn_popen_close_files(POPEN_INSTANCE *pi) {
    if(pi->child_stdin_fp) {
        fclose(pi->child_stdin_fp);
        pi->child_stdin_fp = NULL;
        spawn_server_instance_write_fd_unset(pi->si);
    }

    if(pi->child_stdout_fp) {
        fclose(pi->child_stdout_fp);
        pi->child_stdout_fp = NULL;
        spawn_server_instance_read_fd_unset(pi->si);
    }
}

int spawn_popen_wait(POPEN_INSTANCE *pi) {
    if(!pi) return -1;

    spawn_popen_close_files(pi);
    int status = spawn_server_exec_wait(netdata_main_spawn_server, pi->si);
    freez(pi);
    return spawn_popen_status_rc(status);
}

SPAWN_TIMEDWAIT_RESULT spawn_popen_timedwait(POPEN_INSTANCE *pi, int timeout_ms, int *code) {
    if(!pi) {
        if(code) *code = -1;
        return SPAWN_TIMEDWAIT_EXITED;
    }

    spawn_popen_close_files(pi);

    int status = 0;
    SPAWN_TIMEDWAIT_RESULT rc = spawn_server_exec_timedwait(netdata_main_spawn_server, pi->si, timeout_ms, &status);
    if(rc != SPAWN_TIMEDWAIT_EXITED)
        // RUNNING or ERROR: pi->si is still valid, so pi stays alive for the caller
        return rc;

    // EXITED: spawn_server_exec_timedwait() has freed pi->si; free the wrapper too
    freez(pi);
    if(code) *code = spawn_popen_status_rc(status);
    return SPAWN_TIMEDWAIT_EXITED;
}

int spawn_popen_kill(POPEN_INSTANCE *pi, int timeout_ms) {
    if(!pi) return -1;

    spawn_popen_close_files(pi);
    int status = spawn_server_exec_kill(netdata_main_spawn_server, pi->si, timeout_ms);
    freez(pi);
    return spawn_popen_status_rc(status);
}

pid_t spawn_popen_pid(POPEN_INSTANCE *pi) {
    if(!pi) return -1;
    return spawn_server_instance_pid(pi->si);
}

int spawn_popen_read_fd(POPEN_INSTANCE *pi) {
    if(!pi) return -1;
    return spawn_server_instance_read_fd(pi->si);
}

int spawn_popen_write_fd(POPEN_INSTANCE *pi) {
    if(!pi) return -1;
    return spawn_server_instance_write_fd(pi->si);
}
