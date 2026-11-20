// SPDX-License-Identifier: GPL-3.0-or-later

#include "privileged_dir.h"

// not defined on all build targets; 0 makes the sticky case degrade to UNSAFE
#ifndef S_ISVTX
#define S_ISVTX 0
#endif

bool os_dir_path_trim(const char *path, char *dst, size_t dst_size) {
    if(!path || !*path || !dst || !dst_size) {
        errno = EINVAL;
        return false;
    }

    size_t len = strlen(path);

    // Remove everything that would make the kernel resolve the final component
    // as a directory rather than look at the entry itself. Both trailing
    // slashes and trailing "." components do that, and they can be interleaved
    // and repeated ("evil/.", "evil/./", "evil/.//./"), so keep going until
    // neither applies.
    for(bool trimmed = true; trimmed; ) {
        trimmed = false;

        while(len > 1 && path[len - 1] == '/') {
            len--;
            trimmed = true;
        }

        if(len >= 2 && path[len - 1] == '.' && path[len - 2] == '/') {
            len--;
            trimmed = true;
        }
    }

    // What is left must name an entry. A final "." or ".." names a directory
    // *through* the component before it, so there is nothing left to check and
    // we would end up judging the wrong inode - refuse instead.
    size_t cut = len;
    while(cut > 0 && path[cut - 1] != '/')
        cut--;

    size_t component = len - cut;
    if((component == 1 && path[cut] == '.') ||
       (component == 2 && path[cut] == '.' && path[cut + 1] == '.')) {
        errno = EINVAL;
        return false;
    }

    if(len >= dst_size) {
        errno = ENAMETOOLONG;
        return false;
    }

    memcpy(dst, path, len);
    dst[len] = '\0';
    return true;
}

// Split a trimmed path into the directory holding its final component and that
// component. Returns false for "/", which is its own parent and has no final
// component to judge.
static bool os_dir_split_parent(const char *dir, char *parent, size_t parent_size, const char **leaf) {
    // walk back to the slash separating the final component
    size_t cut = strlen(dir);
    while(cut > 0 && dir[cut - 1] != '/')
        cut--;

    if(cut == 0) {
        // a relative path with no slash at all - the parent is the cwd
        if(parent_size < 2) return false;
        parent[0] = '.';
        parent[1] = '\0';
    }
    else if(cut == 1) {
        if(!dir[1]) return false; // dir is "/" itself
        if(parent_size < 2) return false;
        parent[0] = '/';
        parent[1] = '\0';
    }
    else {
        size_t plen = cut - 1; // drop the separating slash
        if(plen >= parent_size) return false;
        memcpy(parent, dir, plen);
        parent[plen] = '\0';
    }

    *leaf = dir + cut;
    return true;
}

// The trust rules, applied to a directory we have already examined.
static OS_DIR_PARENT_TRUST os_dir_trust_of_stat(const struct stat *st) {
    // root, or ourselves when netdata runs unprivileged: in both cases nobody
    // with less privilege than us can create an entry here
    if(st->st_uid != 0 && st->st_uid != geteuid())
        return OS_DIR_PARENT_UNSAFE;

    if(!(st->st_mode & (S_IWGRP | S_IWOTH)))
        return OS_DIR_PARENT_EXCLUSIVE;

    if(st->st_mode & S_ISVTX)
        return OS_DIR_PARENT_STICKY;

    return OS_DIR_PARENT_UNSAFE;
}

OS_DIR_PARENT_TRUST os_dir_parent_trust(const char *path) {
    // Trim first, always: without it "<symlink>/." would have us compute the
    // string parent "<symlink>" and then stat() straight through it, so the
    // trust class would be read off the symlink's target instead of the
    // directory that actually holds the entry.
    char dir[FILENAME_MAX + 1];
    if(!os_dir_path_trim(path, dir, sizeof(dir)))
        return OS_DIR_PARENT_UNKNOWN;

    char parent[FILENAME_MAX + 1];
    const char *leaf;
    if(!os_dir_split_parent(dir, parent, sizeof(parent), &leaf)) {
        // "/" is its own parent
        parent[0] = '/';
        parent[1] = '\0';
    }

    struct stat st;
    if(stat(parent, &st) == -1)
        return OS_DIR_PARENT_UNKNOWN;

    return os_dir_trust_of_stat(&st);
}

int os_open_dir_privileged(const char *path) {
    // A configured path may carry a trailing slash, which would make O_NOFOLLOW
    // below a no-op. Decide and open on the trimmed path, never on the original.
    char dir[FILENAME_MAX + 1];
    if(!os_dir_path_trim(path, dir, sizeof(dir)))
        // errno is already EINVAL (nothing left to name) or ENAMETOOLONG
        return -1;

    int flags = O_RDONLY | O_DIRECTORY | O_CLOEXEC;

    char parent[FILENAME_MAX + 1];
    const char *leaf;
    if(!os_dir_split_parent(dir, parent, sizeof(parent), &leaf))
        return open(dir, flags); // dir is "/": there is no final component to protect

    // Judge and use the same directory. Deciding on stat(parent) and then
    // open()ing the whole path resolves the intermediate components a second
    // time, so the directory we trusted need not be the one the final component
    // is then looked up in: anyone able to retarget or rename a component above
    // the parent can make the two disagree. Holding one descriptor and looking
    // the entry up through it removes that gap.
    int parent_fd = open(parent, O_RDONLY | O_DIRECTORY | O_CLOEXEC);
    if(parent_fd == -1)
        return -1;

    struct stat st;
    if(fstat(parent_fd, &st) == -1) {
        int e = errno;
        close(parent_fd);
        errno = e;
        return -1;
    }

    // Following a symlink at the final component is only safe when we are the
    // only account that could have put it there. So /var/cache/netdata may point
    // to another filesystem (its parent /var/cache is root-only), while
    // /var/lib/netdata/cloud.d may not (its parent is writable by netdata).
    if(os_dir_trust_of_stat(&st) != OS_DIR_PARENT_EXCLUSIVE)
        flags |= O_NOFOLLOW;

    // os_dir_path_trim() guarantees the final component is neither "." nor "..",
    // so this cannot be pointed back at the parent or above it.
    int fd = openat(parent_fd, leaf, flags);
    int e = errno;
    close(parent_fd);
    errno = e;
    return fd;
}
