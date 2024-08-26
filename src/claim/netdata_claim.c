// SPDX-License-Identifier: GPL-3.0-or-later

#define UNICODE
#define _UNICODE
#include <windows.h>
#include <shellapi.h>
#include <stdlib.h>
#include <stdio.h>
#include <string.h>
#include <signal.h>

#include "netdata_claim.h"

LPWSTR token = NULL;
LPWSTR room = NULL;
LPWSTR *argv = NULL;

char *aToken = NULL;
char *aRoom = NULL;

/**
 *  Parse Args
 *
 *  Parse arguments identifying necessity to make a window
 *
 * @param argc number of arguments
 * @param argv A pointer for all arguments given
 *
 * @return it return the number of arguments parsed.
 */
int nd_claim_parse_args(int argc, LPWSTR *argv)
{
    int i;
    for (i = 1 ; i < argc; i++) {
        // We are working with Microsoft, thus it does not make sense wait for only smallcase
        if(wcscasecmp(L"claim-token", argv[i]) == 0 ||
           wcscasecmp(L"/T", argv[i]) == 0) {
            i++;
            token = argv[i];
        }

        if(wcscasecmp(L"claim-rooms", argv[i]) == 0 ||
           wcscasecmp(L"/R", argv[i]) == 0) {
            i++;
            room = argv[i];
        }
    }

    if (!token || !room)
        return 0;

    return argc;
}

static inline void netdata_claim_convert_str(char *dst, wchar_t *src, size_t len) {
    size_t copied = wcstombs(dst, src, len);
    dst[copied] = '\0';
}

static int netdata_claim_prepare_strings()
{
    if (!token || !room)
        return -1;

    size_t length = wcslen(token) + 1;
    aToken = calloc(sizeof(char), length);
    if (!aToken)
        return -1;

    netdata_claim_convert_str(aToken, token, length - 1);

    length = wcslen(room) + 1;
    aRoom = calloc(sizeof(char), length - 1);
    if (!aRoom)
        return -1;

    netdata_claim_convert_str(aRoom, room, length - 1);

    return 0;
}

static void netdata_claim_exit_callback(int signal)
{
    (void)signal;
    if (aToken)
        free(aToken);
    if (aRoom)
        free(aRoom);

    if (argv)
        LocalFree(argv);
}

static void netdata_claim_error_exit(LPCTSTR function)
{
    LPVOID lpMsgBuf;
    LPVOID lpDisplayBuf;
    DWORD dw = GetLastError();

    FormatMessage(FORMAT_MESSAGE_ALLOCATE_BUFFER | FORMAT_MESSAGE_FROM_SYSTEM | FORMAT_MESSAGE_IGNORE_INSERTS,
                 NULL,
                 dw,
                 MAKELANGID(LANG_NEUTRAL, SUBLANG_DEFAULT),
                 (LPTSTR) &lpMsgBuf,
                 0, NULL );

    lpDisplayBuf = (LPVOID)LocalAlloc(LMEM_ZEROINIT,
                                      (lstrlen((LPCTSTR)lpMsgBuf) + lstrlen((LPCTSTR)function) + 40) * sizeof(TCHAR));

    snprintf((char *)lpDisplayBuf,
                    LocalSize(lpDisplayBuf) / sizeof(TCHAR),
                    "%s failed with error %d: %s",
                    function,
                    dw,
                    lpMsgBuf);

    MessageBox(NULL, (LPCTSTR)lpDisplayBuf, TEXT("Error"), MB_OK);

    LocalFree(lpMsgBuf);
    LocalFree(lpDisplayBuf);
    netdata_claim_exit_callback(0);

    ExitProcess(dw);
}

static inline void netdata_claim_create_process(char *cmd)
{
    /*
    STARTUPINFO si;
    PROCESS_INFORMATION pi;

    ZeroMemory( &si, sizeof(si));
    si.cb = sizeof(si);
    ZeroMemory( &pi, sizeof(pi));

    if( !CreateProcessA(NULL,
                        cmd,
                        NULL,
                        NULL,
                        FALSE,
                        0,
                        NULL,
                        NULL,
                        &si,
                        &pi)
                        )
    {
        netdata_claim_error_exit(TEXT("CreateProcessA"));
    }

    WaitForSingleObject( pi.hProcess, INFINITE );

    CloseHandle( pi.hProcess );
    CloseHandle( pi.hThread );

    DWORD ret;
    GetExitCodeProcess(pi.hProcess, ret);

     //TODO: WORK WITH ERRORS:
     */
}

static void netdata_claim_execute_command()
{
    char *usrPath = { "\\usr\\bin" };
    char runCmd[WINDOWS_MAX_PATH];
    DWORD length = GetCurrentDirectoryA(WINDOWS_MAX_PATH, runCmd);
    if (!length) {
        netdata_claim_error_exit(TEXT("GetCurrentDirectoryA"));
    }

    // When we run from installer, usr\bin is ommited, so we have to check its presence
    if (!strstr(runCmd, usrPath)) {
        strncpy(&runCmd[length], usrPath, sizeof(usrPath));
        length += sizeof(usrPath);
        runCmd[length] = '\0';
    }

    char *path = strdup(runCmd);

    snprintf(&runCmd[length], WINDOWS_MAX_PATH - length,
             "\\bash.exe -c \"%s\\netdata-claim.sh --claim-token %s --claim-rooms %s --claim-url https://app.netdata.cloud\"",
             path, aToken, aRoom);

    netdata_claim_create_process(runCmd);

    free(path);
}

int WINAPI WinMain(HINSTANCE hInstance, HINSTANCE hPrevInstance, LPSTR lpCmdLine, int nCmdShow)
{
    signal(SIGABRT, netdata_claim_exit_callback);
    signal(SIGINT, netdata_claim_exit_callback);
    signal(SIGTERM, netdata_claim_exit_callback);

    int argc;
    LPWSTR *argv = CommandLineToArgvW(GetCommandLineW(), &argc);
    if (argc)
        argc = nd_claim_parse_args(argc, argv);

    // When no data is given, user must to use graphic mode
    int ret = 0;
    if (!argc) {
        ret = netdata_claim_window_loop(hInstance, nCmdShow);
    } else {
        if (!netdata_claim_prepare_strings())
            netdata_claim_execute_command();
    }

    netdata_claim_exit_callback(0);

    return ret;
}
