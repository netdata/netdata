extern "C" {

#include "daemon.h"
#include "libnetdata/libnetdata.h"
#include "daemon/daemon-shutdown.h"
#include "daemon/daemon-shutdown-watcher.h"

int netdata_main(int argc, char *argv[]);
void nd_process_signals(void);

}


static SERVICE_STATUS_HANDLE svc_status_handle = nullptr;
static SERVICE_STATUS svc_status = {};

static HANDLE svc_stop_event_handle = nullptr;
static DWORD svc_stop_control_code = SERVICE_CONTROL_STOP;

static ND_THREAD *cleanup_thread = nullptr;

// Signals the stop-pending heartbeat thread to exit.
static HANDLE svc_heartbeat_done_event = nullptr;

// Handle for the stop-pending heartbeat thread. The abort path
// (netdata_svc_shutdown_aborted) waits on this handle before reporting
// SERVICE_STOPPED, so the heartbeat cannot publish SERVICE_STOP_PENDING
// after the SCM has been told the service is stopped.
static HANDLE heartbeat_thread = nullptr;

static bool ReportSvcStatus(DWORD dwCurrentState, DWORD dwWin32ExitCode, DWORD dwWaitHint, DWORD dwControlsAccepted)
{
    static DWORD dwCheckPoint = 1;

    // Take the lock so the heartbeat cannot publish SERVICE_STOP_PENDING
    // between our writes to svc_status and the SetServiceStatus call below.
    // The timeout path joins the heartbeat before taking this lock, so the
    // heartbeat cannot be blocked while it exits.
    svc_status_lock_ensure_init();
    EnterCriticalSection(&svc_status_lock);

    svc_status.dwCurrentState = dwCurrentState;
    svc_status.dwWin32ExitCode = dwWin32ExitCode;
    svc_status.dwWaitHint = dwWaitHint;
    svc_status.dwControlsAccepted = dwControlsAccepted;

    if (dwCurrentState == SERVICE_RUNNING || dwCurrentState == SERVICE_STOPPED)
    {
        svc_status.dwCheckPoint = 0;
    }
    else
    {
        svc_status.dwCheckPoint = dwCheckPoint++;
    }

    bool ok = SetServiceStatus(svc_status_handle, &svc_status) != 0;

    LeaveCriticalSection(&svc_status_lock);
    return ok;
}

static HANDLE CreateEventHandle(void)
{
    HANDLE h = CreateEvent(NULL, TRUE, FALSE, NULL);

    if (!h)
    {
        ReportSvcStatus(SERVICE_STOPPED, GetLastError(), 1000, 0);
        return NULL;
    }

    return h;
}

// Serializes updates to the SCM-visible svc_status and the abort path. The
// shutdown-timeout callback (`svc_report_stopped_before_abort`) and the
// stop-pending heartbeat thread both write svc_status; without this lock the
// heartbeat can publish SERVICE_STOP_PENDING *after* the callback has set
// SERVICE_STOPPED, and the SCM records a "stopped after stop pending" race.
static CRITICAL_SECTION svc_status_lock;
static bool svc_status_lock_init_done = false;

static void svc_status_lock_ensure_init(void)
{
    if (svc_status_lock_init_done)
        return;

    // InitializeCriticalSection is not async-signal-safe, but the heartbeat
    // thread and the abort callback both run from normal thread context, not
    // from a signal handler. Calling it once at startup is safe.
    InitializeCriticalSection(&svc_status_lock);
    svc_status_lock_init_done = true;
}

// Stops the stop-pending heartbeat (if any) so the abort callback can publish
// SERVICE_STOPPED without racing it. Returns after the heartbeat thread has
// either signalled completion or been observed alive; the caller does not
// own the heartbeat handle. Idempotent and safe to call from any thread,
// including the shutdown-timeout callback.
extern "C" void netdata_svc_shutdown_aborted(void)
{
    svc_status_lock_ensure_init();
    if (svc_heartbeat_done_event) {
        // Signal the heartbeat to exit and wait briefly. We are already past
        // the SCM's dwWaitHint, so a few extra seconds here only delay an
        // abort that is going to tear the process down anyway.
        SetEvent(svc_heartbeat_done_event);
        if (heartbeat_thread) {
            WaitForSingleObject(heartbeat_thread, 3000);
        }
    }

    // Join before taking the status lock: the heartbeat publishes through
    // ReportSvcStatus() and therefore needs this same lock to exit.
    EnterCriticalSection(&svc_status_lock);
    ReportSvcStatus(SERVICE_STOPPED, 0, 0, 0);

    LeaveCriticalSection(&svc_status_lock);
}

// Called by the watcher before abort() when a shutdown step times out.
// Reports SERVICE_STOPPED so the SCM marks the service as stopped rather than
// crashed; the process then terminates via abort() a few instructions later.
static void svc_report_stopped_before_abort(void)
{
    netdata_svc_shutdown_aborted();
}

// Map a Windows Service Control Manager control code to a netdata EXIT_REASON.
// Extracted so the call site can use a C++17 init-statement and keep the
// controlCode variable scoped tight (SonarQube S6004 init-statement rule).
static EXIT_REASON svc_control_code_to_exit_reason(DWORD controlCode)
{
    switch(controlCode) {
        case SERVICE_CONTROL_SHUTDOWN:
            return (EXIT_REASON)(EXIT_REASON_SERVICE_STOP|EXIT_REASON_SYSTEM_SHUTDOWN);

        // SERVICE_CONTROL_STOP (and any other unrecognised code) fall through
        // to the default case below.
        default:
            return EXIT_REASON_SERVICE_STOP;
    }
}

// Heartbeat thread: keeps re-sending SERVICE_STOP_PENDING every 2 s so the
// SCM does not fire error 1053 while netdata_exit_gracefully() runs.
// Exits when svc_heartbeat_done_event is signalled.
static DWORD WINAPI stop_pending_heartbeat(LPVOID /*unused*/)
{
    // dwWaitHint of 5000 ms; heartbeat fires every 2000 ms — well within the hint.
    while (WaitForSingleObject(svc_heartbeat_done_event, 2000) == WAIT_TIMEOUT)
        ReportSvcStatus(SERVICE_STOP_PENDING, 0, 5000, 0);

    return 0;
}

// Handle for the stop-pending heartbeat thread. The path that waits for
// the cleanup to finish uses this handle to join the heartbeat thread
// before reporting SERVICE_STOPPED.
static NORETURN void call_netdata_cleanup(void *arg)
{
    UNUSED(arg);

    // Wait until we have to stop the service
    WaitForSingleObject(svc_stop_event_handle, INFINITE);

    // Keep the SCM informed while cleanup runs; without periodic
    // SERVICE_STOP_PENDING updates the SCM times out (error 1053) if
    // netdata_exit_gracefully() takes longer than dwWaitHint (5 s).
    svc_heartbeat_done_event = CreateEvent(NULL, TRUE, FALSE, NULL);
    heartbeat_thread = nullptr;
    if (svc_heartbeat_done_event)
        heartbeat_thread = CreateThread(NULL, 0, stop_pending_heartbeat, NULL, 0, NULL);

    // Stop the agent
    // C++17 init-statement scopes the loaded `controlCode` to the helper
    // call so the variable does not leak into the rest of the function
    // (SonarQube S6004 init-statement rule). The switch that maps control
    // codes to EXIT_REASON values lives inside the helper.
    EXIT_REASON reason = svc_control_code_to_exit_reason(
        __atomic_load_n(&svc_stop_control_code, __ATOMIC_ACQUIRE));

    // If the shutdown watcher times out and calls abort(), report SERVICE_STOPPED
    // to the SCM first so the service is not recorded as crashed.
    nd_register_shutdown_timeout_cb(svc_report_stopped_before_abort);
    netdata_exit_gracefully(reason, false);
    // Drain the WEL/ETW async writer before the process exits so no log entries are lost.
    nd_log_stop_windows_async();
    // Cleanup completed normally — no longer need the abort callback.
    nd_register_shutdown_timeout_cb(NULL);

    // Stop the heartbeat before reporting SERVICE_STOPPED.
    if (svc_heartbeat_done_event) {
        SetEvent(svc_heartbeat_done_event);
        if (heartbeat_thread) {
            WaitForSingleObject(heartbeat_thread, 5000);
            CloseHandle(heartbeat_thread);
        }
        CloseHandle(svc_heartbeat_done_event);
        svc_heartbeat_done_event = nullptr;
        heartbeat_thread = nullptr;
    }

    // Set status to stopped
    ReportSvcStatus(SERVICE_STOPPED, 0, 0, 0);

    // SERVICE_STOPPED closes the SCM context; terminate instead of returning to
    // stale service code.
    exit(0);
}

static void WINAPI ServiceControlHandler(DWORD controlCode)
{
    switch (controlCode)
    {
        case SERVICE_CONTROL_SHUTDOWN:
        case SERVICE_CONTROL_STOP:
        {
            if (svc_status.dwCurrentState != SERVICE_RUNNING)
                return;

            // Set service status to stop-pending
            if (!ReportSvcStatus(SERVICE_STOP_PENDING, 0, 5000, 0))
                return;

            __atomic_store_n(&svc_stop_control_code, controlCode, __ATOMIC_RELEASE);

            // Create cleanup thread
            char tag[NETDATA_THREAD_TAG_MAX + 1];
            snprintfz(tag, NETDATA_THREAD_TAG_MAX, "%s", "CLEANUP");
            cleanup_thread = nd_thread_create(tag, NETDATA_THREAD_OPTION_DEFAULT, call_netdata_cleanup, NULL);

            // There is no other worker that can consume the stop request when
            // creation fails.  Report a terminal state and terminate instead
            // of leaving the service stuck in SERVICE_STOP_PENDING forever.
            if (!cleanup_thread) {
                ReportSvcStatus(SERVICE_STOPPED, ERROR_NOT_ENOUGH_MEMORY, 0, 0);
                ExitProcess(ERROR_NOT_ENOUGH_MEMORY);
            }

            // Signal the stop request
            SetEvent(svc_stop_event_handle);
            break;
        }
        case SERVICE_CONTROL_INTERROGATE:
        {
            ReportSvcStatus(svc_status.dwCurrentState, svc_status.dwWin32ExitCode, svc_status.dwWaitHint, svc_status.dwControlsAccepted);
            break;
        }
        default:
            break;
    }
}

void WINAPI ServiceMain(DWORD argc, LPSTR* argv)
{
    UNUSED(argc);
    UNUSED(argv);

    // Create service status handle
    svc_status_handle = RegisterServiceCtrlHandler("Netdata", ServiceControlHandler);
    if (!svc_status_handle)
        return;

    // Set status to start-pending
    svc_status.dwServiceType = SERVICE_WIN32_OWN_PROCESS;
    svc_status.dwServiceSpecificExitCode = 0;
    svc_status.dwCheckPoint = 0;
    if (!ReportSvcStatus(SERVICE_START_PENDING, 0, 5000, 0))
        return;

    // Create stop service event handle
    svc_stop_event_handle = CreateEventHandle();
    if (!svc_stop_event_handle)
        return;

    // Set status to running
    if (!ReportSvcStatus(SERVICE_RUNNING, 0, 5000, SERVICE_ACCEPT_STOP | SERVICE_ACCEPT_SHUTDOWN))
        return;

    // Run the agent. C++17 init-statement scopes `rc` to the if body
    // (SonarQube S6004 init-statement rule); `rc` is not used after the
    // early-exit block.
    if (int rc = netdata_main(argc, argv); rc != 10) {
        // netdata_main() exited early — bad arguments, --help, or an
        // initialisation error.  Transition to STOPPED so the SCM records
        // the failure instead of leaving the service stuck in SERVICE_RUNNING
        // waiting for a stop event that will never be signalled internally.
        // The same `if (rc != 10) return rc;` early-exit also appears in
        // main() below for the CLI entry path; both are intentional because
        // the SCM path must report SERVICE_STOPPED before returning.
        svc_status.dwServiceSpecificExitCode = rc;
        ReportSvcStatus(SERVICE_STOPPED, ERROR_SERVICE_SPECIFIC_ERROR, 0, 0);
        return;
    }

    // netdata_main() spawns background threads and returns once the agent is
    // running.  Without blocking here, ServiceMain would return, which causes
    // StartServiceCtrlDispatcher (in main()) to return, main() to return 0,
    // and ExitProcess to silently kill every background thread before any
    // useful work is done.  Block on the stop event until the SCM sends
    // SERVICE_CONTROL_STOP or SERVICE_CONTROL_SHUTDOWN.
    WaitForSingleObject(svc_stop_event_handle, INFINITE);

    // ServiceControlHandler already created the cleanup thread and signalled
    // the stop event.  The cleanup thread runs netdata_exit_gracefully() and
    // calls exit(0) when done.  Loop here so that ServiceMain never returns
    // before that exit(0) fires; returning would let StartServiceCtrlDispatcher
    // → main() → ExitProcess race the cleanup thread and lose.
    while(true)
        Sleep(1000);
}

static bool update_path() {
    const char *old_path = getenv("PATH");

    if (!old_path) {
        if (setenv("PATH", "/usr/bin", 1) != 0)
            return false;

        return true;
    }

    size_t new_path_length = strlen(old_path) + strlen("/usr/bin") + 2;
    char *new_path = (char *) callocz(new_path_length, sizeof(char));
    snprintfz(new_path, new_path_length, "/usr/bin:%s", old_path);

    if (setenv("PATH", new_path, 1) != 0) {
        freez(new_path);
        return false;
    }

    freez(new_path);
    return true;
}

int main(int argc, char *argv[])
{
    if (!update_path()) {
        return 1;
    }

    // Derive the install prefix from the binary location and override
    // all netdata_configured_* path globals with the real installed
    // paths before netdata_main() or StartServiceCtrlDispatcher() runs.
    // Without this, compile-time POSIX staging paths (/opt/netdata/...)
    // would be used, which UCRT64 resolves as C:\opt\netdata\... —
    // a path that never exists on a target machine.
    nd_windows_detect_prefix_and_override_paths();

    SERVICE_TABLE_ENTRY serviceTable[] = {
        { strdupz("Netdata"), ServiceMain },
        { nullptr, nullptr }
    };

    if (!StartServiceCtrlDispatcher(serviceTable))
    {
        DWORD err = GetLastError();
        if (err == ERROR_FAILED_SERVICE_CONTROLLER_CONNECT)
        {
            // Not invoked by the Service Control Manager — run in CLI mode.
            // This covers interactive terminals, GDB, PowerShell, CLion, and
            // CI scripts.  StartServiceCtrlDispatcher() is the authoritative
            // Windows API for detecting service context: unlike isatty(), it
            // returns ERROR_FAILED_SERVICE_CONTROLLER_CONNECT correctly on
            // UCRT64 even when Windows allocates an invisible console for the
            // process (which causes isatty() to return true for service
            // processes linked against the native CRT).
            int rc = netdata_main(argc, argv);
            if (rc != 10)
                return rc;

            nd_process_signals();
            return 1;
        }

        return 1;
    }

    return 0;
}
