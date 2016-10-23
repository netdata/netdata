#include <fcntl.h>
#include <pthread.h>
#include <signal.h>
#include <stdlib.h>
#include <string.h>
#include <sys/poll.h>
#include <sys/stat.h>
#include <sys/types.h>

#include "common.h"
#include "poll.h"
#include "log.h"

// ---------------------------------------------------------------------------
// internal data structures

/*
 * We maintain two concurrent arrays.
 * poll_fd_array which is the data structue given to poll and
 * poll_data_array which contains metadata we also need.
 */
struct pollfd *poll_fd_array = NULL;
struct poll_data {
	char *path;
	uint32_t path_hash;
	// Last update timestamp
	struct timeval *tv;
	int num_checker;
} *poll_data_array;

int poll_num = 0; // Size of poll_fd_array|poll_data_array

// Every registrator holds a void pointer to one of these.
struct poll_check {
	int poll_array_index;
	// Last check timestamp.
	struct timeval tv;
};

pthread_t poll_thread;

// ---------------------------------------------------------------------------
// poll locks

// Lock poll data structure (poll_fd_array, poll_data_array, poll_num and 
// poll_first_deprecated.
pthread_mutex_t poll_array_mtx = PTHREAD_MUTEX_INITIALIZER;

static inline int poll_array_lock() {
	if(pthread_mutex_lock(&poll_array_mtx) == 0) {
		return 0;
	} else {
		error("Unable to lock poll file list");
		return -1;
	}
}

static inline int poll_array_unlock() {
	if(pthread_mutex_unlock(&poll_array_mtx) == 0) {
		return 0;
	} else {
		error("Unable to unlock poll file list");
		return -1;
	}
}

// ---------------------------------------------------------------------------
// internal signal handler used to interrupt poll.
void poll_handler(int signo)
{
	if(signo){} // Remove compiler warning
}

// ---------------------------------------------------------------------------
// internal methods
int poll_time_update_nolock(struct timeval *tv) {
	int retv = gettimeofday(tv, NULL);
	if(retv != 0) {
		switch(errno) {
			case EFAULT:
			case EINVAL:
				error("Could not get current time");
				break;
			case EPERM:
				error("CAP_SYS_TIME capability is required to get current time");
				break;
		}
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

int poll_array_search_nolock(char *path, int type) {
	uint32_t hash = simple_hash(path);
	int i;
	for(i = 0; i < poll_num; i++)
		if(poll_fd_array[i].fd < 0 && poll_data_array[i].path_hash == hash && poll_fd_array[i].events == type && \
				strncmp(poll_data_array[i].path, path, strlen(poll_data_array[i].path)) == 0)
			return i;
	return -1;
}

int poll_array_find_first_deprecated_nolock() {
	int index = 0;
	for(index; index<poll_num; index++)
		if(poll_fd_array[index].fd < 0)
			return index;
	return -1;
}

void poll_array_remove_nolock(int index) {
	if(poll_fd_array[index].fd >= 0)
		if(close(poll_fd_array[index].fd) != 0)
			error("Failed to proper close file descriptor %d", poll_fd_array[index].fd);
	poll_fd_array[index].fd = -1;	
}

int poll_array_add_nolock(char *path, int type) {
	int index;

	index = poll_array_find_first_deprecated_nolock();
	if(index >= 0) {
		poll_num++;
		poll_fd_array = realloc(poll_fd_array, sizeof(struct pollfd) * poll_num);
		if(!poll_fd_array) {
			error("Cannot allocate memory for poll_file");
			poll_num--;
			return -1;
		}

		poll_data_array = realloc(poll_data_array, sizeof(struct poll_data) * poll_num);
		if(!poll_data_array) {
			error("Cannot allocate memory for poll_data");
			poll_num--;
			return -1;
		}

		index = poll_num - 1;

		// Initialize data structure.
		poll_data_array[index].path = NULL;
		poll_data_array[index].path_hash = 0;
		poll_data_array[index].tv = NULL;
		poll_fd_array[index].fd = -1;
		poll_fd_array[index].events = 0;
		poll_fd_array[index].revents = 0;
	}

	// Open file pathname
	int access_mode = O_RDONLY;
	if(type & POLLOUT) {
		if(type & (POLLIN | POLLPRI)) {
			access_mode = O_RDWR;
		} else {
			access_mode = O_WRONLY;
		}
	}
	if( (poll_fd_array[index].fd = open(path, access_mode)) == -1 ) {
		error("Cannot open %s for reading", path);
		poll_array_remove_nolock(index);
		return -1;
	}

	if(poll_data_array[index].path)
		free(poll_data_array[index].path);
	if(!(poll_data_array[index].path = strndup(path, FILENAME_MAX))) {
		error("Cannot duplicate %s", path);
		poll_array_remove_nolock(index);
		return -1;
	}
	poll_data_array[index].path_hash = simple_hash(path);

	if(!poll_data_array[index].tv) {
		poll_data_array[index].tv = malloc(sizeof(struct timeval));
		if(!poll_data_array[index].tv) {
			error("Cannot allocate memory for timeval");
			poll_array_remove_nolock(index);
			return -1;
		}
	}
	if(poll_time_update_nolock(poll_data_array[index].tv) != 0) {
		poll_data_array[index].tv->tv_sec = 0;
		poll_data_array[index].tv->tv_usec = 0;
	}

	poll_fd_array[index].events = type;
	poll_fd_array[index].revents = 0;

	poll_data_array[index].num_checker = 0;

	return index;
}

struct poll_check *poll_check_init_nolock(int poll_array_index) {
	struct poll_check *retv = malloc(sizeof(struct poll_check));
	if(!retv) {
		error("Cannot allocate memory for poll_check");
		return NULL;
	}

	retv->poll_array_index = poll_array_index;
	retv->tv.tv_sec = poll_data_array[poll_array_index].tv->tv_sec;
	retv->tv.tv_usec = poll_data_array[poll_array_index].tv->tv_usec;
	poll_data_array[poll_array_index].num_checker++;

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

	int i;

	// Infinite Poll
	while(1) {
		poll(poll_fd_array, poll_num, -1);

		//-------------------------------------------------------------------------
		// Handle polled files
		poll_array_lock();
		for(i = 0; i < poll_num; i++) {
			if(poll_fd_array[i].events & poll_fd_array[i].revents) {
				poll_time_update_nolock(poll_data_array[i].tv);
			}
		}
		poll_array_unlock();
		//-------------------------------------------------------------------------
	}
}

// ---------------------------------------------------------------------------
// API - basic methods
void *poll_file_register(char *path, int type) {
	int i;

	// Search if we already poll this file.
	poll_array_lock();
	i = poll_array_search_nolock(path, type);
	poll_array_unlock();

	// If not found, init and add poll_file.
	if(i < 0) {
		poll_array_lock();
		i = poll_array_add_nolock(path, type);
		poll_array_unlock();
	}

	// Add a new checker
	if(i >= 0) {
		struct poll_check *retv = poll_check_init_nolock(i);
		// Notify poll thread to start watching the new file
		poll_interrupt();
		return retv;
	} else {
		return NULL;
	}
}

unsigned long long poll_occured(void *poll_descriptor) {
	if(!poll_descriptor) return 0;

	struct poll_check *p_check = poll_descriptor;
	unsigned long long retv;

	poll_array_lock();
	retv = poll_time_difference_nolock(poll_data_array[p_check->poll_array_index].tv, &p_check->tv);
	poll_array_unlock();

	if(poll_time_update_nolock(&p_check->tv) != 0) return 0;

	return retv;
}

// Remove a file from the list of polled files
unsigned long long poll_file_unregister(void *poll_descriptor) {
	if(!poll_descriptor) return 0;

	// Lock the mutex and interrupt select at the polling thread.
	// We close the file handler at the end.
	// We do this because closing a file handler used by select is unspecified.
	poll_array_lock();
	poll_interrupt();

	struct poll_check *p_check = poll_descriptor;
	unsigned long long retv = poll_time_difference_nolock(poll_data_array[p_check->poll_array_index].tv, &p_check->tv);

	if(!(poll_data_array[p_check->poll_array_index].num_checker--)) {
		// Remove poll file from the list.
		poll_array_remove_nolock(p_check->poll_array_index);
	}

	poll_check_free(p_check);

	poll_array_unlock();
	return retv;
}
