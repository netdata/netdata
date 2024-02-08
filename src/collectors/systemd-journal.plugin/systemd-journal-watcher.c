// SPDX-License-Identifier: GPL-3.0-or-later

#include "systemd-internals.h"
#include <sys/inotify.h>

#define EVENT_SIZE (sizeof(struct inotify_event))
#define INITIAL_WATCHES 256

#define WATCH_FOR (IN_CREATE | IN_MODIFY | IN_DELETE | IN_DELETE_SELF | IN_MOVED_FROM | IN_MOVED_TO | IN_UNMOUNT)

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
    int i;
    for (i = 0; i < watcher->watchCount; ++i) {
        if (watcher->watchList[i].wd == wd) {

            nd_log(NDLS_COLLECTORS, NDLP_DEBUG,
                   "JOURNAL WATCHER: removing watch from directory: '%s'",
                   watcher->watchList[i].path);

            inotify_rm_watch(inotifyFd, watcher->watchList[i].wd);
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
            inotify_rm_watch(inotifyFd, watcher->watchList[i].wd);
            free_slot(watcher, &watcher->watchList[i]);
        }
    }
    freez(watcher->watchList);
    watcher->watchList = NULL;

    dictionary_destroy(watcher->pending);
    watcher->pending = NULL;
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

    journal_directory_scan_recursively(NULL, dirs, basePath, 0);

    void *x;
    dfe_start_read(dirs, x) {
        const char *dirname = x_dfe.name;
        // Check if this directory is already being watched
        if (!is_directory_watched(watcher, dirname)) {
            add_watch(watcher, inotifyFd, dirname);
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
        if (t->wd != -1 && is_subpath(t->path, dirPath)) {
            inotify_rm_watch(inotifyFd, t->wd);
            free_slot(watcher, t);
        }
    }

    struct journal_file *jf;
    dfe_start_write(journal_files_registry, jf) {
        if(is_subpath(jf->filename, dirPath))
            dictionary_del(journal_files_registry, jf->filename);
    }
    dfe_done(jf);

    dictionary_garbage_collect(journal_files_registry);
}

void process_event(Watcher *watcher, int inotifyFd, struct inotify_event *event) {
    if(!event->len) {
        nd_log(NDLS_COLLECTORS, NDLP_NOTICE
               , "JOURNAL WATCHER: received event with mask %u and len %u (this is zero) for path: '%s' - ignoring it."
               , event->mask, event->len, event->name);
        return;
    }

    char *dirPath = get_path_from_wd(watcher, event->wd);
    if(!dirPath) {
        nd_log(NDLS_COLLECTORS, NDLP_NOTICE,
               "JOURNAL WATCHER: received event with mask %u and len %u for path: '%s' - "
               "but we can't find its watch descriptor - ignoring it."
               , event->mask, event->len, event->name);
        return;
    }

    if(event->mask & IN_DELETE_SELF) {
        remove_watch(watcher, inotifyFd, event->wd);
        return;
    }

    static __thread char fullPath[PATH_MAX];
    snprintfz(fullPath, sizeof(fullPath), "%s/%s", dirPath, event->name);
    // fullPath contains the full path to the file

    size_t len = strlen(event->name);

    if(event->mask & IN_ISDIR) {
        if (event->mask & (IN_DELETE | IN_MOVED_FROM)) {
            // A directory is deleted or moved out
            nd_log(NDLS_COLLECTORS, NDLP_DEBUG,
                    "JOURNAL WATCHER: Directory deleted or moved out: '%s'",
                    fullPath);

            // Remove the watch - implement this function based on how you manage your watches
            remove_directory_watch(watcher, inotifyFd, fullPath);
        }
        else if (event->mask & (IN_CREATE | IN_MOVED_TO)) {
            // A new directory is created or moved in
            nd_log(NDLS_COLLECTORS, NDLP_DEBUG,
                    "JOURNAL WATCHER: New directory created or moved in: '%s'",
                    fullPath);

            // Start watching the new directory - recursive watch
            watch_directory_and_subdirectories(watcher, inotifyFd, fullPath);
        }
        else
            nd_log(NDLS_COLLECTORS, NDLP_WARNING,
                   "JOURNAL WATCHER: Received unhandled event with mask %u for directory '%s'",
                   event->mask, fullPath);
    }
    else if(len > sizeof(".journal") - 1 && strcmp(&event->name[len - (sizeof(".journal") - 1)], ".journal") == 0) {
        // It is a file that ends in .journal
        // add it to our pending list
        dictionary_set(watcher->pending, fullPath, NULL, 0);
    }
    else
        nd_log(NDLS_COLLECTORS, NDLP_DEBUG,
               "JOURNAL WATCHER: ignoring event with mask %u for file '%s'",
               event->mask, fullPath);
}

static void process_pending(Watcher *watcher) {
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

void *journal_watcher_main(void *arg __maybe_unused) {
    while(1) {
        size_t journal_watcher_session_id = journal_watcher_wanted_session_id;

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
        while (journal_watcher_session_id == __atomic_load_n(&journal_watcher_wanted_session_id, __ATOMIC_RELAXED)) {
            buffered_reader_ret_t rc = buffered_reader_read_timeout(
                    &reader, inotifyFd, SYSTEMD_JOURNAL_EXECUTE_WATCHER_PENDING_EVERY_MS, false);

            if (rc != BUFFERED_READER_READ_OK && rc != BUFFERED_READER_READ_POLL_TIMEOUT) {
                nd_log(NDLS_COLLECTORS, NDLP_CRIT,
                       "JOURNAL WATCHER: cannot read inotify events, buffered_reader_read_timeout() returned %d - "
                       "restarting the watcher.",
                       rc);
                break;
            }

            if(rc == BUFFERED_READER_READ_OK) {
                bool unmount_event = false;

                ssize_t i = 0;
                while (i < reader.read_len) {
                    struct inotify_event *event = (struct inotify_event *) &reader.read_buffer[i];

                    if(event->mask & IN_UNMOUNT) {
                        unmount_event = true;
                        break;
                    }

                    process_event(&watcher, inotifyFd, event);
                    i += (ssize_t)EVENT_SIZE + event->len;
                }

                reader.read_buffer[0] = '\0';
                reader.read_len = 0;
                reader.pos = 0;

                if(unmount_event)
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

        // this will scan the directories and cleanup the registry
        journal_files_registry_update();

        sleep_usec(2 * USEC_PER_SEC);
    }

    return NULL;
}
