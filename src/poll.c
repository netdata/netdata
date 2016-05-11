#include <fcntl.h>
#include <pthread.h>
#include <signal.h>
#include <stdlib.h>
#include <string.h>
#include <sys/select.h>
#include <sys/stat.h>
#include <sys/types.h>

#include "common.h"
#include "log.h"

#include "poll.h"

// ---------------------------------------------------------------------------
// internal data structures
struct poll_file {
	int fd;
	char *path;
	uint32_t path_hash;
	// One of POLLIN, POLLOUT or POLLERR
	short type;
	// Last update timestamp
	struct timeval tv;
	int num_checker;
	struct poll_file *next;
};
// Linked list of files to poll.
struct poll_file *poll_file_head;

// Every registrator holds a void pointer to one of these.
struct poll_check {
	struct poll_file *poll_file;
	struct timeval tv;
};

pthread_t poll_thread;

// ---------------------------------------------------------------------------
// poll locks

// lock the global linked list.
pthread_mutex_t poll_file_list_mtx = PTHREAD_MUTEX_INITIALIZER;

static inline int poll_file_list_lock() {
	if(pthread_mutex_lock(&poll_file_list_mtx) == 0) {
		return 0;
	} else {
		error("Unable to lock poll file list");
		return -1;
	}
}

static inline int poll_file_list_unlock() {
	if(pthread_mutex_unlock(&poll_file_list_mtx) == 0) {
		return 0;
	} else {
		error("Unable to unlock poll file list");
		return -1;
	}
}

// ---------------------------------------------------------------------------
// internal signal handler.
void poll_handler(int signo)
{
	if(signo){} // Remove compiler warning
}

// ---------------------------------------------------------------------------
// internal methods
int poll_time_update_nolock(struct timeval *tv) {
	int retv = gettimeofday(tv, NULL);
	if(retv != 0) {
		error("Could not get current time");
	}
	return retv;
}

// if bigger is less than lower we return 0.
unsigned long long poll_time_difference_nolock(struct timeval *bigger, struct timeval *lower) {
	struct timeval diff;
	if(timercmp(bigger, lower, >)) {
		timersub(bigger, lower, &diff);
		return (diff.tv_sec * 1000000) + diff.tv_usec;
	} else {
		return 0;
	}
}

struct poll_file *poll_file_list_search_nolock(char *path, int type) {
	struct poll_file *p = NULL;
	uint32_t hash = simple_hash(path);
	for(p = poll_file_head; p ; p = p->next)
		if(p->path_hash == hash && p->type == type && strncmp(p->path, path, FILENAME_MAX) == 0)
			break;
	return p;
}

void poll_file_list_prepend_nolock(struct poll_file *p) {
	struct poll_file *buff = poll_file_head;
	poll_file_head = p;
	p->next = buff;
}

struct poll_file *poll_file_list_remove_nolock(struct poll_file *p) {
	struct poll_file *curr = poll_file_head;

	if(!curr)
		return NULL;

	if(curr == p) {
		poll_file_head = curr->next;
		return curr->next;
	}

	while(curr->next != NULL) {
		if(curr->next == p) {
			struct poll_file *retv = curr->next;
			curr->next=curr->next->next;
			return retv;
		} else {
			curr = curr->next;
		}
	}  
	return NULL;
}

struct poll_file *poll_file_init(char *path, int type) {
	struct poll_file *p;

	p = malloc(sizeof(struct poll_file));
	if(!p) {
		error("Cannot allocate memory for poll file.");
		return NULL;
	}

	// Open file pathname
	if( (p->fd = open(path, O_RDONLY)) == -1 ) {
		error("Cannot open %s for reading", path);
		free(p);
		return NULL;
	}

	if(!(p->path = strndup(path, FILENAME_MAX))) {
		error("Cannot duplicate %s", path);
		free(p);
		return NULL;
	}

	p->path_hash = simple_hash(path);

	if( type == POLLIN || type == POLLOUT || type == POLLERR ) {
		p->type = type;
	} else {
		error("poll_file_register: Wrong type specified");
		free(p->path);
		free(p);
		return NULL;
	}

	if(poll_time_update_nolock(&p->tv) != 0) {
		p->tv.tv_sec = 0;
		p->tv.tv_usec = 0;
	}

	p->num_checker = 0;
	p->next = NULL;

	return p;
}

void poll_file_free(struct poll_file *p) {
	free(p->path);
	close(p->fd);
	free(p);
}

struct poll_check *poll_check_init_nolock(struct poll_file *p) {
	struct poll_check *retv = malloc(sizeof(struct poll_check));
	if(!retv) {
		error("Cannot allocate memory for poll_check");
		return NULL;
	}

	retv->poll_file = p;
	retv->tv.tv_sec = p->tv.tv_sec;
	retv->tv.tv_usec = p->tv.tv_usec;
	p->num_checker++;

	return retv;
}

void poll_check_free(struct poll_check *p_check) {
	free(p_check);
}

// Interrupts select if the process is currently blocking.
void poll_interrupt() {
	if(poll_thread) {
		pthread_kill(poll_thread, SIGUSR2);
	} else {
		// We do not return NULL here. After the next poll of any other file
		// the thread will include the new file anyways.
		error("Cannot notify the polling thread.\n");
	}
}

// ---------------------------------------------------------------------------
// API - main loop
void *poll_main(void *ptr) {
	if(ptr) { ; }

	info("TC thread created with task id %d", gettid());

	if(pthread_setcanceltype(PTHREAD_CANCEL_DEFERRED, NULL) != 0)
		error("Cannot set pthread cancel type to DEFERRED.");

	if(pthread_setcancelstate(PTHREAD_CANCEL_ENABLE, NULL) != 0)
		error("Cannot set pthread cancel state to ENABLE.");

	poll_thread = pthread_self();

	// Recieve signal USR2 and add a signal handler.
	{
		sigset_t sigset;
		sigemptyset(&sigset);
		sigaddset(&sigset, SIGUSR2);

		if(pthread_sigmask(SIG_UNBLOCK, &sigset, NULL) == -1) {
			error("Could not unblock USR2 for the polling thread");
		}

		struct sigaction sa;
		sigemptyset(&sa.sa_mask);
		sa.sa_handler = poll_handler;
		sa.sa_flags = 0;
		if(sigaction(SIGUSR2, &sa, NULL) == -1) {
			error("Failed to change signal handler for SIGHUP");
		}
	}

	fd_set readfds, writefds, exceptfds;
	struct poll_file *p;
	int nfds;

	// Infinite Poll
	while(1) {
		FD_ZERO(&readfds);
		FD_ZERO(&writefds);
		FD_ZERO(&exceptfds);
		nfds = 0;

		//-------------------------------------------------------------------------
		// Initialize select
		poll_file_list_lock();
		for(p = poll_file_head; p ; p = p->next) {
			switch(p->type) {
				case POLLIN:
					FD_SET(p->fd, &readfds);
					break;
				case POLLOUT:
					FD_SET(p->fd, &writefds);
					break;
				case POLLERR:
					FD_SET(p->fd, &exceptfds);
					break;
				default: continue;
			}
			if(p->fd > nfds)
				nfds = p->fd;
		}
		poll_file_list_unlock();
		//-------------------------------------------------------------------------

		select(nfds + 1, &readfds, &writefds, &exceptfds, NULL);

		//-------------------------------------------------------------------------
		// Handle polled files
		poll_file_list_lock();
		for(p = poll_file_head; p ; p = p->next) {
			switch(p->type) {
				case POLLIN:
					if(FD_ISSET(p->fd, &readfds)) {
						poll_time_update_nolock(&p->tv);
					}
					break;
				case POLLOUT:
					if(FD_ISSET(p->fd, &writefds)) {
						poll_time_update_nolock(&p->tv);
					}
					break;
				case POLLERR:
					if(FD_ISSET(p->fd, &exceptfds)) {
						poll_time_update_nolock(&p->tv);
					}
					break;
			}
		}
		poll_file_list_unlock();
		//-------------------------------------------------------------------------
	}
}

// ---------------------------------------------------------------------------
// API - basic methods
void *poll_file_register(char *path, int type) {
	struct poll_file *p;

	// Search if we already poll this file.
	poll_file_list_lock();
	p = poll_file_list_search_nolock(path, type);
	poll_file_list_unlock();

	// If not found, init and add poll_file.
	if(!p) {
		p = poll_file_init(path, type);
		poll_file_list_lock();
		poll_file_list_prepend_nolock(p);
		poll_file_list_unlock();
	}

	// Add a new checker
	poll_file_list_lock();
	struct poll_check *retv = poll_check_init_nolock(p);
	poll_file_list_unlock();

	// Notify poll thread to start watching the new file
	poll_interrupt();

	return retv;
}

unsigned long long poll_occured(void *poll_descriptor) {
	struct poll_check *p_check = poll_descriptor;
	unsigned long long retv;

	poll_file_list_lock();
	retv = poll_time_difference_nolock(&p_check->poll_file->tv, &p_check->tv);
	poll_file_list_unlock();

	if(poll_time_update_nolock(&p_check->tv) != 0) return 0;

	return retv;
}

// Remove a file from the list of polled files
unsigned long long poll_file_unregister(void *poll_descriptor) {
	// Lock the mutex and interrupt select at the polling thread.
	// We close the file handler at the end.
	// We do this because closing a file handler used by select is unspecified.
	poll_file_list_lock();
	poll_interrupt();

	struct poll_check *p_check = poll_descriptor;
	unsigned long long retv = poll_time_difference_nolock(&p_check->poll_file->tv, &p_check->tv);

	if(!(p_check->poll_file->num_checker--)) {
		// Remove poll file from the list.
		if(!poll_file_list_remove_nolock(p_check->poll_file)) {
			error("Could not remove poll file from the list.");
			poll_file_list_unlock();
			return -1;
		}
		poll_file_free(p_check->poll_file);
	}

	poll_check_free(p_check);

	poll_file_list_unlock();
	return retv;
}
