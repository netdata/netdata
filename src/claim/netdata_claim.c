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

    MessageBoxA(NULL, (LPCTSTR)lpDisplayBuf, TEXT("Error"), MB_OK|MB_ICONERROR);

    LocalFree(lpMsgBuf);
    LocalFree(lpDisplayBuf);
    netdata_claim_exit_callback(0);

    ExitProcess(dw);
}

static inline void netdata_claim_create_process(char *cmd)
{
    STARTUPINFOA si;
    PROCESS_INFORMATION pi;

    ZeroMemory( &si, sizeof(si));
    si.cb = sizeof(si);
    ZeroMemory( &pi, sizeof(pi));

    if( !CreateProcessA(NULL,
                        cmd,
                        NULL, NULL, FALSE, 0, NULL, NULL,
                        &si,
                        &pi)
                        )
    {
        netdata_claim_error_exit(TEXT("CreateProcessA"));
    }

    WaitForSingleObject( pi.hProcess, INFINITE );

    CloseHandle( pi.hProcess );
    CloseHandle( pi.hThread );

    // This Function does not get the child, so we are testing whether we could get the final result
    DWORD ret;
    GetExitCodeProcess(pi.hProcess, &ret);
    if (ret)
        MessageBoxA(NULL, cmd, "Error: Cannot run the command!", MB_OK|MB_ICONERROR);
}

static inline int netdata_claim_prepare_data(char *out, size_t length)
{
    return snprintf(out,
                    length,
                    "[global]\n    url = https://app.netdata.cloud\n    token = %s\n   rooms = %s",
                    aToken,
                    aRoom
                    );
}

static void netdata_claim_write_config(char *path)
{
    char configPath[WINDOWS_MAX_PATH + 1];
    char data[WINDOWS_MAX_PATH + 1];
    snprintf(configPath, WINDOWS_MAX_PATH - 1, "%s\\etc\\netdata\\claim.conf", path);

    HANDLE hf = CreateFileA(configPath, GENERIC_WRITE, 0, NULL, OPEN_ALWAYS, FILE_ATTRIBUTE_NORMAL, NULL);
    if (hf == INVALID_HANDLE_VALUE)
        netdata_claim_error_exit(TEXT("CreateFileA"));

    DWORD length = netdata_claim_prepare_data(data, WINDOWS_MAX_PATH);
    DWORD written = 0;

    BOOL ret = WriteFile(hf, data, length, &written, NULL);
    if (!ret)
        netdata_claim_error_exit(TEXT("WriteFileA"));

    if (length != written)
        MessageBoxW(NULL, L"Cannot write claim.conf.", L"Error", MB_OK|MB_ICONERROR);

    CloseHandle(hf);
}

static void netdata_claim_execute_command()
{
    char *usrPath = { "\\usr\\bin" };
    char basePath[WINDOWS_MAX_PATH];
    char runCmd[WINDOWS_MAX_PATH];
    DWORD length = GetCurrentDirectoryA(WINDOWS_MAX_PATH, basePath);
    if (!length) {
        netdata_claim_error_exit(TEXT("GetCurrentDirectoryA"));
    }

    if (strstr(basePath, usrPath)) {
        length -= 7;
        basePath[length] = '\0';
    }

    snprintf(runCmd, WINDOWS_MAX_PATH - length,
             "msys2_shell.cmd -c \"chmod 0640 %s/etc/netdata/claim.conf; %s/usr/bin/netdatacli reload-claiming-state\"",
             basePath);

    netdata_claim_create_process(runCmd);
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
