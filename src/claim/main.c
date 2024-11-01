// SPDX-License-Identifier: GPL-3.0-or-later

#define UNICODE
#define _UNICODE
#include <windows.h>
#include <shellapi.h>
#include <stdlib.h>
#include <stdio.h>
#include <string.h>
#include <signal.h>

#include "main.h"

LPWSTR token = NULL;
LPWSTR room = NULL;
LPWSTR proxy = NULL;
LPWSTR url = NULL;
LPWSTR extPath = NULL;
LPWSTR *argv = NULL;

char *aToken = NULL;
char *aRoom = NULL;
char *aProxy = NULL;
char *aURL = NULL;
int insecure = 0;

LPWSTR netdata_claim_get_formatted_message(LPWSTR pMessage, ...)
{
    LPWSTR pBuffer = NULL;

    va_list args = NULL;
    va_start(args, pMessage);

    FormatMessage(FORMAT_MESSAGE_FROM_STRING | FORMAT_MESSAGE_ALLOCATE_BUFFER, pMessage, 0, 0, (LPWSTR)&pBuffer,
    0, &args);
    va_end(args);

    return pBuffer;
}

// Common Functions
void netdata_claim_error_exit(wchar_t *function)
{
    DWORD error = GetLastError();
    LPWSTR pMessage = L"The function %1 failed with error %2.";
    LPWSTR pBuffer = netdata_claim_get_formatted_message(pMessage, function, error);

    if (pBuffer) {
        MessageBoxW(NULL, pBuffer, L"Error", MB_OK|MB_ICONERROR);
        LocalFree(pBuffer);
    }

    ExitProcess(error);
}

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
        if(wcscasecmp(L"/T", argv[i]) == 0) {
            if (argc <= i + 1)
                continue;
            i++;
            token = argv[i];
        }

        if(wcscasecmp(L"/R", argv[i]) == 0) {
            if (argc <= i + 1)
                continue;
            i++;
            room = argv[i];
        }

        if(wcscasecmp(L"/P", argv[i]) == 0) {
            if (argc <= i + 1)
                continue;
            i++;
            // Minimum IPV4
            if(wcslen(argv[i]) >= 8) {
                proxy = argv[i];
            }
        }

        if(wcscasecmp(L"/F", argv[i]) == 0) {
            if (argc <= i + 1)
                continue;
            i++;
            extPath = argv[i];
        }

        if(wcscasecmp(L"/U", argv[i]) == 0) {
            if (argc <= i + 1)
                continue;
            i++;
            url = argv[i];
        }

        if(wcscasecmp(L"/I", argv[i]) == 0) {
            if (argc <= i + 1)
                continue;

            i++;
            size_t length = wcslen(argv[i]);
            char *tmp = calloc(sizeof(char), length);
            if (!tmp)
                ExitProcess(1);

            netdata_claim_convert_str(tmp, argv[i], length - 1);
            if (i < argc)
                insecure = atoi(tmp);
            else
                insecure = 1;

            free(tmp);
        }
    }

    if (!token || !room)
        return 0;

    return argc;
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

    if (proxy) {
        length = wcslen(proxy) + 1;
        aProxy = calloc(sizeof(char), length - 1);
        if (!aProxy)
            return -1;

        netdata_claim_convert_str(aProxy, proxy, length - 1);
    }

    if (url) {
        length = wcslen(url) + 1;
        aURL = calloc(sizeof(char), length - 1);
        if (!aURL)
            return -1;

        netdata_claim_convert_str(aURL, url, length - 1);
    }
    return 0;
}

static void netdata_claim_exit_callback(int signal)
{
    (void)signal;
    if (aToken)
        free(aToken);

    if (aRoom)
        free(aRoom);

    if (aProxy)
        free(aProxy);

    if (aURL)
        free(aURL);

    if (argv)
        LocalFree(argv);

    if (extPath)
        LocalFree(extPath);
}

static inline int netdata_claim_prepare_data(char *out, size_t length)
{
    char *proxyLabel = (aProxy) ? "proxy = " : "#    proxy = ";
    char *proxyValue = (aProxy) ? aProxy : "";

    char *urlValue = (aURL) ? aURL : "https://app.netdata.cloud";
    return snprintf(out,
                    length,
                    "[global]\n    url = %s\n    token = %s\n    rooms = %s\n    %s%s\n    insecure = %s",
                    urlValue,
                    aToken,
                    aRoom,
                    proxyLabel,
                    proxyValue,
                    (insecure) ? "yes" : "no"
                    );
}

static int netdata_claim_get_path(char *path)
{
    if (extPath) {
        size_t length = wcslen(extPath) + 1;
	if (length >= WINDOWS_MAX_PATH) 
            return -1;

        netdata_claim_convert_str(path, extPath, length - 1);
	return 0;
    }

    char *usrPath = { "\\usr\\bin" };
    DWORD length = GetCurrentDirectoryA(WINDOWS_MAX_PATH, path);
    if (!length) {
        return -1;
    }

    if (strstr(path, usrPath)) {
        length -= 7;
        path[length] = '\0';
    }

    return 0;
}

static void netdata_claim_write_config(char *path)
{
#define NETDATA_MIN_CLOUD_LENGTH 135
#define NETDATA_MIN_ROOM_LENGTH 36
    if (strlen(aToken) != NETDATA_MIN_CLOUD_LENGTH || strlen(aRoom) < NETDATA_MIN_ROOM_LENGTH)
        return;

    char configPath[WINDOWS_MAX_PATH + 1];
    char data[WINDOWS_MAX_PATH + 1];
    char *filename;
    if (!extPath) {
        snprintf(configPath, WINDOWS_MAX_PATH - 1, "%s\\etc\\netdata\\claim.conf", path);
        filename = configPath;
    } else {
        filename = path;
    }

    HANDLE hf = CreateFileA(filename, GENERIC_WRITE, 0, NULL, CREATE_ALWAYS, FILE_ATTRIBUTE_NORMAL, NULL);
    if (hf == INVALID_HANDLE_VALUE)
        netdata_claim_error_exit(L"CreateFileA");

    DWORD length = netdata_claim_prepare_data(data, WINDOWS_MAX_PATH);
    DWORD written = 0;

    BOOL ret = WriteFile(hf, data, length, &written, NULL);
    if (!ret) {
        CloseHandle(hf);
        netdata_claim_error_exit(L"WriteFileA");
    }

    if (length != written)
        MessageBoxW(NULL, L"Cannot write claim.conf.", L"Error", MB_OK|MB_ICONERROR);

    CloseHandle(hf);
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
        if (netdata_claim_prepare_strings()) {
            goto exit_claim;
        }

        char basePath[WINDOWS_MAX_PATH];
        if (!netdata_claim_get_path(basePath)) {
            netdata_claim_write_config(basePath);
        }
    }

exit_claim:
    netdata_claim_exit_callback(0);

    return ret;
}
