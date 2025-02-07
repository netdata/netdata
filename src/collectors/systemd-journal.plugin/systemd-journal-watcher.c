// SPDX-License-Identifier: GPL-3.0-or-later

#include "systemd-internals.h"
#include <sys/inotify.h>

#define INITIAL_WATCHES 256

#define WATCH_FOR (IN_CREATE | IN_MODIFY | IN_DELETE | IN_DELETE_SELF | IN_MOVED_FROM | IN_MOVED_TO | IN_UNMOUNT)

typedef uint32_t INOTIFY_MASK;

ENUM_STR_MAP_DEFINE(INOTIFY_MASK) = {
    // helpers (combine multiple flags)
    // must be first in the list
    {.id = IN_ALL_EVENTS, .name = "IN_ALL_EVENTS"},
    {.id = IN_CLOSE, .name = "IN_CLOSE"},
    {.id = IN_MOVE, .name = "IN_MOVE"},

    // individual flags
    {.id = IN_ACCESS, .name = "IN_ACCESS"},
    {.id = IN_MODIFY, .name = "IN_MODIFY"},
    {.id = IN_ATTRIB, .name = "IN_ATTRIB"},
    {.id = IN_CLOSE_WRITE, .name = "IN_CLOSE_WRITE"},
    {.id = IN_CLOSE_NOWRITE, .name = "IN_CLOSE_NOWRITE"},
    {.id = IN_OPEN, .name = "IN_OPEN"},
    {.id = IN_MOVED_FROM, .name = "IN_MOVED_FROM"},
    {.id = IN_MOVED_TO, .name = "IN_MOVED_TO"},
    {.id = IN_CREATE, .name = "IN_CREATE"},
    {.id = IN_DELETE, .name = "IN_DELETE"},
    {.id = IN_DELETE_SELF, .name = "IN_DELETE_SELF"},
    {.id = IN_MOVE_SELF, .name = "IN_MOVE_SELF"},
    {.id = IN_UNMOUNT, .name = "IN_UNMOUNT"},
    {.id = IN_Q_OVERFLOW, .name = "IN_Q_OVERFLOW"},
    {.id = IN_IGNORED, .name = "IN_IGNORED"},
    {.id = IN_ONLYDIR, .name = "IN_ONLYDIR"},
    {.id = IN_DONT_FOLLOW, .name = "IN_DONT_FOLLOW"},
    {.id = IN_EXCL_UNLINK, .name = "IN_EXCL_UNLINK"},
#ifdef IN_MASK_CREATE
    {.id = IN_MASK_CREATE, .name = "IN_MASK_CREATE"},
#endif
    {.id = IN_MASK_ADD, .name = "IN_MASK_ADD"},
    {.id = IN_ISDIR, .name = "IN_ISDIR"},
    {.id = IN_ONESHOT, .name = "IN_ONESHOT"},

    // terminator
    {.id = 0, .name = NULL}
};

BITMAP_STR_DEFINE_FUNCTIONS(INOTIFY_MASK, 0, "UNKNOWN");

DEFINE_JUDYL_TYPED(SYMLINKED_DIRS, STRING *);

typedef struct watch_entry {
    int slot;

    int wd;             // Watch descriptor
    char *path;         // Dynamically allocated path

    struct watch_entry *next; // for the free list
} WatchEntry;

typedef struct {
    WatchEntry *watchList;
    WatchEntry *freeList;
    int watchCount;
    int watchListSize;

    size_t errors;

    SYMLINKED_DIRS_JudyLSet symlinkedDirs;
    DICTIONARY *pending;
} Watcher;

static WatchEntry *get_slot(Watcher *watcher) {
    WatchEntry *t;

    if (watcher->freeList != NULL) {
        t = watcher->freeList;
        watcher->freeList = t->next;
        t->next = NULL;
        return t;
    }

    if (watcher->watchCount == watcher->watchListSize) {
        watcher->watchListSize *= 2;
        watcher->watchList = reallocz(watcher->watchList, watcher->watchListSize * sizeof(WatchEntry));
    }

    watcher->watchList[watcher->watchCount] = (WatchEntry){
            .slot = watcher->watchCount,
            .wd = -1,
            .path = NULL,
            .next = NULL,
    };
    t = &watcher->watchList[watcher->watchCount];
    watcher->watchCount++;

    return t;
}

static void free_slot(Watcher *watcher, WatchEntry *t) {
    t->wd = -1;
    freez(t->path);
    t->path = NULL;

    // link it to the free list
    t->next = watcher->freeList;
    watcher->freeList = t;
}

static int add_watch(Watcher *watcher, int inotifyFd, const char *path) {
    WatchEntry *t = get_slot(watcher);

    errno_clear();
    t->wd = inotify_add_watch(inotifyFd, path, WATCH_FOR);
    if (t->wd == -1) {
        nd_log(NDLS_COLLECTORS, NDLP_ERR,
               "JOURNAL WATCHER: cannot watch directory: '%s'",
               path);

        free_slot(watcher, t);

        struct stat info;
        if(stat(path, &info) == 0 && S_ISDIR(info.st_mode)) {
            // the directory exists, but we failed to add the watch
            // increase errors
            watcher->errors++;
        }
    }
    else {
        t->path = strdupz(path);

        nd_log(NDLS_COLLECTORS, NDLP_DEBUG,
               "JOURNAL WATCHER: watching directory: '%s'",
               path);

    }
    return t->wd;
}

static void remove_watch(Watcher *watcher, int inotifyFd, int wd) {
    errno_clear();

    int i;
    for (i = 0; i < watcher->watchCount; ++i) {
        if (watcher->watchList[i].wd == wd) {

            nd_log(NDLS_COLLECTORS, NDLP_DEBUG,
                   "JOURNAL WATCHER: removing watch from directory: '%s'",
                   watcher->watchList[i].path);

            if(inotify_rm_watch(inotifyFd, watcher->watchList[i].wd) == -1)
                nd_log(NDLS_COLLECTORS, NDLP_ERR, "JOURNAL WATCHER: inotify_rm_watch() returned -1");

            free_slot(watcher, &watcher->watchList[i]);
            return;
        }
    }

    nd_log(NDLS_COLLECTORS, NDLP_WARNING,
           "JOURNAL WATCHER: cannot find directory watch %d to remove.",
           wd);
}

static void free_watches(Watcher *watcher, int inotifyFd) {
    for (int i = 0; i < watcher->watchCount; ++i) {
        if (watcher->watchList[i].wd != -1) {
            if(inotify_rm_watch(inotifyFd, watcher->watchList[i].wd) == -1)
                nd_log(NDLS_COLLECTORS, NDLP_ERR, "JOURNAL WATCHER: inotify_rm_watch() returned -1");
            free_slot(watcher, &watcher->watchList[i]);
        }
    }
    freez(watcher->watchList);
    watcher->watchList = NULL;

    dictionary_destroy(watcher->pending);
    watcher->pending = NULL;
}

static void free_symlinked_dirs(Watcher *watcher) {
    Word_t idx = 0;
    STRING *value;
    while((value = SYMLINKED_DIRS_FIRST(&watcher->symlinkedDirs, &idx))) {
        SYMLINKED_DIRS_DEL(&watcher->symlinkedDirs, idx);
        STRING *key = (STRING *)idx;
        string_freez(key);
        string_freez(value);
    }
}

static char* get_path_from_wd(Watcher *watcher, int wd) {
    for (int i = 0; i < watcher->watchCount; ++i) {
        if (watcher->watchList[i].wd == wd)
            return watcher->watchList[i].path;
    }
    return NULL;
}

static bool is_directory_watched(Watcher *watcher, const char *path) {
    for (int i = 0; i < watcher->watchCount; ++i) {
        if (watcher->watchList[i].wd != -1 && strcmp(watcher->watchList[i].path, path) == 0) {
            return true;
        }
    }
    return false;
}

static void watch_directory_and_subdirectories(Watcher *watcher, int inotifyFd, const char *basePath) {
    DICTIONARY *dirs = dictionary_create(DICT_OPTION_SINGLE_THREADED | DICT_OPTION_DONT_OVERWRITE_VALUE);

    // First resolve any symlinks in the base path
    char real_path[PATH_MAX];
    if (realpath(basePath, real_path) == NULL) {
        // If realpath fails, try using the original path
        strncpyz(real_path, basePath, sizeof(real_path));
    }

    journal_directory_scan_recursively(NULL, dirs, real_path, 0);

    void *x;
    dfe_start_read(dirs, x) {
        const char *dirname = x_dfe.name;
        char resolved_path[PATH_MAX];

        // Resolve symlinks for each subdirectory
        if (realpath(dirname, resolved_path) != NULL) {
            // Check if this directory is already being watched
            if (!is_directory_watched(watcher, resolved_path)) {
                add_watch(watcher, inotifyFd, resolved_path);
            }
        } else {
            // If realpath fails, try with original path
            if (!is_directory_watched(watcher, dirname)) {
                add_watch(watcher, inotifyFd, dirname);
            }
        }
    }
    dfe_done(x);

    dictionary_destroy(dirs);
}

static bool is_subpath(const char *path, const char *subpath) {
    // Use strncmp to compare the paths
    if (strncmp(path, subpath, strlen(path)) == 0) {
        // Ensure that the next character is a '/' or '\0'
        char next_char = subpath[strlen(path)];
        return next_char == '/' || next_char == '\0';
    }

    return false;
}

void remove_directory_watch(Watcher *watcher, int inotifyFd, const char *dirPath) {
    for (int i = 0; i < watcher->watchCount; ++i) {
        WatchEntry *t = &watcher->watchList[i];
        if (t->wd != -1 && is_subpath(dirPath, t->path)) {
            if(inotify_rm_watch(inotifyFd, t->wd) == -1)
                nd_log(NDLS_COLLECTORS, NDLP_ERR, "JOURNAL WATCHER: inotify_rm_watch() on path '%s' returned -1", t->path);
            else
                nd_log(NDLS_COLLECTORS, NDLP_DEBUG, "JOURNAL WATCHER: stopped watching directory '%s'", t->path);
            free_slot(watcher, t);
        }
    }

    struct journal_file *jf;
    dfe_start_write(journal_files_registry, jf) {
        if(is_subpath(dirPath, jf->filename))
            dictionary_del(journal_files_registry, jf->filename);
    }
    dfe_done(jf);

    dictionary_garbage_collect(journal_files_registry);
}

void process_event(Watcher *watcher, int inotifyFd, struct inotify_event *event) {
    errno_clear();

    if(!event->len) {
        CLEAN_BUFFER *wb = buffer_create(0, NULL);
        INOTIFY_MASK_2buffer(wb, event->mask, ", ");
        nd_log(NDLS_COLLECTORS, NDLP_NOTICE,
               "JOURNAL WATCHER: received event with mask %u (%s) and len %u (this is zero) - ignoring it.",
               event->mask, buffer_tostring(wb), event->len);
        return;
    }

    char *dirPath = get_path_from_wd(watcher, event->wd);
    if(!dirPath) {
        CLEAN_BUFFER *wb = buffer_create(0, NULL);
        INOTIFY_MASK_2buffer(wb, event->mask, ", ");
        nd_log(NDLS_COLLECTORS, NDLP_NOTICE,
               "JOURNAL WATCHER: received event with mask %u (%s) and len %u for path: '%s' - "
               "but we can't find its watch descriptor - ignoring it.",
               event->mask, buffer_tostring(wb), event->len, event->name);
        return;
    }

#ifdef NETDATA_INTERNAL_CHECKS
    {
        CLEAN_BUFFER *wb = buffer_create(0, NULL);
        INOTIFY_MASK_2buffer(wb, event->mask, ", ");
        nd_log(NDLS_COLLECTORS, NDLP_DEBUG,
               "JOURNAL WATCHER: received event with mask %u (%s) for path: '%s' inside '%s'",
               event->mask, buffer_tostring(wb), event->name, dirPath);
    }
#endif

    if(event->mask & IN_DELETE_SELF) {
        remove_watch(watcher, inotifyFd, event->wd);
        return;
    }

    static __thread char fullPath[PATH_MAX];
    snprintfz(fullPath, sizeof(fullPath), "%s/%s", dirPath, event->name);

    bool is_dir = event->mask & IN_ISDIR;
    char resolved_path[PATH_MAX];
    const char *path_to_use = fullPath;

    if (event->mask & (IN_CREATE | IN_MOVED_TO)) {
        // Give the system a moment to establish the symlink
        sleep_usec(1000); // 1ms sleep

        struct stat st;
        if (lstat(fullPath, &st) == 0) {
            if (S_ISLNK(st.st_mode)) {
                // It's a symlink - resolve it
                if (realpath(fullPath, resolved_path) != NULL) {
                    path_to_use = resolved_path;

                    // Check if it points to a directory
                    if (stat(resolved_path, &st) == 0 && S_ISDIR(st.st_mode)) {
                        is_dir = true;

                        STRING *fullPathString = string_strdupz(fullPath);
                        STRING *symlinked = SYMLINKED_DIRS_GET(&watcher->symlinkedDirs, (uintptr_t)fullPathString);
                        if (!symlinked) {
                            SYMLINKED_DIRS_SET(
                                &watcher->symlinkedDirs, (uintptr_t)fullPathString, string_strdupz(resolved_path));

                            // we leave fullPathString allocated, as it's now in the JudyL set

                            nd_log(NDLS_COLLECTORS, NDLP_DEBUG,
                                   "JOURNAL WATCHER: New symlinked directory created: '%s' -> '%s'",
                                   fullPath, resolved_path);
                        }
                        else if (string_strcmp(symlinked, resolved_path) != 0) {
                            SYMLINKED_DIRS_SET(
                                &watcher->symlinkedDirs, (uintptr_t)fullPathString, string_strdupz(resolved_path));

                            nd_log(NDLS_COLLECTORS, NDLP_DEBUG,
                                   "JOURNAL WATCHER: Updated symlinked directory: '%s' -> '%s' (was '%s')",
                                   fullPath, resolved_path, string2str(symlinked));

                            string_freez(symlinked);

                            // we need to free this, since it was already in the JudyL set
                            string_freez(fullPathString);
                        }
                        else
                            string_freez(fullPathString); // we don't need it anymore
                    }
                }
            }
        }
    }
    else if(event->mask & IN_DELETE) {
        // Check if it was a symlink
        STRING *fullPathString = string_strdupz(fullPath);
        STRING *symlinked = SYMLINKED_DIRS_GET(&watcher->symlinkedDirs, (uintptr_t)fullPathString);
        if (symlinked) {
            strncpyz(resolved_path, string2str(symlinked), sizeof(resolved_path) - 1);
            path_to_use = resolved_path;
            SYMLINKED_DIRS_DEL(&watcher->symlinkedDirs, (uintptr_t)fullPathString);
            string_freez(fullPathString); // to remove also the one referenced in the JudyL set
            string_freez(symlinked);
            is_dir = true;

            nd_log(NDLS_COLLECTORS, NDLP_DEBUG,
                   "JOURNAL WATCHER: Deleted symlinked directory: '%s' -> '%s'",
                   fullPath, resolved_path);
        }
        string_freez(fullPathString); // the one we allocated above
    }

    if(is_dir) {
        if (event->mask & (IN_DELETE | IN_MOVED_FROM)) {
            nd_log(NDLS_COLLECTORS, NDLP_DEBUG,
                   "JOURNAL WATCHER: Directory deleted or moved out: '%s'",
                   path_to_use);

            remove_directory_watch(watcher, inotifyFd, path_to_use);
        }
        else if (event->mask & (IN_CREATE | IN_MOVED_TO)) {
            nd_log(NDLS_COLLECTORS, NDLP_DEBUG,
                   "JOURNAL WATCHER: New directory created or moved in: '%s'",
                   path_to_use);

            watch_directory_and_subdirectories(watcher, inotifyFd, path_to_use);
        }
        else {
            CLEAN_BUFFER *wb = buffer_create(0, NULL);
            INOTIFY_MASK_2buffer(wb, event->mask, ", ");
            nd_log(NDLS_COLLECTORS, NDLP_WARNING,
                   "JOURNAL WATCHER: Received unhandled event with mask %u (%s) for directory '%s'",
                   event->mask, buffer_tostring(wb), path_to_use);
        }
    }
    else if(is_journal_file(event->name, (ssize_t)strlen(event->name), NULL)) {
        dictionary_set(watcher->pending, path_to_use, NULL, 0);
    }
    else {
        CLEAN_BUFFER *wb = buffer_create(0, NULL);
        INOTIFY_MASK_2buffer(wb, event->mask, ", ");
        nd_log(NDLS_COLLECTORS, NDLP_DEBUG,
               "JOURNAL WATCHER: ignoring event with mask %u (%s) for file '%s' ('%s')",
               event->mask, buffer_tostring(wb), path_to_use, fullPath);
    }
}

static void process_pending(Watcher *watcher) {
    errno_clear();

    void *x;
    dfe_start_write(watcher->pending, x) {
        struct stat info;
        const char *fullPath = x_dfe.name;

        if(stat(fullPath, &info) != 0) {
            nd_log(NDLS_COLLECTORS, NDLP_DEBUG,
                   "JOURNAL WATCHER: file '%s' no longer exists, removing it from the registry",
                   fullPath);

            dictionary_del(journal_files_registry, fullPath);
        }
        else if(S_ISREG(info.st_mode)) {
            nd_log(NDLS_COLLECTORS, NDLP_DEBUG,
                    "JOURNAL WATCHER: file '%s' has been added/updated, updating the registry",
                    fullPath);

            struct journal_file t = {
                    .file_last_modified_ut = info.st_mtim.tv_sec * USEC_PER_SEC +
                                             info.st_mtim.tv_nsec / NSEC_PER_USEC,
                    .last_scan_monotonic_ut = now_monotonic_usec(),
                    .size = info.st_size,
                    .max_journal_vs_realtime_delta_ut = JOURNAL_VS_REALTIME_DELTA_DEFAULT_UT,
            };
            struct journal_file *jf = dictionary_set(journal_files_registry, fullPath, &t, sizeof(t));
            journal_file_update_header(jf->filename, jf);
        }

        dictionary_del(watcher->pending, fullPath);
    }
    dfe_done(x);

    dictionary_garbage_collect(watcher->pending);
}

size_t journal_watcher_wanted_session_id = 0;

void journal_watcher_restart(void) {
    __atomic_add_fetch(&journal_watcher_wanted_session_id, 1, __ATOMIC_RELAXED);
}

static bool process_inotify_events(struct buffered_reader *reader, Watcher *watcher, int inotifyFd) {
    errno_clear();

    bool unmount_event = false;
    ssize_t processed = 0;

    // Process as many complete events as we can
    while (processed + (ssize_t)sizeof(struct inotify_event) <= reader->read_len) {
        struct inotify_event *event = (struct inotify_event *)(reader->read_buffer + processed);

        if(event->len > NAME_MAX + 1) {
            // The event length is impossibly large
            nd_log(NDLS_COLLECTORS, NDLP_ERR,
                   "JOURNAL WATCHER: received impossibly large event length %u - restarting",
                   event->len);
            return true; // force a restart
        }

        // Check if we have the complete event including the name
        ssize_t total_size = (ssize_t)sizeof(struct inotify_event) + event->len;
        if (processed + total_size > reader->read_len)
            break;  // Wait for more data

        if(event->mask & IN_UNMOUNT) {
            unmount_event = true;
            break;
        }

        process_event(watcher, inotifyFd, event);
        processed += total_size;
    }

    // If we have unprocessed data, move it to the start
    if (processed < reader->read_len) {
        memmove(reader->read_buffer,
                reader->read_buffer + processed,
                reader->read_len - processed);
        reader->read_len -= processed;
    }
    else
        reader->read_len = 0;

    reader->read_buffer[reader->read_len] = '\0';
    return unmount_event;
}

void *journal_watcher_main(void *arg __maybe_unused) {
    while(1) {
        size_t journal_watcher_session_id = __atomic_load_n(&journal_watcher_wanted_session_id, __ATOMIC_RELAXED);

        Watcher watcher = {
            .watchList = mallocz(INITIAL_WATCHES * sizeof(WatchEntry)),
            .freeList = NULL,
            .watchCount = 0,
            .watchListSize = INITIAL_WATCHES,
            .pending = dictionary_create(DICT_OPTION_DONT_OVERWRITE_VALUE|DICT_OPTION_SINGLE_THREADED),
            .errors = 0,
        };

        int inotifyFd = inotify_init();
        if (inotifyFd < 0) {
            nd_log(NDLS_COLLECTORS, NDLP_ERR, "inotify_init() failed.");
            free_watches(&watcher, inotifyFd);
            return NULL;
        }

        for (unsigned i = 0; i < MAX_JOURNAL_DIRECTORIES; i++) {
            if (!journal_directories[i].path) break;
            watch_directory_and_subdirectories(&watcher, inotifyFd, string2str(journal_directories[i].path));
        }

        usec_t last_headers_update_ut = now_monotonic_usec();
        struct buffered_reader reader;
        buffered_reader_init(&reader);

        while (journal_watcher_session_id == __atomic_load_n(&journal_watcher_wanted_session_id, __ATOMIC_RELAXED)) {
            buffered_reader_ret_t rc = buffered_reader_read_timeout(
                    &reader, inotifyFd, SYSTEMD_JOURNAL_EXECUTE_WATCHER_PENDING_EVERY_MS, false);

            if(rc == BUFFERED_READER_READ_OK || rc == BUFFERED_READER_READ_BUFFER_FULL) {
                if (process_inotify_events(&reader, &watcher, inotifyFd))
                    break;
            }
            else if (rc != BUFFERED_READER_READ_POLL_TIMEOUT) {
                nd_log(NDLS_COLLECTORS, NDLP_ERR,
                       "JOURNAL WATCHER: cannot read inotify events, buffered_reader_read_timeout() returned %d - "
                       "restarting the watcher.",
                       rc);
                break;
            }

            usec_t ut = now_monotonic_usec();
            if (dictionary_entries(watcher.pending) && (rc == BUFFERED_READER_READ_POLL_TIMEOUT ||
                last_headers_update_ut + (SYSTEMD_JOURNAL_EXECUTE_WATCHER_PENDING_EVERY_MS * USEC_PER_MS) <= ut)) {
                process_pending(&watcher);
                last_headers_update_ut = ut;
            }

            if(watcher.errors) {
                nd_log(NDLS_COLLECTORS, NDLP_NOTICE,
                       "JOURNAL WATCHER: there were errors in setting up inotify watches - restarting the watcher.");
            }
        }

        close(inotifyFd);
        free_watches(&watcher, inotifyFd);
        free_symlinked_dirs(&watcher);

        // this will scan the directories and cleanup the registry
        journal_files_registry_update();

        sleep_usec(2 * USEC_PER_SEC);
    }

    return NULL;
}
