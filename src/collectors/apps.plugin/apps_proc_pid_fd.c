// SPDX-License-Identifier: GPL-3.0-or-later

#include "apps_plugin.h"

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

    if(unlikely(!p->updated)) {
        // the process is not running
        return;
    }

    struct target
        *w = p->target,
#if (PROCESSES_HAVE_UID == 1)
        *u = p->uid_target,
#endif
#if (PROCESSES_HAVE_GID == 1)
        *g = p->gid_target,
#endif
        *t = p->tree_target;

    reallocate_target_fds(w);
#if (PROCESSES_HAVE_UID == 1)
    reallocate_target_fds(u);
#endif
#if (PROCESSES_HAVE_GID == 1)
    reallocate_target_fds(g);
#endif
    reallocate_target_fds(t);

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
        aggregate_fd_on_target(fd, t);
    }
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

    // there is no empty slot
    all_files = reallocz(all_files, (all_files_size + FILE_DESCRIPTORS_INCREASE_STEP) * sizeof(struct file_descriptor));

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

    for(uint32_t i = all_files_size; i < (all_files_size + FILE_DESCRIPTORS_INCREASE_STEP); i++) {
        all_files[i].count = 0;
        all_files[i].name = NULL;
#ifdef NETDATA_INTERNAL_CHECKS
        all_files[i].magic = 0x00000000;
#endif /* NETDATA_INTERNAL_CHECKS */
        all_files[i].pos = i;
    }

    if(unlikely(!all_files_size)) all_files_len = 1;
    all_files_size += FILE_DESCRIPTORS_INCREASE_STEP;
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

static inline uint32_t file_descriptor_find_or_add(const char *name, uint32_t hash) {
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

static inline void make_all_pid_fds_negative(struct pid_stat *p) {
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

#if defined(OS_MACOS)
static bool read_pid_file_descriptors_per_os(struct pid_stat *p, void *ptr __maybe_unused) {
    static struct proc_fdinfo *fds = NULL;
    static int fdsCapacity = 0;

    int bufferSize = proc_pidinfo(p->pid, PROC_PIDLISTFDS, 0, NULL, 0);
    if (bufferSize <= 0) {
        netdata_log_error("Failed to get the size of file descriptors for PID %d", p->pid);
        return false;
    }

    // Resize buffer if necessary
    if (bufferSize > fdsCapacity) {
        if(fds)
            freez(fds);

        fds = mallocz(bufferSize);
        fdsCapacity = bufferSize;
    }

    int num_fds = proc_pidinfo(p->pid, PROC_PIDLISTFDS, 0, fds, bufferSize) / PROC_PIDLISTFD_SIZE;
    if (num_fds <= 0) {
        netdata_log_error("Failed to get the file descriptors for PID %d", p->pid);
        return false;
    }

    for (int i = 0; i < num_fds; i++) {
        switch (fds[i].proc_fdtype) {
            case PROX_FDTYPE_VNODE: {
                struct vnode_fdinfowithpath vi;
                if (proc_pidfdinfo(p->pid, fds[i].proc_fd, PROC_PIDFDVNODEPATHINFO, &vi, sizeof(vi)) > 0)
                    p->openfds.files++;
                else
                    p->openfds.other++;

                break;
            }
            case PROX_FDTYPE_SOCKET: {
                p->openfds.sockets++;
                break;
            }
            case PROX_FDTYPE_PIPE: {
                p->openfds.pipes++;
                break;
            }

            default:
                p->openfds.other++;
                break;
        }
    }

    return true;
}
#endif

#if defined(OS_FREEBSD)
static bool read_pid_file_descriptors_per_os(struct pid_stat *p, void *ptr) {
    int mib[4];
    size_t size;
    struct kinfo_file *fds;
    static char *fdsbuf;
    char *bfdsbuf, *efdsbuf;
    char fdsname[FILENAME_MAX + 1];
#define SHM_FORMAT_LEN 31 // format: 21 + size: 10
    char shm_name[FILENAME_MAX - SHM_FORMAT_LEN + 1];

    // we make all pid fds negative, so that
    // we can detect unused file descriptors
    // at the end, to free them
    make_all_pid_fds_negative(p);

    mib[0] = CTL_KERN;
    mib[1] = KERN_PROC;
    mib[2] = KERN_PROC_FILEDESC;
    mib[3] = p->pid;

    if (unlikely(sysctl(mib, 4, NULL, &size, NULL, 0))) {
        netdata_log_error("sysctl error: Can't get file descriptors data size for pid %d", p->pid);
        return false;
    }
    if (likely(size > 0))
        fdsbuf = reallocz(fdsbuf, size);
    if (unlikely(sysctl(mib, 4, fdsbuf, &size, NULL, 0))) {
        netdata_log_error("sysctl error: Can't get file descriptors data for pid %d", p->pid);
        return false;
    }

    bfdsbuf = fdsbuf;
    efdsbuf = fdsbuf + size;
    while (bfdsbuf < efdsbuf) {
        fds = (struct kinfo_file *)(uintptr_t)bfdsbuf;
        if (unlikely(fds->kf_structsize == 0))
            break;

        // do not process file descriptors for current working directory, root directory,
        // jail directory, ktrace vnode, text vnode and controlling terminal
        if (unlikely(fds->kf_fd < 0)) {
            bfdsbuf += fds->kf_structsize;
            continue;
        }

        // get file descriptors array index
        size_t fdid = fds->kf_fd;

        // check if the fds array is small
        if (unlikely(fdid >= p->fds_size)) {
            // it is small, extend it

            debug_log("extending fd memory slots for %s from %d to %d", pid_stat_comm(p), p->fds_size, fdid + MAX_SPARE_FDS);

            p->fds = reallocz(p->fds, (fdid + MAX_SPARE_FDS) * sizeof(struct pid_fd));

            // and initialize it
            init_pid_fds(p, p->fds_size, (fdid + MAX_SPARE_FDS) - p->fds_size);
            p->fds_size = fdid + MAX_SPARE_FDS;
        }

        if (unlikely(p->fds[fdid].fd == 0)) {
            // we don't know this fd, get it

            switch (fds->kf_type) {
                case KF_TYPE_FIFO:
                case KF_TYPE_VNODE:
                    if (unlikely(!fds->kf_path[0])) {
                        sprintf(fdsname, "other: inode: %lu", fds->kf_un.kf_file.kf_file_fileid);
                        break;
                    }
                    sprintf(fdsname, "%s", fds->kf_path);
                    break;
                case KF_TYPE_SOCKET:
                    switch (fds->kf_sock_domain) {
                        case AF_INET:
                        case AF_INET6:
#if __FreeBSD_version < 1400074
                            if (fds->kf_sock_protocol == IPPROTO_TCP)
                                sprintf(fdsname, "socket: %d %lx", fds->kf_sock_protocol, fds->kf_un.kf_sock.kf_sock_inpcb);
                            else
#endif
                                sprintf(fdsname, "socket: %d %lx", fds->kf_sock_protocol, fds->kf_un.kf_sock.kf_sock_pcb);
                            break;
                        case AF_UNIX:
                            /* print address of pcb and connected pcb */
                            sprintf(fdsname, "socket: %lx %lx", fds->kf_un.kf_sock.kf_sock_pcb, fds->kf_un.kf_sock.kf_sock_unpconn);
                            break;
                        default:
                            /* print protocol number and socket address */
#if __FreeBSD_version < 1200031
                            sprintf(fdsname, "socket: other: %d %s %s", fds->kf_sock_protocol, fds->kf_sa_local.__ss_pad1, fds->kf_sa_local.__ss_pad2);
#else
                            sprintf(fdsname, "socket: other: %d %s %s", fds->kf_sock_protocol, fds->kf_un.kf_sock.kf_sa_local.__ss_pad1, fds->kf_un.kf_sock.kf_sa_local.__ss_pad2);
#endif
                    }
                    break;
                case KF_TYPE_PIPE:
                    sprintf(fdsname, "pipe: %lu %lu", fds->kf_un.kf_pipe.kf_pipe_addr, fds->kf_un.kf_pipe.kf_pipe_peer);
                    break;
                case KF_TYPE_PTS:
#if __FreeBSD_version < 1200031
                    sprintf(fdsname, "other: pts: %u", fds->kf_un.kf_pts.kf_pts_dev);
#else
                    sprintf(fdsname, "other: pts: %lu", fds->kf_un.kf_pts.kf_pts_dev);
#endif
                    break;
                case KF_TYPE_SHM:
                    strncpyz(shm_name, fds->kf_path, FILENAME_MAX - SHM_FORMAT_LEN);
                    sprintf(fdsname, "other: shm: %s size: %lu", shm_name, fds->kf_un.kf_file.kf_file_size);
                    break;
                case KF_TYPE_SEM:
                    sprintf(fdsname, "other: sem: %u", fds->kf_un.kf_sem.kf_sem_value);
                    break;
                default:
                    sprintf(fdsname, "other: pid: %d fd: %d", fds->kf_un.kf_proc.kf_pid, fds->kf_fd);
            }

            // if another process already has this, we will get
            // the same id
            p->fds[fdid].fd = file_descriptor_find_or_add(fdsname, 0);
        }

        // else make it positive again, we need it
        // of course, the actual file may have changed

        else
            p->fds[fdid].fd = -p->fds[fdid].fd;

        bfdsbuf += fds->kf_structsize;
    }

    return true;
}
#endif

#if defined(OS_WINDOWS)
static bool read_pid_file_descriptors_per_os(struct pid_stat *p, void *ptr) {
    // TODO: get file descriptors per process, if available
    return false;
}
#endif

#if defined(OS_LINUX)
static bool read_pid_file_descriptors_per_os(struct pid_stat *p, void *ptr __maybe_unused) {
    if(unlikely(!p->fds_dirname)) {
        char dirname[FILENAME_MAX+1];
        snprintfz(dirname, FILENAME_MAX, "%s/proc/%d/fd", netdata_configured_host_prefix, p->pid);
        p->fds_dirname = strdupz(dirname);
    }

    DIR *fds = opendir(p->fds_dirname);
    if(unlikely(!fds)) return false;

    struct dirent *de;
    char linkname[FILENAME_MAX + 1];

    // we make all pid fds negative, so that
    // we can detect unused file descriptors
    // at the end, to free them
    make_all_pid_fds_negative(p);

    while((de = readdir(fds))) {
        // we need only files with numeric names

        if(unlikely(de->d_name[0] < '0' || de->d_name[0] > '9'))
            continue;

        // get its number
        int fdid = (int) str2l(de->d_name);
        if(unlikely(fdid < 0)) continue;

        // check if the fds array is small
        if(unlikely((size_t)fdid >= p->fds_size)) {
            // it is small, extend it

            debug_log("extending fd memory slots for %s from %d to %d"
                      , pid_stat_comm(p)
                      , p->fds_size
                      , fdid + MAX_SPARE_FDS
            );

            p->fds = reallocz(p->fds, (fdid + MAX_SPARE_FDS) * sizeof(struct pid_fd));

            // and initialize it
            init_pid_fds(p, p->fds_size, (fdid + MAX_SPARE_FDS) - p->fds_size);
            p->fds_size = (size_t)fdid + MAX_SPARE_FDS;
        }

        if(unlikely(p->fds[fdid].fd < 0 && de->d_ino != p->fds[fdid].inode)) {
            // inodes do not match, clear the previous entry
            inodes_changed_counter++;
            file_descriptor_not_used(-p->fds[fdid].fd);
            clear_pid_fd(&p->fds[fdid]);
        }

        if(p->fds[fdid].fd < 0 && p->fds[fdid].cache_iterations_counter > 0) {
            p->fds[fdid].fd = -p->fds[fdid].fd;
            p->fds[fdid].cache_iterations_counter--;
            continue;
        }

        if(unlikely(!p->fds[fdid].filename)) {
            filenames_allocated_counter++;
            char fdname[FILENAME_MAX + 1];
            snprintfz(fdname, FILENAME_MAX, "%s/proc/%d/fd/%s", netdata_configured_host_prefix, p->pid, de->d_name);
            p->fds[fdid].filename = strdupz(fdname);
        }

        file_counter++;
        ssize_t l = readlink(p->fds[fdid].filename, linkname, FILENAME_MAX);
        if(unlikely(l == -1)) {
            // cannot read the link

            if(debug_enabled || (p->target && p->target->debug_enabled))
                netdata_log_error("Cannot read link %s", p->fds[fdid].filename);

            if(unlikely(p->fds[fdid].fd < 0)) {
                file_descriptor_not_used(-p->fds[fdid].fd);
                clear_pid_fd(&p->fds[fdid]);
            }

            continue;
        }
        else
            linkname[l] = '\0';

        uint32_t link_hash = simple_hash(linkname);

        if(unlikely(p->fds[fdid].fd < 0 && p->fds[fdid].link_hash != link_hash)) {
            // the link changed
            links_changed_counter++;
            file_descriptor_not_used(-p->fds[fdid].fd);
            clear_pid_fd(&p->fds[fdid]);
        }

        if(unlikely(p->fds[fdid].fd == 0)) {
            // we don't know this fd, get it

            // if another process already has this, we will get
            // the same id
            p->fds[fdid].fd = (int)file_descriptor_find_or_add(linkname, link_hash);
            p->fds[fdid].inode = de->d_ino;
            p->fds[fdid].link_hash = link_hash;
        }
        else {
            // else make it positive again, we need it
            p->fds[fdid].fd = -p->fds[fdid].fd;
        }

        // caching control
        // without this we read all the files on every iteration
        if(max_fds_cache_seconds > 0) {
            size_t spread = ((size_t)max_fds_cache_seconds > 10) ? 10 : (size_t)max_fds_cache_seconds;

            // cache it for a few iterations
            size_t max = ((size_t) max_fds_cache_seconds + (fdid % spread)) / (size_t) update_every;
            p->fds[fdid].cache_iterations_reset++;

            if(unlikely(p->fds[fdid].cache_iterations_reset % spread == (size_t) fdid % spread))
                p->fds[fdid].cache_iterations_reset++;

            if(unlikely((fdid <= 2 && p->fds[fdid].cache_iterations_reset > 5) ||
                         p->fds[fdid].cache_iterations_reset > max)) {
                // for stdin, stdout, stderr (fdid <= 2) we have checked a few times, or if it goes above the max, goto max
                p->fds[fdid].cache_iterations_reset = max;
            }

            p->fds[fdid].cache_iterations_counter = p->fds[fdid].cache_iterations_reset;
        }
    }

    closedir(fds);

    return true;
}
#endif

int read_pid_file_descriptors(struct pid_stat *p, void *ptr) {
    bool ret = read_pid_file_descriptors_per_os(p, ptr);
    cleanup_negative_pid_fds(p);

    return ret ? 1 : 0;
}
