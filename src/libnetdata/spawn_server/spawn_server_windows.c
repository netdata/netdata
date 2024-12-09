// SPDX-License-Identifier: GPL-3.0-or-later

#include "spawn_server_internals.h"

#if defined(SPAWN_SERVER_VERSION_WINDOWS)

int spawn_server_instance_read_fd(SPAWN_INSTANCE *si) { return si->read_fd; }
int spawn_server_instance_write_fd(SPAWN_INSTANCE *si) { return si->write_fd; }
void spawn_server_instance_read_fd_unset(SPAWN_INSTANCE *si) { si->read_fd = -1; }
void spawn_server_instance_write_fd_unset(SPAWN_INSTANCE *si) { si->write_fd = -1; }

pid_t spawn_server_instance_pid(SPAWN_INSTANCE *si) {
    if(si->child_pid != -1)
        return si->child_pid;

    return (pid_t)si->dwProcessId;
}

static void update_cygpath_env(void) {
    static volatile bool done = false;

    if(done) return;
    done = true;

    char win_path[MAX_PATH];

    // Convert Cygwin root path to Windows path
    cygwin_conv_path(CCP_POSIX_TO_WIN_A, "/", win_path, sizeof(win_path));

    nd_setenv("NETDATA_CYGWIN_BASE_PATH", win_path, 1);

    nd_log(NDLS_COLLECTORS, NDLP_INFO, "Cygwin/MSYS2 base path set to '%s'", win_path);
}

SPAWN_SERVER* spawn_server_create(SPAWN_SERVER_OPTIONS options __maybe_unused, const char *name, spawn_request_callback_t cb  __maybe_unused, int argc __maybe_unused, const char **argv __maybe_unused) {
    update_cygpath_env();

    SPAWN_SERVER* server = callocz(1, sizeof(SPAWN_SERVER));
    if(name)
        server->name = strdupz(name);
    else
        server->name = strdupz("unnamed");

    server->log_forwarder = log_forwarder_start();

    return server;
}

void spawn_server_destroy(SPAWN_SERVER *server) {
    if (server) {
        if (server->log_forwarder) {
            log_forwarder_stop(server->log_forwarder);
            server->log_forwarder = NULL;
        }
        freez((void *)server->name);
        freez(server);
    }
}

static BUFFER *argv_to_windows(const char **argv) {
    BUFFER *wb = buffer_create(0, NULL);

    // argv[0] is the path
    char b[strlen(argv[0]) * 2 + FILENAME_MAX];
    cygwin_conv_path(CCP_POSIX_TO_WIN_A | CCP_ABSOLUTE, argv[0], b, sizeof(b));

    for(size_t i = 0; argv[i] ;i++) {
        const char *s = (i == 0) ? b : argv[i];
        size_t len = strlen(s);
        buffer_need_bytes(wb, len * 2 + 1);

        bool needs_quotes = false;
        for(const char *c = s; !needs_quotes && *c ; c++) {
            switch(*c) {
                case ' ':
                case '\v':
                case '\t':
                case '\n':
                case '"':
                    needs_quotes = true;
                    break;

                default:
                    break;
            }
        }

        if(buffer_strlen(wb)) {
            if (needs_quotes)
                buffer_strcat(wb, " \"");
            else
                buffer_putc(wb, ' ');
        }
        else if (needs_quotes)
            buffer_putc(wb, '"');

        for(const char *c = s; *c ; c++) {
            switch(*c) {
                case '"':
                    buffer_putc(wb, '\\');
                    // fall through

                default:
                    buffer_putc(wb, *c);
                    break;
            }
        }

        if(needs_quotes)
            buffer_strcat(wb, "\"");
    }

    return wb;
}

int set_fd_blocking(int fd) {
    int flags = fcntl(fd, F_GETFL, 0);
    if (flags == -1) {
        nd_log(NDLS_COLLECTORS, NDLP_ERR, "SPAWN PARENT: fcntl(F_GETFL) failed");
        return -1;
    }

    flags &= ~O_NONBLOCK;
    if (fcntl(fd, F_SETFL, flags) == -1) {
        nd_log(NDLS_COLLECTORS, NDLP_ERR, "SPAWN PARENT: fcntl(F_SETFL) failed");
        return -1;
    }

    return 0;
}

//static void print_environment_block(char *env_block) {
//    if (env_block == NULL) {
//        fprintf(stderr, "Environment block is NULL\n");
//        return;
//    }
//
//    char *env = env_block;
//    while (*env) {
//        fprintf(stderr, "ENVIRONMENT: %s\n", env);
//        // Move to the next string in the block
//        env += strlen(env) + 1;
//    }
//}

SPAWN_INSTANCE* spawn_server_exec(SPAWN_SERVER *server, int stderr_fd __maybe_unused, int custom_fd __maybe_unused, const char **argv, const void *data __maybe_unused, size_t data_size __maybe_unused, SPAWN_INSTANCE_TYPE type) {
    static SPINLOCK spinlock = SPINLOCK_INITIALIZER;

    if (type != SPAWN_INSTANCE_TYPE_EXEC)
        return NULL;

    int pipe_stdin[2] = { -1, -1 }, pipe_stdout[2] = { -1, -1 }, pipe_stderr[2] = { -1, -1 };

    errno_clear();

    SPAWN_INSTANCE *instance = callocz(1, sizeof(*instance));
    instance->request_id = __atomic_add_fetch(&server->request_id, 1, __ATOMIC_RELAXED);

    CLEAN_BUFFER *wb = argv_to_windows(argv);
    char *command = (char *)buffer_tostring(wb);

    if (pipe(pipe_stdin) == -1) {
        nd_log(NDLS_COLLECTORS, NDLP_ERR,
               "SPAWN PARENT: Cannot create stdin pipe() for request No %zu, command: %s",
               instance->request_id, command);
        goto cleanup;
    }

    if (pipe(pipe_stdout) == -1) {
        nd_log(NDLS_COLLECTORS, NDLP_ERR,
               "SPAWN PARENT: Cannot create stdout pipe() for request No %zu, command: %s",
               instance->request_id, command);
        goto cleanup;
    }

    if (pipe(pipe_stderr) == -1) {
        nd_log(NDLS_COLLECTORS, NDLP_ERR,
               "SPAWN PARENT: Cannot create stderr pipe() for request No %zu, command: %s",
               instance->request_id, command);
        goto cleanup;
    }

    // Ensure pipes are in blocking mode
    if (set_fd_blocking(pipe_stdin[PIPE_READ]) == -1 || set_fd_blocking(pipe_stdin[PIPE_WRITE]) == -1 ||
        set_fd_blocking(pipe_stdout[PIPE_READ]) == -1 || set_fd_blocking(pipe_stdout[PIPE_WRITE]) == -1 ||
        set_fd_blocking(pipe_stderr[PIPE_READ]) == -1 || set_fd_blocking(pipe_stderr[PIPE_WRITE]) == -1) {
        nd_log(NDLS_COLLECTORS, NDLP_ERR,
               "SPAWN PARENT: Failed to set blocking I/O on pipes for request No %zu, command: %s",
               instance->request_id, command);
        goto cleanup;
    }

    // do not run multiple times this section
    // to prevent handles leaking
    spinlock_lock(&spinlock);

    // Convert POSIX file descriptors to Windows handles
    HANDLE stdin_read_handle = (HANDLE)_get_osfhandle(pipe_stdin[PIPE_READ]);
    HANDLE stdout_write_handle = (HANDLE)_get_osfhandle(pipe_stdout[PIPE_WRITE]);
    HANDLE stderr_write_handle = (HANDLE)_get_osfhandle(pipe_stderr[PIPE_WRITE]);

    if (stdin_read_handle == INVALID_HANDLE_VALUE || stdout_write_handle == INVALID_HANDLE_VALUE || stderr_write_handle == INVALID_HANDLE_VALUE) {
        spinlock_unlock(&spinlock);
        nd_log(NDLS_COLLECTORS, NDLP_ERR,
               "SPAWN PARENT: Invalid handle value(s) for request No %zu, command: %s",
               instance->request_id, command);
        goto cleanup;
    }

    // Set handle inheritance
    if (!SetHandleInformation(stdin_read_handle, HANDLE_FLAG_INHERIT, HANDLE_FLAG_INHERIT) ||
        !SetHandleInformation(stdout_write_handle, HANDLE_FLAG_INHERIT, HANDLE_FLAG_INHERIT) ||
        !SetHandleInformation(stderr_write_handle, HANDLE_FLAG_INHERIT, HANDLE_FLAG_INHERIT)) {
        spinlock_unlock(&spinlock);
        nd_log(NDLS_COLLECTORS, NDLP_ERR,
               "SPAWN PARENT: Cannot set handle(s) inheritance for request No %zu, command: %s",
               instance->request_id, command);
        goto cleanup;
    }

    // Set up the STARTUPINFO structure
    STARTUPINFO si;
    PROCESS_INFORMATION pi;
    ZeroMemory(&si, sizeof(si));
    si.cb = sizeof(si);
    si.dwFlags = STARTF_USESTDHANDLES;
    si.hStdInput = stdin_read_handle;
    si.hStdOutput = stdout_write_handle;
    si.hStdError = stderr_write_handle;

    // Retrieve the current environment block
    char* env_block = GetEnvironmentStrings();
//    print_environment_block(env_block);

    nd_log(NDLS_COLLECTORS, NDLP_INFO,
           "SPAWN PARENT: Running request No %zu, command: '%s'",
           instance->request_id, command);

    int fds_to_keep_open[] = { pipe_stdin[PIPE_READ], pipe_stdout[PIPE_WRITE], pipe_stderr[PIPE_WRITE] };
    os_close_all_non_std_open_fds_except(fds_to_keep_open, 3, CLOSE_RANGE_CLOEXEC);

    // Spawn the process
    errno_clear();
    if (!CreateProcess(NULL, command, NULL, NULL, TRUE, 0, env_block, NULL, &si, &pi)) {
        spinlock_unlock(&spinlock);
        nd_log(NDLS_COLLECTORS, NDLP_ERR,
               "SPAWN PARENT: cannot CreateProcess() for request No %zu, command: %s",
               instance->request_id, command);
        goto cleanup;
    }

    FreeEnvironmentStrings(env_block);

    // When we create a process with the CreateProcess function, it returns two handles:
    // - one for the process (pi.hProcess) and
    // - one for the primary thread of the new process (pi.hThread).
    // Both of these handles need to be explicitly closed when they are no longer needed.
    CloseHandle(pi.hThread);

    // end of the critical section
    spinlock_unlock(&spinlock);

    // Close unused pipe ends
    close(pipe_stdin[PIPE_READ]); pipe_stdin[PIPE_READ] = -1;
    close(pipe_stdout[PIPE_WRITE]); pipe_stdout[PIPE_WRITE] = -1;
    close(pipe_stderr[PIPE_WRITE]); pipe_stderr[PIPE_WRITE] = -1;

    // Store process information in instance
    instance->dwProcessId = pi.dwProcessId;
    instance->child_pid = cygwin_winpid_to_pid((pid_t)pi.dwProcessId);
    instance->process_handle = pi.hProcess;

    // Convert handles to POSIX file descriptors
    instance->write_fd = pipe_stdin[PIPE_WRITE];
    instance->read_fd = pipe_stdout[PIPE_READ];
    instance->stderr_fd = pipe_stderr[PIPE_READ];

    // Add stderr_fd to the log forwarder
    log_forwarder_add_fd(server->log_forwarder, instance->stderr_fd);
    log_forwarder_annotate_fd_name(server->log_forwarder, instance->stderr_fd, command);
    log_forwarder_annotate_fd_pid(server->log_forwarder, instance->stderr_fd, spawn_server_instance_pid(instance));

    errno_clear();
    nd_log(NDLS_COLLECTORS, NDLP_INFO,
           "SPAWN PARENT: created process for request No %zu, pid %d (winpid %d), command: %s",
           instance->request_id, (int)instance->child_pid, (int)pi.dwProcessId, command);

    return instance;

    cleanup:
    if (pipe_stdin[PIPE_READ] >= 0) close(pipe_stdin[PIPE_READ]);
    if (pipe_stdin[PIPE_WRITE] >= 0) close(pipe_stdin[PIPE_WRITE]);
    if (pipe_stdout[PIPE_READ] >= 0) close(pipe_stdout[PIPE_READ]);
    if (pipe_stdout[PIPE_WRITE] >= 0) close(pipe_stdout[PIPE_WRITE]);
    if (pipe_stderr[PIPE_READ] >= 0) close(pipe_stderr[PIPE_READ]);
    if (pipe_stderr[PIPE_WRITE] >= 0) close(pipe_stderr[PIPE_WRITE]);
    freez(instance);
    return NULL;
}

static char* GetErrorString(DWORD errorCode) {
    DWORD lastError = GetLastError();

    LPVOID lpMsgBuf;
    DWORD bufLen = FormatMessage(
            FORMAT_MESSAGE_ALLOCATE_BUFFER |
            FORMAT_MESSAGE_FROM_SYSTEM |
            FORMAT_MESSAGE_IGNORE_INSERTS,
            NULL,
            errorCode,
            MAKELANGID(LANG_NEUTRAL, SUBLANG_DEFAULT),
            (LPTSTR) &lpMsgBuf,
            0, NULL );

    SetLastError(lastError);

    if (bufLen) {
        char* errorString = (char*)LocalAlloc(LMEM_FIXED, bufLen + 1);
        if (errorString) {
            strcpy(errorString, (char*)lpMsgBuf);
        }
        LocalFree(lpMsgBuf);
        return errorString;
    }

    return NULL;
}

static void TerminateChildProcesses(SPAWN_INSTANCE *si) {
    HANDLE hSnapshot = CreateToolhelp32Snapshot(TH32CS_SNAPPROCESS, 0);
    if (hSnapshot == INVALID_HANDLE_VALUE)
        return;

    PROCESSENTRY32 pe;
    pe.dwSize = sizeof(PROCESSENTRY32);

    if (Process32First(hSnapshot, &pe)) {
        do {
            if (pe.th32ParentProcessID == si->dwProcessId) {
                HANDLE hChildProcess = OpenProcess(PROCESS_TERMINATE, FALSE, pe.th32ProcessID);
                if (hChildProcess) {
                    nd_log(NDLS_COLLECTORS, NDLP_WARNING,
                           "SPAWN PARENT: killing subprocess %u of request No %zu, pid %d (winpid %u)",
                           pe.th32ProcessID, si->request_id, (int)si->child_pid, si->dwProcessId);

                    TerminateProcess(hChildProcess, STATUS_CONTROL_C_EXIT);
                    CloseHandle(hChildProcess);
                }
            }
        } while (Process32Next(hSnapshot, &pe));
    }

    CloseHandle(hSnapshot);
}

int map_status_code_to_signal(DWORD status_code) {
    switch (status_code) {
        case STATUS_ACCESS_VIOLATION:
            return SIGSEGV;
        case STATUS_ILLEGAL_INSTRUCTION:
            return SIGILL;
        case STATUS_FLOAT_DIVIDE_BY_ZERO:
        case STATUS_INTEGER_DIVIDE_BY_ZERO:
        case STATUS_ARRAY_BOUNDS_EXCEEDED:
        case STATUS_FLOAT_OVERFLOW:
        case STATUS_FLOAT_UNDERFLOW:
        case STATUS_FLOAT_INVALID_OPERATION:
            return SIGFPE;
        case STATUS_BREAKPOINT:
        case STATUS_SINGLE_STEP:
            return SIGTRAP;
        case STATUS_STACK_OVERFLOW:
        case STATUS_INVALID_HANDLE:
        case STATUS_INVALID_PARAMETER:
        case STATUS_NO_MEMORY:
        case STATUS_PRIVILEGED_INSTRUCTION:
        case STATUS_DLL_NOT_FOUND:
        case STATUS_DLL_INIT_FAILED:
        case STATUS_ORDINAL_NOT_FOUND:
        case STATUS_ENTRYPOINT_NOT_FOUND:
        case STATUS_CONTROL_STACK_VIOLATION:
        case STATUS_STACK_BUFFER_OVERRUN:
        case STATUS_ASSERTION_FAILURE:
        case STATUS_INVALID_CRUNTIME_PARAMETER:
        case STATUS_HEAP_CORRUPTION:
            return SIGABRT;
        case STATUS_CONTROL_C_EXIT:
            return SIGTERM; // we use this internally as such
        case STATUS_FATAL_APP_EXIT:
            return SIGTERM;
        default:
            return (status_code & 0xFF) << 8;
    }
}

int spawn_server_exec_kill(SPAWN_SERVER *server __maybe_unused, SPAWN_INSTANCE *si, int timeout_ms __maybe_unused) {
    // this gives some warnings at the spawn-tester, but it is generally better
    // to have them, to avoid abnormal shutdown of the plugins
    if(si->read_fd != -1) { close(si->read_fd); si->read_fd = -1; }
    if(si->write_fd != -1) { close(si->write_fd); si->write_fd = -1; }

    if(timeout_ms > 0)
        WaitForSingleObject(si->process_handle, timeout_ms);

    errno_clear();
    if(si->child_pid != -1 && kill(si->child_pid, SIGTERM) != 0)
        nd_log(NDLS_COLLECTORS, NDLP_ERR,
               "SPAWN PARENT: child of request No %zu, pid %d (winpid %u), failed to be killed",
               si->request_id, (int)si->child_pid, si->dwProcessId);

    errno_clear();
    if(TerminateProcess(si->process_handle, STATUS_CONTROL_C_EXIT) == 0)
        nd_log(NDLS_COLLECTORS, NDLP_ERR,
               "SPAWN PARENT: child of request No %zu, pid %d (winpid %u), failed to be terminated",
               si->request_id, (int)si->child_pid, si->dwProcessId);

    errno_clear();
    TerminateChildProcesses(si);

    if(si->stderr_fd != -1) {
        if(!log_forwarder_del_and_close_fd(server->log_forwarder, si->stderr_fd))
            close(si->stderr_fd);

        si->stderr_fd = -1;
    }

    return spawn_server_exec_wait(server, si);
}

int spawn_server_exec_wait(SPAWN_SERVER *server __maybe_unused, SPAWN_INSTANCE *si) {
    if(si->read_fd != -1) { close(si->read_fd); si->read_fd = -1; }
    if(si->write_fd != -1) { close(si->write_fd); si->write_fd = -1; }

    // wait for the process to end
    WaitForSingleObject(si->process_handle, INFINITE);

    DWORD exit_code = -1;
    GetExitCodeProcess(si->process_handle, &exit_code);
    CloseHandle(si->process_handle);

    char *err = GetErrorString(exit_code);

    nd_log(NDLS_COLLECTORS, NDLP_INFO,
           "SPAWN PARENT: child of request No %zu, pid %d (winpid %u), exited with code %u (0x%x): %s",
           si->request_id, (int)si->child_pid, si->dwProcessId,
           (unsigned)exit_code, (unsigned)exit_code, err ? err : "(no reason text)");

    if(err)
        LocalFree(err);

    if(si->stderr_fd != -1) {
        if(!log_forwarder_del_and_close_fd(server->log_forwarder, si->stderr_fd))
            close(si->stderr_fd);

        si->stderr_fd = -1;
    }

    freez(si);
    return map_status_code_to_signal(exit_code);
}

#endif
