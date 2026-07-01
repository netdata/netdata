// SPDX-License-Identifier: GPL-3.0-or-later

#include "nd_log-internals.h"

#if defined(OS_WINDOWS) && (defined(HAVE_ETW) || defined(HAVE_WEL))

// --------------------------------------------------------------------------------------------------------------------
// construct an event id

// load message resources generated header
#include "wevt_netdata.h"

// include the common definitions with the message resources and manifest generator
#include "nd_log-to-windows-common.h"

#if defined(HAVE_ETW)
// we need the manifest, only in ETW mode

// eliminate compiler warnings and load manifest generated header
#undef EXTERN_C
#define EXTERN_C
#undef __declspec
#define __declspec(x)
#include "wevt_netdata_manifest.h"

static REGHANDLE regHandle;
#endif

// Function to construct EventID
static DWORD complete_event_id(DWORD facility, DWORD severity, DWORD event_code) {
    DWORD event_id = 0;

    // Set Severity
    event_id |= ((DWORD)(severity) << EVENT_ID_SEV_SHIFT) & EVENT_ID_SEV_MASK;

    // Set Customer Code Flag (C)
    event_id |= (0x0 << EVENT_ID_C_SHIFT) & EVENT_ID_C_MASK;

    // Set Reserved Bit (R) - typically 0
    event_id |= (0x0 << EVENT_ID_R_SHIFT) & EVENT_ID_R_MASK;

    // Set Facility
    event_id |= ((DWORD)(facility) << EVENT_ID_FACILITY_SHIFT) & EVENT_ID_FACILITY_MASK;

    // Set Code
    event_id |= ((DWORD)(event_code) << EVENT_ID_CODE_SHIFT) & EVENT_ID_CODE_MASK;

    return event_id;
}

DWORD construct_event_id(ND_LOG_SOURCES source, ND_LOG_FIELD_PRIORITY priority, MESSAGE_ID messageID) {
    DWORD event_code = construct_event_code(source, priority, messageID);
    return complete_event_id(FACILITY_NETDATA, get_severity_from_priority(priority), event_code);
}

static bool check_event_id(ND_LOG_SOURCES source __maybe_unused, ND_LOG_FIELD_PRIORITY priority __maybe_unused, MESSAGE_ID messageID __maybe_unused, DWORD event_code __maybe_unused) {
#ifdef NETDATA_INTERNAL_CHECKS
    DWORD generated = construct_event_id(source, priority, messageID);
    if(generated != event_code) {

        // this is just used for a break point, to see the values in hex
        char current[UINT64_HEX_MAX_LENGTH];
        print_uint64_hex(current, generated);

        char wanted[UINT64_HEX_MAX_LENGTH];
        print_uint64_hex(wanted, event_code);

        const char *got = current;
        const char *good = wanted;
        internal_fatal(true, "EventIDs mismatch, expected %s, got %s", good, got);
    }
#endif

    return true;
}

// --------------------------------------------------------------------------------------------------------------------
// initialization

// Define provider (source) names per source (only when not using ETW)
static const wchar_t *wel_provider_per_source[_NDLS_MAX] = {
        [NDLS_UNSET]        = NULL,                              // not used, linked to NDLS_DAEMON
        [NDLS_ACCESS]       = NETDATA_WEL_PROVIDER_ACCESS_W,
        [NDLS_ACLK]         = NETDATA_WEL_PROVIDER_ACLK_W,
        [NDLS_COLLECTORS]   = NETDATA_WEL_PROVIDER_COLLECTORS_W,
        [NDLS_DAEMON]       = NETDATA_WEL_PROVIDER_DAEMON_W,
        [NDLS_HEALTH]       = NETDATA_WEL_PROVIDER_HEALTH_W,
        [NDLS_DEBUG]        = NULL,                              // used, linked to NDLS_DAEMON
};

// Classic WEL channel names per source — match ETW channel names so events appear in the same channels
static const wchar_t *wel_channel_per_source[_NDLS_MAX] = {
        [NDLS_UNSET]        = NULL,                              // linked to NDLS_DAEMON
        [NDLS_ACCESS]       = NETDATA_WEL_CHANNEL_ACCESS_W,
        [NDLS_ACLK]         = NETDATA_WEL_CHANNEL_ACLK_W,
        [NDLS_COLLECTORS]   = NETDATA_WEL_CHANNEL_COLLECTORS_W,
        [NDLS_DAEMON]       = NETDATA_WEL_CHANNEL_DAEMON_W,
        [NDLS_HEALTH]       = NETDATA_WEL_CHANNEL_HEALTH_W,
        [NDLS_DEBUG]        = NULL,                              // linked to NDLS_DAEMON
};

bool wel_replace_program_with_wevt_netdata_dll(wchar_t *str, size_t size) {
    const wchar_t *replacement = L"\\wevt_netdata.dll";

    // Find the last occurrence of '\\' to isolate the filename
    wchar_t *lastBackslash = wcsrchr(str, L'\\');

    if (lastBackslash != NULL) {
        // Calculate new length after replacement
        size_t newLen = (lastBackslash - str) + wcslen(replacement);

        // Ensure new length does not exceed buffer size
        if (newLen >= size)
            return false; // Not enough space in the buffer

        // Terminate the string at the last backslash
        *lastBackslash = L'\0';

        // Append the replacement filename
        wcsncat(str, replacement, size - wcslen(str) - 1);

        // Check if the new file exists
        if (GetFileAttributesW(str) != INVALID_FILE_ATTRIBUTES)
            return true; // The file exists
        else
            return false; // The file does not exist
    }

    return false; // No backslash found (likely invalid input)
}

static bool wel_add_to_registry(const wchar_t *channel, const wchar_t *provider, DWORD defaultMaxSize) {
    // Build the registry path: SYSTEM\CurrentControlSet\Services\EventLog\<LogName>\<SourceName>
    wchar_t key[MAX_PATH];
    if(!provider)
        swprintf(key, MAX_PATH, L"SYSTEM\\CurrentControlSet\\Services\\EventLog\\%ls", channel);
    else
        swprintf(key, MAX_PATH, L"SYSTEM\\CurrentControlSet\\Services\\EventLog\\%ls\\%ls", channel, provider);

    HKEY hRegKey;
    DWORD disposition;
    LONG result = RegCreateKeyExW(HKEY_LOCAL_MACHINE, key,
                                  0, NULL, REG_OPTION_NON_VOLATILE, KEY_SET_VALUE, NULL, &hRegKey, &disposition);

    if (result != ERROR_SUCCESS)
        return false; // Could not create the registry key

    // Check if MaxSize is already set
    DWORD maxSize = 0;
    DWORD size = sizeof(maxSize);
    if (RegQueryValueExW(hRegKey, L"MaxSize", NULL, NULL, (LPBYTE)&maxSize, &size) != ERROR_SUCCESS) {
        // MaxSize is not set, set it to the default value
        RegSetValueExW(hRegKey, L"MaxSize", 0, REG_DWORD, (const BYTE*)&defaultMaxSize, sizeof(defaultMaxSize));
    }

    wchar_t modulePath[MAX_PATH];
    if (GetModuleFileNameW(NULL, modulePath, MAX_PATH) == 0) {
        RegCloseKey(hRegKey);
        return false;
    }

    if(wel_replace_program_with_wevt_netdata_dll(modulePath, _countof(modulePath))) {
        RegSetValueExW(hRegKey, L"EventMessageFile", 0, REG_EXPAND_SZ,
                       (LPBYTE)modulePath, (wcslen(modulePath) + 1) * sizeof(wchar_t));

        DWORD types_supported = EVENTLOG_SUCCESS | EVENTLOG_ERROR_TYPE | EVENTLOG_WARNING_TYPE | EVENTLOG_INFORMATION_TYPE;
        RegSetValueExW(hRegKey, L"TypesSupported", 0, REG_DWORD, (LPBYTE)&types_supported, sizeof(DWORD));
    }

    RegCloseKey(hRegKey);
    return true;
}

#if !defined(HAVE_ETW)
// Registry key where WINEVT stores registered publishers (providers).
#define WINEVT_PUBLISHERS_KEY L"SOFTWARE\\Microsoft\\Windows\\CurrentVersion\\WINEVT\\Publishers\\"
// GUID of the NetdataDaemon importChannel provider — only present when the current
// manifest (the one with importChannel declarations) has been registered via wevtutil im.
#define WEL_DAEMON_IMPORT_GUID L"{5CA72004-9BD8-4634-81E5-000014E7DAAD}"

static bool wel_manifest_is_current(void) {
    wchar_t key[MAX_PATH];
    swprintf(key, _countof(key), L"%ls%ls", WINEVT_PUBLISHERS_KEY, WEL_DAEMON_IMPORT_GUID);
    HKEY hKey;
    if (RegOpenKeyExW(HKEY_LOCAL_MACHINE, key, 0, KEY_READ, &hKey) == ERROR_SUCCESS) {
        RegCloseKey(hKey);
        return true;
    }
    return false;
}

static bool wel_run_silent(const wchar_t *exe, const wchar_t *params) {
    wchar_t cmdline[MAX_PATH * 6];
    swprintf(cmdline, _countof(cmdline), L"\"%ls\" %ls", exe, params);

    STARTUPINFOW si = { .cb = sizeof(si) };
    PROCESS_INFORMATION pi = {0};
    if (!CreateProcessW(NULL, cmdline, NULL, NULL, FALSE,
                        CREATE_NO_WINDOW | CREATE_NEW_PROCESS_GROUP,
                        NULL, NULL, &si, &pi))
        return false;

    WaitForSingleObject(pi.hProcess, 30000);
    DWORD exitCode = 1;
    GetExitCodeProcess(pi.hProcess, &exitCode);
    CloseHandle(pi.hProcess);
    CloseHandle(pi.hThread);
    return exitCode == 0;
}

// Minimal manifest embedded in the binary so we can register the Netdata WINEVT
// channels and importChannel links at runtime, regardless of how netdata.exe was
// deployed (MSI, direct copy, dev build). Each %s is replaced with the System32
// DLL path (12 substitutions: 2 per provider × 6 providers).
// The template omits event/template definitions — wevtutil im only needs channels
// and providers to set up routing; the DLL handles message formatting at query time.
#define WEL_MANIFEST_XML_FMT \
    "<?xml version=\"1.0\" encoding=\"UTF-8\"?>\r\n" \
    "<instrumentationManifest xmlns=\"http://schemas.microsoft.com/win/2004/08/events\">\r\n" \
    "  <instrumentation\r\n" \
    "    xmlns:win=\"http://manifests.microsoft.com/win/2004/08/windows/events\"\r\n" \
    "    xmlns:xs=\"http://www.w3.org/2001/XMLSchema\"\r\n" \
    "    xmlns:xsi=\"http://www.w3.org/2001/XMLSchema-instance\">\r\n" \
    "    <events xmlns=\"http://schemas.microsoft.com/win/2004/08/events\">\r\n" \
    "      <provider name=\"Netdata\"\r\n" \
    "        guid=\"{96c5ca72-9bd8-4634-81e5-000014e7da7a}\"\r\n" \
    "        resourceFileName=\"%s\"\r\n" \
    "        messageFileName=\"%s\"\r\n" \
    "        symbol=\"NETDATA_ETW_PROVIDER\">\r\n" \
    "        <channels>\r\n" \
    "          <channel chid=\"CHANNEL_DAEMON\" name=\"Netdata/Daemon\" symbol=\"CHANNEL_DAEMON\" type=\"Operational\" enabled=\"true\"/>\r\n" \
    "          <channel chid=\"CHANNEL_COLLECTORS\" name=\"Netdata/Collectors\" symbol=\"CHANNEL_COLLECTORS\" type=\"Operational\" enabled=\"true\"/>\r\n" \
    "          <channel chid=\"CHANNEL_ACCESS\" name=\"Netdata/Access\" symbol=\"CHANNEL_ACCESS\" type=\"Operational\" enabled=\"true\"/>\r\n" \
    "          <channel chid=\"CHANNEL_HEALTH\" name=\"Netdata/Health\" symbol=\"CHANNEL_HEALTH\" type=\"Operational\" enabled=\"true\"/>\r\n" \
    "          <channel chid=\"CHANNEL_ACLK\" name=\"Netdata/Aclk\" symbol=\"CHANNEL_ACLK\" type=\"Operational\" enabled=\"true\"/>\r\n" \
    "        </channels>\r\n" \
    "      </provider>\r\n" \
    "      <provider name=\"NetdataDaemon\"\r\n" \
    "        guid=\"{5CA72004-9BD8-4634-81E5-000014E7DAAD}\"\r\n" \
    "        resourceFileName=\"%s\" messageFileName=\"%s\">\r\n" \
    "        <channels><importChannel chid=\"IMPORT_DAEMON\" name=\"Netdata/Daemon\"/></channels>\r\n" \
    "      </provider>\r\n" \
    "      <provider name=\"NetdataCollectors\"\r\n" \
    "        guid=\"{5CA72003-9BD8-4634-81E5-000014E7DAAC}\"\r\n" \
    "        resourceFileName=\"%s\" messageFileName=\"%s\">\r\n" \
    "        <channels><importChannel chid=\"IMPORT_COLLECTORS\" name=\"Netdata/Collectors\"/></channels>\r\n" \
    "      </provider>\r\n" \
    "      <provider name=\"NetdataAccess\"\r\n" \
    "        guid=\"{5CA72002-9BD8-4634-81E5-000014E7DAAB}\"\r\n" \
    "        resourceFileName=\"%s\" messageFileName=\"%s\">\r\n" \
    "        <channels><importChannel chid=\"IMPORT_ACCESS\" name=\"Netdata/Access\"/></channels>\r\n" \
    "      </provider>\r\n" \
    "      <provider name=\"NetdataHealth\"\r\n" \
    "        guid=\"{5CA72005-9BD8-4634-81E5-000014E7DAAA}\"\r\n" \
    "        resourceFileName=\"%s\" messageFileName=\"%s\">\r\n" \
    "        <channels><importChannel chid=\"IMPORT_HEALTH\" name=\"Netdata/Health\"/></channels>\r\n" \
    "      </provider>\r\n" \
    "      <provider name=\"NetdataAclk\"\r\n" \
    "        guid=\"{5CA72001-9BD8-4634-81E5-000014E7DAA9}\"\r\n" \
    "        resourceFileName=\"%s\" messageFileName=\"%s\">\r\n" \
    "        <channels><importChannel chid=\"IMPORT_ACLK\" name=\"Netdata/Aclk\"/></channels>\r\n" \
    "      </provider>\r\n" \
    "    </events>\r\n" \
    "  </instrumentation>\r\n" \
    "  <localization>\r\n" \
    "    <resources culture=\"en-US\">\r\n" \
    "      <stringTable>\r\n" \
    "        <string id=\"ND_PROVIDER_NAME\" value=\"Netdata\"/>\r\n" \
    "        <string id=\"Channel.Daemon\" value=\"Daemon\"/>\r\n" \
    "        <string id=\"Channel.Collectors\" value=\"Collectors\"/>\r\n" \
    "        <string id=\"Channel.Access\" value=\"Access\"/>\r\n" \
    "        <string id=\"Channel.Health\" value=\"Health\"/>\r\n" \
    "        <string id=\"Channel.Aclk\" value=\"Aclk\"/>\r\n" \
    "      </stringTable>\r\n" \
    "    </resources>\r\n" \
    "  </localization>\r\n" \
    "</instrumentationManifest>\r\n"

// Runs at WEL startup when wel_manifest_is_current() returns false.
// Works for every deployment: MSI install (DLL in System32), dev build (DLL next to
// netdata.exe), or a bare netdata.exe copy (DLL in System32 from a prior install).
// Generates the manifest XML from the embedded template and writes it to System32,
// then registers it via wevtutil. No external manifest file is required.
static void wel_ensure_manifest_installed(void) {
    if (wel_manifest_is_current()) {
        nd_win_trace("wel_ensure_manifest_installed: importChannel providers already registered");
        return;
    }

    nd_win_trace("wel_ensure_manifest_installed: installing manifest...");

    wchar_t system32[MAX_PATH];
    if (!GetSystemDirectoryW(system32, _countof(system32))) return;

    wchar_t dllDst[MAX_PATH];
    swprintf(dllDst, _countof(dllDst), L"%ls\\wevt_netdata.dll", system32);

    // If wevt_netdata.dll exists next to netdata.exe (dev build / manual deploy),
    // copy it to System32 so Event Viewer can resolve message strings.
    {
        wchar_t dllSrc[MAX_PATH];
        if (GetModuleFileNameW(NULL, dllSrc, _countof(dllSrc)) &&
            wel_replace_program_with_wevt_netdata_dll(dllSrc, _countof(dllSrc))) {
            if (!CopyFileW(dllSrc, dllDst, FALSE))
                nd_win_trace("wel_ensure_manifest_installed: CopyFile err=%lu (may already exist)",
                             (unsigned long)GetLastError());
            else
                nd_win_trace("wel_ensure_manifest_installed: copied DLL to System32");
        }
    }

    // DLL must exist in System32 (either we just copied it or MSI put it there).
    if (GetFileAttributesW(dllDst) == INVALID_FILE_ATTRIBUTES) {
        nd_win_trace("wel_ensure_manifest_installed: wevt_netdata.dll not in System32 err=%lu, skipping",
                     (unsigned long)GetLastError());
        return;
    }

    // Grant EventLog service read access to the DLL (required for manifest channels).
    wchar_t icaclsPath[MAX_PATH];
    swprintf(icaclsPath, _countof(icaclsPath), L"%ls\\icacls.exe", system32);
    wchar_t icaclsParams[MAX_PATH * 2];
    swprintf(icaclsParams, _countof(icaclsParams), L"\"%ls\" /grant \"NT SERVICE\\EventLog\":R", dllDst);
    if (!wel_run_silent(icaclsPath, icaclsParams))
        nd_win_trace("wel_ensure_manifest_installed: icacls err=%lu", (unsigned long)GetLastError());

    // Build manifest content from the embedded template. The manifest is generated
    // at runtime so it always matches this binary's channel layout, regardless of
    // what manifest file (if any) is on disk.
    char dllDstNarrow[MAX_PATH];
    WideCharToMultiByte(CP_UTF8, 0, dllDst, -1, dllDstNarrow, _countof(dllDstNarrow), NULL, NULL);

    char manifest[8192];
    int mlen = snprintf(manifest, sizeof(manifest), WEL_MANIFEST_XML_FMT,
                        dllDstNarrow, dllDstNarrow,   // Netdata provider
                        dllDstNarrow, dllDstNarrow,   // NetdataDaemon
                        dllDstNarrow, dllDstNarrow,   // NetdataCollectors
                        dllDstNarrow, dllDstNarrow,   // NetdataAccess
                        dllDstNarrow, dllDstNarrow,   // NetdataHealth
                        dllDstNarrow, dllDstNarrow);  // NetdataAclk
    if (mlen <= 0 || mlen >= (int)sizeof(manifest)) {
        nd_win_trace("wel_ensure_manifest_installed: manifest buffer overflow");
        return;
    }

    // Write manifest to System32 (same location as the MSI installer uses).
    wchar_t manifestDst[MAX_PATH];
    swprintf(manifestDst, _countof(manifestDst), L"%ls\\wevt_netdata_manifest.xml", system32);

    HANDLE hFile = CreateFileW(manifestDst, GENERIC_WRITE, 0, NULL, CREATE_ALWAYS,
                               FILE_ATTRIBUTE_NORMAL, NULL);
    if (hFile == INVALID_HANDLE_VALUE) {
        nd_win_trace("wel_ensure_manifest_installed: cannot create manifest err=%lu",
                     (unsigned long)GetLastError());
        return;
    }
    DWORD written;
    BOOL fileOk = WriteFile(hFile, manifest, (DWORD)mlen, &written, NULL);
    CloseHandle(hFile);
    if (!fileOk || written != (DWORD)mlen) {
        nd_win_trace("wel_ensure_manifest_installed: write failed err=%lu", (unsigned long)GetLastError());
        return;
    }

    wchar_t wevtutil[MAX_PATH];
    swprintf(wevtutil, _countof(wevtutil), L"%ls\\wevtutil.exe", system32);

    nd_win_trace("wel_ensure_manifest_installed: manifest written to '%ls'", manifestDst);

    // Unregister any previously registered manifest (ignore errors — normal on first run).
    wchar_t umParams[MAX_PATH * 2];
    swprintf(umParams, _countof(umParams), L"um \"%ls\"", manifestDst);
    if (wel_run_silent(wevtutil, umParams))
        nd_win_trace("wel_ensure_manifest_installed: wevtutil um ok");
    else
        nd_win_trace("wel_ensure_manifest_installed: wevtutil um failed/skipped (ok if not previously installed)");

    // Register the new manifest. The /mf and /rf flags tell wevtutil where the DLL is,
    // overriding the resourceFileName/messageFileName attributes in the XML.
    wchar_t imParams[MAX_PATH * 4];
    swprintf(imParams, _countof(imParams), L"im \"%ls\" \"/mf:%ls\" \"/rf:%ls\"",
             manifestDst, dllDst, dllDst);
    if (wel_run_silent(wevtutil, imParams))
        nd_win_trace("wel_ensure_manifest_installed: manifest installed successfully");
    else
        nd_win_trace("wel_ensure_manifest_installed: wevtutil im failed err=%lu",
                     (unsigned long)GetLastError());
}
#endif // !HAVE_ETW

#if defined(HAVE_ETW)
static void etw_set_source_meta(struct nd_log_source *source, USHORT channelID, const EVENT_DESCRIPTOR *ed) {
    // It turns out that the keyword varies per only per channel!
    // so, to log with the right keyword, Task, Opcode we copy the ids from the header
    // the messages compiler (mc.exe) generated from the manifest.

    source->channelID = channelID;
    source->Opcode = ed->Opcode;
    source->Task = ed->Task;
    source->Keyword = ed->Keyword;
}

// Callback for provider enable/disable notifications
static void NTAPI ProviderEnableCallback(
        LPCGUID SourceId __maybe_unused,
        ULONG IsEnabled,
        UCHAR Level __maybe_unused,
        ULONGLONG MatchAnyKeyword __maybe_unused,
        ULONGLONG MatchAllKeyword __maybe_unused,
        PEVENT_FILTER_DESCRIPTOR FilterData __maybe_unused,
        PVOID CallbackContext __maybe_unused
) {
    spinlock_lock(&nd_log.eventlog.provider_lock);
    nd_log.eventlog.provider_enabled = IsEnabled ? true : false;
    spinlock_unlock(&nd_log.eventlog.provider_lock);
}

static bool etw_register_provider(void) {
    spinlock_init(&nd_log.eventlog.provider_lock);
    nd_log.eventlog.provider_enabled = false;

    // Register the ETW provider
    if (EventRegister(&NETDATA_ETW_PROVIDER_GUID, ProviderEnableCallback, NULL, &regHandle) != ERROR_SUCCESS)
        return false;

    etw_set_source_meta(&nd_log.sources[NDLS_DAEMON], CHANNEL_DAEMON, &ED_DAEMON_INFO_MESSAGE_ONLY);
    etw_set_source_meta(&nd_log.sources[NDLS_COLLECTORS], CHANNEL_COLLECTORS, &ED_COLLECTORS_INFO_MESSAGE_ONLY);
    etw_set_source_meta(&nd_log.sources[NDLS_ACCESS], CHANNEL_ACCESS, &ED_ACCESS_INFO_MESSAGE_ONLY);
    etw_set_source_meta(&nd_log.sources[NDLS_HEALTH], CHANNEL_HEALTH, &ED_HEALTH_INFO_MESSAGE_ONLY);
    etw_set_source_meta(&nd_log.sources[NDLS_ACLK], CHANNEL_ACLK, &ED_ACLK_INFO_MESSAGE_ONLY);
    etw_set_source_meta(&nd_log.sources[NDLS_UNSET], CHANNEL_DAEMON, &ED_DAEMON_INFO_MESSAGE_ONLY);
    etw_set_source_meta(&nd_log.sources[NDLS_DEBUG], CHANNEL_DAEMON, &ED_DAEMON_INFO_MESSAGE_ONLY);

    DWORD wait_start = GetTickCount();
    while(true) {
        spinlock_lock(&nd_log.eventlog.provider_lock);
        bool enabled = nd_log.eventlog.provider_enabled;
        spinlock_unlock(&nd_log.eventlog.provider_lock);

        if(enabled)
            return true;

        // Timeout after 5 seconds
        if(GetTickCount() - wait_start > 5000) {
            EventUnregister(regHandle);
            return false;
        }

        Sleep(10); // Short sleep between checks
    }
}
#endif

bool nd_log_init_windows(void) {
    nd_win_trace("nd_log_init_windows: entered etw=%d initialized=%d",
                 (int)nd_log.eventlog.etw, (int)nd_log.eventlog.initialized);

    if(nd_log.eventlog.initialized)
        return true;

    // validate we have the right keys
    if(
            !check_event_id(NDLS_COLLECTORS, NDLP_INFO, MSGID_MESSAGE_ONLY, MC_COLLECTORS_INFO_MESSAGE_ONLY) ||
            !check_event_id(NDLS_DAEMON, NDLP_ERR, MSGID_MESSAGE_ONLY, MC_DAEMON_ERR_MESSAGE_ONLY) ||
            !check_event_id(NDLS_ACCESS, NDLP_WARNING, MSGID_ACCESS_USER, MC_ACCESS_WARN_ACCESS_USER) ||
            !check_event_id(NDLS_HEALTH, NDLP_CRIT, MSGID_ALERT_TRANSITION, MC_HEALTH_CRIT_ALERT_TRANSITION) ||
            !check_event_id(NDLS_DEBUG, NDLP_ALERT, MSGID_ACCESS_FORWARDER_USER, MC_DEBUG_ALERT_ACCESS_FORWARDER_USER))
       return false;

#if defined(HAVE_ETW)
    if(nd_log.eventlog.etw && !etw_register_provider())
        return false;
#endif

    if(!nd_log.eventlog.etw) {
#if !defined(HAVE_ETW)
        // Auto-install manifest with importChannel entries so classic ReportEventW events
        // appear in the Netdata WINEVT channels without any manual setup by the user.
        // Fast no-op when the manifest is already current (registry check only).
        wel_ensure_manifest_installed();
#endif
        // Remove legacy NetdataWEL registry entries. Classic WEL source lookup uses creation order,
        // so stale NetdataWEL\<source> keys would be found before the new per-channel keys, causing
        // ReportEventW to keep writing to the old channel.
        wchar_t legacy_key[MAX_PATH];
        swprintf(legacy_key, MAX_PATH,
                 L"SYSTEM\\CurrentControlSet\\Services\\EventLog\\NetdataWEL");
        if(RegDeleteTreeW(HKEY_LOCAL_MACHINE, legacy_key) == ERROR_SUCCESS)
            nd_win_trace("nd_log_init_windows: removed legacy EventLog\\NetdataWEL registry key");
    }

    // Loop through each source and add it to the registry
    for(size_t i = 0; i < _NDLS_MAX; i++) {
        nd_log.sources[i].source = i;

        const wchar_t *sub_channel = wel_provider_per_source[i];

        if(!sub_channel)
            // we will map these to NDLS_DAEMON
            continue;

        DWORD defaultMaxSize = 0;
        switch (i) {
            case NDLS_ACLK:
                defaultMaxSize = 5 * 1024 * 1024;
                break;

            case NDLS_HEALTH:
                defaultMaxSize = 35 * 1024 * 1024;
                break;

            default:
            case NDLS_ACCESS:
            case NDLS_COLLECTORS:
            case NDLS_DAEMON:
                defaultMaxSize = 20 * 1024 * 1024;
                break;
        }

        if(!nd_log.eventlog.etw) {
            const wchar_t *wel_channel = wel_channel_per_source[i];
            nd_win_trace("nd_log_init_windows: wel_add_to_registry channel=%ls source=%ls...",
                         wel_channel, sub_channel);
            if(!wel_add_to_registry(wel_channel, sub_channel, defaultMaxSize)) {
                nd_win_trace("nd_log_init_windows: wel_add_to_registry FAILED channel=%ls source=%ls err=%lu",
                             wel_channel, sub_channel, (unsigned long)GetLastError());
                return false;
            }
            nd_win_trace("nd_log_init_windows: wel_add_to_registry ok channel=%ls source=%ls",
                         wel_channel, sub_channel);

            // when not using a manifest, each source is a provider
            nd_win_trace("nd_log_init_windows: RegisterEventSourceW source[%zu/%s]...",
                         i, nd_log_id2source(i));
            nd_log.sources[i].hEventLog = RegisterEventSourceW(NULL, sub_channel);
            if (!nd_log.sources[i].hEventLog) {
                nd_win_trace("nd_log_init_windows: RegisterEventSourceW FAILED source[%zu/%s] err=%lu",
                             i, nd_log_id2source(i), (unsigned long)GetLastError());
                return false;
            }
            nd_win_trace("nd_log_init_windows: RegisterEventSourceW ok source[%zu/%s]",
                         i, nd_log_id2source(i));
        }
    }

    if(!nd_log.eventlog.etw) {
        // Map the unset ones to NDLS_DAEMON
        for (size_t i = 0; i < _NDLS_MAX; i++) {
            if (!nd_log.sources[i].hEventLog)
                nd_log.sources[i].hEventLog = nd_log.sources[NDLS_DAEMON].hEventLog;
        }
    }

    nd_log.eventlog.initialized = true;
    nd_win_trace("nd_log_init_windows: done initialized=true etw=%d; "
                 "events go to Netdata/{Daemon,Collectors,Access,Health,Aclk}",
                 (int)nd_log.eventlog.etw);
    return true;
}

bool nd_log_init_etw(void) {
    nd_log.eventlog.etw = true;
    return nd_log_init_windows();
}

bool nd_log_init_wel(void) {
    nd_log.eventlog.etw = false;
    return nd_log_init_windows();
}

// --------------------------------------------------------------------------------------------------------------------
// we pass all our fields to the windows events logs
// numbered the same way we have them in memory.
//
// to avoid runtime memory allocations, we use a static allocations with ready to use buffers
// which are immediately available for logging.

#define SMALL_WIDE_BUFFERS_SIZE 256
#define MEDIUM_WIDE_BUFFERS_SIZE 2048
#define BIG_WIDE_BUFFERS_SIZE 16384
static wchar_t small_wide_buffers[_NDF_MAX][SMALL_WIDE_BUFFERS_SIZE];
static wchar_t medium_wide_buffers[2][MEDIUM_WIDE_BUFFERS_SIZE];
static wchar_t big_wide_buffers[2][BIG_WIDE_BUFFERS_SIZE];

static struct {
    size_t size;
    wchar_t *buf;
} fields_buffers[_NDF_MAX] = { 0 };

#if defined(HAVE_ETW)
static EVENT_DATA_DESCRIPTOR etw_eventData[_NDF_MAX - 1];
#endif

static LPCWSTR wel_messages[_NDF_MAX - 1];

__attribute__((constructor)) void wevents_initialize_buffers(void) {
    for(size_t i = 0; i < _NDF_MAX ;i++) {
        fields_buffers[i].buf = small_wide_buffers[i];
        fields_buffers[i].size = SMALL_WIDE_BUFFERS_SIZE;
    }

    fields_buffers[NDF_NIDL_INSTANCE].buf = medium_wide_buffers[0];
    fields_buffers[NDF_NIDL_INSTANCE].size = MEDIUM_WIDE_BUFFERS_SIZE;

    fields_buffers[NDF_REQUEST].buf = big_wide_buffers[0];
    fields_buffers[NDF_REQUEST].size = BIG_WIDE_BUFFERS_SIZE;
    fields_buffers[NDF_MESSAGE].buf = big_wide_buffers[1];
    fields_buffers[NDF_MESSAGE].size = BIG_WIDE_BUFFERS_SIZE;

    for(size_t i = 1; i < _NDF_MAX ;i++)
        wel_messages[i - 1] = fields_buffers[i].buf;
}

// --------------------------------------------------------------------------------------------------------------------

#define is_field_set(fields, fields_max, field) ((field) < (fields_max) && (fields)[field].entry.set)

static const char *get_field_value_unsafe(struct log_field *fields, ND_LOG_FIELD_ID i, size_t fields_max, BUFFER **tmp) {
    if(!is_field_set(fields, fields_max, i) || !fields[i].eventlog)
        return "";

    static char number_str[MAX(MAX(UINT64_MAX_LENGTH, DOUBLE_MAX_LENGTH), UUID_STR_LEN)];

    const char *s = NULL;
    if (fields[i].logfmt_annotator)
        s = fields[i].logfmt_annotator(&fields[i]);

    else
        switch (fields[i].entry.type) {
            case NDFT_TXT:
                s = fields[i].entry.txt;
                break;
            case NDFT_STR:
                s = string2str(fields[i].entry.str);
                break;
            case NDFT_BFR:
                s = buffer_tostring(fields[i].entry.bfr);
                break;
            case NDFT_U64:
                print_uint64(number_str, fields[i].entry.u64);
                s = number_str;
                break;
            case NDFT_I64:
                print_int64(number_str, fields[i].entry.i64);
                s = number_str;
                break;
            case NDFT_DBL:
                print_netdata_double(number_str, fields[i].entry.dbl);
                s = number_str;
                break;
            case NDFT_UUID:
                if (!uuid_is_null(*fields[i].entry.uuid)) {
                    uuid_unparse_lower_compact(*fields[i].entry.uuid, number_str);
                    s = number_str;
                }
                break;
            case NDFT_CALLBACK:
                if (!*tmp)
                    *tmp = buffer_create(1024, NULL);
                else
                    buffer_flush(*tmp);

                if (fields[i].entry.cb.formatter(*tmp, fields[i].entry.cb.formatter_data))
                    s = buffer_tostring(*tmp);
                else
                    s = NULL;
                break;

            default:
                s = "UNHANDLED";
                break;
        }

    if(!s || !*s) return "";
    return s;
}
static void etw_replace_percent_with_unicode(wchar_t *s, size_t size) {
    size_t original_len = wcslen(s);

    // Traverse the string, replacing '%' with the Unicode fullwidth percent sign
    for (size_t i = 0; i < original_len && i < size - 1; i++) {
        if (s[i] == L'%' && iswdigit(s[i + 1])) {
            // s[i] = 0xFF05;  // Replace '%' with fullwidth percent sign '％'
            // s[i] = 0x29BC; // ⦼
            s[i] = 0x2105; // ℅
        }
    }

    // Ensure null termination if needed
    s[size - 1] = L'\0';
}

static void wevt_generate_all_fields_unsafe(struct log_field *fields, size_t fields_max, BUFFER **tmp) {
    for (size_t i = 0; i < fields_max; i++) {
        fields_buffers[i].buf[0] = L'\0';

        if (!fields[i].entry.set || !fields[i].eventlog)
            continue;

        const char *s = get_field_value_unsafe(fields, i, fields_max, tmp);
        if (s && *s) {
            utf8_to_utf16(fields_buffers[i].buf, (int) fields_buffers[i].size, s, -1);

            if(nd_log.eventlog.etw)
                // UNBELIEVABLE! they do recursive parameter expansion in ETW...
                etw_replace_percent_with_unicode(fields_buffers[i].buf, fields_buffers[i].size);
        }
    }
}

static bool has_user_role_permissions(struct log_field *fields, size_t fields_max, BUFFER **tmp) {
    const char *t;

    t = get_field_value_unsafe(fields, NDF_USER_NAME, fields_max, tmp);
    if (*t) return true;

    t = get_field_value_unsafe(fields, NDF_USER_ROLE, fields_max, tmp);
    if (*t && strcmp(t, "none") != 0) return true;

    t = get_field_value_unsafe(fields, NDF_USER_ACCESS, fields_max, tmp);
    if (*t && strcmp(t, "0x0") != 0) return true;

    return false;
}

static bool nd_logger_windows(struct nd_log_source *source, struct log_field *fields, size_t fields_max) {
    if (!nd_log.eventlog.initialized)
        return false;

    // trace the first call from each source independently so all sources are confirmed in the log
    static bool first_call[_NDLS_MAX];
    if(source->source < _NDLS_MAX && !__atomic_exchange_n(&first_call[source->source], true, __ATOMIC_RELAXED))
        nd_win_trace("nd_logger_windows: first call source=%s", nd_log_id2source(source->source));

    ND_LOG_FIELD_PRIORITY priority = NDLP_INFO;
    if (fields[NDF_PRIORITY].entry.set)
        priority = (ND_LOG_FIELD_PRIORITY) fields[NDF_PRIORITY].entry.u64;

    DWORD wType = get_event_type_from_priority(priority);
    (void) wType;

    CLEAN_BUFFER *tmp = NULL;

    static SPINLOCK spinlock = SPINLOCK_INITIALIZER;
    spinlock_lock(&spinlock);
    wevt_generate_all_fields_unsafe(fields, fields_max, &tmp);

    MESSAGE_ID messageID;
    switch (source->source) {
        default:
        case NDLS_DEBUG:
        case NDLS_DAEMON:
        case NDLS_COLLECTORS:
            messageID = MSGID_MESSAGE_ONLY;
            break;

        case NDLS_HEALTH:
            messageID = MSGID_ALERT_TRANSITION;
            break;

        case NDLS_ACCESS:
            if (is_field_set(fields, fields_max, NDF_MESSAGE)) {
                messageID = MSGID_ACCESS_MESSAGE;

                if (has_user_role_permissions(fields, fields_max, &tmp))
                    messageID = MSGID_ACCESS_MESSAGE_USER;
                else if (*get_field_value_unsafe(fields, NDF_REQUEST, fields_max, &tmp))
                    messageID = MSGID_ACCESS_MESSAGE_REQUEST;
            } else if (is_field_set(fields, fields_max, NDF_RESPONSE_CODE)) {
                messageID = MSGID_ACCESS;

                if (*get_field_value_unsafe(fields, NDF_SRC_FORWARDED_FOR, fields_max, &tmp))
                    messageID = MSGID_ACCESS_FORWARDER;

                if (has_user_role_permissions(fields, fields_max, &tmp)) {
                    if (messageID == MSGID_ACCESS)
                        messageID = MSGID_ACCESS_USER;
                    else
                        messageID = MSGID_ACCESS_FORWARDER_USER;
                }
            } else
                messageID = MSGID_REQUEST_ONLY;
            break;

        case NDLS_ACLK:
            messageID = MSGID_MESSAGE_ONLY;
            break;
    }

    if (messageID == MSGID_MESSAGE_ONLY && (
            *get_field_value_unsafe(fields, NDF_ERRNO, fields_max, &tmp) ||
            *get_field_value_unsafe(fields, NDF_WINERROR, fields_max, &tmp))) {
        messageID = MSGID_MESSAGE_ERRNO;
    }

    DWORD eventID = construct_event_id(source->source, priority, messageID);

    // wType
    //
    // without a manifest => this determines the Level of the event
    // with a manifest    => Level from the manifest is used (wType ignored)
    //                       [however it is good to have, in case the manifest is not accessible somehow]
    //

    // wCategory
    //
    // without a manifest => numeric Task values appear
    // with a manifest    => Task from the manifest is used (wCategory ignored)

    BOOL rc;
#if defined(HAVE_ETW)
    if (nd_log.eventlog.etw) {
        // metadata based logging - ETW

        for (size_t i = 1; i < _NDF_MAX; i++)
            EventDataDescCreate(&etw_eventData[i - 1], fields_buffers[i].buf,
                                (wcslen(fields_buffers[i].buf) + 1) * sizeof(WCHAR));

        EVENT_DESCRIPTOR EventDesc = {
                .Id = eventID & EVENT_ID_CODE_MASK, // ETW needs the raw event id
                .Version = 0,
                .Channel = source->channelID,
                .Level = get_level_from_priority(priority),
                .Opcode = source->Opcode,
                .Task = source->Task,
                .Keyword = source->Keyword,
        };

        rc = ERROR_SUCCESS == EventWrite(regHandle, &EventDesc, _NDF_MAX - 1, etw_eventData);

    }
    else
#endif
    {
        // eventID based logging - WEL
        rc = ReportEventW(source->hEventLog, wType, 0, eventID, NULL, _NDF_MAX - 1, 0, wel_messages, NULL);
        if(!rc) {
            // trace only the first failure per source — persistent failure would otherwise
            // emit a trace line on every log call, causing unbounded trace-file growth
            static bool first_failure[_NDLS_MAX];
            if(source->source < _NDLS_MAX && !__atomic_exchange_n(&first_failure[source->source], true, __ATOMIC_RELAXED)) {
                DWORD err = GetLastError();
                nd_win_trace("nd_logger_windows[%s]: ReportEventW FAILED err=%lu eventID=0x%lx",
                             nd_log_id2source(source->source), (unsigned long)err, (unsigned long)eventID);
            }
        }
        else {
            // trace the first successful write per source so the log confirms events reach WEL
            static bool first_success[_NDLS_MAX];
            if(source->source < _NDLS_MAX && !__atomic_exchange_n(&first_success[source->source], true, __ATOMIC_RELAXED)) {
                const wchar_t *wch = wel_channel_per_source[source->source];
                nd_win_trace("nd_logger_windows[%s]: ReportEventW ok eventID=0x%lx -> channel=%ls",
                             nd_log_id2source(source->source), (unsigned long)eventID,
                             wch ? wch : NETDATA_WEL_CHANNEL_DAEMON_W);
            }
        }
    }

    spinlock_unlock(&spinlock);

    return rc == TRUE;
}

#if defined(HAVE_ETW)
bool nd_logger_etw(struct nd_log_source *source, struct log_field *fields, size_t fields_max) {
    return nd_logger_windows(source, fields, fields_max);
}
#endif

#if defined(HAVE_WEL)
bool nd_logger_wel(struct nd_log_source *source, struct log_field *fields, size_t fields_max) {
    return nd_logger_windows(source, fields, fields_max);
}
#endif

#endif
