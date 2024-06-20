extern "C" {

#include "daemon.h"
#include "libnetdata/libnetdata.h"

int netdata_main(int argc, char *argv[]);

}

#include <windows.h>

#ifdef NETDATA_INTERNAL_CHECKS
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

#else
__attribute__((format(printf, 1, 2)))
static void netdata_service_log(const char *fmt, ...)
{
    UNUSED(fmt);
}
#endif

static SERVICE_STATUS_HANDLE svc_status_handle = nullptr;
static SERVICE_STATUS svc_status = {};
static HANDLE svc_stop_event_handle = nullptr;

static ND_THREAD *exit_thread = nullptr;

static void *call_netdata_exit(void *arg)
{
    UNUSED(arg);

    // Wait until we have to stop the service
    WaitForSingleObject(svc_stop_event_handle, INFINITE);

    // Stop the agent
    netdata_service_log("@call_netdata_exit() - calling netdata_cleanup_and_exit()");
    netdata_cleanup_and_exit(0, NULL, NULL, NULL);

    // Set status to stopped
    CloseHandle(svc_stop_event_handle);
    svc_status.dwControlsAccepted = 0;
    svc_status.dwCurrentState = SERVICE_STOPPED;
    svc_status.dwWin32ExitCode = 0;
    svc_status.dwCheckPoint++;
    if (!SetServiceStatus(svc_status_handle, &svc_status))
    {
        netdata_service_log("@call_netdata_exit() - SetServiceStatus() to SERVICE_STOPPED failed...");
    }

    netdata_service_log("@call_netdata_exit() - returning to god");
    return nullptr;
}

static void WINAPI ServiceControlHandler(DWORD controlCode)
{

    switch (controlCode)
    {
        case SERVICE_CONTROL_STOP: {
            netdata_service_log("ServiceControlHandler(SERVICE_CONTROL_STOP)");

            if (svc_status.dwCurrentState != SERVICE_RUNNING)
                break;

            // Set status to stop-pending
            svc_status.dwControlsAccepted = 0;
            svc_status.dwCurrentState = SERVICE_STOP_PENDING;
            svc_status.dwWin32ExitCode = 0;
            svc_status.dwCheckPoint++;
            if (!SetServiceStatus(svc_status_handle, &svc_status))
            {
                netdata_service_log("@ServiceControlHandler() - SetServiceStatus() to SERVICE_STOP_PENDING failed...");
            }

            SetEvent(svc_stop_event_handle);
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

    svc_status_handle = RegisterServiceCtrlHandler("Netdata", ServiceControlHandler);
    if (!svc_status_handle)
    {
        netdata_service_log("@ServiceMain() - RegisterServiceCtrlHandler() failed...");
        return;
    }

    // Set status to start-pending
    svc_status.dwServiceType = SERVICE_WIN32_OWN_PROCESS;
    svc_status.dwCurrentState = SERVICE_START_PENDING;
    svc_status.dwControlsAccepted = 0;
    svc_status.dwWin32ExitCode = 0;
    svc_status.dwServiceSpecificExitCode = 0;
    svc_status.dwCheckPoint = 0;
    svc_status.dwWaitHint = 5;
    if (!SetServiceStatus(svc_status_handle, &svc_status))
    {
        netdata_service_log("@ServiceMain() - SetServiceStatus() to SERVICE_START_PENDING failed...");
        return;
    }

    // Create stop event handle
    svc_stop_event_handle = CreateEvent(NULL, TRUE, FALSE, NULL);
    if (svc_stop_event_handle == NULL)
    {
        // Set status to stopped if we failed to create the handle
        netdata_service_log("@ServiceMain() - CreateEvent() failed...");
        svc_status.dwControlsAccepted = 0;
        svc_status.dwCurrentState = SERVICE_STOPPED;
        svc_status.dwWin32ExitCode = GetLastError();
        svc_status.dwCheckPoint = 1;
        if (SetServiceStatus(svc_status_handle, &svc_status) == FALSE)
        {
            netdata_service_log("@ServiceMain() - SetServiceStatus() to SERVICE_STOPPED failed...");
        }

        return;
    }

    // Create a thread for stopping/exiting the service
    {
        char tag[NETDATA_THREAD_TAG_MAX + 1];
        snprintfz(tag, NETDATA_THREAD_TAG_MAX, "%s", "WINSVC");
        netdata_service_log("@ServiceMain() - Creating thread for handling stopping the agent service");
        exit_thread = nd_thread_create(tag, NETDATA_THREAD_OPTION_JOINABLE, call_netdata_exit, NULL);
        netdata_service_log("@ServiceMain(): Exit thread created (exit_thread=%p)", exit_thread);
    }

    // Set status to running
    svc_status.dwControlsAccepted = SERVICE_ACCEPT_STOP;
    svc_status.dwCurrentState = SERVICE_RUNNING;
    svc_status.dwWin32ExitCode = 0;
    svc_status.dwCheckPoint = 2;
    if (!SetServiceStatus(svc_status_handle, &svc_status))
    {
        netdata_service_log("@ServiceMain() - SetServiceStatus() to SERVICE_RUNNING failed...");
        return;
    }

    // Run the agent
    {
        netdata_service_log("@ServiceMain() - calling netdata_main()...");
        int nd_argc = 2;
        char *nd_argv[] = {strdupz("/usr/sbin/netdata"), strdupz("-D"), NULL};
        netdata_main(nd_argc, nd_argv);
    }

    netdata_service_log("@ServiceMain() - returning to caller...");
}

int main()
{
#if RUN_FROM_CLI
    int nd_argc = 2;
    char *nd_argv[] = {strdupz("/usr/bin/netdata"), strdupz("-D"), NULL};
    netdata_main(nd_argc, nd_argv);

    while (true) {
        sleep(1);
    }
#else
    SERVICE_TABLE_ENTRY serviceTable[] = {
        { strdupz("Netdata"), ServiceMain },
        { nullptr, nullptr }
    };

    if (!StartServiceCtrlDispatcher(serviceTable))
    {
        netdata_service_log("@main() - StartServiceCtrlDispatcher() failed...");
        return 1;
    }
#endif

    return 0;
}
