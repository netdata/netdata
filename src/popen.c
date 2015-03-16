#include <signal.h>
#include <unistd.h>
#include <stdlib.h>
#include <sys/types.h>
#include <sys/wait.h>

#include "log.h"
#include "popen.h"

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
		return(fp);
	}
	// the child

	// close all files
	int i;
	for(i = sysconf(_SC_OPEN_MAX); i > 0; i--)
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
		fprintf(stderr, "Cannot set a new process group for pid %d (%s)\n", getpid(), strerror(errno));

	if( getpgid(0) != getpid() )
		fprintf(stderr, "Process group set is incorrect. Expected %d, found %d\n", getpid(), getpgid(0));

	if( setsid() != 0 )
		fprintf(stderr, "Cannot set session id for pid %d (%s)\n", getpid(), strerror(errno));

	fprintf(stdout, "MYPID %d\n", getpid());
	fflush(NULL);
#endif
	
	// ignore all signals
	for (i = 1 ; i < 65 ;i++) if(i != SIGSEGV) signal(i, SIG_DFL);

	fprintf(stderr, "executing command: '%s' on pid %d.\n", command, getpid());
 	execl("/bin/sh", "sh", "-c", command, NULL);
	exit(1);
}

void mypclose(FILE *fp)
{
	// this is a very poor implementation of pclose()
	// the caller should catch SIGCHLD and waitpid() on the exited child
	// otherwise the child will be a zombie forever

	fclose(fp);
}

void process_childs(int wait)
{
	siginfo_t info;
	int options = WEXITED;
	if(!wait) options |= WNOHANG;

	info.si_pid = 0;
	while(waitid(P_ALL, 0, &info, options) == 0) {
		if(!info.si_pid) break;
		switch(info.si_code) {
			case CLD_EXITED:
				error("pid %d exited with code %d.", info.si_pid, info.si_status);
				break;

			case CLD_KILLED:
				error("pid %d killed by signal %d.", info.si_pid, info.si_status);
				break;

			case CLD_DUMPED: 
				error("pid %d core dumped by signal %d.", info.si_pid, info.si_status);
				break;

			case CLD_STOPPED:
				error("pid %d stopped by signal %d.", info.si_pid, info.si_status);
				break;

			case CLD_TRAPPED:
				error("pid %d trapped by signal %d.", info.si_pid, info.si_status);
				break;

			case CLD_CONTINUED:
				error("pid %d continued by signal %d.", info.si_pid, info.si_status);
				break;

			default:
				error("pid %d gave us a SIGCHLD with code %d and status %d.", info.si_pid, info.si_code, info.si_status);
				break;
		}

		// prevent an infinite loop
		info.si_pid = 0;
	}
}
