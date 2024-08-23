// SPDX-License-Identifier: GPL-3.0-or-later

#include <windows.h>
#include <shellapi.h>
#include <wchar.h>

LPWSTR token = NULL;
LPWSTR room = NULL;

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

int WINAPI WinMain(HINSTANCE hInstance, HINSTANCE hPrevInstance, LPSTR lpCmdLine, int nCmdShow)
{
    int argc;
    // TODO: We are using ASCII here, but we should work with UTF on MS
    LPWSTR *argv = CommandLineToArgvW(GetCommandLineW(), &argc);
    if (argc)
        argc = nd_claim_parse_args(argc, argv);

    LocalFree(argv);

    return 0;
}

