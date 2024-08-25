// SPDX-License-Identifier: GPL-3.0-or-later

#define UNICODE
#define _UNICODE
#include <windows.h>
#include "richedit.h"
#include "tchar.h"
#include "netdata_claim.h"

static LPCTSTR szWindowClass = _T("DesktopApp");

static HINSTANCE hInst;
static HWND hToken;
static HWND hRoom;

LRESULT CALLBACK WndProc(HWND hNetdatawnd, UINT message, WPARAM wParam, LPARAM lParam)
{
    PAINTSTRUCT ps;
    HDC hdc;
    LPCTSTR topMsg[] = { _T("                                         Help"),
                         _T(" "),
                         _T("In this initial version of the software, there are no fields for data"),
                         _T(" entry. To claim your agent, you must use the following options:"),
                         _T(" "),
                         _T("/T TOKEN: The cloud token; "),
                         _T("/R ROOMS: A list of rooms to claim;")};

    switch (message)
    {
        case WM_PAINT: {
            hdc = BeginPaint(hNetdatawnd, &ps);

            int i;
            for (i = 0; i < sizeof(topMsg) / sizeof(LPCTSTR); i++) {
                TextOut(hdc, 5, 5 + 15*i, topMsg[i], wcslen(topMsg[i]));
            }
            EndPaint(hNetdatawnd, &ps);
            break;
        }
        case WM_COMMAND:
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

int netdata_claim_window_loop(HINSTANCE hInstance, int nCmdShow)
{
    WNDCLASSEX wcex;

    wcex.cbSize         = sizeof(WNDCLASSEX);
    wcex.style          = CS_HREDRAW | CS_VREDRAW;
    wcex.lpfnWndProc    = WndProc;
    wcex.cbClsExtra     = 0;
    wcex.cbWndExtra     = 0;
    wcex.hInstance      = hInstance;
    wcex.hIcon          = LoadIcon(wcex.hInstance, IDI_APPLICATION);
    wcex.hCursor        = LoadCursor(NULL, IDC_ARROW);
    wcex.hbrBackground  = (HBRUSH)(COLOR_WINDOW+1);
    wcex.lpszMenuName   = NULL;
    wcex.lpszClassName  = szWindowClass;
    wcex.hIconSm        = LoadIcon(wcex.hInstance, IDI_APPLICATION);

    if (!RegisterClassEx(&wcex)) {
        MessageBoxW(NULL, L"Call to RegisterClassEx failed!", L"Error", 0);
        return 1;
    }

    hInst = hInstance;

    HWND hNetdatawnd = CreateWindowExW(WS_EX_OVERLAPPEDWINDOW,
                                      szWindowClass,
                                      L"Netdata Claim",
                                      WS_OVERLAPPEDWINDOW,
                                      CW_USEDEFAULT, CW_USEDEFAULT,
                                      460, 180,
                                      NULL,
                                      NULL,
                                      hInstance,
                                      NULL
        );

    if (!hNetdatawnd) {
        MessageBoxW(NULL, L"Call to CreateWindow failed!", L"Error", 0);
        return 1;
    }

    ShowWindow(hNetdatawnd, nCmdShow);
    UpdateWindow(hNetdatawnd);

    MSG msg;
    while (GetMessage(&msg, NULL, 0, 0)) {
        TranslateMessage(&msg);
        DispatchMessage(&msg);
    }

    return (int) msg.wParam;
}
