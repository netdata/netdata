extern "C" {

#include "daemon.h"
#include "libnetdata/libnetdata.h"
#include "daemon/daemon-shutdown.h"
#include "daemon/daemon-shutdown-watcher.h"

int netdata_main(int argc, char *argv[]);
void nd_process_signals(void);

}

__attribute__((format(printf, 1, 2)))
static void netdata_service_log(const char *fmt, ...)
{
    char path[FILENAME_MAX + 1];
    snprintfz(path, FILENAME_MAX, "%s/service.log", LOG_DIR);

    FILE *fp = fopen(path, "a");
    if (fp == NULL) {
        // LOG_DIR is a POSIX path compiled for the staging environment
        // (e.g. /opt/netdata/var/log/netdata).  UCRT64 binaries use the
        // native Windows CRT directly — there is no msys-2.0.dll POSIX path
        // translation layer — so /opt/netdata/... resolves to
        // C:\opt\netdata\... on the target machine, which never exists.
        // Fall back to the Windows temp directory, which is always writable
        // by the service account (SYSTEM writes to C:\Windows\Temp\).
        char tmp_dir[FILENAME_MAX + 1];
        DWORD tmp_len = GetTempPathA(FILENAME_MAX, tmp_dir);
        if (tmp_len > 0 && tmp_len < FILENAME_MAX - 24) {
            snprintfz(path, FILENAME_MAX, "%snetdata-service.log", tmp_dir);
            fp = fopen(path, "a");
        }
        if (fp == NULL)
            return;
    }

    SYSTEMTIME time;
    GetSystemTime(&time);
    fprintf(fp, "%d:%d:%d - ", time.wHour, time.wMinute, time.wSecond);

    va_list args;
    va_start(args, fmt);
    vfprintf(fp, fmt, args);
    va_end(args);

    fprintf(fp, "\n");

    fflush(fp);
    fclose(fp);
}

static SERVICE_STATUS_HANDLE svc_status_handle = nullptr;
static SERVICE_STATUS svc_status = {};

static HANDLE svc_stop_event_handle = nullptr;
static DWORD svc_stop_control_code = SERVICE_CONTROL_STOP;

static ND_THREAD *cleanup_thread = nullptr;

// Signals the stop-pending heartbeat thread to exit.
static HANDLE svc_heartbeat_done_event = nullptr;

static bool ReportSvcStatus(DWORD dwCurrentState, DWORD dwWin32ExitCode, DWORD dwWaitHint, DWORD dwControlsAccepted)
{
    static DWORD dwCheckPoint = 1;
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

    if (!SetServiceStatus(svc_status_handle, &svc_status)) {
        netdata_service_log("@ReportSvcStatus: SetServiceStatusFailed (%d)", GetLastError());
        return false;
    }

    return true;
}

static HANDLE CreateEventHandle(const char *msg)
{
    HANDLE h = CreateEvent(NULL, TRUE, FALSE, NULL);

    if (!h)
    {
        netdata_service_log("%s", msg);

        if (!ReportSvcStatus(SERVICE_STOPPED, GetLastError(), 1000, 0))
        {
            netdata_service_log("Failed to set service status to stopped.");
        }

        return NULL;
    }

    return h;
}

// Called by the watcher before abort() when a shutdown step times out.
// Reports SERVICE_STOPPED so the SCM marks the service as stopped rather than
// crashed; the process then terminates via abort() a few instructions later.
static void svc_report_stopped_before_abort(void)
{
    ReportSvcStatus(SERVICE_STOPPED, 0, 0, 0);
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

static void call_netdata_cleanup(void *arg)
{
    UNUSED(arg);

    // Wait until we have to stop the service
    netdata_service_log("Cleanup thread waiting for stop event...");
    WaitForSingleObject(svc_stop_event_handle, INFINITE);

    // Keep the SCM informed while cleanup runs; without periodic
    // SERVICE_STOP_PENDING updates the SCM times out (error 1053) if
    // netdata_exit_gracefully() takes longer than dwWaitHint (5 s).
    svc_heartbeat_done_event = CreateEvent(NULL, TRUE, FALSE, NULL);
    HANDLE heartbeat = nullptr;
    if (svc_heartbeat_done_event)
        heartbeat = CreateThread(NULL, 0, stop_pending_heartbeat, NULL, 0, NULL);

    // Stop the agent
    netdata_service_log("Running netdata cleanup...");
    EXIT_REASON reason;
    DWORD controlCode = __atomic_load_n(&svc_stop_control_code, __ATOMIC_ACQUIRE);
    switch(controlCode) {
        case SERVICE_CONTROL_SHUTDOWN:
            reason = (EXIT_REASON)(EXIT_REASON_SERVICE_STOP|EXIT_REASON_SYSTEM_SHUTDOWN);
            break;

        case SERVICE_CONTROL_STOP:
            // fall-through

        default:
            reason = EXIT_REASON_SERVICE_STOP;
            break;
    }

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
        if (heartbeat) {
            WaitForSingleObject(heartbeat, 5000);
            CloseHandle(heartbeat);
        }
        CloseHandle(svc_heartbeat_done_event);
        svc_heartbeat_done_event = nullptr;
    }

    // Close event handle
    netdata_service_log("Closing stop event handle...");
    CloseHandle(svc_stop_event_handle);

    // Set status to stopped
    netdata_service_log("Reporting the service as stopped...");
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
            netdata_service_log("Setting service status to stop-pending...");
            if (!ReportSvcStatus(SERVICE_STOP_PENDING, 0, 5000, 0))
                return;

            __atomic_store_n(&svc_stop_control_code, controlCode, __ATOMIC_RELEASE);

            // Create cleanup thread
            netdata_service_log("Creating cleanup thread...");
            char tag[NETDATA_THREAD_TAG_MAX + 1];
            snprintfz(tag, NETDATA_THREAD_TAG_MAX, "%s", "CLEANUP");
            cleanup_thread = nd_thread_create(tag, NETDATA_THREAD_OPTION_DEFAULT, call_netdata_cleanup, NULL);

            // Signal the stop request
            netdata_service_log("Signalling the cleanup thread...");
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
    netdata_service_log("Creating service status handle...");
    svc_status_handle = RegisterServiceCtrlHandler("Netdata", ServiceControlHandler);
    if (!svc_status_handle)
    {
        netdata_service_log("@ServiceMain() - RegisterServiceCtrlHandler() failed...");
        return;
    }

    // Set status to start-pending
    netdata_service_log("Setting service status to start-pending...");
    svc_status.dwServiceType = SERVICE_WIN32_OWN_PROCESS;
    svc_status.dwServiceSpecificExitCode = 0;
    svc_status.dwCheckPoint = 0;
    if (!ReportSvcStatus(SERVICE_START_PENDING, 0, 5000, 0))
    {
        netdata_service_log("Failed to set service status to start pending.");
        return;
    }

    // Create stop service event handle
    netdata_service_log("Creating stop service event handle...");
    svc_stop_event_handle = CreateEventHandle("Failed to create stop event handle");
    if (!svc_stop_event_handle)
        return;

    // Set status to running
    netdata_service_log("Setting service status to running...");
    if (!ReportSvcStatus(SERVICE_RUNNING, 0, 5000, SERVICE_ACCEPT_STOP | SERVICE_ACCEPT_SHUTDOWN))
    {
        netdata_service_log("Failed to set service status to running.");
        return;
    }

    // Run the agent
    netdata_service_log("Running the agent...");
    int rc = netdata_main(argc, argv);

    if (rc != 10) {
        // netdata_main() exited early — bad arguments, --help, or an
        // initialisation error.  Transition to STOPPED so the SCM records
        // the failure instead of leaving the service stuck in SERVICE_RUNNING
        // waiting for a stop event that will never be signalled internally.
        netdata_service_log("Agent exited early with rc=%d, stopping service.", rc);
        svc_status.dwServiceSpecificExitCode = rc;
        ReportSvcStatus(SERVICE_STOPPED, ERROR_SERVICE_SPECIFIC_ERROR, 0, 0);
        return;
    }

    netdata_service_log("Agent has been started...");

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
    while(1)
        Sleep(1000);
}

static bool update_path() {
    const char *old_path = getenv("PATH");

    if (!old_path) {
        if (setenv("PATH", "/usr/bin", 1) != 0) {
            netdata_service_log("Failed to set PATH to /usr/bin");
            return false;
        }

        return true;
    }

    size_t new_path_length = strlen(old_path) + strlen("/usr/bin") + 2;
    char *new_path = (char *) callocz(new_path_length, sizeof(char));
    snprintfz(new_path, new_path_length, "/usr/bin:%s", old_path);

    if (setenv("PATH", new_path, 1) != 0) {
        netdata_service_log("Failed to add /usr/bin to PATH");
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

        netdata_service_log("@main() - StartServiceCtrlDispatcher() failed (%lu)", (unsigned long)err);
        return 1;
    }

    return 0;
}
