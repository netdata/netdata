// SPDX-License-Identifier: GPL-3.0-or-later

#ifndef UNICODE
#define UNICODE
#endif

#ifndef _UNICODE
#define _UNICODE
#endif

#include <windows.h>
#include <shellapi.h>
#include <stdlib.h>
#include <stdio.h>
#include <string.h>
#include <signal.h>

#include "tchar.h"
#include "ui.h"
#include "windows-services.h"

static LPCTSTR szWindowClass = _T("DesktopApp");

HWND hNetdataWND = NULL;

// TODO: Convert to thread
static inline void netdata_cli_run_specific_command(wchar_t *cmd, BOOL root)
{
    wchar_t localPath[MAX_PATH + 1] = { };
    DWORD length = GetCurrentDirectoryW(MAX_PATH, localPath);
    if (!length) {
        MessageBoxW(NULL, L"Cannot find binary.", L"Error", MB_OK|MB_ICONERROR);
        return;
    }
    wchar_t *hasUsr = wcsstr(localPath, L"\\usr\\bin");
    if (hasUsr && root) {
        // Remove usr\bin
        length -= 8;
    }

    if (!hasUsr && !root) {
        wcscpy(&localPath[length], L"\\usr\\bin");
        length += 8;
    }
    wcscpy(&localPath[length], cmd);

    STARTUPINFO si;
    PROCESS_INFORMATION pi;

    ZeroMemory( &si, sizeof(si) );
    si.cb = sizeof(si);
    si.dwFlags = STARTF_USESHOWWINDOW;
    si.wShowWindow = SW_MAXIMIZE;
    ZeroMemory( &pi, sizeof(pi) );

    if(!CreateProcess(NULL, localPath, NULL, NULL, FALSE, 0, NULL, NULL, &si, &pi )) {
        MessageBoxW(NULL, L"Cannot start process.", L"Error", MB_OK|MB_ICONERROR);
        return;
    }
}

static inline void netdata_cli_open_dashboard()
{
    (void)ShellExecuteW(NULL, NULL, L"http://localhost:19999", NULL, NULL, SW_SHOW);
}

static inline void netdata_cli_check_service_status(wchar_t *status, size_t length)
{
    SC_HANDLE sm = GetWServiceMagagerHandle(SC_MANAGER_ALL_ACCESS);
    if (!sm) {
        swprintf(status, length, L"Cannot open Service Management!");
        return;
    }

    SC_HANDLE service = GetWServiceHandle(sm, L"Netdata", SERVICE_ALL_ACCESS);
    if (!service) {
        swprintf(status, length, L"Cannot find the service Netdata!");
        CloseServiceHandle(sm);
        return;
    }

    int isrunning = IsServiceRunning(sm, service);
    swprintf(status, length, L"Netdata service is %s!", (isrunning)? "running" : "stopped");

    CloseServiceHandle(sm);
    CloseServiceHandle(service);
}

static LRESULT CALLBACK NetdataCliProc(HWND hNetdatawnd, UINT message, WPARAM wParam, LPARAM lParam)
{
    HWND hwndReloadHealth, hwndReloadLabels, hwndSaveDatabase, hwndReopenLogs, hwndStopService, hwndOpenMsys,
        hwndExit;
    static HINSTANCE hShell32 = NULL;

    switch (message) {
        case WM_CREATE: {
            hwndReloadHealth = CreateWindowExW(0, L"BUTTON", L"Open Dashboard",
                                               WS_CHILD | WS_VISIBLE,
                                               20, 20, 120, 30,
                                               hNetdatawnd, (HMENU)IDC_NETDATA_DASHBOARD,
                                               NULL, NULL);

            hwndReloadLabels = CreateWindowExW(0, L"BUTTON", L"Run edit-config",
                                               WS_CHILD | WS_VISIBLE,
                                               280, 20, 120, 30,
                                               hNetdatawnd, (HMENU)IDC_EDIT_CONFIG,
                                               NULL, NULL);

            hwndStopService = CreateWindowExW(0, L"BUTTON", L"Check Update!",
                                              WS_CHILD | WS_VISIBLE,
                                              280, 60, 120, 30,
                                              hNetdatawnd, (HMENU)IDC_CHECK_UPDATE,
                                              NULL, NULL);

            hwndOpenMsys = CreateWindowExW(0, L"BUTTON", L"Open terminal",
                                           WS_CHILD | WS_VISIBLE,
                                           20, 60, 120, 30,
                                           hNetdatawnd, (HMENU)IDC_OPEN_MSYS,
                                           NULL, NULL);

            hwndExit = CreateWindowExW(0, L"BUTTON", L"Exit",
                                       WS_CHILD | WS_VISIBLE,
                                       140, 100, 120, 30,
                                       hNetdatawnd, (HMENU)IDC_CLOSE_WINDOW,
                                       NULL, NULL);
            break;
        }
        case WM_PAINT: {
            LPCTSTR screenMessages[] = {
              L"Netdata",
              L"Client"
            };

            wchar_t statusMsg[64];
            netdata_cli_check_service_status(statusMsg, 63);

            PAINTSTRUCT ps;
            HDC hdc = BeginPaint(hNetdatawnd, &ps);;
            int i;
            for (i = 0; i < sizeof(screenMessages) / sizeof(LPCTSTR); i++) {
                TextOut(hdc, 180, 40 + 20 * i, screenMessages[i], wcslen(screenMessages[i]));
            }
            TextOut(hdc, 20, 150, statusMsg, wcslen(statusMsg));
            EndPaint(hNetdatawnd, &ps);

            break;
        }
        case WM_COMMAND: {
            if (HIWORD(wParam) == BN_CLICKED) {
                switch(LOWORD(wParam)) {
                    case IDC_OPEN_MSYS: {
                        netdata_cli_run_specific_command(L"\\msys2.exe", TRUE);
                        break;
                    }
                    case IDC_NETDATA_DASHBOARD: {
                        netdata_cli_open_dashboard();
                        break;
                    }
                    case IDC_CHECK_UPDATE: {
                        MessageBoxW(NULL, L"This option is a working in progress.", L"Info", MB_OK|MB_ICONINFORMATION);
                        break;
                    }
                    case IDC_EDIT_CONFIG: {
                        netdata_cli_run_specific_command(L"\\bash.exe -l -c \"cd /etc/netdata; ./edit-config; $SHELL\"",
                                                         FALSE);
                        break;
                    }
                    case IDC_CLOSE_WINDOW: {
                        ExitProcess(0);
                    }
                }
            }
            break;
        }
        case WM_DRAWITEM: {
            break;
        }
        case WM_DESTROY: {
            PostQuitMessage(0);
            break;
        }
        default: {
            return DefWindowProc(hNetdatawnd, message, wParam, lParam);
            break;
        }
    }
    return 0;
}

static void netdata_cli_exit_callback(int signal)
{
    (void)signal;
}

int WINAPI WinMain(HINSTANCE hInstance, HINSTANCE hPrevInstance, LPSTR lpCmdLine, int nCmdShow)
{
    signal(SIGABRT, netdata_cli_exit_callback);
    signal(SIGINT, netdata_cli_exit_callback);
    signal(SIGTERM, netdata_cli_exit_callback);

    WNDCLASSEX wcex;

    wcex.cbSize         = sizeof(WNDCLASSEX);
    wcex.style          = CS_HREDRAW | CS_VREDRAW;
    wcex.lpfnWndProc    = NetdataCliProc;
    wcex.cbClsExtra     = 0;
    wcex.cbWndExtra     = 0;
    wcex.hInstance      = hInstance;
    wcex.hIcon          = LoadIcon(wcex.hInstance, MAKEINTRESOURCEW(11));
    wcex.hCursor        = LoadCursor(NULL, IDC_ARROW);
    wcex.hbrBackground  = (HBRUSH)(COLOR_WINDOW+1);
    wcex.lpszMenuName   = NULL;
    wcex.lpszClassName  = szWindowClass;
    wcex.hIconSm        = LoadIcon(wcex.hInstance, IDI_APPLICATION);

    if (!RegisterClassEx(&wcex)) {
        MessageBoxW(NULL, L"Call to RegisterClassEx failed!", L"Error", 0);
        return (int)GetLastError();
    }

    hNetdataWND = CreateWindowExW(WS_EX_OVERLAPPEDWINDOW,
                                  szWindowClass,
                                  L"Netdata Client",
                                  WS_OVERLAPPEDWINDOW,
                                  CW_USEDEFAULT, CW_USEDEFAULT,
                                  440, 230,
                                  HWND_DESKTOP,
                                  NULL,
                                  hInstance,
                                  NULL
                                  );

    if (!hNetdataWND) {
        MessageBoxW(NULL, L"Call to CreateWindow failed!", L"Error", 0);
        return (int)GetLastError();
    }

    ShowWindow(hNetdataWND, nCmdShow);
    UpdateWindow(hNetdataWND);

    MSG msg;
    while (GetMessage(&msg, NULL, 0, 0)) {
        TranslateMessage(&msg);
        DispatchMessage(&msg);
    }

    return (int) msg.wParam;
}
