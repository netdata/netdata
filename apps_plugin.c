#include <stdio.h>
#include <stdlib.h>
#include <string.h>
#include <time.h>
#include <unistd.h>
#include <sys/types.h>
#include <sys/time.h>
#include <sys/wait.h>

#include <sys/resource.h>
#include <sys/stat.h>

#include <errno.h>
#include <stdarg.h>
#include <locale.h>
#include <ctype.h>
#include <fcntl.h>

#include <malloc.h>
#include <inttypes.h>
#include <dirent.h>
#include <arpa/inet.h>

#define MAX_COMPARE_NAME 15
#define MAX_NAME 100


unsigned long long Hertz = 1;

long pid_max = 32768;
int debug = 0;

struct wanted {
	char compare[MAX_COMPARE_NAME + 3];
	char id[MAX_NAME + 1];
	char name[MAX_NAME + 1];

	unsigned long long minflt;
	unsigned long long cminflt;
	unsigned long long majflt;
	unsigned long long cmajflt;
	unsigned long long utime;
	unsigned long long stime;
	unsigned long long cutime;
	unsigned long long cstime;
	unsigned long long num_threads;
	unsigned long long rss;

	unsigned long merge_count;	// how many processes have been merged to this
	int exposed;				// if set, we have sent this to netdata

	struct wanted *target;	// the one that will be reported to netdata
	struct wanted *next;
} *wanted_root = NULL, *default_target = NULL;

int update_every = 1;

struct wanted *add_wanted(const char *id, struct wanted *target)
{
	struct wanted *w = calloc(sizeof(struct wanted), 1);
	if(!w) {
		fprintf(stderr, "Cannot allocate %ld bytes of memory\n", sizeof(struct wanted));
		return NULL;
	}

	strncpy(w->id, id, MAX_NAME);
	strncpy(w->name, id, MAX_NAME);
	snprintf(w->compare, MAX_COMPARE_NAME+1, "(%.*s)", MAX_COMPARE_NAME, id);
	w->target = target;

	w->next = wanted_root;
	wanted_root = w;

	if(debug) fprintf(stderr, "Adding hook for process '%s', compare '%s' on target '%s'\n", w->id, w->compare, w->target?w->target->id:"");

	return w;
}

void parse_args(int argc, char **argv)
{
	int i = 1;

	debug = 0;
	if(strcmp(argv[i], "debug") == 0) {
		debug = 1;
		i++;
	}

	update_every = atoi(argv[i++]);
	if(update_every == 0) {
		i = 1;
		update_every = 1;
	}

	for(; i < argc ; i++) {
		struct wanted *w = NULL;
		char *s = argv[i];
		char *t;

		while((t = strsep(&s, " "))) {
			if(w && strcmp(t, "as") == 0 && s && *s) {
				strncpy(w->name, s, MAX_NAME);
				if(debug) fprintf(stderr, "Setting dimension name to '%s' on target '%s'\n", w->name, w->id);
				break;
			}

			struct wanted *n = add_wanted(t, w);
			if(!w) w = n;
		}
	}

	default_target = add_wanted("all_other_processes", NULL);
	strncpy(default_target->name, "other", MAX_NAME);
}

// see: man proc
struct pid_stat {
	int32_t pid;
	char comm[MAX_COMPARE_NAME + 3];
	char state;
	int32_t ppid;
	int32_t pgrp;
	int32_t session;
	int32_t tty_nr;
	int32_t tpgid;
	uint64_t flags;
	unsigned long long minflt;
	unsigned long long cminflt;
	unsigned long long majflt;
	unsigned long long cmajflt;
	unsigned long long utime;
	unsigned long long stime;
	unsigned long long cutime;
	unsigned long long cstime;
	int64_t priority;
	int64_t nice;
	int32_t num_threads;
	int64_t itrealvalue;
	unsigned long long starttime;
	unsigned long long vsize;
	unsigned long long rss;
	unsigned long long rsslim;
	unsigned long long starcode;
	unsigned long long endcode;
	unsigned long long startstack;
	unsigned long long kstkesp;
	unsigned long long kstkeip;
	uint64_t signal;
	uint64_t blocked;
	uint64_t sigignore;
	uint64_t sigcatch;
	uint64_t wchan;
	uint64_t nswap;
	uint64_t cnswap;
	int32_t exit_signal;
	int32_t processor;
	uint32_t rt_priority;
	uint32_t policy;
	unsigned long long delayacct_blkio_ticks;
	uint64_t guest_time;
	int64_t cguest_time;

	int childs;	// number of processes directly referencing this
	int updated;
	int merged;
	int new_entry;
	unsigned long merge_count;
	struct wanted *target;
	struct pid_stat *parent;
	struct pid_stat *prev;
	struct pid_stat *next;
} *root = NULL;

struct pid_stat **all_pids;

#define PID_STAT_LINE_MAX 4096

struct pid_stat *get_entry(pid_t pid)
{
	if(all_pids[pid]) {
		all_pids[pid]->new_entry = 0;
		return all_pids[pid];
	}

	all_pids[pid] = calloc(sizeof(struct pid_stat), 1);
	if(!all_pids[pid]) {
		fprintf(stderr, "Cannot allocate %lu bytes of memory", sizeof(struct pid_stat));
		return NULL;
	}

	if(root) root->prev = all_pids[pid];
	all_pids[pid]->next = root;
	root = all_pids[pid];

	all_pids[pid]->new_entry = 1;

	return all_pids[pid];
}

void del_entry(pid_t pid)
{
	if(!all_pids[pid]) return;

	if(debug) fprintf(stderr, "Process %d %s exited, deleting it.\n", pid, all_pids[pid]->comm);

	if(root == all_pids[pid]) root = all_pids[pid]->next;
	if(all_pids[pid]->next) all_pids[pid]->next->prev = all_pids[pid]->prev;
	if(all_pids[pid]->prev) all_pids[pid]->prev->next = all_pids[pid]->next;

	free(all_pids[pid]);
	all_pids[pid] = NULL;
}

int update_from_proc(void)
{
	char buffer[PID_STAT_LINE_MAX + 1];
	char name[PID_STAT_LINE_MAX + 1];
	char filename[FILENAME_MAX+1];
	DIR *dir = opendir("/proc");
	if(!dir) return 0;

	struct dirent *file = NULL;
	struct pid_stat *p = NULL;

	// mark them all as un-updated
	for(p = root; p ; p = p->next) {
		p->parent = NULL;
		p->updated = 0;
		p->childs = 0;
		p->merged = 0;
		p->new_entry = 0;
		p->merge_count = 0;
	}

	while((file = readdir(dir))) {
		char *endptr = file->d_name;
		pid_t pid = strtoul(file->d_name, &endptr, 10);
		if(pid <= 0 || pid > pid_max || endptr == file->d_name || *endptr != '\0') continue;

		snprintf(filename, FILENAME_MAX, "/proc/%s/stat", file->d_name);

		int fd = open(filename, O_RDONLY);
		if(fd == -1) {
			if(errno != ENOENT && errno != ESRCH) fprintf(stderr, "Cannot open file '%s' for reading (%s).\n", filename, strerror(errno));
			continue;
		}

		int bytes = read(fd, buffer, PID_STAT_LINE_MAX);
		if(bytes == -1) {
			fprintf(stderr, "Cannot read from file '%s' (%s).\n", filename, strerror(errno));
			close(fd);
			continue;
		}

		close(fd);
		if(bytes < 100) continue;
		buffer[bytes] = '\0';
		if(debug) fprintf(stderr, "READ: %s", buffer);

		p = get_entry(pid);
		if(!p) continue;

		int parsed = sscanf(buffer,
			"%d %s %c"							// pid, comm, state
			" %d %d %d %d %d"					// ppid, pgrp, session, tty_nr, tpgid
			" %lu %llu %llu %llu %llu"			// flags, minflt, cminflt, majflt, cmajflt
			" %llu %llu %llu %llu"				// utime, stime, cutime, cstime
			" %ld %ld"							// priority, nice
			" %d"								// num_threads
			" %ld"								// itrealvalue
			" %llu"								// starttime
			" %llu"								// vsize
			" %llu"								// rss
			" %llu %llu %llu %llu %llu %llu"	// rsslim, starcode, endcode, startstack, kstkesp, kstkeip
			" %lu %lu %lu %lu"					// signal, blocked, sigignore, sigcatch
			" %lu %lu %lu"						// wchan, nswap, cnswap
			" %d %d"							// exit_signal, processor
			" %u %u"							// rt_priority, policy
			" %llu %lu %ld"
			, &p->pid, name, &p->state
			, &p->ppid, &p->pgrp, &p->session, &p->tty_nr, &p->tpgid
			, &p->flags, &p->minflt, &p->cminflt, &p->majflt, &p->cmajflt
			, &p->utime, &p->stime, &p->cutime, &p->cstime
			, &p->priority, &p->nice
			, &p->num_threads
			, &p->itrealvalue
			, &p->starttime
			, &p->vsize
			, &p->rss
			, &p->rsslim, &p->starcode, &p->endcode, &p->startstack, &p->kstkesp, &p->kstkeip
			, &p->signal, &p->blocked, &p->sigignore, &p->sigcatch
			, &p->wchan, &p->nswap, &p->cnswap
			, &p->exit_signal, &p->processor
			, &p->rt_priority, &p->policy
			, &p->delayacct_blkio_ticks, &p->guest_time, &p->cguest_time
			);
		strncpy(p->comm, name, MAX_COMPARE_NAME + 2);
		p->comm[MAX_COMPARE_NAME + 2] = '\0';

		if(debug) fprintf(stderr, "VALUES: %s utime=%llu, stime=%llu, cutime=%llu, cstime=%llu, minflt=%llu, majflt=%llu, cminflt=%llu, cmajflt=%llu\n", p->comm, p->utime, p->stime, p->cutime, p->cstime, p->minflt, p->majflt, p->cminflt, p->cmajflt);

		if(parsed < 39) fprintf(stderr, "file %s gave %d results (expected 44)\n", filename, parsed);

		// check if it is wanted
		// we do this only once, the first time this pid is loaded
		if(p->new_entry) {
			if(debug) fprintf(stderr, "\tJust added %s\n", p->comm);

			struct wanted *w;
			for(w = wanted_root; w ; w = w->next) {
				// if(debug) fprintf(stderr, "\t\tcomparing '%s' with '%s'\n", w->compare, p->comm);

				if(strcmp(w->compare, p->comm) == 0) {
					if(w->target) p->target = w->target;
					else p->target = w;

					if(debug) fprintf(stderr, "\t\t%s linked to target %s\n", p->comm, p->target->name);
				}
			}
		}

		// just a few checks
		if(p->ppid < 0 || p->ppid > pid_max) p->ppid = 0;

		// mark it as updated
		p->updated = 1;
	}
	closedir(dir);

	// cleanup all un-updated processed (exited, killed, etc)
	int c;
	for(p = root, c = 0; p ; c++) {
		if(!p->updated) {
			
			pid_t r = p->pid;
			p = p->next;
			del_entry(r);
		}
		else p = p->next;
	}

	return 1;
}

void walk_down(pid_t pid, int level) {
	struct pid_stat *p = NULL;
	char b[level+3];
	int i;

	for(i = 0; i < level; i++) b[i] = '\t';
	b[level] = '|';
	b[level+1] = '-';
	b[level+2] = '\0';

	p = all_pids[pid];
	if(p) fprintf(stderr, "%s %s %d [%s] c=%d u=%llu, s=%llu, cu=%llu, cs=%llu, n=%llu, j=%llu, cn=%llu, cj=%llu\n", b, p->comm, p->pid, p->updated?"OK":"KILLED", p->childs, p->utime, p->stime, p->cutime, p->cstime, p->minflt, p->majflt, p->cminflt, p->cmajflt);

	for(p = root; p ; p = p->next) {
		if(p->ppid == pid) {
			walk_down(p->pid, level+1);
		}
	}
}

void merge_processes(void)
{
	struct pid_stat *p = NULL;

	if(debug) walk_down(0, 1);

	// link all parents and update childs count
	for(p = root; p ; p = p->next) {
		if(p->ppid > 0 && p->ppid <= pid_max && all_pids[p->ppid]) {
			if(debug) fprintf(stderr, "\tParent of %d %s is %d %s\n", p->pid, p->comm, p->ppid, all_pids[p->ppid]->comm);
			
			p->parent = all_pids[p->ppid];
			p->parent->childs++;
		}
		else if(p->ppid != 0) fprintf(stderr, "\t\tWRONG! pid %d %s states parent %d, but the later does not exist.\n", p->pid, p->comm, p->ppid);
	}

	// find all the procs with 0 childs and merge them to their parents
	// repeat, until nothing more can be done.
	int found = 1;
	while(found) {
		found = 0;
		for(p = root; p ; p = p->next) {
			// if this process does not have any childs, and
			// is not already merged, and
			// its parent has childs waiting to be merged, and
			// the target of this process and its parent is the same, or the parent does not have a target, or this process does not have a parent
			// and its parent is not init
			// then... merge them!
			if(!p->childs && !p->merged && p->parent && p->parent->childs && (p->target == p->parent->target || !p->parent->target || !p->target) && p->ppid != 1) {

				if(debug) fprintf(stderr, "\n\t\tPARENT BEFORE MERGE: %d %s utime=%llu, stime=%llu, cutime=%llu, cstime=%llu, minflt=%llu, majflt=%llu, cminflt=%llu, cmajflt=%llu\n", p->parent->pid, p->parent->comm, p->parent->utime, p->parent->stime, p->parent->cutime, p->parent->cstime, p->parent->minflt, p->parent->majflt, p->parent->cminflt, p->parent->cmajflt);

				p->parent->minflt += p->minflt;
				p->parent->majflt += p->majflt;
				p->parent->utime += p->utime;
				p->parent->stime += p->stime;

				p->parent->cminflt += p->cminflt;
				p->parent->cmajflt += p->cmajflt;
				p->parent->cutime += p->cutime;
				p->parent->cstime += p->cstime;

				p->parent->num_threads += p->num_threads;
				p->parent->rss += p->rss;

				if(debug) fprintf(stderr, "\tMERGED %d %s to %d %s utime=%llu, stime=%llu, cutime=%llu, cstime=%llu, minflt=%llu, majflt=%llu, cminflt=%llu, cmajflt=%llu\n", p->pid, p->comm, p->ppid, all_pids[p->ppid]->comm, p->utime, p->stime, p->cutime, p->cstime, p->minflt, p->majflt, p->cminflt, p->cmajflt);

				p->parent->childs--;
				p->merged = 1;

				if(debug) fprintf(stderr, "\t\tPARENT AFTER  MERGE: %d %s utime=%llu, stime=%llu, cutime=%llu, cstime=%llu, minflt=%llu, majflt=%llu, cminflt=%llu, cmajflt=%llu\n", p->parent->pid, p->parent->comm, p->parent->utime, p->parent->stime, p->parent->cutime, p->parent->cstime, p->parent->minflt, p->parent->majflt, p->parent->cminflt, p->parent->cmajflt);

				p->parent->merge_count += p->merge_count + 1;

				// the parent inherits the child's target, if it does not have a target itself
				if(p->target && !p->parent->target) {
					p->parent->target = p->target;
					if(debug) fprintf(stderr, "\t\ttarget %s is inherited by %d %s from its child %d %s.\n", p->target->name, p->parent->pid, p->parent->comm, p->pid, p->comm);
				}

				found++;
			}
		}
		if(debug) fprintf(stderr, "Merged %d processes\n", found);
	}

	// give a default target on all top level processes
	// init goes always to default target
	if(all_pids[1]) all_pids[1]->target = default_target;

	for(p = root, found = 0; p ; p = p->next) {
		// if the process is not merged itself
		// then is is a top level process
		if(!p->merged && !p->target) p->target = default_target;
	}

	// give a target to all merged child processes
	found = 1;
	while(found) {
		found = 0;
		for(p = root; p ; p = p->next) {
			if(!p->target && p->merged && p->parent && p->parent->target) {
				p->target = p->parent->target;
				found++;
			}
		}
	}

	// zero all the targets
	struct wanted *w;
	for (w = wanted_root; w ; w = w->next) {
		w->minflt = 0;
		w->majflt = 0;
		w->utime = 0;
		w->stime = 0;
		w->cminflt = 0;
		w->cmajflt = 0;
		w->cutime = 0;
		w->cstime = 0;
		w->num_threads = 0;
		w->rss = 0;
		w->merge_count = 0;
	}

	// concentrate everything on the targets
	for(p = root; p ; p = p->next) {
		//if(p->parent && !p->merged) fprintf(stderr, "\tprocess %s pid %d has a parent, but has not been merged!\n", p->comm, p->pid);
		//if(p->childs) fprintf(stderr, "\tprocess %s pid %d has %d childs that have not been merged!\n", p->comm, p->pid, p->childs);

		if(!p->target) {
			fprintf(stderr, "WRONG! pid %d %s was left without a target!\n", p->pid, p->comm);
			continue;
		}

		if(p->merged) continue;

		p->target->minflt += p->minflt;
		p->target->majflt += p->majflt;
		p->target->utime += p->utime;
		p->target->stime += p->stime;
		p->target->cminflt += p->cminflt;
		p->target->cmajflt += p->cmajflt;
		p->target->cutime += p->cutime;
		p->target->cstime += p->cstime;
		p->target->num_threads += p->num_threads;
		p->target->rss += p->rss;

		p->target->merge_count += p->merge_count + 1;

		if(debug) fprintf(stderr, "\tAgregating %s pid %d on %s (count: %lu) utime=%llu, stime=%llu, cutime=%llu, cstime=%llu, minflt=%llu, majflt=%llu, cminflt=%llu, cmajflt=%llu\n", p->comm, p->pid, p->target->name, p->target->merge_count, p->utime, p->stime, p->cutime, p->cstime, p->minflt, p->majflt, p->cminflt, p->cmajflt);
	}
}


void show_dimensions(void)
{
	struct wanted *w;

	fprintf(stdout, "BEGIN apps.cpu\n");
	for (w = wanted_root; w ; w = w->next) {
		if(w->target || (!w->merge_count && !w->exposed)) continue;

		fprintf(stdout, "SET %s = %llu\n", w->name, w->utime + w->stime);
	}
	fprintf(stdout, "END\n");

	fprintf(stdout, "BEGIN apps.cpu_user\n");
	for (w = wanted_root; w ; w = w->next) {
		if(w->target || (!w->merge_count && !w->exposed)) continue;

		fprintf(stdout, "SET %s = %llu\n", w->name, w->utime);
	}
	fprintf(stdout, "END\n");

	fprintf(stdout, "BEGIN apps.cpu_system\n");
	for (w = wanted_root; w ; w = w->next) {
		if(w->target || (!w->merge_count && !w->exposed)) continue;

		fprintf(stdout, "SET %s = %llu\n", w->name, w->stime);
	}
	fprintf(stdout, "END\n");

	fprintf(stdout, "BEGIN apps.threads\n");
	for (w = wanted_root; w ; w = w->next) {
		if(w->target || (!w->merge_count && !w->exposed)) continue;

		fprintf(stdout, "SET %s = %llu\n", w->name, w->num_threads);
	}
	fprintf(stdout, "END\n");

	fprintf(stdout, "BEGIN apps.processes\n");
	for (w = wanted_root; w ; w = w->next) {
		if(w->target || (!w->merge_count && !w->exposed)) continue;

		fprintf(stdout, "SET %s = %lu\n", w->name, w->merge_count);
	}
	fprintf(stdout, "END\n");

	fprintf(stdout, "BEGIN apps.rss\n");
	for (w = wanted_root; w ; w = w->next) {
		if(w->target || (!w->merge_count && !w->exposed)) continue;

		fprintf(stdout, "SET %s = %llu\n", w->name, (unsigned long long)w->rss);
	}
	fprintf(stdout, "END\n");

	fprintf(stdout, "BEGIN apps.minor_faults\n");
	for (w = wanted_root; w ; w = w->next) {
		if(w->target || (!w->merge_count && !w->exposed)) continue;

		fprintf(stdout, "SET %s = %llu\n", w->name, w->minflt);
	}
	fprintf(stdout, "END\n");

	fprintf(stdout, "BEGIN apps.major_faults\n");
	for (w = wanted_root; w ; w = w->next) {
		if(w->target || (!w->merge_count && !w->exposed)) continue;

		fprintf(stdout, "SET %s = %llu\n", w->name, w->majflt);
	}
	fprintf(stdout, "END\n");

	fflush(NULL);
}

void show_charts(void)
{
	struct wanted *w;
	int newly_added = 0;

	for(w = wanted_root ; w ; w = w->next)
		if(!w->exposed && w->merge_count) {
			newly_added++;
			w->exposed = 1;
			if(debug) fprintf(stderr, "%s just added - regenerating charts.\n", w->name);
		}

	// nothing more to show
	if(!newly_added) return;

	// we have something new to show
	// update the charts
	fprintf(stdout, "CHART apps.cpu '' 'Applications CPU Time' 'cpu time %%' apps apps stacked 20001 %d\n", update_every);
	for (w = wanted_root; w ; w = w->next) {
		if(w->target || (!w->merge_count && !w->exposed)) continue;

		fprintf(stdout, "DIMENSION %s '' incremental 100 %llu\n", w->name, Hertz);
	}

	fprintf(stdout, "CHART apps.rss '' 'Applications Memory' 'MB' apps apps stacked 20002 %d\n", update_every);
	for (w = wanted_root; w ; w = w->next) {
		if(w->target || (!w->merge_count && !w->exposed)) continue;

		fprintf(stdout, "DIMENSION %s '' absolute %ld %ld\n", w->name, sysconf(_SC_PAGESIZE), 1024L*1024L);
	}

	fprintf(stdout, "CHART apps.threads '' 'Applications Threads' 'threads' apps apps stacked 20005 %d\n", update_every);
	for (w = wanted_root; w ; w = w->next) {
		if(w->target || (!w->merge_count && !w->exposed)) continue;

		fprintf(stdout, "DIMENSION %s '' absolute 1 1\n", w->name);
	}

	fprintf(stdout, "CHART apps.processes '' 'Applications Processes' 'processes' apps apps stacked 20004 %d\n", update_every);
	for (w = wanted_root; w ; w = w->next) {
		if(w->target || (!w->merge_count && !w->exposed)) continue;

		fprintf(stdout, "DIMENSION %s '' absolute 1 1\n", w->name);
	}

	fprintf(stdout, "CHART apps.cpu_user '' 'Applications CPU User Time' 'cpu time %%' apps none stacked 20020 %d\n", update_every);
	for (w = wanted_root; w ; w = w->next) {
		if(w->target || (!w->merge_count && !w->exposed)) continue;

		fprintf(stdout, "DIMENSION %s '' incremental 100 %llu\n", w->name, Hertz);
	}

	fprintf(stdout, "CHART apps.cpu_system '' 'Applications CPU System Time' 'cpu time %%' apps none stacked 20021 %d\n", update_every);
	for (w = wanted_root; w ; w = w->next) {
		if(w->target || (!w->merge_count && !w->exposed)) continue;

		fprintf(stdout, "DIMENSION %s '' incremental 100 %llu\n", w->name, Hertz);
	}

	fprintf(stdout, "CHART apps.major_faults '' 'Applications Major Page Faults' 'page faults/s' apps apps stacked 20010 %d\n", update_every);
	for (w = wanted_root; w ; w = w->next) {
		if(w->target || (!w->merge_count && !w->exposed)) continue;

		fprintf(stdout, "DIMENSION %s '' incremental 1 1\n", w->name);
	}

	fprintf(stdout, "CHART apps.minor_faults '' 'Applications Minor Page Faults' 'page faults/s' apps apps stacked 20011 %d\n", update_every);
	for (w = wanted_root; w ; w = w->next) {
		if(w->target || (!w->merge_count && !w->exposed)) continue;

		fprintf(stdout, "DIMENSION %s '' incremental 1 1\n", w->name);
	}

	fflush(NULL);
}

unsigned long long get_hertz(void)
{
	unsigned long long hz = 1;

#ifdef _SC_CLK_TCK
	if((hz = sysconf(_SC_CLK_TCK)) > 0) {
		return hz;
	}
#endif

#ifdef HZ
	hz = (unsigned long long)HZ;    /* <asm/param.h> */
#else
	/* If 32-bit or big-endian (not Alpha or ia64), assume HZ is 100. */
	hz = (sizeof(long)==sizeof(int) || htons(999)==999) ? 100UL : 1024UL;
#endif

	fprintf(stderr, "Unknown HZ value. Assuming %llu.\n", hz);
	return hz;
}


long get_pid_max(void)
{
	return 32768L;
}

unsigned long long usecdiff(struct timeval *now, struct timeval *last) {
		return ((((now->tv_sec * 1000000ULL) + now->tv_usec) - ((last->tv_sec * 1000000ULL) + last->tv_usec)));
}

void mysleep(unsigned long long susec)
{
	while(susec) {
		struct timeval t1, t2;

		gettimeofday(&t1, NULL);

		if(debug) fprintf(stderr, "Sleeping for %llu microseconds\n", susec);
		usleep(susec);

		gettimeofday(&t2, NULL);

		unsigned long long diff = usecdiff(&t2, &t1);
		
		if(diff < susec) susec -= diff;
		else susec = 0;
	}
}

int main(int argc, char **argv)
{
	Hertz = get_hertz();
	pid_max = get_pid_max();

	parse_args(argc, argv);

	all_pids = calloc(sizeof(struct pid_stat *), pid_max);
	if(!all_pids) {
		fprintf(stderr, "Cannot allocate %lu bytes of memory.\n", sizeof(struct pid_stat *) * pid_max);
		printf("DISABLE\n");
		exit(1);
	}

	unsigned long long counter = 1;
	unsigned long long usec = 0, susec = 0;
	struct timeval last, now;
	gettimeofday(&last, NULL);

	for(;1; counter++) {
		if(!update_from_proc()) {
			fprintf(stderr, "Cannot allocate %lu bytes of memory.\n", sizeof(struct pid_stat *) * pid_max);
			printf("DISABLE\n");
			exit(1);
		}

		merge_processes();
		show_charts();		// this is smart enough to show only newly added apps, when needed
		show_dimensions();

		if(debug) fprintf(stderr, "Done Loop No %llu\n", counter);
		// if(counter == 1000) exit(0);

		gettimeofday(&now, NULL);

		usec = usecdiff(&now, &last) - susec;
		if(debug) fprintf(stderr, "last loop took %llu usec (worked for %llu, sleeped for %llu).\n", usec + susec, usec, susec);

		if(usec < (update_every * 1000000ULL)) susec = (update_every * 1000000ULL) - usec;
		else susec = 0;

		mysleep(susec);

		bcopy(&now, &last, sizeof(struct timeval));
	}
}
