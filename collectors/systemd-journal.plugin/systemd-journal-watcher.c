// SPDX-License-Identifier: GPL-3.0-or-later

#include "systemd-internals.h"
#include <sys/inotify.h>

#define MAX_EVENTS 1024
#define LEN_NAME 256
#define EVENT_SIZE (sizeof(struct inotify_event))
#define BUF_LEN (MAX_EVENTS * (EVENT_SIZE + LEN_NAME))
#define INITIAL_WATCHES 256

#define WATCH_FOR (IN_CREATE | IN_MODIFY | IN_DELETE | IN_DELETE_SELF | IN_MOVED_TO)

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
               "JOURNAL WATCHER: cannot watch '%s': %s",
               path);

        free_slot(watcher, t);
    }
    else {
        t->path = strdupz(path);

        nd_log(NDLS_COLLECTORS, NDLP_DEBUG,
               "JOURNAL WATCHER: watching: '%s'", path);

    }
    return t->wd;
}

static void remove_watch(Watcher *watcher, int inotifyFd, int wd) {
    int i;
    for (i = 0; i < watcher->watchCount; ++i) {
        if (watcher->watchList[i].wd == wd) {

            nd_log(NDLS_COLLECTORS, NDLP_DEBUG,
                   "JOURNAL WATCHER: removing watch %d on: '%s'\n", wd, watcher->watchList[i].path);

            inotify_rm_watch(inotifyFd, watcher->watchList[i].wd);
            free_slot(watcher, &watcher->watchList[i]);
            break;
        }
    }

    if(i == watcher->watchCount)
        nd_log(NDLS_COLLECTORS, NDLP_WARNING,
               "JOURNAL WATCHER: cannot find watch %d to remove.",
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
}

static char* get_path_from_wd(Watcher *watcher, int wd) {
    for (int i = 0; i < watcher->watchCount; ++i) {
        if (watcher->watchList[i].wd == wd)
            return watcher->watchList[i].path;
    }
    return NULL;
}

static void watch_directory_recursively(Watcher *watcher, int inotifyFd, const char *basePath) {
    char path[PATH_MAX];
    struct dirent *dp;
    DIR *dir = opendir(basePath);

    if (!dir)
        return;

    while ((dp = readdir(dir)) != NULL) {
        if (strcmp(dp->d_name, ".") != 0 && strcmp(dp->d_name, "..") != 0) {
            snprintfz(path, sizeof(path), "%s/%s", basePath, dp->d_name);

            if (dp->d_type == DT_DIR) {
                add_watch(watcher, inotifyFd, path);
                watch_directory_recursively(watcher, inotifyFd, path);
            }
        }
    }

    closedir(dir);
}

void process_event(Watcher *watcher, int inotifyFd, struct inotify_event *event, size_t *have_updates) {
    char *dirPath = get_path_from_wd(watcher, event->wd);
    if (dirPath && event->len) {
        if (event->mask & IN_DELETE_SELF)
            remove_watch(watcher, inotifyFd, event->wd);

        else {
            size_t len = strlen(event->name);
            if (len > sizeof(".journal") - 1 && strcmp(&event->name[len - (sizeof(".journal") - 1)], ".journal") == 0) {
                // It is a file that ends in .journal
                static __thread char fullPath[PATH_MAX];
                snprintf(fullPath, PATH_MAX, "%s/%s", dirPath, event->name);
                // fullPath contains the full path to the file

                if (event->mask & IN_DELETE) {
                    struct stat info;
                    if (stat(fullPath, &info) != 0) {
                        dictionary_del(journal_files_registry, fullPath);
                    }
                    else {
                        nd_log(NDLS_COLLECTORS, NDLP_ERR,
                               "JOURNAL WATCHER: got DELETE event, but the file is still there: '%s",
                               fullPath);
                    }
                }
                else if(event->mask & (IN_CREATE|IN_MODIFY)) {
                    struct stat info;
                    if (stat(fullPath, &info) != 0) {
                        nd_log(NDLS_COLLECTORS, NDLP_ERR,
                               "JOURNAL WATCHER: failed to stat(): '%s", fullPath);
                    }
                    else if (S_ISREG(info.st_mode)) {
                        struct journal_file t = {
                                .file_last_modified_ut = info.st_mtim.tv_sec * USEC_PER_SEC +
                                                         info.st_mtim.tv_nsec / NSEC_PER_USEC,
                                .last_scan_monotonic_ut = now_monotonic_usec(),
                                .size = info.st_size,
                                .max_journal_vs_realtime_delta_ut = JOURNAL_VS_REALTIME_DELTA_DEFAULT_UT,
                        };
                        dictionary_set(journal_files_registry, fullPath, &t, sizeof(t));
                        have_updates++;
                    }
                }
            }
        }
    }
}

void *journal_watcher_main(void *arg __maybe_unused) {
    while(1) {
        Watcher watcher = {
                .watchList = mallocz(INITIAL_WATCHES * sizeof(WatchEntry)),
                .freeList = NULL,
                .watchCount = 0,
                .watchListSize = INITIAL_WATCHES
        };

        int inotifyFd = inotify_init();
        if (inotifyFd < 0) {
            nd_log(NDLS_COLLECTORS, NDLP_ERR, "inotify_init");
            free_watches(&watcher, inotifyFd);
            return NULL;
        }

        for (unsigned i = 0; i < MAX_JOURNAL_DIRECTORIES; i++) {
            if (!journal_directories[i].path) break;
            watch_directory_recursively(&watcher, inotifyFd, journal_directories[i].path);
        }

        size_t have_updates = 0;
        usec_t last_headers_update_ut = now_monotonic_usec();
        struct buffered_reader reader;
        while (1) {
            buffered_reader_ret_t rc = buffered_reader_read_timeout(&reader, inotifyFd, 100, false);

            if (rc != BUFFERED_READER_READ_OK && rc != BUFFERED_READER_READ_POLL_TIMEOUT) {
                nd_log(NDLS_COLLECTORS, NDLP_CRIT,
                       "JOURNAL WATCHER: cannot read inotify events, buffered_reader_read_timeout() returned %d", rc);
                break;
            }

            if(rc == BUFFERED_READER_READ_OK) {
                ssize_t i;
                while (i < reader.read_len) {
                    struct inotify_event *event = (struct inotify_event *) &reader.read_buffer[i];
                    process_event(&watcher, inotifyFd, event, &have_updates);
                    i += EVENT_SIZE + event->len;
                }

                reader.read_buffer[0] = '\0';
                reader.read_len = 0;
                reader.pos = 0;
            }

            usec_t ut = now_monotonic_usec();
            if (have_updates && (rc == BUFFERED_READER_READ_POLL_TIMEOUT || last_headers_update_ut + 100 * NSEC_PER_USEC < ut)) {
                journal_files_updater_all_headers_sorted();
                last_headers_update_ut = ut;
                have_updates = 0;
            }
        }

        close(inotifyFd);
        free_watches(&watcher, inotifyFd);
    }
    return NULL;
}
