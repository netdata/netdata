// SPDX-License-Identifier: GPL-3.0-or-later

#define UNICODE
#define _UNICODE
#include <windows.h>
#include <shellapi.h>
#include <stdlib.h>
#include <stdio.h>
#include <string.h>
#include <signal.h>

static void netdata_claim_exit_callback(int signal)
{
    (void)signal;
}

int WINAPI WinMain(HINSTANCE hInstance, HINSTANCE hPrevInstance, LPSTR lpCmdLine, int nCmdShow)
{
    signal(SIGABRT, netdata_claim_exit_callback);
    signal(SIGINT, netdata_claim_exit_callback);
    signal(SIGTERM, netdata_claim_exit_callback);

    return 0;
}
