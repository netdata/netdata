// SPDX-License-Identifier: GPL-3.0-or-later

#include "libnetdata/libnetdata.h"

#if defined(OS_WINDOWS)
bool os_hostname(char *dst, size_t dst_size, const char *filesystem_root __maybe_unused) {
    WCHAR wbuf[HOST_NAME_MAX * 4 + 1];
    char buf[HOST_NAME_MAX * 4 + 1];
    DWORD size = _countof(wbuf);
    bool success = false;

    // First try DNS hostname
    if (GetComputerNameExW(ComputerNameDnsHostname, wbuf, &size)) {
        success = true;
    }
    // Then try NetBIOS name
    else {
        size = _countof(wbuf);
        if (GetComputerNameW(wbuf, &size)) {
            success = true;
        }
    }

    if (success) {
        // Convert UTF-16 to UTF-8
        int utf8_size = WideCharToMultiByte(CP_UTF8, 0, wbuf, -1, NULL, 0, NULL, NULL);
        if (utf8_size > 0 && utf8_size <= (int)sizeof(buf)) {
            WideCharToMultiByte(CP_UTF8, 0, wbuf, -1, buf, utf8_size, NULL, NULL);
        }
        else {
            success = false;
        }
    }

    if (!success) {
        // Try getting the machine GUID first
        HKEY hKey;
        if (RegOpenKeyExW(HKEY_LOCAL_MACHINE, L"SOFTWARE\\Microsoft\\Cryptography", 0, KEY_READ, &hKey) == ERROR_SUCCESS) {
            WCHAR guidW[64];
            DWORD guidSize = sizeof(guidW);
            DWORD type = REG_SZ;

            if (RegQueryValueExW(hKey, L"MachineGuid", NULL, &type, (LPBYTE)guidW, &guidSize) == ERROR_SUCCESS) {
                // Convert GUID to UTF-8
                if (WideCharToMultiByte(CP_UTF8, 0, guidW, -1, buf, sizeof(buf), NULL, NULL) > 0) {
                    success = true;
                }
            }
            RegCloseKey(hKey);
        }

        if (!success) {
            // If machine GUID fails, try getting volume serial number of C: drive
            WCHAR rootPath[] = L"C:\\";
            DWORD serialNumber;
            if (GetVolumeInformationW(rootPath, NULL, 0, &serialNumber, NULL, NULL, NULL, 0)) {
                snprintf(buf, sizeof(buf), "host%08lx", (long unsigned)serialNumber);
                success = true;
            }
            else {
                // Last resort: use process ID
                snprintf(buf, sizeof(buf), "host%lu", (long unsigned)GetCurrentProcessId());
            }
        }
    }

    char *hostname = trim(buf);
    rrdlabels_sanitize_value(dst, hostname, dst_size);
    return *dst != '\0';
}
#else // !OS_WINDOWS

#ifdef HAVE_LIBICONV
#include <iconv.h>

static const char *get_current_locale(void) {
    const char *locale = getenv("LC_ALL");  // LC_ALL overrides all other locale settings

    if (!locale || !*locale) {
        locale = getenv("LC_CTYPE");  // Check LC_CTYPE for character encoding

        if (!locale || !*locale)
            locale = getenv("LANG");  // Fallback to LANG
    }

    return locale;
}

static const char *get_encoding_from_locale(const char *locale) {
    if(!locale || !*locale)
        return NULL;

    const char *dot = strchr(locale, '.');
    if (dot)
        return dot + 1;

    return locale;
}

// Function to convert from a source encoding to UTF-8
static bool iconv_convert_to_utf8(const char *input, const char *src_encoding, char *output, size_t output_size) {
    iconv_t cd = iconv_open("UTF-8", src_encoding);
    if (cd == (iconv_t)-1) {
        int i = errno;
        return false;
    }

    char *input_ptr = (char *)input;    // iconv() may modify this pointer
    char *output_ptr = output;          // iconv() modifies this pointer
    size_t input_len = strlen(input);
    size_t output_len = output_size;

    // Perform the conversion
    if (iconv(cd, &input_ptr, &input_len, &output_ptr, &output_len) == (size_t)-1) {
        iconv_close(cd);
        return false;
    }

    // Null-terminate the output string
    *output_ptr = '\0';

    iconv_close(cd);
    return true;
}
#endif

bool os_hostname(char *dst, size_t dst_size, const char *filesystem_root) {
    *dst = '\0';

    char buf[HOST_NAME_MAX * 4 + 1];
    *buf = '\0';

    if (filesystem_root && *filesystem_root) {
        char filename[FILENAME_MAX + 1];
        snprintfz(filename, FILENAME_MAX, "%s/etc/hostname", netdata_configured_host_prefix);

        if (read_txt_file(filename, buf, sizeof(buf)))
            *buf = '\0';
    }

    if(!*buf && gethostname(buf, sizeof(buf)) != 0)
        snprintf(buf, sizeof(buf), "host%ld", gethostid());

    char *original_hostname = trim(buf);

#ifdef HAVE_LIBICONV
    const char *locale = get_current_locale();
    if (locale && *locale) {
        char utf8_output[HOST_NAME_MAX * 4 + 1] = "";
        if(iconv_convert_to_utf8(original_hostname, get_encoding_from_locale(locale), utf8_output, sizeof(utf8_output))) {
            rrdlabels_sanitize_value(dst, trim(utf8_output), dst_size);
            return *dst != '\0';
        }
    }
#endif

    rrdlabels_sanitize_value(dst, original_hostname, dst_size);
    return *dst != '\0';
}

#endif // !OS_WINDOWS
