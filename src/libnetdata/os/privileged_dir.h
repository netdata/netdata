// SPDX-License-Identifier: GPL-3.0-or-later

#ifndef NETDATA_PRIVILEGED_DIR_H
#define NETDATA_PRIVILEGED_DIR_H

#include "libnetdata/libnetdata.h"

// How much the directory containing a path can be trusted, i.e. which accounts
// are able to create, rename or delete the final component. This is what decides
// whether a symlink found at that name could have been planted by somebody else,
// and therefore whether a still-privileged netdata may follow it.
//
// "ours" below means owned by root or by our own effective uid - the two cases
// where nobody with less privilege than us could have placed the entry.
typedef enum {
    // the parent could not be examined - assume the worst
    OS_DIR_PARENT_UNKNOWN = 0,

    // ours and not writable by group or others: only we can put anything here,
    // so even a symlink here is one we placed
    OS_DIR_PARENT_EXCLUSIVE,

    // ours and sticky (/tmp): anyone can create entries, but only the owner of
    // an entry can rename or delete it
    OS_DIR_PARENT_STICKY,

    // somebody else can replace entries here at will
    OS_DIR_PARENT_UNSAFE,
} OS_DIR_PARENT_TRUST;

// Copy path into dst so that its final component names the entry itself ("/"
// stays "/"). Trailing slashes and trailing "." components are removed, in any
// combination and repetition.
// This MUST happen before any check that depends on the final component being
// the entry: with "dir/" or "dir/." the kernel resolves that component as a
// directory, which silently defeats both O_NOFOLLOW and lstat()'s S_ISLNK.
// Returns false when there is no entry left to name - a NULL/empty path, one
// whose final component is "." or ".." (those reach a directory *through* the
// component before them), or one that does not fit in dst.
bool os_dir_path_trim(const char *path, char *dst, size_t dst_size);

// Classify the directory that contains path's final component. Intermediate
// symlinks are resolved normally (/var/run -> /run); only the final component
// is treated as untrusted.
OS_DIR_PARENT_TRUST os_dir_parent_trust(const char *path);

// Open path as a directory descriptor, for a process that is about to act on it
// with root privileges. A symlink at the final component is followed only when
// the directory containing it is exclusively ours; everywhere else O_NOFOLLOW
// refuses it. Returns the descriptor (the caller closes it) or -1.
int os_open_dir_privileged(const char *path);

#endif //NETDATA_PRIVILEGED_DIR_H
