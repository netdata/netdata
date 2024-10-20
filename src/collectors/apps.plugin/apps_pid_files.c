// SPDX-License-Identifier: GPL-3.0-or-later

#include "apps_plugin.h"

static uint32_t
        all_files_len = 0,
        all_files_size = 0;

uint32_t all_files_len_get(void) {
    (void)all_files_size;
    return all_files_len;
}

#if (PROCESSES_HAVE_FDS == 1)
// ----------------------------------------------------------------------------
// file descriptor
//
// this is used to keep a global list of all open files of the system.
// it is needed in order to calculate the unique files processes have open.

#define FILE_DESCRIPTORS_INCREASE_STEP 100

// types for struct file_descriptor->type
typedef enum __attribute__((packed)) fd_filetype {
    FILETYPE_OTHER,
    FILETYPE_FILE,
    FILETYPE_PIPE,
    FILETYPE_SOCKET,
    FILETYPE_INOTIFY,
    FILETYPE_EVENTFD,
    FILETYPE_EVENTPOLL,
    FILETYPE_TIMERFD,
    FILETYPE_SIGNALFD
} FD_FILETYPE;

struct file_descriptor {
    avl_t avl;

#ifdef NETDATA_INTERNAL_CHECKS
    uint32_t magic;
#endif /* NETDATA_INTERNAL_CHECKS */

    const char *name;
    uint32_t hash;
    uint32_t count;
    uint32_t pos;
    FD_FILETYPE type;
} *all_files = NULL;

// ----------------------------------------------------------------------------

static inline void reallocate_target_fds(struct target *w) {
    if(unlikely(!w))
        return;

    if(unlikely(!w->target_fds || w->target_fds_size < all_files_size)) {
        w->target_fds = reallocz(w->target_fds, sizeof(int) * all_files_size);
        memset(&w->target_fds[w->target_fds_size], 0, sizeof(int) * (all_files_size - w->target_fds_size));
        w->target_fds_size = all_files_size;
    }
}

static void aggregage_fd_type_on_openfds(FD_FILETYPE type, struct openfds *openfds) {
    switch(type) {
        case FILETYPE_SOCKET:
            openfds->sockets++;
            break;

        case FILETYPE_FILE:
            openfds->files++;
            break;

        case FILETYPE_PIPE:
            openfds->pipes++;
            break;

        case FILETYPE_INOTIFY:
            openfds->inotifies++;
            break;

        case FILETYPE_EVENTFD:
            openfds->eventfds++;
            break;

        case FILETYPE_TIMERFD:
            openfds->timerfds++;
            break;

        case FILETYPE_SIGNALFD:
            openfds->signalfds++;
            break;

        case FILETYPE_EVENTPOLL:
            openfds->eventpolls++;
            break;

        case FILETYPE_OTHER:
            openfds->other++;
            break;
    }
}

static inline void aggregate_fd_on_target(int fd, struct target *w) {
    if(unlikely(!w))
        return;

    if(unlikely(w->target_fds[fd])) {
        // it is already aggregated
        // just increase its usage counter
        w->target_fds[fd]++;
        return;
    }

    // increase its usage counter
    // so that we will not add it again
    w->target_fds[fd]++;

    aggregage_fd_type_on_openfds(all_files[fd].type, &w->openfds);
}

void aggregate_pid_fds_on_targets(struct pid_stat *p) {
    if(enable_file_charts == CONFIG_BOOLEAN_AUTO && all_files_len > MAX_SYSTEM_FD_TO_ALLOW_FILES_PROCESSING) {
        nd_log(NDLS_COLLECTORS, NDLP_NOTICE, "apps.plugin: the number of system file descriptors are too many (%u), "
                                             "disabling file charts. If you want this enabled, set the 'with-files' "
                                             "parameter to [plugin:apps] section of netdata.conf", all_files_size);

        enable_file_charts = CONFIG_BOOLEAN_NO;
        obsolete_file_charts = true;
        return;
    }

    if(unlikely(!p->updated)) {
        // the process is not running
        return;
    }

    struct target
#if (PROCESSES_HAVE_UID == 1)
        *u = p->uid_target,
#endif
#if (PROCESSES_HAVE_GID == 1)
        *g = p->gid_target,
#endif
        *w = p->target;

    reallocate_target_fds(w);
#if (PROCESSES_HAVE_UID == 1)
    reallocate_target_fds(u);
#endif
#if (PROCESSES_HAVE_GID == 1)
    reallocate_target_fds(g);
#endif

#if (PROCESSES_HAVE_FDS == 1)
    p->openfds.files = 0;
    p->openfds.pipes = 0;
    p->openfds.sockets = 0;
    p->openfds.inotifies = 0;
    p->openfds.eventfds = 0;
    p->openfds.timerfds = 0;
    p->openfds.signalfds = 0;
    p->openfds.eventpolls = 0;
    p->openfds.other = 0;

    uint32_t c, size = p->fds_size;
    struct pid_fd *fds = p->fds;
    for(c = 0; c < size ;c++) {
        int fd = fds[c].fd;

        if(likely(fd <= 0 || (uint32_t)fd >= all_files_size))
            continue;

        aggregage_fd_type_on_openfds(all_files[fd].type, &p->openfds);

        aggregate_fd_on_target(fd, w);
#if (PROCESSES_HAVE_UID == 1)
        aggregate_fd_on_target(fd, u);
#endif
#if (PROCESSES_HAVE_GID == 1)
        aggregate_fd_on_target(fd, g);
#endif
    }
#endif
}

// ----------------------------------------------------------------------------

int file_descriptor_compare(void* a, void* b) {
#ifdef NETDATA_INTERNAL_CHECKS
    if(((struct file_descriptor *)a)->magic != 0x0BADCAFE || ((struct file_descriptor *)b)->magic != 0x0BADCAFE)
        netdata_log_error("Corrupted index data detected. Please report this.");
#endif /* NETDATA_INTERNAL_CHECKS */

    if(((struct file_descriptor *)a)->hash < ((struct file_descriptor *)b)->hash)
        return -1;

    else if(((struct file_descriptor *)a)->hash > ((struct file_descriptor *)b)->hash)
        return 1;

    else
        return strcmp(((struct file_descriptor *)a)->name, ((struct file_descriptor *)b)->name);
}

// int file_descriptor_iterator(avl_t *a) { if(a) {}; return 0; }

avl_tree_type all_files_index = {
    NULL,
    file_descriptor_compare
};

static struct file_descriptor *file_descriptor_find(const char *name, uint32_t hash) {
    struct file_descriptor tmp;
    tmp.hash = (hash)?hash:simple_hash(name);
    tmp.name = name;
    tmp.count = 0;
    tmp.pos = 0;
#ifdef NETDATA_INTERNAL_CHECKS
    tmp.magic = 0x0BADCAFE;
#endif /* NETDATA_INTERNAL_CHECKS */

    return (struct file_descriptor *)avl_search(&all_files_index, (avl_t *) &tmp);
}

#define file_descriptor_add(fd) avl_insert(&all_files_index, (avl_t *)(fd))
#define file_descriptor_remove(fd) avl_remove(&all_files_index, (avl_t *)(fd))

// ----------------------------------------------------------------------------

void file_descriptor_not_used(int id) {
    if(id > 0 && (uint32_t)id < all_files_size) {

#ifdef NETDATA_INTERNAL_CHECKS
        if(all_files[id].magic != 0x0BADCAFE) {
            netdata_log_error("Ignoring request to remove empty file id %d.", id);
            return;
        }
#endif /* NETDATA_INTERNAL_CHECKS */

        debug_log("decreasing slot %d (count = %d).", id, all_files[id].count);

        if(all_files[id].count > 0) {
            all_files[id].count--;

            if(!all_files[id].count) {
                debug_log("  >> slot %d is empty.", id);

                if(unlikely(file_descriptor_remove(&all_files[id]) != (void *)&all_files[id]))
                    netdata_log_error("INTERNAL ERROR: removal of unused fd from index, removed a different fd");

#ifdef NETDATA_INTERNAL_CHECKS
                all_files[id].magic = 0x00000000;
#endif /* NETDATA_INTERNAL_CHECKS */
                all_files_len--;
            }
        }
        else
            netdata_log_error("Request to decrease counter of fd %d (%s), while the use counter is 0",
                              id, all_files[id].name);
    }
    else
        netdata_log_error("Request to decrease counter of fd %d, which is outside the array size (1 to %"PRIu32")",
                          id, all_files_size);
}

static inline void all_files_grow() {
    void *old = all_files;

    uint32_t new_size = (all_files_size > 0) ? all_files_size * 2 : 2048;

    // there is no empty slot
    all_files = reallocz(all_files, new_size * sizeof(struct file_descriptor));

    // if the address changed, we have to rebuild the index
    // since all pointers are now invalid

    if(unlikely(old && old != (void *)all_files)) {
        all_files_index.root = NULL;
        for(uint32_t i = 0; i < all_files_size; i++) {
            if(!all_files[i].count) continue;
            if(unlikely(file_descriptor_add(&all_files[i]) != (void *)&all_files[i]))
                netdata_log_error("INTERNAL ERROR: duplicate indexing of fd during realloc.");
        }
    }

    // initialize the newly added entries

    for(uint32_t i = all_files_size; i < new_size; i++) {
        all_files[i].count = 0;
        all_files[i].name = NULL;
#ifdef NETDATA_INTERNAL_CHECKS
        all_files[i].magic = 0x00000000;
#endif /* NETDATA_INTERNAL_CHECKS */
        all_files[i].pos = i;
    }

    if(unlikely(!all_files_size)) all_files_len = 1;
    all_files_size = new_size;
}

static inline uint32_t file_descriptor_set_on_empty_slot(const char *name, uint32_t hash, FD_FILETYPE type) {
    // check we have enough memory to add it
    if(!all_files || all_files_len == all_files_size)
        all_files_grow();

    debug_log("  >> searching for empty slot.");

    // search for an empty slot

    static int last_pos = 0;
    uint32_t i, c;
    for(i = 0, c = last_pos ; i < all_files_size ; i++, c++) {
        if(c >= all_files_size) c = 0;
        if(c == 0) continue;

        if(!all_files[c].count) {
            debug_log("  >> Examining slot %d.", c);

#ifdef NETDATA_INTERNAL_CHECKS
            if(all_files[c].magic == 0x0BADCAFE && all_files[c].name && file_descriptor_find(all_files[c].name, all_files[c].hash))
                netdata_log_error("fd on position %"PRIu32" is not cleared properly. It still has %s in it.", c, all_files[c].name);
#endif /* NETDATA_INTERNAL_CHECKS */

            debug_log("  >> %s fd position %d for %s (last name: %s)", all_files[c].name?"re-using":"using", c, name, all_files[c].name);

            freez((void *)all_files[c].name);
            all_files[c].name = NULL;
            last_pos = c;
            break;
        }
    }

    all_files_len++;

    if(i == all_files_size) {
        fatal("We should find an empty slot, but there isn't any");
        exit(1);
    }
    // else we have an empty slot in 'c'

    debug_log("  >> updating slot %d.", c);

    all_files[c].name = strdupz(name);
    all_files[c].hash = hash;
    all_files[c].type = type;
    all_files[c].pos  = c;
    all_files[c].count = 1;
#ifdef NETDATA_INTERNAL_CHECKS
    all_files[c].magic = 0x0BADCAFE;
#endif /* NETDATA_INTERNAL_CHECKS */
    if(unlikely(file_descriptor_add(&all_files[c]) != (void *)&all_files[c]))
        netdata_log_error("INTERNAL ERROR: duplicate indexing of fd.");

    return c;
}

uint32_t file_descriptor_find_or_add(const char *name, uint32_t hash) {
    if(unlikely(!hash))
        hash = simple_hash(name);

    debug_log("adding or finding name '%s' with hash %u", name, hash);

    struct file_descriptor *fd = file_descriptor_find(name, hash);
    if(fd) {
        // found
        debug_log("  >> found on slot %d", fd->pos);

        fd->count++;
        return fd->pos;
    }
    // not found

    FD_FILETYPE type;
    if(likely(name[0] == '/')) type = FILETYPE_FILE;
    else if(likely(strncmp(name, "pipe:", 5) == 0)) type = FILETYPE_PIPE;
    else if(likely(strncmp(name, "socket:", 7) == 0)) type = FILETYPE_SOCKET;
    else if(likely(strncmp(name, "anon_inode:", 11) == 0)) {
        const char *t = &name[11];

        if(strcmp(t, "inotify") == 0) type = FILETYPE_INOTIFY;
        else if(strcmp(t, "[eventfd]") == 0) type = FILETYPE_EVENTFD;
        else if(strcmp(t, "[eventpoll]") == 0) type = FILETYPE_EVENTPOLL;
        else if(strcmp(t, "[timerfd]") == 0) type = FILETYPE_TIMERFD;
        else if(strcmp(t, "[signalfd]") == 0) type = FILETYPE_SIGNALFD;
        else {
            debug_log("UNKNOWN anonymous inode: %s", name);
            type = FILETYPE_OTHER;
        }
    }
    else if(likely(strcmp(name, "inotify") == 0)) type = FILETYPE_INOTIFY;
    else {
        debug_log("UNKNOWN linkname: %s", name);
        type = FILETYPE_OTHER;
    }

    return file_descriptor_set_on_empty_slot(name, hash, type);
}

void clear_pid_fd(struct pid_fd *pfd) {
    pfd->fd = 0;

#if defined(OS_LINUX)
    pfd->link_hash = 0;
    pfd->inode = 0;
    pfd->cache_iterations_counter = 0;
    pfd->cache_iterations_reset = 0;
#endif
}

void make_all_pid_fds_negative(struct pid_stat *p) {
    struct pid_fd *pfd = p->fds, *pfdend = &p->fds[p->fds_size];
    while(pfd < pfdend) {
        pfd->fd = -(pfd->fd);
        pfd++;
    }
}

static inline void cleanup_negative_pid_fds(struct pid_stat *p) {
    struct pid_fd *pfd = p->fds, *pfdend = &p->fds[p->fds_size];

    while(pfd < pfdend) {
        int fd = pfd->fd;

        if(unlikely(fd < 0)) {
            file_descriptor_not_used(-(fd));
            clear_pid_fd(pfd);
        }

        pfd++;
    }
}

void init_pid_fds(struct pid_stat *p, size_t first, size_t size) {
    struct pid_fd *pfd = &p->fds[first], *pfdend = &p->fds[first + size];

    while(pfd < pfdend) {
#if defined(OS_LINUX)
        pfd->filename = NULL;
#endif
        clear_pid_fd(pfd);
        pfd++;
    }
}

int read_pid_file_descriptors(struct pid_stat *p, void *ptr) {
    bool ret = OS_FUNCTION(apps_os_read_pid_fds)(p, ptr);
    cleanup_negative_pid_fds(p);

    return ret ? 1 : 0;
}
#endif