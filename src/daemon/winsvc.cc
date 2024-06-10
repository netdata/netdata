extern "C" {

#include "daemon.h"
#include "libnetdata/libnetdata.h"

}

#include <windows.h>
#include <iostream>
#include <fstream>

extern "C" int netdata_main(int argc, char *argv[]);

static SERVICE_STATUS_HANDLE svc_status_handle = nullptr;
static SERVICE_STATUS svc_status = {};
static HANDLE svc_stop_event_handle = nullptr;

static ND_THREAD *exit_thread = nullptr;

#ifdef NETDATA_INTERNAL_CHECKS
static void WriteLog(const std::string &message)
{
    SYSTEMTIME time;
    GetSystemTime(&time);

    std::ofstream log("/var/log/netdata/service.log", std::ios_base::app);
    log << time.wHour << ":" << time.wMinute << ":" << time.wSecond << " - " << message << std::endl;
}
#else
static void WriteLog(const std::string &message)
{
    UNUSED(message);
}
#endif

static void *call_netdata_main(void *arg)
{
    UNUSED(arg);

    int nd_argc = 2;
    char *nd_argv[] = {strdupz("/usr/bin/netdata"), strdupz("-D"), NULL};
    netdata_main(nd_argc, nd_argv);
    return nullptr;
}

static void *call_netdata_exit(void *arg)
{
    UNUSED(arg);

    // Wait until we have to stop the service
    WaitForSingleObject(svc_stop_event_handle, INFINITE);

    // Stop the agent
    WriteLog("@call_netdata_exit() - calling netdata_cleanup_and_exit()");
    netdata_cleanup_and_exit(0, NULL, NULL, NULL);

    // Set status to stopped
    CloseHandle(svc_stop_event_handle);
    svc_status.dwControlsAccepted = 0;
    svc_status.dwCurrentState = SERVICE_STOPPED;
    svc_status.dwWin32ExitCode = 0;
    svc_status.dwCheckPoint = 3;
    if (!SetServiceStatus(svc_status_handle, &svc_status))
    {
        WriteLog("@call_netdata_exit() - SetServiceStatus() to SERVICE_STOPPED failed...");
        return nullptr;
    }

    // Exit - Start a new process instead of keeping alive the service.
    exit(0);
}

static void WINAPI ServiceControlHandler(DWORD controlCode)
{

    switch (controlCode)
    {
        case SERVICE_CONTROL_STOP: {
            WriteLog("ServiceControlHandler(SERVICE_CONTROL_STOP)");

            if (svc_status.dwCurrentState != SERVICE_RUNNING)
                break;

            // Set status to stop-pending
            svc_status.dwControlsAccepted = 0;
            svc_status.dwCurrentState = SERVICE_STOP_PENDING;
            svc_status.dwWin32ExitCode = 0;
            svc_status.dwCheckPoint = 4;
            if (!SetServiceStatus(svc_status_handle, &svc_status))
            {
                WriteLog("@ServiceControlHandler() - SetServiceStatus() to SERVICE_STOP_PENDING failed...");
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
        WriteLog("@ServiceMain() - RegisterServiceCtrlHandler() failed...");
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
        WriteLog("@ServiceMain() - SetServiceStatus() to SERVICE_START_PENDING failed...");
        return;
    }

    // Create stop event handle
    svc_stop_event_handle = CreateEvent(NULL, TRUE, FALSE, NULL);
    if (svc_stop_event_handle == NULL)
    {
        // Set status to stopped if we failed to create the handle
        WriteLog("@ServiceMain() - CreateEvent() failed...");
        svc_status.dwControlsAccepted = 0;
        svc_status.dwCurrentState = SERVICE_STOPPED;
        svc_status.dwWin32ExitCode = GetLastError();
        svc_status.dwCheckPoint = 1;
        if (SetServiceStatus(svc_status_handle, &svc_status) == FALSE)
        {
            WriteLog("@ServiceMain() - SetServiceStatus() to SERVICE_STOPPED failed...");
        }

        return;
    }

    // Create a thread for stopping/exiting the service
    {
        char tag[NETDATA_THREAD_TAG_MAX + 1];
        snprintfz(tag, NETDATA_THREAD_TAG_MAX, "%s", "WINSVC");
        WriteLog("@ServiceMain() - Creating thread for handling stopping the agent service");
        exit_thread = nd_thread_create(tag, NETDATA_THREAD_OPTION_JOINABLE, call_netdata_exit, NULL);
    }

    // Set status to running
    svc_status.dwControlsAccepted = SERVICE_ACCEPT_STOP;
    svc_status.dwCurrentState = SERVICE_RUNNING;
    svc_status.dwWin32ExitCode = 0;
    svc_status.dwCheckPoint = 2;
    if (!SetServiceStatus(svc_status_handle, &svc_status))
    {
        WriteLog("@ServiceMain() - SetServiceStatus() to SERVICE_RUNNING failed...");
        return;
    }

    // Run the agent
    {
        WriteLog("@ServiceMain() - calling netdata_main()...");

        int nd_argc = 2;
        char *nd_argv[] = {strdupz("/usr/bin/netdata"), strdupz("-D"), NULL};
        netdata_main(nd_argc, nd_argv);
    }
}

int main()
{
    SERVICE_TABLE_ENTRY serviceTable[] = {
        { strdupz("Netdata"), ServiceMain },
        { nullptr, nullptr }
    };

    if (!StartServiceCtrlDispatcher(serviceTable))
    {
        WriteLog("@main() - StartServiceCtrlDispatcher() failed...");
        return 1;
    }

    return 0;
}
