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

static LPCTSTR szWindowClass = _T("DesktopApp");

HWND hNetdataWND = NULL;

static LRESULT CALLBACK NetdataCliProc(HWND hNetdatawnd, UINT message, WPARAM wParam, LPARAM lParam)
{
    HWND hwndReloadHealth, hwndReloadLabels, hwndSaveDatabase, hwndReopenLogs, hwndStopService, hwndOpenMsys,
        hwndExit;
    static HINSTANCE hShell32 = NULL;

    switch (message) {
        case WM_CREATE: {
            hwndReloadHealth = CreateWindowExW(0, L"BUTTON", L"Reload Health",
                                               WS_CHILD | WS_VISIBLE,
                                               20, 20, 120, 30,
                                               hNetdatawnd, (HMENU)IDC_RELOAD_HEALTH,
                                               NULL, NULL);

            hwndReloadLabels = CreateWindowExW(0, L"BUTTON", L"Reload Labels",
                                               WS_CHILD | WS_VISIBLE,
                                               20, 60, 120, 30,
                                               hNetdatawnd, (HMENU)IDC_RELOAD_LABELS,
                                               NULL, NULL);

            hwndReopenLogs = CreateWindowExW(0, L"BUTTON", L"Reopen Logs",
                                             WS_CHILD | WS_VISIBLE,
                                             20, 100, 120, 30,
                                             hNetdatawnd, (HMENU)IDC_SAVE_DATABASE,
                                             NULL, NULL);

            hwndSaveDatabase = CreateWindowExW(0, L"BUTTON", L"Save Database",
                                               WS_CHILD | WS_VISIBLE,
                                               280, 20, 120, 30,
                                               hNetdatawnd, (HMENU)IDC_SAVE_DATABASE,
                                               NULL, NULL);

            hwndStopService = CreateWindowExW(0, L"BUTTON", L"Stop Service",
                                              WS_CHILD | WS_VISIBLE,
                                              280, 60, 120, 30,
                                              hNetdatawnd, (HMENU)IDC_SAVE_DATABASE,
                                              NULL, NULL);

            hwndOpenMsys = CreateWindowExW(0, L"BUTTON", L"Open Msys2",
                                           WS_CHILD | WS_VISIBLE,
                                           280, 100, 120, 30,
                                           hNetdatawnd, (HMENU)IDC_SAVE_DATABASE,
                                           NULL, NULL);

            hwndExit = CreateWindowExW(0, L"BUTTON", L"Exit",
                                       WS_CHILD | WS_VISIBLE,
                                       140, 140, 120, 30,
                                       hNetdatawnd, (HMENU)IDC_CLOSE_WINDOW,
                                       NULL, NULL);
            break;
        }
        case WM_COMMAND: {
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
                                  440, 250,
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
