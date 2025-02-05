// SPDX-License-Identifier: GPL-3.0-or-later

#include "nd-poll.h"

#ifndef POLLRDHUP
#define POLLRDHUP 0
#endif

#if defined(OS_LINUX)
#include <sys/epoll.h>

struct fd_info {
    uint32_t events;
    uint32_t last_served;
    const void *data;
};

DEFINE_JUDYL_TYPED(POINTERS, struct fd_info *);

#define MAX_EVENTS_PER_CALL 100

// Event poll context
struct nd_poll_t {
    int epoll_fd;

    struct epoll_event ev[MAX_EVENTS_PER_CALL];
    size_t last_pos;
    size_t used;

    POINTERS_JudyLSet pointers; // Judy array to store user data

    uint32_t nfds; // the number of sockets we have
    uint32_t iteration_counter;
};

// Initialize the event poll context
nd_poll_t *nd_poll_create() {
    nd_poll_t *ndpl = callocz(1, sizeof(nd_poll_t));

    ndpl->epoll_fd = epoll_create1(0);
    if (ndpl->epoll_fd < 0) {
        freez(ndpl);
        return NULL;
    }

    return ndpl;
}

static inline uint32_t nd_poll_events_to_epoll_events(nd_poll_event_t events) {
    uint32_t pevents = EPOLLERR | EPOLLHUP;
    if (events & ND_POLL_READ) pevents |= EPOLLIN;
    if (events & ND_POLL_WRITE) pevents |= EPOLLOUT;
    return pevents;
}

static inline nd_poll_event_t nd_poll_events_from_epoll_events(uint32_t events) {
    nd_poll_event_t nd_poll_events = ND_POLL_NONE;

    if (events & (EPOLLIN|EPOLLPRI|EPOLLRDNORM|EPOLLRDBAND))
        nd_poll_events |= ND_POLL_READ;

    if (events & (EPOLLOUT|EPOLLWRNORM|EPOLLWRBAND))
        nd_poll_events |= ND_POLL_WRITE;

    if (events & EPOLLERR)
        nd_poll_events |= ND_POLL_ERROR;

    if (events & (EPOLLHUP|EPOLLRDHUP))
        nd_poll_events |= ND_POLL_HUP;

    return nd_poll_events;
}

// Add a file descriptor to the event poll
bool nd_poll_add(nd_poll_t *ndpl, int fd, nd_poll_event_t events, const void *data) {
    internal_fatal(!data, "nd_poll() does not support NULL data pointers");

    struct fd_info *fdi = mallocz(sizeof(*fdi));
    fdi->data = data;
    fdi->last_served = 0;
    fdi->events = nd_poll_events_to_epoll_events(events);

    if(POINTERS_GET(&ndpl->pointers, fd) || !POINTERS_SET(&ndpl->pointers, fd, fdi)) {
        freez(fdi);
        return false;
    }

    struct epoll_event ev = {
        .events = fdi->events,
        .data.fd = fd,
    };

    bool rc = epoll_ctl(ndpl->epoll_fd, EPOLL_CTL_ADD, fd, &ev) == 0;
    if(rc)
        ndpl->nfds++;
    else {
        POINTERS_DEL(&ndpl->pointers, fd);
        freez(fdi);
    }

    internal_fatal(!rc, "epoll_ctl() failed");

    return rc;
}

// Remove a file descriptor from the event poll
bool nd_poll_del(nd_poll_t *ndpl, int fd) {
    struct fd_info *fdi = POINTERS_GET(&ndpl->pointers, fd);
    if(!fdi) return false;

    POINTERS_DEL(&ndpl->pointers, fd);
    freez(fdi);

    ndpl->nfds--; // we can't check for success/failure here, because epoll() removes fds when they are closed
    bool rc = epoll_ctl(ndpl->epoll_fd, EPOLL_CTL_DEL, fd, NULL) == 0;
    internal_error(!rc, "epoll_ctl() failed (is the socket already closed)"); // this is ok if the socket is already closed
    return rc;
}

// Update an existing file descriptor in the event poll
ALWAYS_INLINE_HOT_FLATTEN
bool nd_poll_upd(nd_poll_t *ndpl, int fd, nd_poll_event_t events) {
    struct fd_info *fdi = POINTERS_GET(&ndpl->pointers, fd);
    if(!fdi) return false;

    fdi->events = nd_poll_events_to_epoll_events(events);

    struct epoll_event ev = {
        .events = fdi->events,
        .data.fd = fd,
    };
    bool rc = epoll_ctl(ndpl->epoll_fd, EPOLL_CTL_MOD, fd, &ev) == 0;
    internal_fatal(!rc, "epoll_ctl() failed"); // this may happen if fd is closed
    return rc;
}

static inline bool nd_poll_get_next_event(nd_poll_t *ndpl, nd_poll_result_t *result) {
    while(ndpl->last_pos < ndpl->used) {
        struct fd_info *fdi = POINTERS_GET(&ndpl->pointers, ndpl->ev[ndpl->last_pos].data.fd);

        // Skip events that have been invalidated by nd_poll_del()
        if(!fdi || !fdi->data) {
            ndpl->last_pos++;
            continue;
        }

        *result = (nd_poll_result_t){
            .events = nd_poll_events_from_epoll_events(ndpl->ev[ndpl->last_pos].events & fdi->events),
            .data = fdi->data,
        };

        ndpl->last_pos++;

        if(!result->events)
            // nd_poll_upd() may have removed some flags since we got this
            continue;

        fdi->last_served = ndpl->iteration_counter;
        return true;
    }

    return false;
}

typedef struct {
    struct epoll_event event;
    uint32_t last_served;
} sortable_event_t;

static int compare_last_served(const void *a, const void *b) {
    const sortable_event_t *ev_a = (const sortable_event_t *)a;
    const sortable_event_t *ev_b = (const sortable_event_t *)b;

    if (ev_a->last_served < ev_b->last_served)
        return -1;
    if (ev_a->last_served > ev_b->last_served)
        return 1;

    return 0;
}

static void sort_events(nd_poll_t *ndpl) {
    if(ndpl->used <= 1) return;

    sortable_event_t sortable_array[ndpl->used];
    for (size_t i = 0; i < ndpl->used; ++i) {
        struct fd_info *fdi = POINTERS_GET(&ndpl->pointers, ndpl->ev[i].data.fd);
        sortable_array[i] = (sortable_event_t){
            .event = ndpl->ev[i],
            .last_served = fdi ? fdi->last_served : UINT32_MAX,
        };
    }

    qsort(sortable_array, ndpl->used, sizeof(sortable_event_t), compare_last_served);

    // Reorder `ndpl->ev` based on the sorted order
    for (size_t i = 0; i < ndpl->used; ++i)
        ndpl->ev[i] = sortable_array[i].event;
}

// Wait for events
ALWAYS_INLINE_HOT_FLATTEN
int nd_poll_wait(nd_poll_t *ndpl, int timeout_ms, nd_poll_result_t *result) {
    ndpl->iteration_counter++;

    if(nd_poll_get_next_event(ndpl, result))
        return 1;

    do {
        errno_clear();
        ndpl->last_pos = 0;
        ndpl->used = 0;

        int n = epoll_wait(ndpl->epoll_fd, &ndpl->ev[0], _countof(ndpl->ev), timeout_ms);

        if(unlikely(n <= 0)) {
            if(n == 0) {
                result->events = ND_POLL_TIMEOUT;
                result->data = NULL;
                return 0;
            }

            if(errno == EINTR || errno == EAGAIN)
                continue;

            result->events = ND_POLL_POLL_FAILED;
            result->data = NULL;
            return -1;
        }

        ndpl->used = n;
        ndpl->last_pos = 0;
        sort_events(ndpl);
        if (nd_poll_get_next_event(ndpl, result))
            return 1;

        internal_fatal(true, "nd_poll_get_next_event() should have 1 event!");
    } while(true);
}

static void nd_poll_free_callback(Word_t fd __maybe_unused, struct fd_info *fdi) {
    freez(fdi);
}

// Destroy the event poll context
void nd_poll_destroy(nd_poll_t *ndpl) {
    if (ndpl) {
        close(ndpl->epoll_fd);
        POINTERS_FREE(&ndpl->pointers, nd_poll_free_callback);
        freez(ndpl);
    }
}
#else

DEFINE_JUDYL_TYPED(POINTERS, const void *);

struct nd_poll_t {
    struct pollfd *fds;         // Array of file descriptors
    nfds_t nfds;                // Number of active file descriptors
    nfds_t capacity;            // Allocated capacity for `fds` array
    nfds_t last_pos;
    POINTERS_JudyLSet pointers; // Judy array to store user data
};

#define INITIAL_CAPACITY 4

// Initialize the event poll context
nd_poll_t *nd_poll_create() {
    nd_poll_t *ndpl = callocz(1, sizeof(nd_poll_t));
    ndpl->fds = mallocz(INITIAL_CAPACITY * sizeof(struct pollfd));
    ndpl->nfds = 0;
    ndpl->capacity = INITIAL_CAPACITY;

    POINTERS_INIT(&ndpl->pointers);

    return ndpl;
}

// Ensure capacity for adding new file descriptors
static void ensure_capacity(nd_poll_t *ndpl) {
    if (ndpl->nfds < ndpl->capacity) return;

    nfds_t new_capacity = ndpl->capacity * 2;
    struct pollfd *new_fds = reallocz(ndpl->fds, new_capacity * sizeof(struct pollfd));

    ndpl->fds = new_fds;
    ndpl->capacity = new_capacity;
}

static inline short int nd_poll_events_to_poll_events(nd_poll_event_t events) {
    short int pevents = POLLERR | POLLHUP | POLLNVAL;
    if (events & ND_POLL_READ) pevents |= POLLIN;
    if (events & ND_POLL_WRITE) pevents |= POLLOUT;
    return pevents;
}

static inline nd_poll_event_t nd_poll_events_from_poll_revents(short int events) {
    nd_poll_event_t nd_poll_events = ND_POLL_NONE;

    if (events & (POLLIN|POLLPRI|POLLRDNORM|POLLRDBAND))
        nd_poll_events |= ND_POLL_READ;

    if (events & (POLLOUT|POLLWRNORM|POLLWRBAND))
        nd_poll_events |= ND_POLL_WRITE;

    if (events & POLLERR)
        nd_poll_events |= ND_POLL_ERROR;

    if (events & (POLLHUP|POLLRDHUP))
        nd_poll_events |= ND_POLL_HUP;

    if (events & (POLLNVAL))
        nd_poll_events |= ND_POLL_INVALID;

    return nd_poll_events;
}

bool nd_poll_add(nd_poll_t *ndpl, int fd, nd_poll_event_t events, const void *data) {
    internal_fatal(POINTERS_GET(&ndpl->pointers, fd) != NULL, "File descriptor %d is already served - cannot add", fd);

    if(POINTERS_GET(&ndpl->pointers, fd) || !POINTERS_SET(&ndpl->pointers, fd, data))
        return false;

    ensure_capacity(ndpl);
    struct pollfd *pfd = &ndpl->fds[ndpl->nfds++];
    pfd->fd = fd;
    pfd->events = nd_poll_events_to_poll_events(events);
    pfd->revents = 0;

    return true;
}

// Remove a file descriptor from the event poll
bool nd_poll_del(nd_poll_t *ndpl, int fd) {
    if(!POINTERS_DEL(&ndpl->pointers, fd))
        return false;

    for (nfds_t i = 0; i < ndpl->nfds; i++) {
        if (ndpl->fds[i].fd == fd) {

            // Remove the file descriptor by shifting the array
            memmove(&ndpl->fds[i], &ndpl->fds[i + 1], (ndpl->nfds - i - 1) * sizeof(struct pollfd));
            ndpl->nfds--;

            if(i < ndpl->last_pos)
                ndpl->last_pos--;

            return true;
        }
    }

    return false; // File descriptor not found
}

// Update an existing file descriptor in the event poll
ALWAYS_INLINE_HOT_FLATTEN
bool nd_poll_upd(nd_poll_t *ndpl, int fd, nd_poll_event_t events) {
    for (nfds_t i = 0; i < ndpl->nfds; i++) {
        if (ndpl->fds[i].fd == fd) {
            struct pollfd *pfd = &ndpl->fds[i];
            pfd->events = nd_poll_events_to_poll_events(events);
            return true;
        }
    }

    // File descriptor not found
    return false;
}

static inline bool nd_poll_get_next_event(nd_poll_t *ndpl, nd_poll_result_t *result) {
    for (nfds_t i = ndpl->last_pos; i < ndpl->nfds; i++) {
        if (ndpl->fds[i].revents != 0) {

            result->data = POINTERS_GET(&ndpl->pointers, ndpl->fds[i].fd);
            if(!result->data)
                continue;

            result->events = nd_poll_events_from_poll_revents(ndpl->fds[i].revents & ndpl->fds[i].events);
            if(!result->events)
                // nd_poll_upd() may have removed some flags since we got this
                continue;

            ndpl->fds[i].revents = 0;

            ndpl->last_pos = i + 1;
            return true;
        }
    }

    ndpl->last_pos = ndpl->nfds;
    return false;
}

// Rotate the fds array to prevent starvation
static inline void rotate_fds(nd_poll_t *ndpl) {
    if (ndpl->nfds == 0 || ndpl->nfds == 1)
        return; // No rotation needed for empty or single-entry arrays

    struct pollfd first = ndpl->fds[0];
    memmove(&ndpl->fds[0], &ndpl->fds[1], (ndpl->nfds - 1) * sizeof(struct pollfd));
    ndpl->fds[ndpl->nfds - 1] = first;
}

// Wait for events
ALWAYS_INLINE_HOT_FLATTEN
int nd_poll_wait(nd_poll_t *ndpl, int timeout_ms, nd_poll_result_t *result) {
    if (nd_poll_get_next_event(ndpl, result))
        return 1; // Return immediately if there's a pending event

    do {
        errno_clear();
        ndpl->last_pos = 0;
        rotate_fds(ndpl); // Rotate the array on every wait
        int ret = poll(ndpl->fds, ndpl->nfds, timeout_ms);

        if(unlikely(ret <= 0)) {
            if(ret == 0) {
                result->events = ND_POLL_TIMEOUT;
                result->data = NULL;
                return 0;
            }

            if(errno == EAGAIN || errno == EINTR)
                continue;

            result->events = ND_POLL_POLL_FAILED;
            result->data = NULL;
            return -1;
        }

        // Process the next event
        if (nd_poll_get_next_event(ndpl, result))
            return 1;

        internal_fatal(true, "nd_poll_get_next_event() should have 1 event!");
    } while (true);
}

// Destroy the event poll context
void nd_poll_destroy(nd_poll_t *ndpl) {
    if (ndpl) {
        free(ndpl->fds);
        POINTERS_FREE(&ndpl->pointers, NULL);
        freez(ndpl);
    }
}

#endif
