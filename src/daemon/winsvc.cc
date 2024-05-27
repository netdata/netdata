#include "libnetdata/libnetdata.h"

#include <windows.h>
#include <iostream>
#include <fstream>

extern "C" int netdata_main(int argc, char *argv[]);

static ND_THREAD *main_thread = nullptr;
static SERVICE_STATUS_HANDLE svc_status_handle = nullptr;

#ifdef NETDATA_INTERNAL_CHECKS
static void WriteLog(const std::string &message)
{
    SYSTEMTIME time;
    GetSystemTime(&time);

    std::ofstream log("/opt/netdata/var/log/netdata/service.log", std::ios_base::app);
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

static void WINAPI ServiceControlHandler(DWORD controlCode)
{
    SERVICE_STATUS svc_status = {};

    switch (controlCode)
    {
        case SERVICE_START: {
            WriteLog("ServiceControlHandler(SERVICE_START)");
            svc_status.dwServiceType = SERVICE_WIN32_OWN_PROCESS;
            svc_status.dwCurrentState = SERVICE_START_PENDING;
            svc_status.dwControlsAccepted = SERVICE_ACCEPT_STOP;
            if (!SetServiceStatus(svc_status_handle, &svc_status)) {
                WriteLog("@ServiceControlHandler() - SetServiceStatus() to SERVICE_START_PENDING failed...");
            }
            break;
        }
        case SERVICE_CONTROL_STOP: {
            WriteLog("ServiceControlHandler(SERVICE_CONTROL_STOP)");
            if (svc_status_handle) {
                svc_status.dwServiceType = SERVICE_WIN32_OWN_PROCESS;
                svc_status.dwCurrentState = SERVICE_STOP_PENDING;
                svc_status.dwControlsAccepted = 0;
                if (!SetServiceStatus(svc_status_handle, &svc_status)) {
                    WriteLog("@ServiceControlHandler() - SetServiceStatus() to SERVICE_STOP_PENDING failed...");
                }

                svc_status.dwCurrentState = SERVICE_STOPPED;
                if (!SetServiceStatus(svc_status_handle, &svc_status)) {
                    WriteLog("@ServiceControlHandler() - SetServiceStatus() to SERVICE_STOPPED failed...");
                }
            }
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

    SERVICE_STATUS svc_status = {};
    svc_status.dwServiceType = SERVICE_WIN32_OWN_PROCESS;
    svc_status.dwCurrentState = SERVICE_START_PENDING;
    svc_status.dwControlsAccepted = SERVICE_ACCEPT_STOP;
    svc_status.dwWin32ExitCode = 0;
    svc_status.dwServiceSpecificExitCode = 0;
    svc_status.dwCheckPoint = 0;
    svc_status.dwWaitHint = 0;

    // FWIW: This seems to be necessary for Windows to transition the service to running status.
    if (!SetServiceStatus(svc_status_handle, &svc_status))
    {
        WriteLog("@ServiceMain() - SetServiceStatus() to SERVICE_START_PENDING failed...");
        return;
    }

    svc_status.dwCurrentState = SERVICE_RUNNING;
    if (!SetServiceStatus(svc_status_handle, &svc_status))
    {
        WriteLog("@ServiceMain() - SetServiceStatus() to SERVICE_RUNNING failed...");
        return;
    }

    {
        char tag[NETDATA_THREAD_TAG_MAX + 1];
        snprintfz(tag, NETDATA_THREAD_TAG_MAX, "%s", "WINSVC");

        WriteLog("@ServiceMain() - Spawning the netdata main thread");
        main_thread = nd_thread_create(tag, NETDATA_THREAD_OPTION_JOINABLE, call_netdata_main, NULL);

        WriteLog("@ServiceMain() - Joining on netdata main thread");
        nd_thread_join(main_thread);
    }
}

#if 1
int main() {
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
#else
int main(int argc, char *argv[]) {
    UNUSED(argc);
    UNUSED(argv);

    {
        char tag[NETDATA_THREAD_TAG_MAX + 1];
        snprintfz(tag, NETDATA_THREAD_TAG_MAX, "%s", "WINSVC");
        main_thread = nd_thread_create(tag, NETDATA_THREAD_OPTION_JOINABLE, call_netdata_main, NULL);
        nd_thread_join(main_thread);
        return 0;
    }
}
#endif
