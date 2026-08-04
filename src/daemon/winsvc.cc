extern "C" {

#include "daemon.h"
#include "libnetdata/libnetdata.h"
#include "daemon/daemon-shutdown.h"

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
static volatile LONG svc_current_state = SERVICE_STOPPED;
static volatile LONG svc_environment_frozen = 0;
static volatile LONG svc_stop_started = 0;
static volatile LONG svc_checkpoint = 0;

static DWORD svc_stop_control_code = SERVICE_CONTROL_STOP;

static bool ReportSvcStatus(
    DWORD dwCurrentState,
    DWORD dwWin32ExitCode,
    DWORD dwWaitHint,
    DWORD dwControlsAccepted,
    DWORD dwServiceSpecificExitCode = 0)
{
    SERVICE_STATUS status = {};
    status.dwServiceType = SERVICE_WIN32_OWN_PROCESS;
    status.dwCurrentState = dwCurrentState;
    status.dwWin32ExitCode = dwWin32ExitCode;
    status.dwServiceSpecificExitCode = dwServiceSpecificExitCode;
    status.dwWaitHint = dwWaitHint;
    status.dwControlsAccepted = dwControlsAccepted;

    if (dwCurrentState == SERVICE_RUNNING || dwCurrentState == SERVICE_STOPPED)
    {
        status.dwCheckPoint = 0;
        InterlockedExchange(&svc_checkpoint, 0);
    }
    else
    {
        status.dwCheckPoint = (DWORD)InterlockedIncrement(&svc_checkpoint);
    }

    if (!SetServiceStatus(svc_status_handle, &status)) {
        netdata_service_log("@ReportSvcStatus: SetServiceStatusFailed (%d)", GetLastError());
        return false;
    }

    InterlockedExchange(&svc_current_state, (LONG)dwCurrentState);
    return true;
}

extern "C" bool netdata_windows_service_environment_frozen(void)
{
    if (!svc_status_handle)
        return true;

    InterlockedExchange(&svc_environment_frozen, 1);

    if (!ReportSvcStatus(SERVICE_RUNNING, NO_ERROR, 0, SERVICE_ACCEPT_STOP | SERVICE_ACCEPT_SHUTDOWN)) {
        InterlockedExchange(&svc_environment_frozen, 0);
        netdata_service_log("Failed to enable service stop and shutdown controls.");
        return false;
    }

    return true;
}

static void call_netdata_cleanup(void *arg)
{
    UNUSED(arg);

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
    netdata_exit_gracefully(reason, false);

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
            if (!InterlockedCompareExchange(&svc_environment_frozen, 0, 0) ||
                InterlockedCompareExchange(&svc_current_state, 0, 0) != SERVICE_RUNNING)
                return;

            if (InterlockedCompareExchange(&svc_stop_started, 1, 0))
                return;

            // Set service status to stop-pending
            netdata_service_log("Setting service status to stop-pending...");
            if (!ReportSvcStatus(SERVICE_STOP_PENDING, NO_ERROR, 5000, 0)) {
                InterlockedExchange(&svc_stop_started, 0);
                return;
            }

            __atomic_store_n(&svc_stop_control_code, controlCode, __ATOMIC_RELEASE);

            // Create cleanup thread
            netdata_service_log("Creating cleanup thread...");
            char tag[NETDATA_THREAD_TAG_MAX + 1];
            snprintfz(tag, NETDATA_THREAD_TAG_MAX, "%s", "CLEANUP");
            ND_THREAD *cleanup_thread = nd_thread_create(
                tag, NETDATA_THREAD_OPTION_DEFAULT, call_netdata_cleanup, NULL);
            if (!cleanup_thread) {
                netdata_service_log("Cannot create the cleanup thread; stopping the service immediately.");
                ReportSvcStatus(SERVICE_STOPPED, ERROR_NOT_ENOUGH_MEMORY, 0, 0);
                ExitProcess(ERROR_NOT_ENOUGH_MEMORY);
            }
            break;
        }
        case SERVICE_CONTROL_INTERROGATE:
            // The status did not change, so the SCM already has the current value.
            break;

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
    if (!ReportSvcStatus(SERVICE_START_PENDING, NO_ERROR, 5000, 0))
    {
        netdata_service_log("Failed to set service status to start pending.");
        return;
    }

    // Set status to running
    netdata_service_log("Setting service status to running...");
    if (!ReportSvcStatus(SERVICE_RUNNING, NO_ERROR, 0, 0))
    {
        netdata_service_log("Failed to set service status to running.");
        return;
    }

    // Run the agent
    netdata_service_log("Running the agent...");
    int rc = netdata_main(argc, argv);

    if (rc == 10) {
        netdata_service_log("Agent has been started.");
        return;
    }

    netdata_service_log("Agent stopped with exit code %d.", rc);
    ReportSvcStatus(
        SERVICE_STOPPED,
        rc ? ERROR_SERVICE_SPECIFIC_ERROR : NO_ERROR,
        0,
        0,
        (DWORD)rc);
}

extern "C" bool netdata_windows_prepare_path(void) {
    CLEAN_CHAR_P *old_path = nd_environment_get_dup("PATH");

    if (!old_path) {
        if (nd_environment_set("PATH", "/usr/bin", true) != 0) {
            netdata_service_log("Failed to set PATH to /usr/bin");
            return false;
        }

        return true;
    }

    size_t new_path_length = strlen(old_path) + strlen("/usr/bin") + 2;
    char *new_path = (char *) callocz(new_path_length, sizeof(char));
    snprintfz(new_path, new_path_length, "/usr/bin:%s", old_path);

    if (nd_environment_set("PATH", new_path, true) != 0) {
        netdata_service_log("Failed to add /usr/bin to PATH");
        freez(new_path);
        return false;
    }

    freez(new_path);
    return true;
}

int main(int argc, char *argv[])
{
#if defined(OS_WINDOWS) && defined(RUN_UNDER_CLION)
    bool tty = true;
#else
    bool tty = isatty(fileno(stdin)) == 1;
#endif

    if (tty)
    {
        int rc = netdata_main(argc, argv);
        if (rc != 10)
            return rc;

        nd_process_signals();
        return 1;
    }
    else
    {
        SERVICE_TABLE_ENTRY serviceTable[] = {
            { strdupz("Netdata"), ServiceMain },
            { nullptr, nullptr }
        };

        if (!StartServiceCtrlDispatcher(serviceTable))
        {
            netdata_service_log("@main() - StartServiceCtrlDispatcher() failed...");
            return 1;
        }

        return 0;
    }
}
