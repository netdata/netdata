// SPDX-License-Identifier: GPL-3.0-or-later

#include "libnetdata/libnetdata.h"

#include <sys/stat.h>

#if defined(OS_WINDOWS)
// Map a POSIX-style st_mode to DT_*. Windows reports S_IFDIR / S_IFREG /
// S_IFCHR via UCRT's stat(); it has no S_IFLNK, S_IFBLK, S_IFIFO, or
// S_IFSOCK -- those simply never match, which is the correct outcome here
// (a Windows directory walk shouldn't surface symlinks/blocks/etc. to
// callers that check those types).
static unsigned char os_dirent_type_from_mode(unsigned short mode) {
    if ((mode & S_IFMT) == S_IFDIR) return DT_DIR;
    if ((mode & S_IFMT) == S_IFREG) return DT_REG;
    if ((mode & S_IFMT) == S_IFCHR) return DT_CHR;
    return DT_UNKNOWN;
}

unsigned char os_dirent_type(const char *parent_path, const struct dirent *entry) {
    if (!entry)
        return DT_UNKNOWN;

    // Compose the absolute path and stat it. parent_path is allowed to be
    // NULL for callers that already chdir'd into the directory; in that
    // case we stat the name as-is.
    char fullpath[FILENAME_MAX + 1];
    if (parent_path && *parent_path)
        snprintfz(fullpath, FILENAME_MAX, "%s/%s", parent_path, entry->d_name);
    else
        strncpyz(fullpath, entry->d_name, FILENAME_MAX);

    struct stat st;
    if (stat(fullpath, &st) != 0)
        return DT_UNKNOWN;

    return os_dirent_type_from_mode(st.st_mode);
}

#else
unsigned char os_dirent_type(const char *parent_path, const struct dirent *entry) {
    (void)parent_path;
    if (!entry)
        return DT_UNKNOWN;
    return (unsigned char)entry->d_type;
}
#endif
