// SPDX-License-Identifier: GPL-3.0-or-later

#ifndef NETDATA_CLAIM_WINDOW_H_
# define NETDATA_CLAIM_WINDOW_H_ 1

// https://learn.microsoft.com/en-us/troubleshoot/windows-client/shell-experience/command-line-string-limitation
// https://sourceforge.net/p/mingw/mailman/mingw-users/thread/4C8FD4EB.4050503@xs4all.nl/
#define WINDOWS_MAX_PATH 8191

int netdata_claim_window_loop(HINSTANCE hInstance, int nCmdShow);

#endif //NETDATA_CLAIM_WINDOW_H_
