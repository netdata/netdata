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

static inline HWND CreateRichEdit(HWND hwnd, int x, int y, int width, int height) // Dimensions.
{
    HWND ptr = CreateWindowExW(0, MSFTEDIT_CLASS, L"Type here",
                                  WS_VISIBLE | WS_CHILD | WS_BORDER | WS_TABSTOP,
                                  x, y, width, height,
                                  hwnd, NULL, hInst, NULL);
    if (!ptr) {
        MessageBoxW(NULL, L"Cannot create a Rich Text!", L"Error", 0);
        return NULL;
    }

    return ptr;
}

LRESULT CALLBACK WndProc(HWND hNetdatawnd, UINT message, WPARAM wParam, LPARAM lParam)
{
    PAINTSTRUCT ps;
    HDC hdc;
    LPCTSTR topMsg =  _T("Claim your Agent for a specific room.");

    switch (message)
    {
        case WM_CREATE: {
            hToken = CreateRichEdit(hNetdatawnd, 30, 20, 400, 40);
            if (!hToken) {
                return 1;
            }
            SendMessage(hToken, EM_EXLIMITTEXT, 0, 125);
            hRoom = CreateRichEdit(hNetdatawnd, 30, 40, 400, 40);
            if (!hToken) {
                return 1;
            }
            SendMessage(hRoom, EM_EXLIMITTEXT, 0, 36);
            break;
        }
        case WM_PAINT: {
            hdc = BeginPaint(hNetdatawnd, &ps);

            TextOut(hdc, 5, 5, topMsg, wcslen(topMsg));

            TextOut(hdc, 5, 20, L"Token", 5);

            TextOut(hdc, 5, 40, L"Room", 4);

            EndPaint(hNetdatawnd, &ps);
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
                                      500, 500,
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
