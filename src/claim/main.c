// SPDX-License-Identifier: GPL-3.0-or-later

#define UNICODE
#define _UNICODE
#include <windows.h>
#include <shellapi.h>
#include <stdlib.h>
#include <stdio.h>
#include <string.h>
#include <ctype.h>
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

// The MSI runs us as a deferred, non-impersonated custom action: that lives in session 0, where a
// window or a message box is invisible to the user and never gets closed, hanging the installation.
// Default to the safe mode and only enable dialogs once we know no arguments were passed.
int nd_claim_interactive = 0;

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
void netdata_claim_error_exit(wchar_t *function, int code)
{
    DWORD error = GetLastError();

    if (nd_claim_interactive) {
        LPWSTR pMessage = L"The function %1 failed with error %2.";
        LPWSTR pBuffer = netdata_claim_get_formatted_message(pMessage, function, error);

        if (pBuffer) {
            MessageBoxW(NULL, pBuffer, L"Error", MB_OK|MB_ICONERROR);
            LocalFree(pBuffer);
        }
    }

    // Report through a stable exit code instead of the raw Win32 error: the installer logs the
    // custom action's return value, and Win32 codes overlap the classes callers need to tell apart.
    ExitProcess((UINT)code);
}

// Installer fields and command lines routinely carry pasted leading/trailing whitespace. Left in
// place it ends up inside claim.conf and Netdata Cloud rejects the credentials.
static void netdata_claim_trim(char *s)
{
    if (!s)
        return;

    char *start = s;
    while (*start && isspace((unsigned char)*start))
        start++;

    if (start != s)
        memmove(s, start, strlen(start) + 1);

    size_t len = strlen(s);
    while (len && isspace((unsigned char)s[len - 1]))
        s[--len] = '\0';
}

/**
 *  Parse Args
 *
 *  Parse the command line given by the installer or by a script.
 *
 * @param argc number of arguments
 * @param argv A pointer for all arguments given
 *
 * @return 1 when a usable configuration was parsed, 0 otherwise.
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
            proxy = argv[i];
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
            size_t length = wcslen(argv[i]) + 1;
            char *tmp = calloc(sizeof(char), length);
            if (!tmp)
                ExitProcess(ND_CLAIM_INTERNAL);

            netdata_claim_convert_str(tmp, argv[i], length);
            // An empty value is the installer's "unchecked" state and parses as disabled.
            insecure = atoi(tmp);

            free(tmp);
        }
    }

    // A token is the only mandatory field. Rooms are optional here exactly as they are for the
    // Linux claiming script and for NETDATA_CLAIM_ROOMS: the Agent then lands in the Space's
    // default room.
    if (!token)
        return 0;

    return 1;
}

static int netdata_claim_prepare_strings()
{
    if (!token)
        return -1;

    size_t length = wcslen(token) + 1;
    aToken = calloc(sizeof(char), length);
    if (!aToken)
        return -1;

    netdata_claim_convert_str(aToken, token, length);
    netdata_claim_trim(aToken);

    if (room) {
        length = wcslen(room) + 1;
        aRoom = calloc(sizeof(char), length);
        if (!aRoom)
            return -1;

        netdata_claim_convert_str(aRoom, room, length);
        netdata_claim_trim(aRoom);
    }

    if (proxy) {
        length = wcslen(proxy) + 1;
        aProxy = calloc(sizeof(char), length);
        if (!aProxy)
            return -1;

        netdata_claim_convert_str(aProxy, proxy, length);
        netdata_claim_trim(aProxy);
    }

    if (url) {
        length = wcslen(url) + 1;
        aURL = calloc(sizeof(char), length);
        if (!aURL)
            return -1;

        netdata_claim_convert_str(aURL, url, length);
        netdata_claim_trim(aURL);
    }
    return 0;
}

static void netdata_claim_exit_callback(int signal)
{
    if (aToken) {
        free(aToken);
        aToken = NULL;
    }

    if (aRoom) {
        free(aRoom);
        aRoom = NULL;
    }

    if (aProxy) {
        free(aProxy);
        aProxy = NULL;
    }

    if (aURL) {
        free(aURL);
        aURL = NULL;
    }

    if (argv) {
        LocalFree(argv);
        argv = NULL;
        token = NULL;
        room = NULL;
        proxy = NULL;
        url = NULL;
        extPath = NULL;
    }

    if (signal)
        ExitProcess((UINT)signal);
}

static inline int netdata_claim_prepare_data(char *out, size_t length)
{
    // Leave unset optional keys commented out. An empty "proxy" would override the "env" default the
    // Agent applies when the key is absent, and a commented "rooms" documents that the Space's
    // default room is used instead of looking like a lost value.
    char *roomsLabel = (aRoom && *aRoom) ? "rooms = " : "#    rooms = ";
    char *roomsValue = (aRoom && *aRoom) ? aRoom : "";

    char *proxyLabel = (aProxy && *aProxy) ? "proxy = " : "#    proxy = ";
    char *proxyValue = (aProxy && *aProxy) ? aProxy : "";

    char *urlValue = (aURL && *aURL) ? aURL : "https://app.netdata.cloud";
    return snprintf(out,
                    length,
                    "[global]\n    url = %s\n    token = %s\n    %s%s\n    %s%s\n    insecure = %s\n",
                    urlValue,
                    aToken,
                    roomsLabel,
                    roomsValue,
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

        netdata_claim_convert_str(path, extPath, length);
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

static int netdata_claim_write_config(char *path)
{
    // Only an empty token is rejected. Netdata Cloud owns the token format, so validating its length
    // here just means a token the installer already accepted disappears without a trace; letting the
    // Agent attempt the claim and log the rejection is diagnosable.
    if (!aToken || !*aToken)
        return ND_CLAIM_BAD_ARGS;

    char configPath[WINDOWS_MAX_PATH + 1];
    char data[WINDOWS_MAX_PATH + 1];
    char *filename;
    if (!extPath) {
        // Refuse a truncated path instead of creating a file under a shortened name that the Agent
        // would never read.
        int pathLength = snprintf(configPath, sizeof(configPath), "%s\\etc\\netdata\\claim.conf", path);
        if (pathLength < 0 || (size_t)pathLength >= sizeof(configPath))
            return ND_CLAIM_INTERNAL;

        filename = configPath;
    } else {
        filename = path;
    }

    int length = netdata_claim_prepare_data(data, WINDOWS_MAX_PATH);
    if (length < 0 || length >= WINDOWS_MAX_PATH)
        return ND_CLAIM_INTERNAL;

    HANDLE hf = CreateFileA(filename, GENERIC_WRITE, 0, NULL, CREATE_ALWAYS, FILE_ATTRIBUTE_NORMAL, NULL);
    if (hf == INVALID_HANDLE_VALUE)
        netdata_claim_error_exit(L"CreateFileA", ND_CLAIM_WRITE_FAILED);

    DWORD written = 0;
    BOOL ret = WriteFile(hf, data, (DWORD)length, &written, NULL);
    CloseHandle(hf);

    if (!ret || (DWORD)length != written)
        return ND_CLAIM_WRITE_FAILED;

    return ND_CLAIM_OK;
}

int WINAPI WinMain(HINSTANCE hInstance, HINSTANCE hPrevInstance, LPSTR lpCmdLine, int nCmdShow)
{
    signal(SIGABRT, netdata_claim_exit_callback);
    signal(SIGINT, netdata_claim_exit_callback);
    signal(SIGTERM, netdata_claim_exit_callback);

    int argc = 0;
    argv = CommandLineToArgvW(GetCommandLineW(), &argc);
    if (!argv)
        netdata_claim_error_exit(L"CommandLineToArgvW", ND_CLAIM_INTERNAL);

    // Any argument means the installer or a script started us. In that mode we must never create a
    // window and never block on a message box, so bad input is reported through the exit code only.
    nd_claim_interactive = (argc <= 1);

    int ret;
    if (nd_claim_interactive) {
        ret = netdata_claim_window_loop(hInstance, nCmdShow);
    } else if (!nd_claim_parse_args(argc, argv)) {
        ret = ND_CLAIM_BAD_ARGS;
    } else if (netdata_claim_prepare_strings()) {
        // The token is already known to be present, so the only failure left here is an allocation.
        ret = ND_CLAIM_INTERNAL;
    } else {
        char basePath[WINDOWS_MAX_PATH];
        ret = (netdata_claim_get_path(basePath)) ? ND_CLAIM_INTERNAL : netdata_claim_write_config(basePath);
    }

    netdata_claim_exit_callback(0);

    return ret;
}
