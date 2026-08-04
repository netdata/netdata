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

bool spawn_server_windows_publish_cygwin_path(void) {
    char win_path[MAX_PATH];

    // Convert Cygwin root path to Windows path
    errno_clear();
    if(cygwin_conv_path(CCP_POSIX_TO_WIN_A, "/", win_path, sizeof(win_path)) != 0) {
        nd_log(NDLS_COLLECTORS, NDLP_ERR, "Cannot convert Cygwin/MSYS2 base path to Windows path: %s", strerror(errno));
        return false;
    }

    if(nd_environment_set("NETDATA_CYGWIN_BASE_PATH", win_path, true) != 0) {
        nd_log(NDLS_COLLECTORS, NDLP_ERR, "Cannot publish the Cygwin/MSYS2 base path");
        return false;
    }

    nd_log(NDLS_COLLECTORS, NDLP_INFO, "Cygwin/MSYS2 base path set to '%s'", win_path);
    return true;
}

SPAWN_SERVER *spawn_server_create(SPAWN_SERVER_OPTIONS options __maybe_unused, const char *name,
                                  spawn_request_callback_t cb __maybe_unused, int argc __maybe_unused,
                                  const char **argv __maybe_unused, ND_ENVIRONMENT *environment,
                                  const char *runtime_directory __maybe_unused) {
    if(!nd_environment_is_process_frozen()) {
        nd_log(NDLS_COLLECTORS, NDLP_ERR,
               "SPAWN SERVER: refusing to start the Windows server before the environment freeze");
        return NULL;
    }

    if(!environment) {
        nd_log(NDLS_COLLECTORS, NDLP_ERR, "SPAWN SERVER: environment context is required");
        return NULL;
    }

    SPAWN_SERVER* server = callocz(1, sizeof(SPAWN_SERVER));
    server->environment = environment;
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
    // argv[0] is the path
    ssize_t converted_size = cygwin_conv_path(CCP_POSIX_TO_WIN_A | CCP_ABSOLUTE, argv[0], NULL, 0);
    if(converted_size <= 0)
        return NULL;

    size_t b_size = (size_t)converted_size;
    CLEAN_CHAR_P *b = mallocz(b_size);
    if(cygwin_conv_path(CCP_POSIX_TO_WIN_A | CCP_ABSOLUTE, argv[0], b, b_size) != 0)
        return NULL;

    BUFFER *wb = buffer_create(0, NULL);

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

static void spawn_server_release_stderr_fd(SPAWN_SERVER *server, SPAWN_INSTANCE *si) {
    if(si->stderr_fd == -1)
        return;

    if(si->stderr_log_token != LOG_FORWARDER_TOKEN_NONE) {
        // a valid token means the forwarder adopted the fd and closes it on
        // every path (worker delete, or thread-exit cleanup); a false return
        // means it is already closed or about to be - raw-closing the number
        // here could close an unrelated, recycled descriptor
        log_forwarder_del_and_close_token(server->log_forwarder, si->stderr_log_token);
        si->stderr_log_token = LOG_FORWARDER_TOKEN_NONE;
    }
    else
        close(si->stderr_fd);

    si->stderr_fd = -1;
}

SPAWN_INSTANCE* spawn_server_exec(SPAWN_SERVER *server, int stderr_fd __maybe_unused, int custom_fd __maybe_unused, const char **argv, const void *data __maybe_unused, size_t data_size __maybe_unused, SPAWN_INSTANCE_TYPE type) {
    static SPINLOCK spinlock = SPINLOCK_INITIALIZER;
    ND_ENV_SNAPSHOT *environment_snapshot = NULL;

    if (type != SPAWN_INSTANCE_TYPE_EXEC)
        return NULL;

    if(!argv || !argv[0] || !*argv[0])
        return NULL;

    int pipe_stdin[2] = { -1, -1 }, pipe_stdout[2] = { -1, -1 }, pipe_stderr[2] = { -1, -1 };

    errno_clear();

    SPAWN_INSTANCE *instance = callocz(1, sizeof(*instance));
    instance->request_id = __atomic_add_fetch(&server->request_id, 1, __ATOMIC_RELAXED);

    CLEAN_BUFFER *wb = argv_to_windows(argv);
    if(!wb) {
        nd_log(NDLS_COLLECTORS, NDLP_ERR,
               "SPAWN PARENT: Cannot convert command path for request No %zu, command: %s",
               instance->request_id, argv[0]);
        goto cleanup;
    }

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

    environment_snapshot = nd_environment_snapshot_acquire(server->environment);
    if(!environment_snapshot) {
        nd_log(NDLS_COLLECTORS, NDLP_ERR,
               "SPAWN PARENT: Cannot acquire environment for request No %zu, command: %s",
               instance->request_id, command);
        goto cleanup;
    }

    size_t environment_block_size = 0;
    const char *environment_block =
        nd_environment_snapshot_windows_block(environment_snapshot, &environment_block_size);
    if(!environment_block || environment_block_size < 2) {
        nd_log(NDLS_COLLECTORS, NDLP_ERR,
               "SPAWN PARENT: Invalid Windows environment for request No %zu, command: %s",
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

    nd_log(NDLS_COLLECTORS, NDLP_INFO,
           "SPAWN PARENT: Running request No %zu, command: '%s'",
           instance->request_id, command);

    int fds_to_keep_open[] = { pipe_stdin[PIPE_READ], pipe_stdout[PIPE_WRITE], pipe_stderr[PIPE_WRITE] };
    os_close_all_non_std_open_fds_except(fds_to_keep_open, 3, CLOSE_RANGE_CLOEXEC);

    // Spawn the process
    errno_clear();
    if (!CreateProcess(NULL, command, NULL, NULL, TRUE, 0, (LPVOID)environment_block, NULL, &si, &pi)) {
        spinlock_unlock(&spinlock);
        nd_log(NDLS_COLLECTORS, NDLP_ERR,
               "SPAWN PARENT: cannot CreateProcess() for request No %zu, command: %s",
               instance->request_id, command);
        goto cleanup;
    }

    // When we create a process with the CreateProcess function, it returns two handles:
    // - one for the process (pi.hProcess) and
    // - one for the primary thread of the new process (pi.hThread).
    // Both of these handles need to be explicitly closed when they are no longer needed.
    CloseHandle(pi.hThread);

    // end of the critical section
    spinlock_unlock(&spinlock);

    nd_environment_snapshot_release(environment_snapshot);

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
    instance->stderr_log_token = log_forwarder_add_fd(server->log_forwarder, instance->stderr_fd);
    log_forwarder_annotate_token_name(server->log_forwarder, instance->stderr_log_token, command);
    log_forwarder_annotate_token_pid(server->log_forwarder, instance->stderr_log_token, spawn_server_instance_pid(instance));

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
    if(environment_snapshot)
        nd_environment_snapshot_release(environment_snapshot);
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

    spawn_server_release_stderr_fd(server, si);

    return spawn_server_exec_wait(server, si);
}

SPAWN_TIMEDWAIT_RESULT spawn_server_exec_timedwait(SPAWN_SERVER *server, SPAWN_INSTANCE *si, int timeout_ms, int *status) {
    if(!si) { if(status) *status = -1; return SPAWN_TIMEDWAIT_EXITED; }

    if(si->read_fd != -1) { close(si->read_fd); si->read_fd = -1; }
    if(si->write_fd != -1) { close(si->write_fd); si->write_fd = -1; }

    // a negative timeout would become a huge DWORD (~INFINITE) to WaitForSingleObject; clamp to poll-once
    if(timeout_ms < 0) timeout_ms = 0;

    DWORD wait_rc = WaitForSingleObject(si->process_handle, (DWORD)timeout_ms);
    if(wait_rc == WAIT_TIMEOUT)
        // the process is still running; the caller decides whether to keep waiting or kill it
        return SPAWN_TIMEDWAIT_RUNNING;

    if(wait_rc == WAIT_FAILED) {
        // the handle is unusable, so we cannot confirm the process exited. We must NOT resolve as
        // EXITED and free the instance (it may still be alive), and we must NOT report RUNNING
        // either (a caller looping on RUNNING with a 0/"wait forever" timeout would spin forever).
        // Report ERROR: the caller keeps the instance and reclaims it by killing it.
        nd_log(NDLS_COLLECTORS, NDLP_ERR,
               "SPAWN PARENT: WaitForSingleObject() failed (err %lu) for request No %zu, pid %d (winpid %u)",
               (unsigned long)GetLastError(), si->request_id, (int)si->child_pid, si->dwProcessId);
        return SPAWN_TIMEDWAIT_ERROR;
    }

    // WAIT_OBJECT_0: the process exited; the blocking wait returns immediately now.
    int st = spawn_server_exec_wait(server, si);
    if(status) *status = st;
    return SPAWN_TIMEDWAIT_EXITED;
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

    spawn_server_release_stderr_fd(server, si);

    freez(si);
    return map_status_code_to_signal(exit_code);
}

#endif
