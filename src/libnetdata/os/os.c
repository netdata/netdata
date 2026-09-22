// SPDX-License-Identifier: GPL-3.0-or-later

#include "../libnetdata.h"

// ----------------------------------------------------------------------------
// system functions
// to retrieve settings of the system

unsigned int system_hz = 100;
void os_get_system_HZ(void) {
    long ticks;

    if ((ticks = sysconf(_SC_CLK_TCK)) <= 0) {
        netdata_log_error("Cannot get system clock ticks");
        ticks = 100;
    }

    system_hz = (unsigned int) ticks;
}

// =====================================================================================================================
// os_type

#if defined(OS_LINUX)
const char *os_type = "linux";
#endif

#if defined(OS_FREEBSD)
const char *os_type = "freebsd";
#endif

#if defined(OS_MACOS)
const char *os_type = "macos";
#endif

#if defined(OS_WINDOWS)
const char *os_type = "windows";

#define OS_WINDOWS_PATH_TRANSLATION_MAX 8191

static char *os_translate_windows_path_fallback(const char *src, const char *package_prefix) {
    size_t src_len = strnlen(src, OS_WINDOWS_PATH_TRANSLATION_MAX);
    bool package_relative_posix_path = package_prefix != NULL;
    // NOSONAR (c:S5813) — package_prefix is NULL-tested on the previous line; strlen is bounded by the caller's contract.
    size_t prefix_len = package_relative_posix_path ? strlen(package_prefix) : 0;
    size_t converted_size = prefix_len + src_len + 3;
    char *converted_path = mallocz(converted_size);
    size_t i = 0;
    size_t j = 0;

    if (package_relative_posix_path) {
        for (; j < prefix_len; j++)
            converted_path[j] = (package_prefix[j] == '/') ? '\\' : package_prefix[j];
    }
    else if (src_len >= 2 && isalpha((unsigned char)src[0]) && src[1] == ':') {
        converted_path[j++] = (char)toupper((unsigned char)src[0]);
        converted_path[j++] = ':';
        i = 2;

        if (src[i] == '\\' || src[i] == '/') {
            converted_path[j++] = '\\';
            i++;
        }
    }
    else if (src_len >= 2 && src[0] == '/' && isalpha((unsigned char)src[1]) && (src_len == 2 || src[2] == '/')) {
        converted_path[j++] = (char)toupper((unsigned char)src[1]);
        converted_path[j++] = ':';
        i = 2;

        if (src_len == 2 || src[i] == '/') {
            converted_path[j++] = '\\';
            if (src[i] == '/')
                i++;
        }
    }
    else if (src_len >= 2 && ((src[0] == '\\' && src[1] == '\\') || (src[0] == '/' && src[1] == '/'))) {
        converted_path[j++] = '\\';
        converted_path[j++] = '\\';
        i = 2;
    }

    for (; i < src_len && j < converted_size - 1; i++)
        converted_path[j++] = (src[i] == '/') ? '\\' : src[i];

    converted_path[j] = '\0';
    return converted_path;
}

char *os_translate_msys_to_windows_path(const char *src) {
    if (!src)
        return strdupz("");

    if (!*src)
        return strdupz("");

    if (src[0] == '/') {
#if defined(__CYGWIN__) || defined(__MSYS__)
        ssize_t converted_size = cygwin_conv_path(CCP_POSIX_TO_WIN_A, src, NULL, 0);
        if (converted_size > 0) {
            char *converted_path = mallocz((size_t)converted_size);
            if (cygwin_conv_path(CCP_POSIX_TO_WIN_A, src, converted_path, (size_t)converted_size) == 0)
                return converted_path;

            freez(converted_path);
        }
#endif
    }

    const char *package_prefix = NULL;
#if !defined(__CYGWIN__) && !defined(__MSYS__)
    size_t src_len = strnlen(src, OS_WINDOWS_PATH_TRANSLATION_MAX);
    bool package_relative_posix_path = src[0] == '/' &&
        !(src_len >= 2 && isalpha((unsigned char)src[1]) && (src_len == 2 || src[2] == '/')) &&
        !(src_len >= 2 && src[1] == '/');
    CLEAN_CHAR_P *runtime_prefix = NULL;
    if (package_relative_posix_path) {
        // UCRT64 has no POSIX mount table, so package paths are relative to
        // the installed prefix instead of the current drive root.
        runtime_prefix = nd_windows_detect_install_prefix();
        package_prefix = runtime_prefix ? runtime_prefix : NETDATA_WINDOWS_PATH_PREFIX;
    }
#endif
    return os_translate_windows_path_fallback(src, package_prefix);
}

wchar_t *os_translate_msys_to_windows_pathW(const char *src) {
    if (!src)
        return NULL;

#if defined(__CYGWIN__) || defined(__MSYS__)
    ssize_t cygwin_size = cygwin_conv_path(CCP_POSIX_TO_WIN_W, src, NULL, 0);
    if (cygwin_size > 0) {
        wchar_t *converted_path = mallocz((size_t)cygwin_size);
        if (cygwin_conv_path(CCP_POSIX_TO_WIN_W, src, converted_path, (size_t)cygwin_size) == 0)
            return converted_path;

        freez(converted_path);
    }

    // Absolute POSIX paths require the MSYS/Cygwin runtime's mount translation.
    if (src[0] == '/')
        return NULL;
#endif

    CLEAN_CHAR_P *translated = os_translate_msys_to_windows_path(src);
    int converted_size = MultiByteToWideChar(CP_UTF8, 0, translated, -1, NULL, 0);
    if (converted_size <= 0)
        return NULL;

    wchar_t *converted_path = mallocz((size_t)converted_size * sizeof(*converted_path));
    if (MultiByteToWideChar(CP_UTF8, 0, translated, -1, converted_path, converted_size) <= 0) {
        freez(converted_path);
        return NULL;
    }

    return converted_path;
}

// Build a protected (non-inheriting) security descriptor from a POSIX mode.
//
// SYSTEM and the local Administrators group always keep full access, so the
// service and an elevated operator can always read and repair the file.
//
// `owner_sid` receives the read/write access the mode asks for, plus
// READ_CONTROL, WRITE_DAC and DELETE for *every* mode. Those three are not
// optional: Windows requires DELETE on the source of a rename, so an owner
// without it cannot replace its own file (machine-guid.c chmods to 0444 and
// then renames), and an owner without WRITE_DAC could never repair the ACL
// afterwards.
//
// POSIX group/other bits are deliberately not mapped. Windows has no POSIX
// group equivalent for these files, and every caller here writes either a
// secret or agent state, so deny-by-default is the safe direction.
static PSECURITY_DESCRIPTOR nd_windows_sd_from_mode(int mode, PSID owner_sid) {
    LPWSTR owner_str = NULL;
    if (!ConvertSidToStringSidW(owner_sid, &owner_str))
        return NULL;

    // RC/SD/WD (READ_CONTROL, DELETE, WRITE_DAC) are present in every variant;
    // only the data access follows the mode.
    const wchar_t *owner_rights =
        (mode & 0600) == 0600 ? L"FRFWRCSDWD" :
        (mode & 0400)         ? L"FRRCSDWD"   :
        (mode & 0200)         ? L"FWRCSDWD"   :
                                L"RCSDWD";

    size_t needed = 64 + wcslen(owner_rights) + wcslen(owner_str);
    wchar_t *sddl = mallocz(needed * sizeof(*sddl));
    swprintf(sddl, needed, L"D:P(A;;FA;;;SY)(A;;FA;;;BA)(A;;%ls;;;%ls)", owner_rights, owner_str);

    PSECURITY_DESCRIPTOR descriptor = NULL;
    bool ok = ConvertStringSecurityDescriptorToSecurityDescriptorW(
                  sddl, SDDL_REVISION_1, &descriptor, NULL) != 0;
    freez(sddl);
    LocalFree(owner_str);
    return ok ? descriptor : NULL;
}

// The SID that will own a file this process creates. Returns NULL on failure;
// on success `*token_user` receives the allocation the SID points into and the
// caller must freez() it once the SID is no longer needed.
static PSID nd_windows_process_user_sid(TOKEN_USER **token_user) {
    *token_user = NULL;

    HANDLE token = NULL;
    if (!OpenProcessToken(GetCurrentProcess(), TOKEN_QUERY, &token))
        return NULL;

    DWORD size = 0;
    GetTokenInformation(token, TokenUser, NULL, 0, &size);
    if (!size) {
        CloseHandle(token);
        return NULL;
    }

    TOKEN_USER *info = mallocz(size);
    if (!GetTokenInformation(token, TokenUser, info, size, &size)) {
        freez(info);
        CloseHandle(token);
        return NULL;
    }

    CloseHandle(token);
    *token_user = info;
    return info->User.Sid;
}

int nd_open_no_follow(const char *path, int flags, int mode) {
    wchar_t *native_path = os_translate_msys_to_windows_pathW(path);
    if (!native_path) {
        errno = EINVAL;
        return -1;
    }

    DWORD access = (flags & O_ACCMODE) == O_RDWR ? GENERIC_READ | GENERIC_WRITE :
                   (flags & O_ACCMODE) == O_WRONLY ? GENERIC_WRITE : GENERIC_READ;
    DWORD creation = (flags & O_CREAT) ?
                     ((flags & O_EXCL) ? CREATE_NEW :
                      ((flags & O_TRUNC) ? CREATE_ALWAYS : OPEN_ALWAYS)) :
                     ((flags & O_TRUNC) ? TRUNCATE_EXISTING : OPEN_EXISTING);

    // CreateFileW only honours the descriptor when it creates the file, so the
    // mode is applied at creation rather than left to a later fchmod(). This is
    // what keeps a freshly created secret (the management API key) from
    // inheriting the parent directory's ACL.
    SECURITY_ATTRIBUTES security_attributes;
    LPSECURITY_ATTRIBUTES security = NULL;
    PSECURITY_DESCRIPTOR descriptor = NULL;
    TOKEN_USER *token_user = NULL;
    if ((flags & O_CREAT) && mode) {
        PSID owner_sid = nd_windows_process_user_sid(&token_user);
        if (!owner_sid) {
            freez(native_path);
            errno = EACCES;
            return -1;
        }

        descriptor = nd_windows_sd_from_mode(mode, owner_sid);
        freez(token_user);
        if (!descriptor) {
            freez(native_path);
            errno = EACCES;
            return -1;
        }

        security_attributes.nLength = sizeof(security_attributes);
        security_attributes.lpSecurityDescriptor = descriptor;
        security_attributes.bInheritHandle = FALSE;
        security = &security_attributes;
    }

    HANDLE handle = CreateFileW(native_path, access,
                                FILE_SHARE_READ | FILE_SHARE_WRITE | FILE_SHARE_DELETE,
                                security, creation,
                                FILE_ATTRIBUTE_NORMAL | FILE_FLAG_OPEN_REPARSE_POINT,
                                NULL);
    if (descriptor)
        LocalFree(descriptor);
    freez(native_path);
    if (handle == INVALID_HANDLE_VALUE) {
        switch (GetLastError()) {
            case ERROR_FILE_EXISTS:
            case ERROR_ALREADY_EXISTS:
                errno = EEXIST;
                break;
            case ERROR_FILE_NOT_FOUND:
            case ERROR_PATH_NOT_FOUND:
                errno = ENOENT;
                break;
            case ERROR_ACCESS_DENIED:
            case ERROR_SHARING_VIOLATION:
                errno = EACCES;
                break;
            default:
                errno = EIO;
                break;
        }
        return -1;
    }

    BY_HANDLE_FILE_INFORMATION info;
    if (!GetFileInformationByHandle(handle, &info) ||
        (info.dwFileAttributes & FILE_ATTRIBUTE_REPARSE_POINT)) {
        CloseHandle(handle);
        errno = ELOOP;
        return -1;
    }

    int crt_flags = flags & (O_ACCMODE | O_APPEND);
    crt_flags |= O_BINARY;
    int fd = _open_osfhandle((intptr_t)handle, crt_flags);
    if (fd == -1) {
        CloseHandle(handle);
        return -1;
    }

    if (flags & O_CLOEXEC)
        SetHandleInformation((HANDLE)_get_osfhandle(fd), HANDLE_FLAG_INHERIT, 0);

    return fd;
}

// POSIX fd-based permission bits. Resolves the file's real owner and applies a
// protected DACL derived from `mode`; see nd_windows_sd_from_mode().
int fchmod(int fd, int mode) {
    intptr_t raw_handle = _get_osfhandle(fd);
    if (raw_handle == -1) {
        errno = EBADF;
        return -1;
    }

    // SetSecurityInfo() requires WRITE_DAC on the handle, and the CRT's open()
    // never requests it, so a descriptor that came from open()/_open() cannot be
    // re-permissioned directly. ReOpenFile() derives a new handle with the rights
    // we need from the existing one -- without a path, so there is no window for
    // the file to be swapped underneath us. Fall back to the original handle for
    // descriptors that already carry WRITE_DAC.
    HANDLE original = (HANDLE)raw_handle;
    HANDLE handle = ReOpenFile(original, READ_CONTROL | WRITE_DAC,
                               FILE_SHARE_READ | FILE_SHARE_WRITE | FILE_SHARE_DELETE, 0);
    bool reopened = handle != INVALID_HANDLE_VALUE;
    if (!reopened)
        handle = original;

    // The owner must be resolved from the file itself: the well-known OWNER
    // RIGHTS SID is not a substitute, because when it is present in a DACL it
    // *replaces* the owner's implicit READ_CONTROL/WRITE_DAC rather than
    // granting them.
    PSID owner_sid = NULL;
    PSECURITY_DESCRIPTOR owner_descriptor = NULL;
    if (GetSecurityInfo(handle, SE_FILE_OBJECT, OWNER_SECURITY_INFORMATION,
                        &owner_sid, NULL, NULL, NULL, &owner_descriptor) != ERROR_SUCCESS ||
        !owner_sid) {
        if (reopened)
            CloseHandle(handle);
        errno = EACCES;
        return -1;
    }

    PSECURITY_DESCRIPTOR descriptor = nd_windows_sd_from_mode(mode, owner_sid);
    if (owner_descriptor)
        LocalFree(owner_descriptor);
    if (!descriptor) {
        if (reopened)
            CloseHandle(handle);
        errno = EACCES;
        return -1;
    }

    BOOL present = FALSE;
    BOOL defaulted = FALSE;
    PACL dacl = NULL;
    bool valid = GetSecurityDescriptorDacl(descriptor, &present, &dacl, &defaulted) && present && dacl;
    DWORD status = valid
        ? SetSecurityInfo(handle, SE_FILE_OBJECT,
                          DACL_SECURITY_INFORMATION | PROTECTED_DACL_SECURITY_INFORMATION,
                          NULL, NULL, dacl, NULL)
        : ERROR_INVALID_SECURITY_DESCR;
    LocalFree(descriptor);
    if (reopened)
        CloseHandle(handle);

    if (status != ERROR_SUCCESS) {
        errno = (status == ERROR_ACCESS_DENIED) ? EACCES : EIO;
        return -1;
    }

    return 0;
}

// POSIX dir-relative timestamp update. machine-guid.c uses it to preserve the
// GUID file's recorded modification time across a rewrite.
int utimensat(int dirfd, const char *pathname, const struct timespec times[2], int flags) {
    if (dirfd != AT_FDCWD || !pathname || !times || flags != 0) {
        errno = EINVAL;
        return -1;
    }

    wchar_t *native_path = os_translate_msys_to_windows_pathW(pathname);
    if (!native_path) {
        errno = EINVAL;
        return -1;
    }

    HANDLE handle = CreateFileW(native_path, FILE_WRITE_ATTRIBUTES,
                                FILE_SHARE_READ | FILE_SHARE_WRITE | FILE_SHARE_DELETE,
                                NULL, OPEN_EXISTING,
                                FILE_ATTRIBUTE_NORMAL | FILE_FLAG_OPEN_REPARSE_POINT,
                                NULL);
    freez(native_path);
    if (handle == INVALID_HANDLE_VALUE) {
        errno = EIO;
        return -1;
    }

    BY_HANDLE_FILE_INFORMATION info;
    if (!GetFileInformationByHandle(handle, &info) ||
        (info.dwFileAttributes & FILE_ATTRIBUTE_REPARSE_POINT)) {
        CloseHandle(handle);
        errno = ELOOP;
        return -1;
    }

    const uint64_t windows_epoch_offset = 116444736000000000ULL;
    FILETIME file_times[2];
    for (size_t i = 0; i < 2; i++) {
        if (times[i].tv_sec < 0 || times[i].tv_nsec < 0 || times[i].tv_nsec >= 1000000000L) {
            CloseHandle(handle);
            errno = EINVAL;
            return -1;
        }

        uint64_t value = (uint64_t)times[i].tv_sec * 10000000ULL +
                         (uint64_t)times[i].tv_nsec / 100ULL + windows_epoch_offset;
        ULARGE_INTEGER integer;
        integer.QuadPart = value;
        file_times[i].dwLowDateTime = integer.LowPart;
        file_times[i].dwHighDateTime = integer.HighPart;
    }

    bool success = SetFileTime(handle, NULL, &file_times[0], &file_times[1]) != 0;
    DWORD error = success ? ERROR_SUCCESS : GetLastError();
    CloseHandle(handle);
    if (!success) {
        errno = (error == ERROR_ACCESS_DENIED) ? EACCES : EIO;
        return -1;
    }

    return 0;
}

char *os_translate_path(char *dst, const char *src, size_t dst_size) {
    if (!dst || !dst_size)
        return dst;

    if (!src) {
        dst[0] = '\0';
        return dst;
    }

    CLEAN_CHAR_P *translated = os_translate_msys_to_windows_path(src);
    snprintfz(dst, dst_size, "%s", translated);
    return dst;
}

int os_windows_path_translation_unittest(void) {
#if !defined(__CYGWIN__) && !defined(__MSYS__)
    static const struct {
        const char *input;
        const char *expected_suffix;
        bool use_package_prefix;
    } cases[] = {
        { "/usr/share/netdata/web", "\\usr\\share\\netdata\\web", true },
        { "/etc/netdata", "\\etc\\netdata", true },
        { "/c", "C:\\", false },
        { "/c/custom", "C:\\custom", false },
        { "C:/custom", "C:\\custom", false },
        { "//server/share", "\\\\server\\share", false },
    };

    int errors = 0;
    CLEAN_CHAR_P *relocated_prefix = nd_windows_install_prefix_from_executable_path(
        "D:\\Relocated Netdata\\usr\\bin\\netdata.exe");
    if (!relocated_prefix || strcmp(relocated_prefix, "D:/Relocated Netdata") != 0) {
        fprintf(stderr, "  FAILED runtime prefix derivation from relocated executable\n");
        return 1;
    }

    nd_windows_set_install_prefix_for_unittest(relocated_prefix);
    for (size_t i = 0; i < sizeof(cases) / sizeof(cases[0]); i++) {
        CLEAN_CHAR_P *translated = os_translate_msys_to_windows_path(cases[i].input);
        const char *expected_prefix = cases[i].use_package_prefix ? "D:\\Relocated Netdata" : "";
        // NOSONAR (c:S5813) — expected_prefix and cases[i].expected_suffix are constant string literals; lengths are bounded.
        size_t expected_size = strlen(expected_prefix) + strlen(cases[i].expected_suffix) + 1; // NOSONAR (c:S5813)
        CLEAN_CHAR_P *expected = mallocz(expected_size);
        if (cases[i].use_package_prefix)
            snprintfz(expected,
                      expected_size,
                      "%s%s", expected_prefix, cases[i].expected_suffix);
        else
            snprintfz(expected,
                      expected_size,
                      "%s", cases[i].expected_suffix);

        for (char *p = expected; *p; p++)
            if (*p == '/') *p = '\\';

        if (strcmp(translated, expected) != 0) {
            fprintf(stderr, "  FAILED path translation for '%s': expected '%s', got '%s'\n",
                    cases[i].input, expected, translated);
            errors++;
        }
    }
    nd_windows_set_install_prefix_for_unittest(NULL);

    fprintf(stderr, "%s() %s\n", __FUNCTION__, errors ? "FAILED" : "passed");
    return errors;
#else
    return 0;
#endif
}

char *os_translate_windows_to_msys_path(const char *src) {
    if (!src)
        return strdupz("");

    // Keep already POSIX-style paths unchanged.
    if (src[0] == '/')
        return strdupz(src);

#if defined(__CYGWIN__) || defined(__MSYS__)
    ssize_t converted_size = cygwin_conv_path(CCP_WIN_A_TO_POSIX, src, NULL, 0);
    if (converted_size > 0) {
        char *converted_path = mallocz((size_t)converted_size);
        if (cygwin_conv_path(CCP_WIN_A_TO_POSIX, src, converted_path, (size_t)converted_size) == 0)
            return converted_path;

        freez(converted_path);
    }
#endif

    size_t src_len = strnlen(src, OS_WINDOWS_PATH_TRANSLATION_MAX);
    char *converted_path = mallocz(src_len + 3);
    size_t converted_size_fallback = src_len + 3;
    size_t i = 0;
    size_t j = 0;

    if (src_len >= 2 && isalpha((unsigned char)src[0]) && src[1] == ':') {
        converted_path[j++] = '/';
        converted_path[j++] = (char)tolower((unsigned char)src[0]);

        i = 2;
        if (src[i] == '\\' || src[i] == '/') {
            converted_path[j++] = '/';
            i++; // consume the separator so the loop below doesn't emit it again
        }
    }
    else if (src_len >= 2 && ((src[0] == '\\' && src[1] == '\\') || (src[0] == '/' && src[1] == '/'))) {
        converted_path[j++] = '/';
        converted_path[j++] = '/';
        i = 2;
    }

    for (; i < src_len && j < converted_size_fallback - 1; i++)
        converted_path[j++] = (src[i] == '\\') ? '/' : src[i];

    converted_path[j] = '\0';
    return converted_path;
}

#endif
