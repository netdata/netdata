// SPDX-License-Identifier: GPL-3.0-or-later

#include "../libnetdata.h"

#if defined(OS_WINDOWS)
// *out is set only when a 4-byte REG_DWORD is read
long netdata_registry_get_dword_from_open_key(unsigned int *out, void *lKey, char *name)
{
    DWORD value = 0;
    DWORD length = sizeof(value);
    long ret = RegGetValueA(lKey, NULL, name, RRF_RT_REG_DWORD, NULL, &value, &length);

    // RegGetValue accepts a REG_DWORD stored with fewer than 4 bytes
    if (ret == ERROR_SUCCESS && length != sizeof(value))
        ret = ERROR_INVALID_DATA;

    if (ret == ERROR_SUCCESS)
        *out = value;

    return ret;
}

bool netdata_registry_get_dword(unsigned int *out, void *hKey, char *subKey, char *name)
{
    HKEY lKey;
    bool status = true;
    long ret = RegOpenKeyEx(hKey,
                            subKey,
                            0,
                            KEY_READ,
                            &lKey);
    if (ret != ERROR_SUCCESS)
        return false;

    ret = netdata_registry_get_dword_from_open_key(out, lKey, name);
    if (ret != ERROR_SUCCESS)
        status = false;

    RegCloseKey(lKey);

    return status;
}

// length is the size of out; on success out holds a NUL-terminated REG_SZ value, otherwise it is empty
long netdata_registry_get_string_from_open_key(char *out, unsigned int length, void *lKey, char *name)
{
    if (!length)
        return ERROR_INSUFFICIENT_BUFFER;

    DWORD size = length;
    long ret = RegGetValueA(lKey, NULL, name, RRF_RT_REG_SZ | RRF_NOEXPAND, NULL, out, &size);
    if (ret != ERROR_SUCCESS)
        out[0] = '\0';

    return ret;
}

bool netdata_registry_get_string(char *out, unsigned int length, void *hKey, char *subKey, char *name)
{
    HKEY lKey;
    bool status = true;
    long ret = RegOpenKeyEx(hKey,
                            subKey,
                            0,
                            KEY_READ,
                            &lKey);
    if (ret != ERROR_SUCCESS) {
        if (length)
            out[0] = '\0';
        return false;
    }

    ret = netdata_registry_get_string_from_open_key(out, length, lKey, name);
    if (ret != ERROR_SUCCESS)
        status = false;

    RegCloseKey(lKey);

    return status;
}

// --------------------------------------------------------------------------------------------------------------------
// unit test: -W registrytest

static bool registry_test_set(HKEY key, const char *name, DWORD type, const void *data, DWORD size) {
    if(RegSetValueExA(key, name, 0, type, (const BYTE *)data, size) == ERROR_SUCCESS)
        return true;

    fprintf(stderr, "REGISTRY TEST: cannot set value '%s'\n", name);
    return false;
}

// a REG_SZ stored without a terminating NUL: the byte after the data is not zero, and the stored size proves it
static bool registry_test_set_unterminated(HKEY key, const wchar_t *name, const wchar_t *chars, DWORD nchars) {
    wchar_t buf[16];
    for(size_t i = 0; i < _countof(buf); i++)
        buf[i] = L'Z';
    memcpy(buf, chars, nchars * sizeof(wchar_t));

    DWORD size = 0;
    if(RegSetValueExW(key, name, 0, REG_SZ, (const BYTE *)buf, nchars * sizeof(wchar_t)) != ERROR_SUCCESS ||
       RegQueryValueExW(key, name, NULL, NULL, NULL, &size) != ERROR_SUCCESS ||
       size != nchars * sizeof(wchar_t)) {
        fprintf(stderr, "REGISTRY TEST: cannot store an unterminated value\n");
        return false;
    }

    return true;
}

static int registry_test_string(HKEY key, const char *name, unsigned int length, bool expect_ok, const char *expect) {
    char buf[16];
    memset(buf, 'X', sizeof(buf));

    bool ok = netdata_registry_get_string_from_open_key(buf, length, key, (char *)name) == ERROR_SUCCESS;

    // the result must be terminated within the buffer, and empty on failure
    bool terminated = memchr(buf, '\0', length) != NULL;
    bool passed = ok == expect_ok && terminated && strcmp(buf, ok ? expect : "") == 0;

    fprintf(stderr, "REGISTRY TEST: string '%s' in %u bytes: %s, %s: %s\n",
            name, length, ok ? "read" : "not read", terminated ? "terminated" : "NOT TERMINATED", passed ? "OK" : "FAILED");
    return passed ? 0 : 1;
}

static int registry_test_dword(HKEY key, const char *name, bool expect_ok, unsigned int expect) {
    unsigned int value = 0xDEADBEEF;
    bool ok = netdata_registry_get_dword_from_open_key(&value, key, (char *)name) == ERROR_SUCCESS;

    // on failure the output must be left alone
    bool passed = ok == expect_ok && value == (ok ? expect : 0xDEADBEEF);

    fprintf(stderr, "REGISTRY TEST: dword '%s': %s (%u): %s\n", name, ok ? "read" : "not read", value, passed ? "OK" : "FAILED");
    return passed ? 0 : 1;
}

int netdata_registry_unittest(void) {
    char path[128];
    snprintfz(path, sizeof(path), "Software\\Netdata-registry-unittest-%u", os_random32());

    // volatile: the key does not survive a reboot, even if the test crashes
    HKEY key;
    DWORD disposition = 0;
    if(RegCreateKeyExA(HKEY_CURRENT_USER, path, 0, NULL, REG_OPTION_VOLATILE, KEY_ALL_ACCESS, NULL, &key, &disposition) != ERROR_SUCCESS) {
        fprintf(stderr, "REGISTRY TEST: cannot create HKCU\\%s\n", path);
        return 1;
    }
    if(disposition != REG_CREATED_NEW_KEY) {
        fprintf(stderr, "REGISTRY TEST: HKCU\\%s already exists\n", path);
        RegCloseKey(key);
        return 1;
    }

    const DWORD dword = 42;
    const uint8_t binary[4] = { 1, 2, 3, 4 };
    int errors = 0;

    if(!registry_test_set(key, "sz", REG_SZ, "hello", 6) ||
       !registry_test_set_unterminated(key, L"sz_no_nul", L"abcd", 4) ||
       !registry_test_set_unterminated(key, L"sz_no_nul_8", L"abcdefgh", 8) ||
       !registry_test_set(key, "multi_sz", REG_MULTI_SZ, "a\0b\0", 5) ||
       !registry_test_set(key, "dword", REG_DWORD, &dword, sizeof(dword)) ||
       !registry_test_set(key, "dword_2", REG_DWORD, &dword, 2) ||
       !registry_test_set(key, "binary4", REG_BINARY, binary, sizeof(binary)) ||
       !registry_test_set(key, "sz_4", REG_SZ, "123", 4))
        errors++;
    else {
        errors += registry_test_string(key, "sz", 16, true, "hello");
        errors += registry_test_string(key, "sz_no_nul", 16, true, "abcd");         // stored without NUL
        errors += registry_test_string(key, "sz_no_nul_8", 8, false, NULL);         // fills the buffer, no room for NUL
        errors += registry_test_string(key, "multi_sz", 16, false, NULL);           // wrong type
        errors += registry_test_string(key, "dword", 16, false, NULL);              // wrong type
        errors += registry_test_string(key, "missing", 16, false, NULL);
        errors += registry_test_dword(key, "dword", true, 42);
        errors += registry_test_dword(key, "dword_2", false, 0);                    // right type, short
        errors += registry_test_dword(key, "binary4", false, 0);                    // wrong type, right size
        errors += registry_test_dword(key, "sz_4", false, 0);                       // wrong type, right size
        errors += registry_test_dword(key, "missing", false, 0);
    }

    // a key that cannot be opened must leave an empty string
    {
        char buf[8];
        memset(buf, 'X', sizeof(buf));
        char missing[160];
        snprintfz(missing, sizeof(missing), "%s\\missing", path);
        bool ok = netdata_registry_get_string(buf, sizeof(buf), HKEY_CURRENT_USER, missing, "sz");
        bool passed = !ok && buf[0] == '\0';
        fprintf(stderr, "REGISTRY TEST: string from a missing key: %s, %s: %s\n",
                ok ? "read" : "not read", buf[0] == '\0' ? "empty" : "NOT EMPTY", passed ? "OK" : "FAILED");
        if(!passed) errors++;
    }

    RegCloseKey(key);
    if(RegDeleteKeyA(HKEY_CURRENT_USER, path) != ERROR_SUCCESS) {
        fprintf(stderr, "REGISTRY TEST: cannot delete HKCU\\%s\n", path);
        errors++;
    }

    fprintf(stderr, "REGISTRY TEST: %s\n", errors ? "FAILED" : "PASSED");
    return errors ? 1 : 0;
}

bool netdata_registry_key_exists(void *hKey, const char *subKey) {
    HKEY lKey;
    long ret = RegOpenKeyEx(hKey, subKey, 0, KEY_READ, &lKey);
    if (ret != ERROR_SUCCESS)
        return false;

    RegCloseKey(lKey);
    return true;
}

bool EnableWindowsPrivilege(const char *privilegeName) {
    HANDLE hToken;
    LUID luid;
    TOKEN_PRIVILEGES tkp;

    // Open the process token with appropriate access rights
    if (!OpenProcessToken(GetCurrentProcess(), TOKEN_ADJUST_PRIVILEGES | TOKEN_QUERY, &hToken))
        return false;

    // Lookup the LUID for the specified privilege
    if (!LookupPrivilegeValue(NULL, privilegeName, &luid)) {
        CloseHandle(hToken);  // Close the token handle before returning
        return false;
    }

    // Set up the TOKEN_PRIVILEGES structure
    tkp.PrivilegeCount = 1;
    tkp.Privileges[0].Luid = luid;
    tkp.Privileges[0].Attributes = SE_PRIVILEGE_ENABLED;

    // Adjust the token's privileges
    if (!AdjustTokenPrivileges(hToken, FALSE, &tkp, sizeof(tkp), NULL, NULL)) {
        CloseHandle(hToken);  // Close the token handle before returning
        return false;
    }

    // Check if AdjustTokenPrivileges succeeded
    if (GetLastError() == ERROR_NOT_ALL_ASSIGNED) {
        CloseHandle(hToken);  // Close the token handle before returning
        return false;
    }

    // Close the handle to the token after success
    CloseHandle(hToken);

    return true;
}

bool os_windows_service_is_running(const wchar_t *service_name, DWORD *current_state) {
    DWORD state = 0;

    if (service_name) {
        SC_HANDLE scm = OpenSCManagerW(NULL, NULL, SC_MANAGER_CONNECT);
        if (scm) {
            SC_HANDLE service = OpenServiceW(scm, service_name, SERVICE_QUERY_STATUS);
            if (service) {
                SERVICE_STATUS_PROCESS status;
                DWORD needed = 0;
                if (QueryServiceStatusEx(
                        service, SC_STATUS_PROCESS_INFO, (LPBYTE)&status, sizeof(status), &needed))
                    state = status.dwCurrentState;
                CloseServiceHandle(service);
            }
            CloseServiceHandle(scm);
        }
    }

    if (current_state)
        *current_state = state;

    return state == SERVICE_RUNNING;
}

#endif
