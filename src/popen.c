#ifdef HAVE_CONFIG_H
#include <config.h>
#endif
#include <signal.h>
#include <unistd.h>
#include <stdlib.h>
#include <sys/types.h>
#include <sys/wait.h>

#include "log.h"
#include "popen.h"
#include "common.h"

/*
struct mypopen {
	pid_t pid;
	FILE *fp;
	struct mypopen *next;
	struct mypopen *prev;
};

static struct mypopen *mypopen_root = NULL;

static void mypopen_add(FILE *fp, pid_t *pid) {
	struct mypopen *mp = malloc(sizeof(struct mypopen));
	if(!mp) {
		fatal("Cannot allocate %zu bytes", sizeof(struct mypopen))
		return;
	}

	mp->fp = fp;
	mp->pid = pid;
	mp->next = popen_root;
	mp->prev = NULL;
	if(mypopen_root) mypopen_root->prev = mp;
	mypopen_root = mp;
}

static void mypopen_del(FILE *fp) {
	struct mypopen *mp;

	for(mp = mypopen_root; mp; mp = mp->next)
		if(mp->fd == fp) break;

	if(!mp) error("Cannot find mypopen() file pointer in open childs.");
	else {
		if(mp->next) mp->next->prev = mp->prev;
		if(mp->prev) mp->prev->next = mp->next;
		if(mypopen_root == mp) mypopen_root = mp->next;
		free(mp);
	}
}
*/
#define PIPE_READ 0
#define PIPE_WRITE 1

FILE *mypopen(const char *command, pid_t *pidptr)
{
	int pipefd[2];

	if(pipe(pipefd) == -1) return NULL;

	int pid = fork();
	if(pid == -1) {
		close(pipefd[PIPE_READ]);
		close(pipefd[PIPE_WRITE]);
		return NULL;
	}
	if(pid != 0) {
		// the parent
		*pidptr = pid;
		close(pipefd[PIPE_WRITE]);
		FILE *fp = fdopen(pipefd[PIPE_READ], "r");
		/*mypopen_add(fp, pid);*/
		return(fp);
	}
	// the child

	// close all files
	int i;
	for(i = (int) (sysconf(_SC_OPEN_MAX) - 1); i > 0; i--)
		if(i != STDIN_FILENO && i != STDERR_FILENO && i != pipefd[PIPE_WRITE]) close(i);

	// move the pipe to stdout
	if(pipefd[PIPE_WRITE] != STDOUT_FILENO) {
		dup2(pipefd[PIPE_WRITE], STDOUT_FILENO);
		close(pipefd[PIPE_WRITE]);
	}

#ifdef DETACH_PLUGINS_FROM_NETDATA
	// this was an attempt to detach the child and use the suspend mode charts.d
	// unfortunatelly it does not work as expected.

	// fork again to become session leader
	pid = fork();
	if(pid == -1) fprintf(stderr, "Cannot fork again on pid %d\n", getpid());
	if(pid != 0) {
		// the parent
		exit(0);
	}

	// set a new process group id for just this child
	if( setpgid(0, 0) != 0 )
		error("Cannot set a new process group for pid %d (%s)", getpid(), strerror(errno));

	if( getpgid(0) != getpid() )
		error("Process group set is incorrect. Expected %d, found %d", getpid(), getpgid(0));

	if( setsid() != 0 )
		error("Cannot set session id for pid %d (%s)", getpid(), strerror(errno));

	fprintf(stdout, "MYPID %d\n", getpid());
	fflush(NULL);
#endif

	// reset all signals
	{
		sigset_t sigset;
		sigfillset(&sigset);

		if(pthread_sigmask(SIG_UNBLOCK, &sigset, NULL) == -1) {
			error("Could not block signals for threads");
		}
		// We only need to reset ignored signals.
		// Signals with signal handlers are reset by default.
		struct sigaction sa;
		sigemptyset(&sa.sa_mask);
		sa.sa_handler = SIG_DFL;
		sa.sa_flags = 0;
		if(sigaction(SIGPIPE, &sa, NULL) == -1) {
			error("Failed to change signal handler for SIGTERM");
		}
	}


	info("executing command: '%s' on pid %d.", command, getpid());
	execl("/bin/sh", "sh", "-c", command, NULL);
	exit(1);
}

int mypclose(FILE *fp, pid_t pid) {
	debug(D_EXIT, "Request to mypclose() on pid %d", pid);

	/*mypopen_del(fp);*/
	fclose(fp);

	siginfo_t info;
	if(waitid(P_PID, (id_t) pid, &info, WEXITED) != -1) {
		switch(info.si_code) {
			case CLD_EXITED:
				error("pid %d exited with code %d.", info.si_pid, info.si_status);
				return(info.si_status);
				break;

			case CLD_KILLED:
				error("pid %d killed by signal %d.", info.si_pid, info.si_status);
				return(-1);
				break;

			case CLD_DUMPED:
				error("pid %d core dumped by signal %d.", info.si_pid, info.si_status);
				return(-2);
				break;

			case CLD_STOPPED:
				error("pid %d stopped by signal %d.", info.si_pid, info.si_status);
				return(0);
				break;

			case CLD_TRAPPED:
				error("pid %d trapped by signal %d.", info.si_pid, info.si_status);
				return(-4);
				break;

			case CLD_CONTINUED:
				error("pid %d continued by signal %d.", info.si_pid, info.si_status);
				return(0);
				break;

			default:
				error("pid %d gave us a SIGCHLD with code %d and status %d.", info.si_pid, info.si_code, info.si_status);
				return(-5);
				break;
		}
	}
	else error("Cannot waitid() for pid %d", pid);
	return 0;
}
