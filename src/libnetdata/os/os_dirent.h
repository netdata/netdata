// SPDX-License-Identifier: GPL-3.0-or-later

#ifndef NETDATA_OS_DIRENT_H
#define NETDATA_OS_DIRENT_H

#include "libnetdata/libnetdata.h"

#include <dirent.h>

#if defined(OS_WINDOWS)
// mingw-w64's <dirent.h> exposes only a minimal POSIX dirent (no d_type,
// no DT_* constants). Define the ones netdata uses so cross-platform code
// can keep referring to them; use the Linux numeric values for consistency.
#ifndef DT_UNKNOWN
#define DT_UNKNOWN 0
#endif
#ifndef DT_FIFO
#define DT_FIFO    1
#endif
#ifndef DT_CHR
#define DT_CHR     2
#endif
#ifndef DT_DIR
#define DT_DIR     4
#endif
#ifndef DT_BLK
#define DT_BLK     6
#endif
#ifndef DT_REG
#define DT_REG     8
#endif
#ifndef DT_LNK
#define DT_LNK    10
#endif
#ifndef DT_SOCK
#define DT_SOCK   12
#endif
#endif

// Portable replacement for `entry->d_type`.
//
// On POSIX (Linux, FreeBSD, macOS) returns `entry->d_type` directly --
// behaviour is unchanged from a direct field access, including the
// possibility of returning DT_UNKNOWN on filesystems that don't track the
// type (callers that care already handle DT_UNKNOWN).
//
// On Windows there is no d_type field at all (mingw-w64's dirent is the
// minimal POSIX form). We stat `parent_path/entry->d_name` and map
// st_mode back to DT_*, returning DT_UNKNOWN when the stat fails.
unsigned char os_dirent_type(const char *parent_path, const struct dirent *entry);

#endif //NETDATA_OS_DIRENT_H
