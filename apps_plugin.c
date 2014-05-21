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

long processors = 1;
long pid_max = 32768;
int debug = 0;

struct target {
	char compare[MAX_COMPARE_NAME + 1];
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

	unsigned long long fix_minflt;
	unsigned long long fix_cminflt;
	unsigned long long fix_majflt;
	unsigned long long fix_cmajflt;
	unsigned long long fix_utime;
	unsigned long long fix_stime;
	unsigned long long fix_cutime;
	unsigned long long fix_cstime;

	unsigned long long statm_size;
	unsigned long long statm_resident;
	unsigned long long statm_share;
	unsigned long long statm_text;
	unsigned long long statm_lib;
	unsigned long long statm_data;
	unsigned long long statm_dirty;

	unsigned long long io_logical_bytes_read;
	unsigned long long io_logical_bytes_written;
	unsigned long long io_read_calls;
	unsigned long long io_write_calls;
	unsigned long long io_storage_bytes_read;
	unsigned long long io_storage_bytes_written;
	unsigned long long io_cancelled_write_bytes;

	unsigned long long fix_io_logical_bytes_read;
	unsigned long long fix_io_logical_bytes_written;
	unsigned long long fix_io_read_calls;
	unsigned long long fix_io_write_calls;
	unsigned long long fix_io_storage_bytes_read;
	unsigned long long fix_io_storage_bytes_written;
	unsigned long long fix_io_cancelled_write_bytes;

	unsigned long processes;	// how many processes have been merged to this
	int exposed;				// if set, we have sent this to netdata

	struct target *target;	// the one that will be reported to netdata
	struct target *next;
} *target_root = NULL, *default_target = NULL;

int update_every = 1;
unsigned long long file_counter = 0;

struct target *get_target(const char *id, struct target *target)
{
	struct target *w;
	for(w = target_root ; w ; w = w->next)
		if(strncmp(id, w->id, MAX_NAME) == 0) return w;
	
	w = calloc(sizeof(struct target), 1);
	if(!w) {
		fprintf(stderr, "Cannot allocate %lu bytes of memory\n", sizeof(struct target));
		return NULL;
	}

	strncpy(w->id, id, MAX_NAME);
	strncpy(w->name, id, MAX_NAME);
	strncpy(w->compare, id, MAX_COMPARE_NAME);
	w->target = target;

	w->next = target_root;
	target_root = w;

	if(debug) fprintf(stderr, "Adding hook for process '%s', compare '%s' on target '%s'\n", w->id, w->compare, w->target?w->target->id:"");

	return w;
}

char *trim(char *s)
{
	// skip leading spaces
	while(*s && isspace(*s)) s++;
	if(!*s) return NULL;

	// skip tailing spaces
	int c = strlen(s) - 1;
	while(c >= 0 && isspace(s[c])) {
		s[c] = '\0';
		c--;
	}
	if(c < 0) return NULL;
	if(!*s) return NULL;
	return s;
}

int read_process_groups(const char *name)
{
	char buffer[4096+1];
	char filename[FILENAME_MAX + 1];

	snprintf(filename, FILENAME_MAX, "%s/apps_%s.conf", CONFIG_DIR, name);

	if(debug) fprintf(stderr, "Process groups file: '%s'\n", filename);
	FILE *fp = fopen(filename, "r");
	if(!fp) {
		fprintf(stderr, "Cannot open file '%s' (%s)\n", filename, strerror(errno));
		return 1;
	}

	long line = 0;
	while(fgets(buffer, 4096, fp) != NULL) {
		line++;

		// if(debug) fprintf(stderr, "\tread %s\n", buffer);

		char *s = buffer, *t, *p;
		s = trim(s);
		if(!s || !*s || *s == '#') continue;

		if(debug) fprintf(stderr, "\tread %s\n", s);

		// the target name
		t = strsep(&s, ":");
		if(t) t = trim(t);
		if(!t || !*t) continue;

		if(debug) fprintf(stderr, "\t\ttarget %s\n", t);

		struct target *w = NULL;
		long count = 0;

		// the process names
		while((p = strsep(&s, " "))) {
			p = trim(p);
			if(!p || !*p) continue;

			struct target *n = get_target(p, w);
			if(!w) w = n;

			count++;
		}

		if(w) strncpy(w->name, t, MAX_NAME);
		if(!count) fprintf(stderr, "the line %ld on file '%s', for group '%s' does not state any process names.\n", line, filename, t);
	}
	fclose(fp);

	default_target = get_target("+p!o@w#e$i^r&7*5(-i)l-o_", NULL); // match nothing
	strncpy(default_target->name, "other", MAX_NAME);

	return 0;
}

void parse_args(int argc, char **argv)
{
	int i, freq = 0;
	char *name = NULL;

	for(i = 1; i < argc; i++) {
		if(!freq) {
			int n = atoi(argv[i]);
			if(n > 0) {
				freq = n;
				continue;
			}
		}

		if(strcmp("debug", argv[i]) == 0) {
			debug = 1;
			continue;
		}

		if(!name) {
			name = argv[i];
			continue;
		}

		fprintf(stderr, "Cannot understand option %s\n", argv[i]);
		exit(1);
	}

	if(freq > 0) update_every = freq;
	if(!name) name = "groups";

	if(read_process_groups(name)) {
		fprintf(stderr, "Cannot read process groups %s\n", name);
		exit(1);
	}
}

// see: man proc
struct pid_stat {
	int32_t pid;
	char comm[MAX_COMPARE_NAME + 1];
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

	unsigned long long statm_size;
	unsigned long long statm_resident;
	unsigned long long statm_share;
	unsigned long long statm_text;
	unsigned long long statm_lib;
	unsigned long long statm_data;
	unsigned long long statm_dirty;

	unsigned long long io_logical_bytes_read;
	unsigned long long io_logical_bytes_written;
	unsigned long long io_read_calls;
	unsigned long long io_write_calls;
	unsigned long long io_storage_bytes_read;
	unsigned long long io_storage_bytes_written;
	unsigned long long io_cancelled_write_bytes;

/*	unsigned long long old_utime;
	unsigned long long old_stime;
	unsigned long long old_minflt;
	unsigned long long old_majflt;

	unsigned long long old_cutime;
	unsigned long long old_cstime;
	unsigned long long old_cminflt;
	unsigned long long old_cmajflt;

	unsigned long long fix_cutime;
	unsigned long long fix_cstime;
	unsigned long long fix_cminflt;
	unsigned long long fix_cmajflt;

	unsigned long long diff_cutime;
	unsigned long long diff_cstime;
	unsigned long long diff_cminflt;
	unsigned long long diff_cmajflt;
*/

	int childs;	// number of processes directly referencing this
	int updated;
	int merged;
	int new_entry;
	struct target *target;
	struct pid_stat *parent;
	struct pid_stat *prev;
	struct pid_stat *next;
} *root = NULL;

struct pid_stat **all_pids;

#define PROC_BUFFER 4096

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
	static long count_open_errors = 0;

	char buffer[PROC_BUFFER + 1];
	char name[PROC_BUFFER + 1];
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
	}

	while((file = readdir(dir))) {
		char *endptr = file->d_name;
		pid_t pid = strtoul(file->d_name, &endptr, 10);
		if(pid <= 0 || pid > pid_max || endptr == file->d_name || *endptr != '\0') continue;


		// --------------------------------------------------------------------
		// /proc/<pid>/stat

		snprintf(filename, FILENAME_MAX, "/proc/%s/stat", file->d_name);
		int fd = open(filename, O_RDONLY);
		if(fd == -1) {
			if(errno != ENOENT && errno != ESRCH) {
				if(!count_open_errors) fprintf(stderr, "Cannot open file '%s' for reading (%d, %s).\n", filename, errno, strerror(errno));
				count_open_errors++;
			}
			continue;
		}
		file_counter++;

		int bytes = read(fd, buffer, PROC_BUFFER);
		close(fd);

		if(bytes == -1) {
			fprintf(stderr, "Cannot read from file '%s' (%s).\n", filename, strerror(errno));
			continue;
		}

		if(bytes < 10) continue;
		buffer[bytes] = '\0';
		if(debug) fprintf(stderr, "READ stat: %s", buffer);

		p = get_entry(pid);
		if(!p) continue;

		int parsed = sscanf(buffer,
			"%d (%[^)]) %c"						// pid, comm, state
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
		strncpy(p->comm, name, MAX_COMPARE_NAME);
		p->comm[MAX_COMPARE_NAME] = '\0';

		if(debug) fprintf(stderr, "VALUES: %s utime=%llu, stime=%llu, cutime=%llu, cstime=%llu, minflt=%llu, majflt=%llu, cminflt=%llu, cmajflt=%llu\n", p->comm, p->utime, p->stime, p->cutime, p->cstime, p->minflt, p->majflt, p->cminflt, p->cmajflt);

		if(parsed < 39) fprintf(stderr, "file %s gave %d results (expected 44)\n", filename, parsed);

		// check if it is target
		// we do this only once, the first time this pid is loaded
		if(p->new_entry) {
			if(debug) fprintf(stderr, "\tJust added %s\n", p->comm);

			struct target *w;
			for(w = target_root; w ; w = w->next) {
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


		// --------------------------------------------------------------------
		// /proc/<pid>/statm

		snprintf(filename, FILENAME_MAX, "/proc/%s/statm", file->d_name);
		fd = open(filename, O_RDONLY);
		if(fd == -1) {
			if(errno != ENOENT && errno != ESRCH) {
				if(!count_open_errors) fprintf(stderr, "Cannot open file '%s' for reading (%d, %s).\n", filename, errno, strerror(errno));
				count_open_errors++;
			}
		}
		else {
			file_counter++;
			bytes = read(fd, buffer, PROC_BUFFER);
			close(fd);

			if(bytes == -1) {
				fprintf(stderr, "Cannot read from file '%s' (%s).\n", filename, strerror(errno));
			}
			else if(bytes > 10) {
				buffer[bytes] = '\0';
				if(debug) fprintf(stderr, "READ statm: %s", buffer);

				parsed = sscanf(buffer,
					"%llu %llu %llu %llu %llu %llu %llu"
					, &p->statm_size
					, &p->statm_resident
					, &p->statm_share
					, &p->statm_text
					, &p->statm_lib
					, &p->statm_data
					, &p->statm_dirty
					);

				if(parsed < 7) fprintf(stderr, "file %s gave %d results (expected 7)\n", filename, parsed);
			}
		}

		// --------------------------------------------------------------------
		// /proc/<pid>/io

		snprintf(filename, FILENAME_MAX, "/proc/%s/io", file->d_name);
		fd = open(filename, O_RDONLY);
		if(fd == -1) {
			if(errno != ENOENT && errno != ESRCH) {
				if(!count_open_errors) fprintf(stderr, "Cannot open file '%s' for reading (%d, %s).\n", filename, errno, strerror(errno));
				count_open_errors++;
			}
		}
		else {
			file_counter++;
			bytes = read(fd, buffer, PROC_BUFFER);
			close(fd);

			if(bytes == -1) {
				fprintf(stderr, "Cannot read from file '%s' (%s).\n", filename, strerror(errno));
			}
			else if(bytes > 10) {
				buffer[bytes] = '\0';
				if(debug) fprintf(stderr, "READ io: %s", buffer);

				parsed = sscanf(buffer,
					"rchar: %llu\nwchar: %llu\nsyscr: %llu\nsyscw: %llu\nread_bytes: %llu\nwrite_bytes: %llu\ncancelled_write_bytes: %llu"
					, &p->io_logical_bytes_read
					, &p->io_logical_bytes_written
					, &p->io_read_calls
					, &p->io_write_calls
					, &p->io_storage_bytes_read
					, &p->io_storage_bytes_written
					, &p->io_cancelled_write_bytes
					);

				if(parsed < 7) fprintf(stderr, "file %s gave %d results (expected 7)\n", filename, parsed);
			}
		}


		// --------------------------------------------------------------------
		// done!

		// mark it as updated
		p->updated = 1;
	}
	if(count_open_errors > 1 && count_open_errors > 1000) {
		fprintf(stderr, "file open errors repeated %ld times\n", count_open_errors - 1);
		count_open_errors = 0;
	}

	closedir(dir);

	return 1;
}
/*
int walk_down(pid_t pid, int level) {
	struct pid_stat *p = NULL;
	char b[level+3];
	int i, ret = 0;

	for(i = 0; i < level; i++) b[i] = '\t';
	b[level] = '|';
	b[level+1] = '-';
	b[level+2] = '\0';

	for(p = root; p ; p = p->next) {
		if(p->ppid == pid) {
			ret += walk_down(p->pid, level+1);
		}
	}

	p = all_pids[pid];
	if(p) {
		if(!p->updated) ret += 1;
		if(ret) fprintf(stderr, "%s %s %d [%s, %s] c=%d u=%llu+%llu, s=%llu+%llu, cu=%llu+%llu, cs=%llu+%llu, n=%llu+%llu, j=%llu+%llu, cn=%llu+%llu, cj=%llu+%llu\n"
			, b, p->comm, p->pid, p->updated?"OK":"KILLED", p->target->name, p->childs
			, p->utime, p->utime - p->old_utime
			, p->stime, p->stime - p->old_stime
			, p->cutime, p->cutime - p->old_cutime
			, p->cstime, p->cstime - p->old_cstime
			, p->minflt, p->minflt - p->old_minflt
			, p->majflt, p->majflt - p->old_majflt
			, p->cminflt, p->cminflt - p->old_cminflt
			, p->cmajflt, p->cmajflt - p->old_cmajflt
			);
	}

	return ret;
}
*/

void update_statistics(void)
{
	struct pid_stat *p = NULL;

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
				p->parent->childs--;
				p->merged = 1;

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

	for(p = root; p ; p = p->next) {
		// if the process is not merged itself
		// then is is a top level process
		if(!p->merged && !p->target) p->target = default_target;

/*		// by the way, update the diffs
		// will be used later for substracting killed process times
		p->diff_cutime = p->utime - p->cutime;
		p->diff_cstime = p->stime - p->cstime;
		p->diff_cminflt = p->minflt - p->cminflt;
		p->diff_cmajflt = p->majflt - p->cmajflt;
*/	}

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

/*	// for each killed process, remove its values from the parents
	// sums (we had already added them in a previous loop)
	for(p = root; p ; p = p->next) {
		if(p->updated) continue;

		fprintf(stderr, "UNMERGING %d %s\n", p->pid, p->comm);

		unsigned long long diff_utime = p->utime + p->cutime + p->fix_cutime;
		unsigned long long diff_stime = p->stime + p->cstime + p->fix_cstime;
		unsigned long long diff_minflt = p->minflt + p->cminflt + p->fix_cminflt;
		unsigned long long diff_majflt = p->majflt + p->cmajflt + p->fix_cmajflt;

		struct pid_stat *t = p;
		while((t = t->parent)) {
			if(!t->updated) continue;

			unsigned long long x;
			if(diff_utime && t->diff_cutime) {
				x = (t->diff_cutime < diff_utime)?t->diff_cutime:diff_utime;
				diff_utime -= x;
				t->diff_cutime -= x;
				t->fix_cutime += x;
				fprintf(stderr, "\t cutime %llu from %d %s %s\n", x, t->pid, t->comm, t->target->name);
			}
			if(diff_stime && t->diff_cstime) {
				x = (t->diff_cstime < diff_stime)?t->diff_cstime:diff_stime;
				diff_stime -= x;
				t->diff_cstime -= x;
				t->fix_cstime += x;
				fprintf(stderr, "\t cstime %llu from %d %s %s\n", x, t->pid, t->comm, t->target->name);
			}
			if(diff_minflt && t->diff_cminflt) {
				x = (t->diff_cminflt < diff_minflt)?t->diff_cminflt:diff_minflt;
				diff_minflt -= x;
				t->diff_cminflt -= x;
				t->fix_cminflt += x;
				fprintf(stderr, "\t cminflt %llu from %d %s %s\n", x, t->pid, t->comm, t->target->name);
			}
			if(diff_majflt && t->diff_cmajflt) {
				x = (t->diff_cmajflt < diff_majflt)?t->diff_cmajflt:diff_majflt;
				diff_majflt -= x;
				t->diff_cmajflt -= x;
				t->fix_cmajflt += x;
				fprintf(stderr, "\t cmajflt %llu from %d %s %s\n", x, t->pid, t->comm, t->target->name);
			}
		}

		if(diff_utime) fprintf(stderr, "\t cannot fix up utime %llu\n", diff_utime);
		if(diff_stime) fprintf(stderr, "\t cannot fix up stime %llu\n", diff_stime);
		if(diff_minflt) fprintf(stderr, "\t cannot fix up minflt %llu\n", diff_minflt);
		if(diff_majflt) fprintf(stderr, "\t cannot fix up majflt %llu\n", diff_majflt);
	}
*/
	// zero all the targets
	struct target *w;
	for (w = target_root; w ; w = w->next) {
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
		w->processes = 0;

		w->statm_size = 0;
		w->statm_resident = 0;
		w->statm_share = 0;
		w->statm_text = 0;
		w->statm_lib = 0;
		w->statm_data = 0;
		w->statm_dirty = 0;

		w->io_logical_bytes_read = 0;
		w->io_logical_bytes_written = 0;
		w->io_read_calls = 0;
		w->io_write_calls = 0;
		w->io_storage_bytes_read = 0;
		w->io_storage_bytes_written = 0;
		w->io_cancelled_write_bytes = 0;
	}

/*	walk_down(0, 1);
*/
	// concentrate everything on the targets
	for(p = root; p ; p = p->next) {
		if(!p->target) {
			fprintf(stderr, "WRONG! pid %d %s was left without a target!\n", p->pid, p->comm);
			continue;
		}

		if(p->updated) {
			p->target->cutime += p->cutime; // - p->fix_cutime;
			p->target->cstime += p->cstime; // - p->fix_cstime;
			p->target->cminflt += p->cminflt; // - p->fix_cminflt;
			p->target->cmajflt += p->cmajflt; // - p->fix_cmajflt;

			p->target->utime += p->utime; //+ (p->pid != 1)?(p->cutime - p->fix_cutime):0;
			p->target->stime += p->stime; //+ (p->pid != 1)?(p->cstime - p->fix_cstime):0;
			p->target->minflt += p->minflt; //+ (p->pid != 1)?(p->cminflt - p->fix_cminflt):0;
			p->target->majflt += p->majflt; //+ (p->pid != 1)?(p->cmajflt - p->fix_cmajflt):0;

			p->target->num_threads += p->num_threads;
			p->target->rss += p->rss;

			p->target->statm_size += p->statm_size;
			p->target->statm_resident += p->statm_resident;
			p->target->statm_share += p->statm_share;
			p->target->statm_text += p->statm_text;
			p->target->statm_lib += p->statm_lib;
			p->target->statm_data += p->statm_data;
			p->target->statm_dirty += p->statm_dirty;

			p->target->io_logical_bytes_read += p->io_logical_bytes_read;
			p->target->io_logical_bytes_written += p->io_logical_bytes_written;
			p->target->io_read_calls += p->io_read_calls;
			p->target->io_write_calls += p->io_write_calls;
			p->target->io_storage_bytes_read += p->io_storage_bytes_read;
			p->target->io_storage_bytes_written += p->io_storage_bytes_written;
			p->target->io_cancelled_write_bytes += p->io_cancelled_write_bytes;

			p->target->processes++;

			if(debug) fprintf(stderr, "\tAgregating %s pid %d on %s utime=%llu, stime=%llu, cutime=%llu, cstime=%llu, minflt=%llu, majflt=%llu, cminflt=%llu, cmajflt=%llu\n", p->comm, p->pid, p->target->name, p->utime, p->stime, p->cutime, p->cstime, p->minflt, p->majflt, p->cminflt, p->cmajflt);

/*			if(p->utime - p->old_utime > 100) fprintf(stderr, "BIG CHANGE: %d %s utime increased by %llu from %llu to %llu\n", p->pid, p->comm, p->utime - p->old_utime, p->old_utime, p->utime);
			if(p->cutime - p->old_cutime > 100) fprintf(stderr, "BIG CHANGE: %d %s cutime increased by %llu from %llu to %llu\n", p->pid, p->comm, p->cutime - p->old_cutime, p->old_cutime, p->cutime);
			if(p->stime - p->old_stime > 100) fprintf(stderr, "BIG CHANGE: %d %s stime increased by %llu from %llu to %llu\n", p->pid, p->comm, p->stime - p->old_stime, p->old_stime, p->stime);
			if(p->cstime - p->old_cstime > 100) fprintf(stderr, "BIG CHANGE: %d %s cstime increased by %llu from %llu to %llu\n", p->pid, p->comm, p->cstime - p->old_cstime, p->old_cstime, p->cstime);
			if(p->minflt - p->old_minflt > 5000) fprintf(stderr, "BIG CHANGE: %d %s minflt increased by %llu from %llu to %llu\n", p->pid, p->comm, p->minflt - p->old_minflt, p->old_minflt, p->minflt);
			if(p->majflt - p->old_majflt > 5000) fprintf(stderr, "BIG CHANGE: %d %s majflt increased by %llu from %llu to %llu\n", p->pid, p->comm, p->majflt - p->old_majflt, p->old_majflt, p->majflt);
			if(p->cminflt - p->old_cminflt > 15000) fprintf(stderr, "BIG CHANGE: %d %s cminflt increased by %llu from %llu to %llu\n", p->pid, p->comm, p->cminflt - p->old_cminflt, p->old_cminflt, p->cminflt);
			if(p->cmajflt - p->old_cmajflt > 15000) fprintf(stderr, "BIG CHANGE: %d %s cmajflt increased by %llu from %llu to %llu\n", p->pid, p->comm, p->cmajflt - p->old_cmajflt, p->old_cmajflt, p->cmajflt);

			p->old_utime = p->utime;
			p->old_cutime = p->cutime;
			p->old_stime = p->stime;
			p->old_cstime = p->cstime;
			p->old_minflt = p->minflt;
			p->old_majflt = p->majflt;
			p->old_cminflt = p->cminflt;
			p->old_cmajflt = p->cmajflt;
*/		}
		else {
			// since the process has exited, the user
			// will see a drop in our charts, because the incremental
			// values of this process will not be there

			// add them to the fix_* values and they will be added to
			// the reported values, so that the report goes steady
			p->target->fix_minflt += p->minflt;
			p->target->fix_majflt += p->majflt;
			p->target->fix_utime += p->utime;
			p->target->fix_stime += p->stime;
			p->target->fix_cminflt += p->cminflt;
			p->target->fix_cmajflt += p->cmajflt;
			p->target->fix_cutime += p->cutime;
			p->target->fix_cstime += p->cstime;

			p->target->fix_io_logical_bytes_read += p->io_logical_bytes_read;
			p->target->fix_io_logical_bytes_written += p->io_logical_bytes_written;
			p->target->fix_io_read_calls += p->io_read_calls;
			p->target->fix_io_write_calls += p->io_write_calls;
			p->target->fix_io_storage_bytes_read += p->io_storage_bytes_read;
			p->target->fix_io_storage_bytes_written += p->io_storage_bytes_written;
			p->target->fix_io_cancelled_write_bytes += p->io_cancelled_write_bytes;
		}
	}

/*	fprintf(stderr, "\n");
*/
	// cleanup all un-updated processed (exited, killed, etc)
	int c;
	for(p = root, c = 0; p ; c++) {
		if(!p->updated) {
/*			fprintf(stderr, "\tEXITED %d %s [parent %d %s, target %s] utime=%llu, stime=%llu, cutime=%llu, cstime=%llu, minflt=%llu, majflt=%llu, cminflt=%llu, cmajflt=%llu\n", p->pid, p->comm, p->parent->pid, p->parent->comm, p->target->name,  p->utime, p->stime, p->cutime, p->cstime, p->minflt, p->majflt, p->cminflt, p->cmajflt);
*/			
			pid_t r = p->pid;
			p = p->next;
			del_entry(r);
		}
		else p = p->next;
	}
}

unsigned long long usecdiff(struct timeval *now, struct timeval *last) {
		return ((((now->tv_sec * 1000000ULL) + now->tv_usec) - ((last->tv_sec * 1000000ULL) + last->tv_usec)));
}

void show_dimensions(void)
{
	static struct timeval last = { 0, 0 };
	static struct rusage me_last;

	struct target *w;
	struct timeval now;
	struct rusage me;

	unsigned long long usec;
	unsigned long long cpuuser;
	unsigned long long cpusyst;

	if(!last.tv_sec) {
		gettimeofday(&last, NULL);
		getrusage(RUSAGE_SELF, &me_last);

		// the first time, give a zero to allow
		// netdata calibrate to the current time
		// usec = update_every * 1000000ULL;
		usec = 0ULL;
		cpuuser = 0;
		cpusyst = 0;
	}
	else {
		gettimeofday(&now, NULL);
		getrusage(RUSAGE_SELF, &me);

		usec = usecdiff(&now, &last);
		cpuuser = me.ru_utime.tv_sec * 1000000ULL + me.ru_utime.tv_usec;
		cpusyst = me.ru_stime.tv_sec * 1000000ULL + me.ru_stime.tv_usec;

		bcopy(&now, &last, sizeof(struct timeval));
		bcopy(&me, &me_last, sizeof(struct rusage));
	}

	fprintf(stdout, "BEGIN apps.cpu %llu\n", usec);
	for (w = target_root; w ; w = w->next) {
		if(w->target || (!w->processes && !w->exposed)) continue;

		fprintf(stdout, "SET %s = %llu\n", w->name, w->utime + w->stime + w->fix_utime + w->fix_stime);
	}
	fprintf(stdout, "END\n");

	fprintf(stdout, "BEGIN apps.cpu_user %llu\n", usec);
	for (w = target_root; w ; w = w->next) {
		if(w->target || (!w->processes && !w->exposed)) continue;

		fprintf(stdout, "SET %s = %llu\n", w->name, w->utime + w->fix_utime);
	}
	fprintf(stdout, "END\n");

	fprintf(stdout, "BEGIN apps.cpu_system %llu\n", usec);
	for (w = target_root; w ; w = w->next) {
		if(w->target || (!w->processes && !w->exposed)) continue;

		fprintf(stdout, "SET %s = %llu\n", w->name, w->stime + w->fix_stime);
	}
	fprintf(stdout, "END\n");

	fprintf(stdout, "BEGIN apps.threads %llu\n", usec);
	for (w = target_root; w ; w = w->next) {
		if(w->target || (!w->processes && !w->exposed)) continue;

		fprintf(stdout, "SET %s = %llu\n", w->name, w->num_threads);
	}
	fprintf(stdout, "END\n");

	fprintf(stdout, "BEGIN apps.processes %llu\n", usec);
	for (w = target_root; w ; w = w->next) {
		if(w->target || (!w->processes && !w->exposed)) continue;

		fprintf(stdout, "SET %s = %lu\n", w->name, w->processes);
	}
	fprintf(stdout, "END\n");

	fprintf(stdout, "BEGIN apps.mem %llu\n", usec);
	for (w = target_root; w ; w = w->next) {
		if(w->target || (!w->processes && !w->exposed)) continue;

		fprintf(stdout, "SET %s = %lld\n", w->name, (long long)w->statm_resident - (long long)w->statm_share);
	}
	fprintf(stdout, "END\n");

	fprintf(stdout, "BEGIN apps.minor_faults %llu\n", usec);
	for (w = target_root; w ; w = w->next) {
		if(w->target || (!w->processes && !w->exposed)) continue;

		fprintf(stdout, "SET %s = %llu\n", w->name, w->minflt + w->fix_minflt);
	}
	fprintf(stdout, "END\n");

	fprintf(stdout, "BEGIN apps.major_faults %llu\n", usec);
	for (w = target_root; w ; w = w->next) {
		if(w->target || (!w->processes && !w->exposed)) continue;

		fprintf(stdout, "SET %s = %llu\n", w->name, w->majflt + w->fix_majflt);
	}
	fprintf(stdout, "END\n");

	fprintf(stdout, "BEGIN apps.lreads %llu\n", usec);
	for (w = target_root; w ; w = w->next) {
		if(w->target || (!w->processes && !w->exposed)) continue;

		fprintf(stdout, "SET %s = %llu\n", w->name, w->io_logical_bytes_read);
	}
	fprintf(stdout, "END\n");

	fprintf(stdout, "BEGIN apps.lwrites %llu\n", usec);
	for (w = target_root; w ; w = w->next) {
		if(w->target || (!w->processes && !w->exposed)) continue;

		fprintf(stdout, "SET %s = %llu\n", w->name, w->io_logical_bytes_written);
	}
	fprintf(stdout, "END\n");

	fprintf(stdout, "BEGIN apps.preads %llu\n", usec);
	for (w = target_root; w ; w = w->next) {
		if(w->target || (!w->processes && !w->exposed)) continue;

		fprintf(stdout, "SET %s = %llu\n", w->name, w->io_storage_bytes_read);
	}
	fprintf(stdout, "END\n");

	fprintf(stdout, "BEGIN apps.pwrites %llu\n", usec);
	for (w = target_root; w ; w = w->next) {
		if(w->target || (!w->processes && !w->exposed)) continue;

		fprintf(stdout, "SET %s = %llu\n", w->name, w->io_storage_bytes_written);
	}
	fprintf(stdout, "END\n");

	fprintf(stdout, "BEGIN netdata.apps_cpu %llu\n", usec);
	fprintf(stdout, "SET user = %llu\n", cpuuser);
	fprintf(stdout, "SET system = %llu\n", cpusyst);
	fprintf(stdout, "END\n");
	fprintf(stdout, "BEGIN netdata.apps_files %llu\n", usec);
	fprintf(stdout, "SET files = %llu\n", file_counter);
	fprintf(stdout, "END\n");

	fflush(stdout);
}

void show_charts(void)
{
	struct target *w;
	int newly_added = 0;

	for(w = target_root ; w ; w = w->next)
		if(!w->exposed && w->processes) {
			newly_added++;
			w->exposed = 1;
			if(debug) fprintf(stderr, "%s just added - regenerating charts.\n", w->name);
		}

	// nothing more to show
	if(!newly_added) return;

	// we have something new to show
	// update the charts
	fprintf(stdout, "CHART apps.cpu '' 'Apps CPU Time (%ld%% = %ld core%s)' 'cpu time %%' apps apps stacked 20001 %d\n", (processors * 100), processors, (processors>1)?"s":"", update_every);
	for (w = target_root; w ; w = w->next) {
		if(w->target || (!w->processes && !w->exposed)) continue;

		fprintf(stdout, "DIMENSION %s '' incremental 100 %llu\n", w->name, Hertz);
	}

	fprintf(stdout, "CHART apps.mem '' 'Apps Dedicated Memory (w/o shared)' 'MB' apps apps stacked 20003 %d\n", update_every);
	for (w = target_root; w ; w = w->next) {
		if(w->target || (!w->processes && !w->exposed)) continue;

		fprintf(stdout, "DIMENSION %s '' absolute %ld %ld\n", w->name, sysconf(_SC_PAGESIZE), 1024L*1024L);
	}

	fprintf(stdout, "CHART apps.threads '' 'Apps Threads' 'threads' apps apps stacked 20005 %d\n", update_every);
	for (w = target_root; w ; w = w->next) {
		if(w->target || (!w->processes && !w->exposed)) continue;

		fprintf(stdout, "DIMENSION %s '' absolute-no-interpolation 1 1\n", w->name);
	}

	fprintf(stdout, "CHART apps.processes '' 'Apps Processes' 'processes' apps apps stacked 20004 %d\n", update_every);
	for (w = target_root; w ; w = w->next) {
		if(w->target || (!w->processes && !w->exposed)) continue;

		fprintf(stdout, "DIMENSION %s '' absolute-no-interpolation 1 1\n", w->name);
	}

	fprintf(stdout, "CHART apps.cpu_user '' 'Apps CPU User Time (%ld%% = %ld core%s)' 'cpu time %%' apps none stacked 20020 %d\n", (processors * 100), processors, (processors>1)?"s":"", update_every);
	for (w = target_root; w ; w = w->next) {
		if(w->target || (!w->processes && !w->exposed)) continue;

		fprintf(stdout, "DIMENSION %s '' incremental 100 %llu\n", w->name, Hertz * processors);
	}

	fprintf(stdout, "CHART apps.cpu_system '' 'Apps CPU System Time (%ld%% = %ld core%s)' 'cpu time %%' apps none stacked 20021 %d\n", (processors * 100), processors, (processors>1)?"s":"", update_every);
	for (w = target_root; w ; w = w->next) {
		if(w->target || (!w->processes && !w->exposed)) continue;

		fprintf(stdout, "DIMENSION %s '' incremental 100 %llu\n", w->name, Hertz * processors);
	}

	fprintf(stdout, "CHART apps.major_faults '' 'Apps Major Page Faults (swaps in)' 'page faults/s' apps apps stacked 20010 %d\n", update_every);
	for (w = target_root; w ; w = w->next) {
		if(w->target || (!w->processes && !w->exposed)) continue;

		fprintf(stdout, "DIMENSION %s '' incremental 1 1\n", w->name);
	}

	fprintf(stdout, "CHART apps.minor_faults '' 'Apps Minor Page Faults' 'page faults/s' apps apps stacked 20011 %d\n", update_every);
	for (w = target_root; w ; w = w->next) {
		if(w->target || (!w->processes && !w->exposed)) continue;

		fprintf(stdout, "DIMENSION %s '' incremental 1 1\n", w->name);
	}

	fprintf(stdout, "CHART apps.lreads '' 'Apps Disk Logical Reads' 'kilobytes/s' apps apps stacked 20042 %d\n", update_every);
	for (w = target_root; w ; w = w->next) {
		if(w->target || (!w->processes && !w->exposed)) continue;

		fprintf(stdout, "DIMENSION %s '' incremental 1 1024\n", w->name);
	}

	fprintf(stdout, "CHART apps.lwrites '' 'Apps I/O Logical Writes' 'kilobytes/s' apps apps stacked 20042 %d\n", update_every);
	for (w = target_root; w ; w = w->next) {
		if(w->target || (!w->processes && !w->exposed)) continue;

		fprintf(stdout, "DIMENSION %s '' incremental 1 1024\n", w->name);
	}

	fprintf(stdout, "CHART apps.preads '' 'Apps Disk Reads' 'kilobytes/s' apps apps stacked 20002 %d\n", update_every);
	for (w = target_root; w ; w = w->next) {
		if(w->target || (!w->processes && !w->exposed)) continue;

		fprintf(stdout, "DIMENSION %s '' incremental 1 1024\n", w->name);
	}

	fprintf(stdout, "CHART apps.pwrites '' 'Apps Disk Writes' 'kilobytes/s' apps apps stacked 20002 %d\n", update_every);
	for (w = target_root; w ; w = w->next) {
		if(w->target || (!w->processes && !w->exposed)) continue;

		fprintf(stdout, "DIMENSION %s '' incremental 1 1024\n", w->name);
	}

	fprintf(stdout, "CHART netdata.apps_cpu '' 'Apps Plugin CPU' 'milliseconds/s' netdata netdata stacked 10000 %d\n", update_every);
	fprintf(stdout, "DIMENSION user '' incremental 1 1000\n");
	fprintf(stdout, "DIMENSION system '' incremental 1 1000\n");

	fprintf(stdout, "CHART netdata.apps_files '' 'Apps Plugin Files' 'files/s' netdata netdata line 10001 %d\n", update_every);
	fprintf(stdout, "DIMENSION files '' incremental-no-interpolation 1 1\n");

	fflush(stdout);
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

long get_processors(void)
{
	char buffer[1025], *s;
	int processors = 0;

	FILE *fp = fopen("/proc/stat", "r");
	if(!fp) return 1;

	while((s = fgets(buffer, 1024, fp))) {
		if(strncmp(buffer, "cpu", 3) == 0) processors++;
	}
	fclose(fp);
	processors--;
	if(processors < 1) processors = 1;
	return processors;
}

long get_pid_max(void)
{
	char buffer[1025], *s;
	long mpid = 32768;

	FILE *fp = fopen("/proc/sys/kernel/pid_max", "r");
	if(!fp) return 1;

	s = fgets(buffer, 1024, fp);
	if(s) mpid = atol(buffer);
	fclose(fp);
	if(mpid < 32768) mpid = 32768;
	return mpid;
}

int main(int argc, char **argv)
{
	Hertz = get_hertz();
	pid_max = get_pid_max();
	processors = get_processors();

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

		update_statistics();
		show_charts();		// this is smart enough to show only newly added apps, when needed
		show_dimensions();
		// printf("FLUSH xxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxx\n");

		if(debug) fprintf(stderr, "Done Loop No %llu\n", counter);
		fflush(NULL);

		gettimeofday(&now, NULL);
		usec = usecdiff(&now, &last) - susec;
		if(debug) fprintf(stderr, "last loop took %llu usec (worked for %llu, sleeped for %llu).\n", usec + susec, usec, susec);

		// if the last loop took less than half the time
		// wait the rest of the time
		if(usec < (update_every * 1000000ULL / 2)) susec = (update_every * 1000000ULL) - usec;
		else susec = update_every * 1000000ULL / 2;

		//fprintf(stderr, "sleeping for %llu...", susec);
		//fflush(NULL);
		//struct timeval before, after;
		//gettimeofday(&before, NULL);
		usleep(susec);
		//gettimeofday(&after, NULL);
		//fprintf(stderr, " sleeped for %llu\n", usecdiff(&after, &before));
		//fflush(NULL);

		bcopy(&now, &last, sizeof(struct timeval));
	}
}
