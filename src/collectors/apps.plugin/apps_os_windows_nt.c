// SPDX-License-Identifier: GPL-3.0-or-later

// this must not include libnetdata.h because STRING is defined in winternl.h

#include "libnetdata/common.h"

#if defined(OS_WINDOWS)
#include <winternl.h>

// --------------------------------------------------------------------------------------------------------------------
// Get the full windows command line

WCHAR* GetProcessCommandLine(HANDLE hProcess) {
    PROCESS_BASIC_INFORMATION pbi;
    ULONG len;
    NTSTATUS status = NtQueryInformationProcess(hProcess, 0, &pbi, sizeof(pbi), &len);
    if (status != 0)
        return NULL;

    // The rest of the function remains the same as before
    PEB peb;
    if (!ReadProcessMemory(hProcess, pbi.PebBaseAddress, &peb, sizeof(peb), NULL))
        return NULL;

    RTL_USER_PROCESS_PARAMETERS procParams;
    if (!ReadProcessMemory(hProcess, peb.ProcessParameters, &procParams, sizeof(procParams), NULL))
        return NULL;

    WCHAR* commandLine = (WCHAR*)malloc(procParams.CommandLine.MaximumLength);
    if (!commandLine)
        return NULL;

    if (!ReadProcessMemory(hProcess, procParams.CommandLine.Buffer, commandLine, procParams.CommandLine.MaximumLength, NULL)) {
        free(commandLine);
        return NULL;
    }

    return commandLine;
}

#endif
