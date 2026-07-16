// SPDX-License-Identifier: GPL-3.0-or-later

#include "nd_log-internals.h"

#if defined(OS_WINDOWS) && (defined(HAVE_ETW) || defined(HAVE_WEL))

// Bounded wide-string length: like wcsnlen, but returns 0 (not maxlen) when the
// string is not null-terminated within `maxlen`. Callers use the result for
// (len + 1) * sizeof(wchar_t) buffer-size calculations and get a deterministic
// 0 instead of an out-of-bounds read on a malformed input.
//
// IMPORTANT: `maxlen` is the caller's claim about the maximum number of wide
// chars the string could contain, not the size of a `const wchar_t *` pointer.
// Do NOT pass `_countof(ptr_to_string)` for a `const wchar_t *ptr` argument —
// that evaluates to `sizeof(const wchar_t *) / sizeof(wchar_t)` (2 on 64-bit
// Windows, 1 on 32-bit), which is the size of the POINTER, not the size of
// the pointed-to string. Use a literal cap (e.g. WEL_LABEL_MAX_CHARS) or
// `_countof(arr)` only when `arr` is a true wide-char array.
static inline size_t wel_wcslen_bounded(const wchar_t *s, size_t maxlen) {
    if (!s) return 0;
    size_t n = wcsnlen(s, maxlen);
    return (n == maxlen) ? 0 : n;
}

// Max wide-char length (excluding the null terminator) of any entry in
// wel_channels[].label. Used as the bound argument to wel_wcslen_bounded()
// when writing a label to the registry.
//
// IMPORTANT: this must be a literal cap, not `_countof(wel_channels[i].label)`.
// The wel_channels[] members are `const wchar_t *`, so `_countof(...)` evaluates
// to `sizeof(const wchar_t *) / sizeof(wchar_t)` (2 on 64-bit Windows, 1 on 32-bit) —
// the size of the POINTER, not the size of the pointed-to string. Using that as
// the wcsnlen bound silently clips every label to 2 wide chars and writes an
// empty string to the registry. 64 wide chars is comfortably more than the
// longest current label ("Collectors" = 10 wide chars incl. null).
#define WEL_LABEL_MAX_CHARS 64

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

// Map each source to its WINEVT channel name (only when not using ETW)
static const wchar_t *wel_channel_per_source[_NDLS_MAX] = {
        [NDLS_UNSET]        = NULL,
        [NDLS_ACCESS]       = NETDATA_WEL_CHANNEL_ACCESS_W,
        [NDLS_ACLK]         = NETDATA_WEL_CHANNEL_ACLK_W,
        [NDLS_COLLECTORS]   = NETDATA_WEL_CHANNEL_COLLECTORS_W,
        [NDLS_DAEMON]       = NETDATA_WEL_CHANNEL_DAEMON_W,
        [NDLS_HEALTH]       = NETDATA_WEL_CHANNEL_HEALTH_W,
        [NDLS_DEBUG]        = NULL,
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

    // Find wevt_netdata.dll: prefer the copy next to netdata.exe (dev/portable), then
    // fall back to System32 (MSI install or placed there by wel_ensure_manifest_installed).
    bool dllFound = wel_replace_program_with_wevt_netdata_dll(modulePath, _countof(modulePath));
    if (!dllFound) {
        wchar_t sys32[MAX_PATH];
        if (GetSystemDirectoryW(sys32, _countof(sys32))) {
            swprintf(modulePath, _countof(modulePath), L"%ls\\wevt_netdata.dll", sys32);
            dllFound = (GetFileAttributesW(modulePath) != INVALID_FILE_ATTRIBUTES);
        }
    }
    if (dllFound) {
        RegSetValueExW(hRegKey, L"EventMessageFile", 0, REG_EXPAND_SZ,
                       (LPBYTE)modulePath,
                       (DWORD)((wel_wcslen_bounded(modulePath, _countof(modulePath)) + 1) * sizeof(wchar_t)));

        DWORD types_supported = EVENTLOG_SUCCESS | EVENTLOG_ERROR_TYPE | EVENTLOG_WARNING_TYPE | EVENTLOG_INFORMATION_TYPE;
        RegSetValueExW(hRegKey, L"TypesSupported", 0, REG_DWORD, (LPBYTE)&types_supported, sizeof(DWORD));
    }

    RegCloseKey(hRegKey);
    return true;
}

// Registry key where WINEVT stores registered publishers (providers).
#define WINEVT_PUBLISHERS_KEY L"SOFTWARE\\Microsoft\\Windows\\CurrentVersion\\WINEVT\\Publishers\\"

// WINEVT Channels registry path prefix
#define WINEVT_CHANNELS_KEY L"SOFTWARE\\Microsoft\\Windows\\CurrentVersion\\WINEVT\\Channels\\"

// Canonical channel-to-label table used by both wel_manifest_is_current() and
// wel_ensure_manifest_installed().  A single definition prevents the two functions
// from drifting out of sync and ensures every channel is validated and configured.
static const struct { const wchar_t *ch; const wchar_t *label; } wel_channels[] = {
    { L"Netdata/Daemon",     L"Daemon"     },
    { L"Netdata/Collectors", L"Collectors" },
    { L"Netdata/Access",     L"Access"     },
    { L"Netdata/Health",     L"Health"     },
    { L"Netdata/Aclk",       L"Aclk"       },
};

static bool wel_manifest_is_current(void) {
    // All five importChannel provider GUIDs must be present; a partial registration
    // (e.g. only Daemon from an older version) must also trigger re-installation.
    static const wchar_t *const provider_guids[] = {
        NETDATA_WEL_PROVIDER_DAEMON_GUID_STR_W,
        NETDATA_WEL_PROVIDER_COLLECTORS_GUID_STR_W,
        NETDATA_WEL_PROVIDER_ACCESS_GUID_STR_W,
        NETDATA_WEL_PROVIDER_HEALTH_GUID_STR_W,
        NETDATA_WEL_PROVIDER_ACLK_GUID_STR_W,
    };

    for (size_t i = 0; i < _countof(provider_guids); i++) {
        wchar_t key[MAX_PATH];
        swprintf(key, _countof(key), L"%ls%ls", WINEVT_PUBLISHERS_KEY, provider_guids[i]);
        HKEY hKey;
        if (RegOpenKeyExW(HKEY_LOCAL_MACHINE, key, 0, KEY_READ, &hKey) != ERROR_SUCCESS)
            return false;
        RegCloseKey(hKey);
    }

    // Verify every channel: must exist, be Admin type (EVT_CHANNEL_TYPE_ADMIN = 0),
    // and carry the correct short display name.  Checking all channels — not just
    // Daemon — catches a partial registration where later label writes failed, which
    // would otherwise be silently accepted as current and never repaired.
    for (size_t i = 0; i < _countof(wel_channels); i++) {
        wchar_t chkey[MAX_PATH];
        swprintf(chkey, _countof(chkey), L"%ls%ls", WINEVT_CHANNELS_KEY, wel_channels[i].ch);
        HKEY hCh;
        if (RegOpenKeyExW(HKEY_LOCAL_MACHINE, chkey, 0, KEY_READ, &hCh) != ERROR_SUCCESS)
            return false;

        DWORD chtype = 1;
        DWORD sz = sizeof(chtype);
        RegQueryValueExW(hCh, L"Type", NULL, NULL, (LPBYTE)&chtype, &sz);

        wchar_t dispName[64] = {0};
        DWORD dnSz = sizeof(dispName);
        LONG dnRc = RegQueryValueExW(hCh, L"DisplayName", NULL, NULL, (LPBYTE)dispName, &dnSz);

        RegCloseKey(hCh);

        if (chtype != 0)  // 0 = Admin
            return false;

        if (dnRc != ERROR_SUCCESS || wcscmp(dispName, wel_channels[i].label) != 0)
            return false;
    }

    return true;
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
    "          <channel chid=\"CHANNEL_DAEMON\" name=\"Netdata/Daemon\" symbol=\"CHANNEL_DAEMON\" type=\"Admin\" enabled=\"true\" message=\"$(string.Channel.Daemon)\"/>\r\n" \
    "          <channel chid=\"CHANNEL_COLLECTORS\" name=\"Netdata/Collectors\" symbol=\"CHANNEL_COLLECTORS\" type=\"Admin\" enabled=\"true\" message=\"$(string.Channel.Collectors)\"/>\r\n" \
    "          <channel chid=\"CHANNEL_ACCESS\" name=\"Netdata/Access\" symbol=\"CHANNEL_ACCESS\" type=\"Admin\" enabled=\"true\" message=\"$(string.Channel.Access)\"/>\r\n" \
    "          <channel chid=\"CHANNEL_HEALTH\" name=\"Netdata/Health\" symbol=\"CHANNEL_HEALTH\" type=\"Admin\" enabled=\"true\" message=\"$(string.Channel.Health)\"/>\r\n" \
    "          <channel chid=\"CHANNEL_ACLK\" name=\"Netdata/Aclk\" symbol=\"CHANNEL_ACLK\" type=\"Admin\" enabled=\"true\" message=\"$(string.Channel.Aclk)\"/>\r\n" \
    "        </channels>\r\n" \
    "      </provider>\r\n" \
    "      <provider name=\"NetdataDaemon\"\r\n" \
    "        guid=\"" NETDATA_WEL_PROVIDER_DAEMON_GUID_STR "\"\r\n" \
    "        resourceFileName=\"%s\" messageFileName=\"%s\">\r\n" \
    "        <channels><importChannel chid=\"IMPORT_DAEMON\" name=\"Netdata/Daemon\"/></channels>\r\n" \
    "      </provider>\r\n" \
    "      <provider name=\"NetdataCollectors\"\r\n" \
    "        guid=\"" NETDATA_WEL_PROVIDER_COLLECTORS_GUID_STR "\"\r\n" \
    "        resourceFileName=\"%s\" messageFileName=\"%s\">\r\n" \
    "        <channels><importChannel chid=\"IMPORT_COLLECTORS\" name=\"Netdata/Collectors\"/></channels>\r\n" \
    "      </provider>\r\n" \
    "      <provider name=\"NetdataAccess\"\r\n" \
    "        guid=\"" NETDATA_WEL_PROVIDER_ACCESS_GUID_STR "\"\r\n" \
    "        resourceFileName=\"%s\" messageFileName=\"%s\">\r\n" \
    "        <channels><importChannel chid=\"IMPORT_ACCESS\" name=\"Netdata/Access\"/></channels>\r\n" \
    "      </provider>\r\n" \
    "      <provider name=\"NetdataHealth\"\r\n" \
    "        guid=\"" NETDATA_WEL_PROVIDER_HEALTH_GUID_STR "\"\r\n" \
    "        resourceFileName=\"%s\" messageFileName=\"%s\">\r\n" \
    "        <channels><importChannel chid=\"IMPORT_HEALTH\" name=\"Netdata/Health\"/></channels>\r\n" \
    "      </provider>\r\n" \
    "      <provider name=\"NetdataAclk\"\r\n" \
    "        guid=\"" NETDATA_WEL_PROVIDER_ACLK_GUID_STR "\"\r\n" \
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
        return;
    }


    wchar_t system32[MAX_PATH];
    if (!GetSystemDirectoryW(system32, _countof(system32))) return;

    wchar_t dllDst[MAX_PATH];
    swprintf(dllDst, _countof(dllDst), L"%ls\\wevt_netdata.dll", system32);

    // If wevt_netdata.dll exists next to netdata.exe (dev build / manual deploy),
    // copy it to System32 so Event Viewer can resolve message strings.
    {
        wchar_t dllSrc[MAX_PATH];
        if (GetModuleFileNameW(NULL, dllSrc, _countof(dllSrc)) &&
            wel_replace_program_with_wevt_netdata_dll(dllSrc, _countof(dllSrc)))
            CopyFileW(dllSrc, dllDst, FALSE);
    }

    // DLL must exist in System32 (either we just copied it or MSI put it there).
    if (GetFileAttributesW(dllDst) == INVALID_FILE_ATTRIBUTES) {
        return;
    }

    // Grant EventLog service read access to the DLL (required for manifest channels).
    wchar_t icaclsPath[MAX_PATH];
    swprintf(icaclsPath, _countof(icaclsPath), L"%ls\\icacls.exe", system32);
    wchar_t icaclsParams[MAX_PATH * 2];
    swprintf(icaclsParams, _countof(icaclsParams), L"\"%ls\" /grant \"NT SERVICE\\EventLog\":R", dllDst);
    (void)wel_run_silent(icaclsPath, icaclsParams);

    // Build manifest content from the embedded template. The manifest is generated
    // at runtime so it always matches this binary's channel layout, regardless of
    // what manifest file (if any) is on disk.
    char dllDstNarrow[MAX_PATH];
    if(!WideCharToMultiByte(CP_UTF8, 0, dllDst, -1, dllDstNarrow, _countof(dllDstNarrow), NULL, NULL)) {
        return;
    }

    char manifest[8192];
    int mlen = snprintf(manifest, sizeof(manifest), WEL_MANIFEST_XML_FMT,
                        dllDstNarrow, dllDstNarrow,   // Netdata provider
                        dllDstNarrow, dllDstNarrow,   // NetdataDaemon
                        dllDstNarrow, dllDstNarrow,   // NetdataCollectors
                        dllDstNarrow, dllDstNarrow,   // NetdataAccess
                        dllDstNarrow, dllDstNarrow,   // NetdataHealth
                        dllDstNarrow, dllDstNarrow);  // NetdataAclk
    if (mlen <= 0 || mlen >= (int)sizeof(manifest)) {
        return;
    }

    // Write manifest to System32 (same location as the MSI installer uses).
    wchar_t manifestDst[MAX_PATH];
    swprintf(manifestDst, _countof(manifestDst), L"%ls\\wevt_netdata_manifest.xml", system32);

    HANDLE hFile = CreateFileW(manifestDst, GENERIC_WRITE, 0, NULL, CREATE_ALWAYS,
                               FILE_ATTRIBUTE_NORMAL, NULL);
    if (hFile == INVALID_HANDLE_VALUE) {
        return;
    }
    DWORD written;
    BOOL fileOk = WriteFile(hFile, manifest, (DWORD)mlen, &written, NULL);
    CloseHandle(hFile);
    if (!fileOk || written != (DWORD)mlen) {
        return;
    }

    wchar_t wevtutil[MAX_PATH];
    swprintf(wevtutil, _countof(wevtutil), L"%ls\\wevtutil.exe", system32);


    // Unregister any previously registered manifest (ignore errors — normal on first run).
    wchar_t umParams[MAX_PATH * 2];
    swprintf(umParams, _countof(umParams), L"um \"%ls\"", manifestDst);
    (void)wel_run_silent(wevtutil, umParams);

    // Register the new manifest. The /mf and /rf flags tell wevtutil where the DLL is,
    // overriding the resourceFileName/messageFileName attributes in the XML.
    wchar_t imParams[MAX_PATH * 4];
    swprintf(imParams, _countof(imParams), L"im \"%ls\" \"/mf:%ls\" \"/rf:%ls\"",
             manifestDst, dllDst, dllDst);
    bool imOk = wel_run_silent(wevtutil, imParams);

    // Only proceed with post-registration cleanup and display-name setting when
    // wevtutil im succeeded.  Deleting stale keys before confirming the new
    // registration is complete would leave the agent with no WEL routing if
    // wevtutil im fails (locked channels, manifest validation error, etc.).
    if (!imOk)
        return;

    // Remove flat classic WEL log entries that pre-manifest builds created directly.
    // wevtutil um removes WINEVT publisher entries but leaves behind any EventLog\*
    // keys our code created before the manifest approach; those flat logs appear as
    // duplicate "Netdata/Daemon" entries in Event Viewer alongside the tree hierarchy.
    // Safe to delete now because wevtutil im has already created the correct
    // channel-linked entries that replace them.
    static const wchar_t *const flat_stale_keys[] = {
        L"SYSTEM\\CurrentControlSet\\Services\\EventLog\\Netdata/Daemon",
        L"SYSTEM\\CurrentControlSet\\Services\\EventLog\\Netdata/Collectors",
        L"SYSTEM\\CurrentControlSet\\Services\\EventLog\\Netdata/Access",
        L"SYSTEM\\CurrentControlSet\\Services\\EventLog\\Netdata/Health",
        L"SYSTEM\\CurrentControlSet\\Services\\EventLog\\Netdata/Aclk",
        L"SYSTEM\\CurrentControlSet\\Services\\EventLog\\NetdataWEL",
    };
    for (size_t j = 0; j < _countof(flat_stale_keys); j++)
        RegDeleteTreeW(HKEY_LOCAL_MACHINE, flat_stale_keys[j]);

    // Set short display names on every channel so Event Viewer shows "Daemon" (not the
    // full channel name "Netdata/Daemon") as the leaf label inside the Netdata folder.
    // wevtutil im does not derive these from $(string.*) references in the embedded
    // manifest, so we set them explicitly after registration.
    for (size_t cl = 0; cl < _countof(wel_channels); cl++) {
        wchar_t chregkey[MAX_PATH];
        swprintf(chregkey, _countof(chregkey), L"%ls%ls",
                 WINEVT_CHANNELS_KEY, wel_channels[cl].ch);
        HKEY hCh;
        if (RegOpenKeyExW(HKEY_LOCAL_MACHINE, chregkey, 0, KEY_SET_VALUE, &hCh) == ERROR_SUCCESS) {
            RegSetValueExW(hCh, L"DisplayName", 0, REG_SZ,
                           (LPBYTE)wel_channels[cl].label,
                           (DWORD)((wel_wcslen_bounded(wel_channels[cl].label,
                                                       WEL_LABEL_MAX_CHARS) + 1) * sizeof(wchar_t)));
            RegCloseKey(hCh);
        }
    }
}

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

static void wel_queue_init(void);

bool nd_log_init_windows(void) {
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

    // Register WINEVT manifest channels when not already current — required for Event
    // Viewer to show the Netdata tree hierarchy (Netdata → Daemon etc.) instead of flat
    // "Netdata/Daemon" entries.  Previously gated on !HAVE_ETW, which meant ETW builds
    // never auto-registered channels, producing the flat display.  The call is now
    // unconditional: ETW dev builds and WEL builds both need channels; MSI-installed
    // builds exit immediately via the registry check in wel_manifest_is_current().
    wel_ensure_manifest_installed();

    if(!nd_log.eventlog.etw) {
        // Remove Application-log source entries left by a prior broken installation that
        // registered sources under Application instead of their Netdata/* channel.  These
        // entries redirect sources away from the WINEVT channels and cause message
        // descriptions to fail because the Application log's DLL resolution path differs.
        // Only leaf keys are deleted here (RegDeleteKeyW, no subtree needed).
        static const wchar_t *const app_stale_keys[] = {
            L"SYSTEM\\CurrentControlSet\\Services\\EventLog\\Application\\NetdataDaemon",
            L"SYSTEM\\CurrentControlSet\\Services\\EventLog\\Application\\NetdataCollectors",
            L"SYSTEM\\CurrentControlSet\\Services\\EventLog\\Application\\NetdataAccess",
            L"SYSTEM\\CurrentControlSet\\Services\\EventLog\\Application\\NetdataHealth",
            L"SYSTEM\\CurrentControlSet\\Services\\EventLog\\Application\\NetdataAclk",
        };
        for (size_t j = 0; j < _countof(app_stale_keys); j++)
            RegDeleteKeyW(HKEY_LOCAL_MACHINE, app_stale_keys[j]);
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
            // Register the source under its own WINEVT channel log ("Netdata/Daemon" etc.).
            // wevtutil im creates EventLog\Netdata/Daemon\NetdataDaemon via importChannel;
            // this call is idempotent and ensures MaxSize is set even when wevtutil fails.
            const wchar_t *wel_channel = wel_channel_per_source[i];
            if(!wel_add_to_registry(wel_channel, sub_channel, defaultMaxSize)) {
                return false;
            }

            nd_log.sources[i].hEventLog = RegisterEventSourceW(NULL, sub_channel);
            if (!nd_log.sources[i].hEventLog) {
                return false;
            }
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
    wel_queue_init();
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
// Largest per-field wide-char buffer (incl. null terminator). Used by
// wel_wcslen_bounded() to satisfy SonarQube S5813 at ETW write sites.
#define NDF_FIELD_MAX_CHARS BIG_WIDE_BUFFERS_SIZE
static wchar_t small_wide_buffers[_NDF_MAX][SMALL_WIDE_BUFFERS_SIZE];
static wchar_t medium_wide_buffers[2][MEDIUM_WIDE_BUFFERS_SIZE];
static wchar_t big_wide_buffers[2][BIG_WIDE_BUFFERS_SIZE];

static struct {
    size_t size;
    wchar_t *buf;
} fields_buffers[_NDF_MAX] = { 0 };

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
}

// ----------------------------------------------------------------------------
// Async WEL/ETW writer — decouples producers from ReportEventW/EventWrite
//
// The global wchar buffers (small/medium/big) are shared state. The old design held a
// single spinlock across BOTH field generation AND ReportEventW; if ReportEventW blocked
// (e.g. a crashed prior run left the WEL channel in bad state) every logging thread spun
// indefinitely, blocking the main thread and the shutdown cleanup thread.
//
// New design:
//   - wel_queue.mutex replaces the spinlock and covers only field generation + enqueue.
//   - A single background thread (wel_async_writer) dequeues entries and calls ReportEventW.
//   - If ReportEventW blocks, only the background thread stalls; producers enqueue and return.
//   - When the queue is full the entry is dropped and the caller falls back to stderr.

#define WEL_QUEUE_DEPTH 8

struct wel_queue_entry {
    HANDLE          hEventLog;
    WORD            wType;
    DWORD           eventID;
    ND_LOG_SOURCES  source_id;
    bool            is_etw;
#if defined(HAVE_ETW)
    USHORT          channelID;
    UCHAR           level;
    UCHAR           opcode;
    USHORT          task;
    ULONGLONG       keyword;
#endif
    // Snapshots of the three global wchar buffer pools (copied under wel_queue.mutex)
    wchar_t small[_NDF_MAX][SMALL_WIDE_BUFFERS_SIZE];
    wchar_t medium[2][MEDIUM_WIDE_BUFFERS_SIZE];
    wchar_t big[2][BIG_WIDE_BUFFERS_SIZE];
};

static struct {
    struct wel_queue_entry  slots[WEL_QUEUE_DEPTH];
    size_t                  head, tail, count;
    bool                    stopped;
    uint64_t                dropped;
    netdata_mutex_t         mutex;
    netdata_cond_t          not_empty;
    HANDLE                  drain_ack;  // auto-reset; consumer sets it on exit
    ND_THREAD              *thread;
    bool                    initialized;
} wel_queue;

// Mirror the fields_buffers[] layout: return the right buffer from a queue entry.
static inline const wchar_t *wel_entry_field(const struct wel_queue_entry *e, size_t i) {
    if(i == NDF_NIDL_INSTANCE) return e->medium[0];
    if(i == NDF_REQUEST)       return e->big[0];
    if(i == NDF_MESSAGE)       return e->big[1];
    return e->small[i];
}

static void wel_entry_process(struct wel_queue_entry *e) {
    BOOL rc;

#if defined(HAVE_ETW)
    if(e->is_etw) {
        EVENT_DATA_DESCRIPTOR desc[_NDF_MAX - 1];
        for(size_t i = 1; i < _NDF_MAX; i++) {
            const wchar_t *buf = wel_entry_field(e, i);
            // Field buffers are fixed-size (small/medium/big) and always null-terminated
            // when populated; the bounded helper is only here to satisfy SonarQube S5813
            // and to return 0 on a malformed buffer rather than overread it.
            EventDataDescCreate(&desc[i - 1], buf,
                                (ULONG)((wel_wcslen_bounded(buf, NDF_FIELD_MAX_CHARS) + 1) * sizeof(WCHAR)));
        }
        EVENT_DESCRIPTOR ed = {
            .Id      = e->eventID & EVENT_ID_CODE_MASK,
            .Version = 0,
            .Channel = (UCHAR)e->channelID,
            .Level   = e->level,
            .Opcode  = e->opcode,
            .Task    = e->task,
            .Keyword = e->keyword,
        };
        rc = ERROR_SUCCESS == EventWrite(regHandle, &ed, _NDF_MAX - 1, desc);
    }
    else
#endif
    {
        LPCWSTR msgs[_NDF_MAX - 1];
        for(size_t i = 1; i < _NDF_MAX; i++)
            msgs[i - 1] = wel_entry_field(e, i);
        rc = ReportEventW(e->hEventLog, e->wType, 0, e->eventID, NULL, _NDF_MAX - 1, 0, msgs, NULL);
    }

}

static void wel_async_writer(void *arg __maybe_unused) {

    for(;;) {
        netdata_mutex_lock(&wel_queue.mutex);

        while(wel_queue.count == 0 && !wel_queue.stopped)
            netdata_cond_wait(&wel_queue.not_empty, &wel_queue.mutex);

        if(wel_queue.count == 0) {
            // stopped and queue drained
            netdata_mutex_unlock(&wel_queue.mutex);
            break;
        }

        // Snapshot head without advancing. count stays > 0, so producers cannot
        // wrap tail back to this slot while we process it outside the mutex.
        size_t idx = wel_queue.head;
        netdata_mutex_unlock(&wel_queue.mutex);

        // ReportEventW/EventWrite may block here — no lock held, producers unaffected
        wel_entry_process(&wel_queue.slots[idx]);

        // Release the slot
        netdata_mutex_lock(&wel_queue.mutex);
        wel_queue.head = (wel_queue.head + 1) % WEL_QUEUE_DEPTH;
        wel_queue.count--;
        netdata_mutex_unlock(&wel_queue.mutex);
    }

    if(wel_queue.drain_ack)
        SetEvent(wel_queue.drain_ack);
}

static void wel_queue_init(void) {
    netdata_mutex_init(&wel_queue.mutex);
    netdata_cond_init(&wel_queue.not_empty);
    wel_queue.drain_ack = CreateEvent(NULL, FALSE, FALSE, NULL);
    wel_queue.thread = nd_thread_create("WEL-ASYNC", NETDATA_THREAD_OPTION_DEFAULT,
                                        wel_async_writer, NULL);
    wel_queue.initialized = true;
}

void nd_log_stop_windows_async(void) {
    if(!wel_queue.initialized)
        return;

    netdata_mutex_lock(&wel_queue.mutex);
    wel_queue.stopped = true;
    netdata_cond_signal(&wel_queue.not_empty);
    netdata_mutex_unlock(&wel_queue.mutex);

    // Wait up to 2 s for the async writer to drain remaining entries.
    // If WEL is still blocked we time out and let the process terminate normally.
    if(wel_queue.drain_ack)
        WaitForSingleObject(wel_queue.drain_ack, 2000);

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
    if (!nd_log.eventlog.initialized || !wel_queue.initialized)
        return false;

    ND_LOG_FIELD_PRIORITY priority = NDLP_INFO;
    if (fields[NDF_PRIORITY].entry.set)
        priority = (ND_LOG_FIELD_PRIORITY) fields[NDF_PRIORITY].entry.u64;

    DWORD wType = get_event_type_from_priority(priority);
    CLEAN_BUFFER *tmp = NULL;

    // wel_queue.mutex replaces the old spinlock:
    //   - protects field generation into the shared global wchar buffers
    //   - protects queue head/tail/count while we claim a slot
    //   - does NOT cover ReportEventW (that runs in the async writer thread)
    netdata_mutex_lock(&wel_queue.mutex);

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

    bool enqueued = false;
    if(wel_queue.count < WEL_QUEUE_DEPTH) {
        struct wel_queue_entry *e = &wel_queue.slots[wel_queue.tail];
        wel_queue.tail = (wel_queue.tail + 1) % WEL_QUEUE_DEPTH;
        wel_queue.count++;
        enqueued = true;

        e->hEventLog  = source->hEventLog;
        e->wType      = wType;
        e->eventID    = eventID;
        e->source_id  = source->source;
        e->is_etw     = nd_log.eventlog.etw;
#if defined(HAVE_ETW)
        e->channelID  = source->channelID;
        e->level      = get_level_from_priority(priority);
        e->opcode     = source->Opcode;
        e->task       = source->Task;
        e->keyword    = source->Keyword;
#endif
        // Snapshot the global buffers into the queue slot before releasing the mutex.
        memcpy(e->small,  small_wide_buffers,  sizeof(small_wide_buffers));
        memcpy(e->medium, medium_wide_buffers, sizeof(medium_wide_buffers));
        memcpy(e->big,    big_wide_buffers,    sizeof(big_wide_buffers));

        netdata_cond_signal(&wel_queue.not_empty);
    } else {
        wel_queue.dropped++;
    }

    netdata_mutex_unlock(&wel_queue.mutex);

    return enqueued;
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
