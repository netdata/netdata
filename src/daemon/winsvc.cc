extern "C" {

#include "daemon.h"
#include "libnetdata/libnetdata.h"

int netdata_main(int argc, char *argv[]);

}

#include <windows.h>

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
static SERVICE_STATUS svc_status = {};

static HANDLE svc_stop_event_handle = nullptr;

static ND_THREAD *cleanup_thread = nullptr;

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
        netdata_service_log(msg);

        if (!ReportSvcStatus(SERVICE_STOPPED, GetLastError(), 1000, 0))
        {
            netdata_service_log("Failed to set service status to stopped.");
        }

        return NULL;
    }

    return h;
}

static void *call_netdata_cleanup(void *arg)
{
    UNUSED(arg);

    // Wait until we have to stop the service
    netdata_service_log("Cleanup thread waiting for stop event...");
    WaitForSingleObject(svc_stop_event_handle, INFINITE);

    // Stop the agent
    netdata_service_log("Running netdata cleanup...");
    netdata_cleanup_and_exit(0, NULL, NULL, NULL);

    // Close event handle
    netdata_service_log("Closing stop event handle...");
    CloseHandle(svc_stop_event_handle);

    // Set status to stopped
    netdata_service_log("Reporting the service as stopped...");
    ReportSvcStatus(SERVICE_STOPPED, 0, 0, 0);

    return nullptr;
}

static void WINAPI ServiceControlHandler(DWORD controlCode)
{
    switch (controlCode)
    {
        case SERVICE_CONTROL_STOP:
        {
            if (svc_status.dwCurrentState != SERVICE_RUNNING)
                return;

            // Set service status to stop-pending
            netdata_service_log("Setting service status to stop-pending...");
            if (!ReportSvcStatus(SERVICE_STOP_PENDING, 0, 5000, 0))
                return;

            // Create cleanup thread
            netdata_service_log("Creating cleanup thread...");
            char tag[NETDATA_THREAD_TAG_MAX + 1];
            snprintfz(tag, NETDATA_THREAD_TAG_MAX, "%s", "CLEANUP");
            cleanup_thread = nd_thread_create(tag, NETDATA_THREAD_OPTION_JOINABLE, call_netdata_cleanup, NULL);

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
    if (!ReportSvcStatus(SERVICE_RUNNING, 0, 5000, SERVICE_ACCEPT_STOP))
    {
        netdata_service_log("Failed to set service status to running.");
        return;
    }

    // Run the agent
    netdata_service_log("Running the agent...");
    int nd_argc = 2;
    char *nd_argv[] = {strdupz("/usr/sbin/netdata"), strdupz("-D"), NULL};
    netdata_main(nd_argc, nd_argv);

    netdata_service_log("Agent has been started...");
}

int main()
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
