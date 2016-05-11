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

pthread_mutex_t poll_mtx = PTHREAD_MUTEX_INITIALIZER;

struct poll_file {
	char *pathname;
	int fd;
	// One of POLLIN, POLLOUT or POLLERR
	short type;
	// Last update timestamp
	struct timeval *tv;
	struct poll_file *next;
};

struct poll_file *poll_file_head;

pthread_t poll_thread;

void poll_handler(int signo)
{
	if(signo){} // Remove compiler warning
}

void *poll_main(void *ptr)
{
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

	// Infinite Poll
	while(1) {
		// Pause when no file was registered for polling
		struct poll_file *p;
		fd_set readfds, writefds, exceptfds;
		int nfds;

		FD_ZERO(&readfds);
		FD_ZERO(&writefds);
		FD_ZERO(&exceptfds);
		nfds = 0;

		//-------------------------------------------------------------------------
		// Initialize select
		if(pthread_mutex_lock(&poll_mtx) != 0)
			error("Unable to lock mutex");

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
		if(pthread_mutex_unlock(&poll_mtx) != 0)
			error("Unable to unlock mutex");
		//-------------------------------------------------------------------------

		select(nfds + 1, &readfds, &writefds, &exceptfds, NULL);

		//-------------------------------------------------------------------------
		// Handle polled files
		if(pthread_mutex_lock(&poll_mtx) != 0)
			error("Unable to lock mutex");
		for(p = poll_file_head; p ; p = p->next) {
			switch(p->type) {
				case POLLIN:
					if(FD_ISSET(p->fd, &readfds)) {
						if( gettimeofday(p->tv, NULL) == -1 )
							error("gettimeofday failed");
					}
					break;
				case POLLOUT:
					if(FD_ISSET(p->fd, &writefds)) {
						if( gettimeofday(p->tv, NULL) == -1 )
							error("gettimeofday failed");
					}
					break;
				case POLLERR:
					if(FD_ISSET(p->fd, &exceptfds)) {
						if( gettimeofday(p->tv, NULL) == -1 )
							error("gettimeofday failed");
					}
					break;
			}
		}
		if(pthread_mutex_unlock(&poll_mtx) != 0)
			error("Unable to unlock mutex");
		//-------------------------------------------------------------------------
	}
}

// Add a file to the list of polled files
struct timeval* poll_file_register(char *pathname, int type) 
{
	struct poll_file *p;

	//-------------------------------------------------------------------------
	// Search file in the list.
	if(pthread_mutex_lock(&poll_mtx) != 0)
		error("Unable to lock mutex");

	for(p = poll_file_head; p ; p = p->next)
		if(p->type == type && strncmp(p->pathname, pathname, FILENAME_MAX) == 0)
			break;

	if(pthread_mutex_unlock(&poll_mtx) != 0)
		error("Unable to unlock mutex");
	//-------------------------------------------------------------------------

	if(p) return p->tv;

	//-------------------------------------------------------------------------
	// not found create new struct poll_file
	p = malloc(sizeof(struct poll_file));
	if(!p) {
		error("Cannot allocate memory for poll file.");
		return NULL;
	}
	p->tv = malloc(sizeof(struct timeval));
	if(!p) {
		error("Cannot allocate memory for poll file.");
		free(p);
		return NULL;
	}

	// Open file pathname
	if( (p->fd = open(pathname, O_RDONLY)) == -1 ) {
		error("Cannot open %s for reading", pathname);
		free(p->tv);
		free(p);
		return NULL;
	}

	if(!(p->pathname = strndup(pathname, FILENAME_MAX))) {
		error("Cannot duplicate %s", pathname);
		free(p->tv);
		free(p);
		return NULL;
	}

	if( type == POLLIN || type == POLLOUT || type == POLLERR ) {
		p->type = type;
	} else {
		error("poll_file_register: Wrong type specified");
		free(p->pathname);
		free(p->tv);
		free(p);
		return NULL;
	}

	if( gettimeofday(p->tv, NULL) == -1 ) {
		error("gettimeofday failed");
		p->tv->tv_sec = 0;
		p->tv->tv_usec = 0;
	}

	p->next = NULL;

	//-------------------------------------------------------------------------
	// prepend to the list.
	if(pthread_mutex_lock(&poll_mtx) != 0)
		error("Unable to lock mutex");

	struct poll_file *buff = poll_file_head;
	poll_file_head = p;
	p->next = buff;

	if(pthread_mutex_unlock(&poll_mtx) != 0)
		error("Unable to unlock mutex");
	//-------------------------------------------------------------------------

	// Notify poll thread to start watching the new file
	if(poll_thread) {
		pthread_kill(poll_thread, SIGUSR2);
	} else {
		// We do not return NULL here. After the next poll of any other file
		// the thread will include the new file anyways.
		error("Cannot notify the polling thread\n");
	}

	return p->tv;
}

// Remove a file from the list of polled files
struct timeval* poll_file_erase(char *pathname, int type) {
	struct poll_file *curr_p;
	struct poll_file *prev_p = NULL;
	struct timeval *tv;

	// Lock the mutex and interrupt select at the polling thread.
	// We close the file handler at the end.
	// We do this because closing a file handler used by select is unspecified.
	if(pthread_mutex_lock(&poll_mtx) != 0)
		error("Unable to lock mutex");

	// Interrupt polling before closing the file descriptor.
	pthread_kill(poll_thread, SIGUSR2);

	// Find the file in the list.
	for(curr_p = poll_file_head; curr_p ; prev_p = curr_p, curr_p = curr_p->next) {
		if(curr_p->type == type && strncmp(curr_p->pathname, pathname, FILENAME_MAX) == 0) {
			if(prev_p) {
				prev_p->next = curr_p->next;
			} else {
				poll_file_head = curr_p->next;
			}
			break;
		}
	}

	if(!curr_p) {
		error("Could not find %s at the list of  files to poll", pathname);
		if(pthread_mutex_unlock(&poll_mtx) != 0)
			error("Unable to unlock mutex");
		return NULL;
	}

	tv = curr_p->tv;
	free(curr_p->pathname);

	close(curr_p->fd);

	free(curr_p);

	if(pthread_mutex_unlock(&poll_mtx) != 0)
		error("Unable to unlock mutex");

	return tv;
}
