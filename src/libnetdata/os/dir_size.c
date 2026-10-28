// SPDX-License-Identifier: GPL-3.0-or-later

#include "libnetdata/libnetdata.h"
#include "dir_size.h"

#include <dirent.h>
#include <sys/stat.h>
#include <unistd.h>
#include <string.h>
#include <errno.h>

// State shared by every level of one traversal.
// Keeping the path and the stat buffers here, instead of on the stack of each level,
// makes a level cost a few dozen bytes, so deep trees fit in small thread stacks.
typedef struct {
    SIMPLE_PATTERN *pattern;
    size_t max_depth;
    DIR_SIZE *result;
    DICTIONARY *visited_inodes;     // directories already counted, to avoid loops

    size_t root_len;                // path + root_len is the path relative to the root
    struct stat st;                 // the stat of the entry at path; not valid across recursion
    char key[UINT64_MAX_LENGTH * 2 + 1];
    char path[FILENAME_MAX + 1];
} DIR_SIZE_WALK;

// path[0 .. len) is a directory and w->st holds its stat
static void dir_size_walk(DIR_SIZE_WALK *w, size_t len, size_t depth) {
    if (w->max_depth > 0 && depth > w->max_depth)
        return;

    if (depth > w->result->depth)
        w->result->depth = depth;

    snprintfz(w->key, sizeof(w->key), "%" PRIu64 "_%" PRIu64, (uint64_t)w->st.st_ino, (uint64_t)w->st.st_dev);
    if (dictionary_get(w->visited_inodes, w->key))
        return;
    dictionary_set(w->visited_inodes, w->key, NULL, sizeof(void *));

    w->result->directories++;

    DIR *dir = opendir(w->path);
    if (!dir) {
        w->result->errors++;
        return;
    }

    size_t sep = (len && w->path[len - 1] == '/') ? 0 : 1;

    struct dirent *entry;
    while ((entry = readdir(dir)) != NULL) {
        if (strcmp(entry->d_name, ".") == 0 || strcmp(entry->d_name, "..") == 0)
            continue;

        size_t name_len = strlen(entry->d_name);
        if (len + sep + name_len > FILENAME_MAX) {
            // the path does not fit; count it instead of truncating it
            w->result->errors++;
            continue;
        }

        if (sep)
            w->path[len] = '/';
        memcpy(&w->path[len + sep], entry->d_name, name_len + 1);

        if (lstat(w->path, &w->st) != 0) {
            w->result->errors++;
            continue;
        }

        if (S_ISDIR(w->st.st_mode))
            dir_size_walk(w, len + sep + name_len, depth + 1);

        else if (S_ISREG(w->st.st_mode)) {
            // files are filtered by their path relative to the root
            if (w->pattern && !simple_pattern_matches(w->pattern, &w->path[w->root_len]))
                continue;

            w->result->files++;
            w->result->bytes += w->st.st_size;
        }
        // other file types (symlinks, etc.) are not counted
    }

    closedir(dir);
    w->path[len] = '\0';
}

DIR_SIZE dir_size(const char *path, SIMPLE_PATTERN *pattern, size_t max_depth) {
    DIR_SIZE result = DIR_SIZE_EMPTY;

    if (!path || !*path)
        return result;

    size_t len = strlen(path);
    if (len > FILENAME_MAX) {
        result.errors++;
        return result;
    }

    DIR_SIZE_WALK *w = callocz(1, sizeof(*w));
    w->pattern = pattern;
    w->max_depth = max_depth;
    w->result = &result;
    memcpy(w->path, path, len + 1);
    w->root_len = (path[len - 1] == '/') ? len : len + 1;

    // the root itself may be a symlink, so follow it
    if (stat(path, &w->st) != 0)
        result.errors++;

    else if (S_ISDIR(w->st.st_mode)) {
        w->visited_inodes = dictionary_create(DICT_OPTION_SINGLE_THREADED);
        dir_size_walk(w, len, 0);
        dictionary_destroy(w->visited_inodes);
    }

    else if (S_ISREG(w->st.st_mode)) {
        // a single file is filtered by its filename
        const char *filename = strrchr(path, '/');
        filename = filename ? filename + 1 : path;

        if (!pattern || simple_pattern_matches(pattern, filename)) {
            result.files = 1;
            result.bytes = w->st.st_size;
        }
    }

    freez(w);
    return result;
}

DIR_SIZE dir_size_multiple(const char **paths, int num_paths, SIMPLE_PATTERN *pattern, size_t max_depth) {
    DIR_SIZE result = DIR_SIZE_EMPTY;
    
    if (!paths || num_paths <= 0)
        return result;

    // Calculate size for each path and combine results
    for (int i = 0; i < num_paths; i++) {
        if (!paths[i] || !*paths[i])
            continue;
            
        DIR_SIZE path_result = dir_size(paths[i], pattern, max_depth);
        
        // Combine results
        result.bytes += path_result.bytes;
        result.files += path_result.files;
        result.directories += path_result.directories;
        result.errors += path_result.errors;
        
        // Take the maximum depth found
        if (path_result.depth > result.depth)
            result.depth = path_result.depth;
    }
    
    return result;
}
// --------------------------------------------------------------------------------------------------------------------
// unittest

#if !defined(OS_WINDOWS)
#include <fcntl.h>

#define DIR_SIZE_UT_DEEP 500

static int dir_size_ut_write(int dirfd, const char *name, size_t bytes) {
    int fd = openat(dirfd, name, O_CREAT | O_WRONLY | O_TRUNC, 0600);
    if(fd == -1) return -1;
    char buf[256] = { 0 };
    int rc = (write(fd, buf, bytes) == (ssize_t)bytes) ? 0 : -1;
    close(fd);
    return rc;
}

// open a chain of directories under dirfd, creating them; returns the fd of the last one
static int dir_size_ut_mkdir_open(int dirfd, const char *name) {
    if(mkdirat(dirfd, name, 0700) != 0 && errno != EEXIST) return -1;
    return openat(dirfd, name, O_RDONLY | O_DIRECTORY);
}

static void dir_size_ut_rm(int dirfd, const char *name) {
    int fd = openat(dirfd, name, O_RDONLY | O_DIRECTORY | O_NOFOLLOW);
    if(fd != -1) {
        DIR *dir = fdopendir(fd);
        if(dir) {
            struct dirent *e;
            while((e = readdir(dir))) {
                if(strcmp(e->d_name, ".") == 0 || strcmp(e->d_name, "..") == 0) continue;
                if(unlinkat(fd, e->d_name, 0) != 0)
                    dir_size_ut_rm(fd, e->d_name);
            }
            closedir(dir);
        }
        else
            close(fd);
    }
    unlinkat(dirfd, name, AT_REMOVEDIR);
}

struct dir_size_ut_job {
    const char *path;
    SIMPLE_PATTERN *pattern;
    DIR_SIZE result;
};

static void dir_size_ut_thread(void *arg) {
    struct dir_size_ut_job *job = arg;
    job->result = dir_size(job->path, job->pattern, 0);
}

// run dir_size() on a thread with the default nd_thread stack, as the agent does
static DIR_SIZE dir_size_ut_run(const char *path, const char *pattern) {
    struct dir_size_ut_job job = {
        .path = path,
        .pattern = pattern ? simple_pattern_create(pattern, " ", SIMPLE_PATTERN_EXACT, false) : NULL,
    };
    ND_THREAD *t = nd_thread_create("DIRSIZETEST", NETDATA_THREAD_OPTION_DEFAULT, dir_size_ut_thread, &job);
    if(t) nd_thread_join(t);
    else job.result.errors = (size_t)-1;
    simple_pattern_free(job.pattern);
    return job.result;
}

#define DIR_SIZE_UT_CHECK(cond, msg) do {                                           \
        if(!(cond)) {                                                               \
            fprintf(stderr, "dir_size unittest FAILED: %s (line %d)\n", msg, __LINE__); \
            errors++;                                                               \
        }                                                                           \
    } while(0)

int dir_size_unittest(void) {
    int errors = 0;
    fprintf(stderr, "\nrunning dir_size unittest\n");

    char root[] = "/tmp/netdata-dir-size-XXXXXX";
    if(!mkdtemp(root)) {
        fprintf(stderr, "dir_size unittest FAILED: cannot create a temporary directory\n");
        return 1;
    }
    int rootfd = open(root, O_RDONLY | O_DIRECTORY);

    // root/a.db (10), root/sub/b.db (20), root/sub/c.txt (30)
    int subfd = dir_size_ut_mkdir_open(rootfd, "sub");
    errors += (dir_size_ut_write(rootfd, "a.db", 10) != 0);
    errors += (dir_size_ut_write(subfd, "b.db", 20) != 0);
    errors += (dir_size_ut_write(subfd, "c.txt", 30) != 0);
    close(subfd);

    // root/d/d/.../d/deep.txt (40): DIR_SIZE_UT_DEEP levels deep
    int fd = dup(rootfd);
    for(size_t i = 0; i < DIR_SIZE_UT_DEEP && fd != -1; i++) {
        int next = dir_size_ut_mkdir_open(fd, "d");
        close(fd);
        fd = next;
    }
    errors += (fd == -1 || dir_size_ut_write(fd, "deep.txt", 40) != 0);
    if(fd != -1) close(fd);

    // root/long/<255>/<255>/... until the path no longer fits, with a file at the bottom (50)
    char name[256];
    memset(name, 'x', 255);
    name[255] = '\0';
    fd = dir_size_ut_mkdir_open(rootfd, "long");
    size_t long_len = strlen(root) + strlen("/long");
    while(fd != -1 && long_len + 1 + 255 <= FILENAME_MAX) {
        int next = dir_size_ut_mkdir_open(fd, name);
        close(fd);
        fd = next;
        long_len += 1 + 255;
    }
    errors += (fd == -1 || dir_size_ut_write(fd, name, 50) != 0);
    if(fd != -1) close(fd);

    if(errors) {
        fprintf(stderr, "dir_size unittest FAILED: cannot build the test tree\n");
        goto cleanup;
    }

    size_t long_dirs = 1 + (long_len - strlen(root) - strlen("/long")) / 256;

    DIR_SIZE r = dir_size_ut_run(root, NULL);
    DIR_SIZE_UT_CHECK(r.bytes == 10 + 20 + 30 + 40, "total bytes");
    DIR_SIZE_UT_CHECK(r.files == 4, "total files");
    DIR_SIZE_UT_CHECK(r.directories == 1 + 1 + DIR_SIZE_UT_DEEP + long_dirs, "total directories");
    DIR_SIZE_UT_CHECK(r.depth == DIR_SIZE_UT_DEEP, "deepest level");
    DIR_SIZE_UT_CHECK(r.errors == 1, "the file whose path does not fit is an error");

    r = dir_size_ut_run(root, "*.db");
    DIR_SIZE_UT_CHECK(r.bytes == 10 + 20 && r.files == 2, "pattern on paths relative to the root");

    r = dir_size_ut_run(root, "sub/*");
    DIR_SIZE_UT_CHECK(r.bytes == 20 + 30 && r.files == 2, "pattern with a subdirectory");

    char path[FILENAME_MAX + 1];
    snprintfz(path, sizeof(path), "%s/", root);
    r = dir_size_ut_run(path, "sub/*");
    DIR_SIZE_UT_CHECK(r.bytes == 20 + 30 && r.files == 2, "root with a trailing slash");

    snprintfz(path, sizeof(path), "%s/a.db", root);
    r = dir_size_ut_run(path, "a.db");
    DIR_SIZE_UT_CHECK(r.bytes == 10 && r.files == 1, "a single file matched by its filename");

    r = dir_size_ut_run(path, "*.txt");
    DIR_SIZE_UT_CHECK(r.bytes == 0 && r.files == 0, "a single file not matched");

    snprintfz(path, sizeof(path), "%s/missing", root);
    r = dir_size_ut_run(path, NULL);
    DIR_SIZE_UT_CHECK(r.bytes == 0 && r.errors == 1, "a missing path is an error");

    // a symlinked root is followed
    snprintfz(path, sizeof(path), "%s-link", root);
    if(symlink(root, path) == 0) {
        r = dir_size_ut_run(path, NULL);
        DIR_SIZE_UT_CHECK(r.bytes == 10 + 20 + 30 + 40, "a symlinked root is followed");
        unlink(path);
    }

cleanup:
    dir_size_ut_rm(AT_FDCWD, root);
    close(rootfd);

    fprintf(stderr, "dir_size unittest %s\n", errors ? "FAILED" : "OK");
    return errors;
}
#else
int dir_size_unittest(void) {
    return 0;
}
#endif
