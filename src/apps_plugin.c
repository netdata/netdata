// TODO
//
// 1. disable RESET_OR_OVERFLOW check in charts

#ifdef HAVE_CONFIG_H
#include <config.h>
#endif
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
#include <dirent.h>
#include <arpa/inet.h>

#include "common.h"
#include "log.h"
#include "avl.h"
#include "procfile.h"

#define MAX_COMPARE_NAME 15
#define MAX_NAME 100

unsigned long long Hertz = 1;

long processors = 1;
long pid_max = 32768;
int debug = 0;

int update_every = 1;
unsigned long long file_counter = 0;

char *host_prefix = "";

// ----------------------------------------------------------------------------
// memory debugger

struct allocations {
	size_t allocations;
	size_t allocated;
	size_t allocated_max;
} allocations = { 0, 0, 0 };

#define MALLOC_MARK (uint32_t)(0x0BADCAFE)
#define MALLOC_PREFIX (sizeof(uint32_t) * 2)
#define MALLOC_SUFFIX (sizeof(uint32_t))
#define MALLOC_OVERHEAD (MALLOC_PREFIX + MALLOC_SUFFIX)

void *mark_allocation(void *allocated_ptr, size_t size_without_overheads) {
	uint32_t *real_ptr = (uint32_t *)allocated_ptr;
	real_ptr[0] = MALLOC_MARK;
	real_ptr[1] = (uint32_t) size_without_overheads;

	uint32_t *end_ptr = (uint32_t *)(allocated_ptr + MALLOC_PREFIX + size_without_overheads);
	end_ptr[0] = MALLOC_MARK;

	// fprintf(stderr, "MEMORY_POINTER: Allocated at %p, returning %p.\n", allocated_ptr, (void *)(allocated_ptr + MALLOC_PREFIX));

	return allocated_ptr + MALLOC_PREFIX;
}

void *check_allocation(const char *file, int line, const char *function, void *marked_ptr, size_t *size_without_overheads_ptr) {
	uint32_t *real_ptr = (uint32_t *)(marked_ptr - MALLOC_PREFIX);

	// fprintf(stderr, "MEMORY_POINTER: Checking pointer at %p, real %p for %s/%u@%s.\n", marked_ptr, (void *)(marked_ptr - MALLOC_PREFIX), function, line, file);

	if(real_ptr[0] != MALLOC_MARK) fatal("MEMORY: prefix MARK is not valid for %s/%u@%s.", function, line, file);

	size_t size = real_ptr[1];

	uint32_t *end_ptr = (uint32_t *)(marked_ptr + size);
	if(end_ptr[0] != MALLOC_MARK) fatal("MEMORY: suffix MARK of allocation with size %zu is not valid for %s/%u@%s.", size, function, line, file);

	if(size_without_overheads_ptr) *size_without_overheads_ptr = size;

	return real_ptr;
}

void *malloc_debug(const char *file, int line, const char *function, size_t size) {
	void *ptr = malloc(size + MALLOC_OVERHEAD);
	if(!ptr) fatal("MEMORY: Cannot allocate %zu bytes for %s/%u@%s.", size, function, line, file);

	allocations.allocated += size;
	allocations.allocations++;

	debug(D_MEMORY, "MEMORY: Allocated %zu bytes for %s/%u@%s."
		" Status: allocated %zu in %zu allocs."
		, size
		, function, line, file
		, allocations.allocated
		, allocations.allocations
	);

	if(allocations.allocated > allocations.allocated_max) {
		debug(D_MEMORY, "MEMORY: total allocation peak increased from %zu to %zu", allocations.allocated_max, allocations.allocated);
		allocations.allocated_max = allocations.allocated;
	}

	size_t csize;
	check_allocation(file, line, function, mark_allocation(ptr, size), &csize);
	if(size != csize) {
		fatal("Invalid size.");
	}

	return mark_allocation(ptr, size);
}

void *calloc_debug(const char *file, int line, const char *function, size_t nmemb, size_t size) {
	void *ptr = malloc_debug(file, line, function, (nmemb * size));
	bzero(ptr, nmemb * size);
	return ptr;
}

void free_debug(const char *file, int line, const char *function, void *ptr) {
	size_t size;
	void *real_ptr = check_allocation(file, line, function, ptr, &size);

	bzero(real_ptr, size + MALLOC_OVERHEAD);

	free(real_ptr);
	allocations.allocated -= size;
	allocations.allocations--;

	debug(D_MEMORY, "MEMORY: freed %zu bytes for %s/%u@%s."
		" Status: allocated %zu in %zu allocs."
		, size
		, function, line, file
		, allocations.allocated
		, allocations.allocations
	);
}

void *realloc_debug(const char *file, int line, const char *function, void *ptr, size_t size) {
	if(!ptr) return malloc_debug(file, line, function, size);
	if(!size) { free_debug(file, line, function, ptr); return NULL; }

	size_t old_size;
	void *real_ptr = check_allocation(file, line, function, ptr, &old_size);

	void *new_ptr = realloc(real_ptr, size + MALLOC_OVERHEAD);
	if(!new_ptr) fatal("MEMORY: Cannot allocate %zu bytes for %s/%u@%s.", size, function, line, file);

	allocations.allocated += size;
	allocations.allocated -= old_size;

	debug(D_MEMORY, "MEMORY: Re-allocated from %zu to %zu bytes for %s/%u@%s."
		" Status: allocated %z in %zu allocs."
		, old_size, size
		, function, line, file
		, allocations.allocated
		, allocations.allocations
	);

	if(allocations.allocated > allocations.allocated_max) {
		debug(D_MEMORY, "MEMORY: total allocation peak increased from %zu to %zu", allocations.allocated_max, allocations.allocated);
		allocations.allocated_max = allocations.allocated;
	}

	return mark_allocation(new_ptr, size);
}

char *strdup_debug(const char *file, int line, const char *function, const char *ptr) {
	size_t size = 0;
	const char *s = ptr;

	while(*s++) size++;
	size++;

	char *p = malloc_debug(file, line, function, size);
	if(!p) fatal("Cannot allocate %zu bytes.", size);

	memcpy(p, ptr, size);
	return p;
}

#define malloc(size) malloc_debug(__FILE__, __LINE__, __FUNCTION__, (size))
#define calloc(nmemb, size) calloc_debug(__FILE__, __LINE__, __FUNCTION__, (nmemb), (size))
#define realloc(ptr, size) realloc_debug(__FILE__, __LINE__, __FUNCTION__, (ptr), (size))
#define free(ptr) free_debug(__FILE__, __LINE__, __FUNCTION__, (ptr))

#ifdef strdup
#undef strdup
#endif
#define strdup(ptr) strdup_debug(__FILE__, __LINE__, __FUNCTION__, (ptr))

// ----------------------------------------------------------------------------
// helper functions

procfile *ff = NULL;

long get_processors(void) {
	int processors = 0;

	char filename[FILENAME_MAX + 1];
	snprintf(filename, FILENAME_MAX, "%s/proc/stat", host_prefix);

	ff = procfile_reopen(ff, filename, "", PROCFILE_FLAG_DEFAULT);
	if(!ff) return 1;

	ff = procfile_readall(ff);
	if(!ff) {
		// procfile_close(ff);
		return 1;
	}

	unsigned int i;
	for(i = 0; i < procfile_lines(ff); i++) {
		if(!procfile_linewords(ff, i)) continue;

		if(strncmp(procfile_lineword(ff, i, 0), "cpu", 3) == 0) processors++;
	}
	processors--;
	if(processors < 1) processors = 1;

	// procfile_close(ff);
	return processors;
}

long get_pid_max(void) {
	long mpid = 32768;

	char filename[FILENAME_MAX + 1];
	snprintf(filename, FILENAME_MAX, "%s/proc/sys/kernel/pid_max", host_prefix);
	ff = procfile_reopen(ff, filename, "", PROCFILE_FLAG_DEFAULT);
	if(!ff) return mpid;

	ff = procfile_readall(ff);
	if(!ff) {
		// procfile_close(ff);
		return mpid;
	}

	mpid = atol(procfile_lineword(ff, 0, 0));
	if(!mpid) mpid = 32768;

	// procfile_close(ff);
	return mpid;
}

unsigned long long get_hertz(void)
{
	unsigned long long myhz = 1;

#ifdef _SC_CLK_TCK
	if((myhz = (unsigned long long int) sysconf(_SC_CLK_TCK)) > 0) {
		return myhz;
	}
#endif

#ifdef HZ
	myhz = HZ;    /* <asm/param.h> */
#else
	/* If 32-bit or big-endian (not Alpha or ia64), assume HZ is 100. */
	hz = (sizeof(long)==sizeof(int) || htons(999)==999) ? 100UL : 1024UL;
#endif

	error("Unknown HZ value. Assuming %llu.", myhz);
	return myhz;
}


// ----------------------------------------------------------------------------
// target
// target is the point to aggregate a process tree values

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

	int *fds;
	unsigned long long openfiles;
	unsigned long long openpipes;
	unsigned long long opensockets;
	unsigned long long openinotifies;
	unsigned long long openeventfds;
	unsigned long long opentimerfds;
	unsigned long long opensignalfds;
	unsigned long long openeventpolls;
	unsigned long long openother;

	unsigned long processes;	// how many processes have been merged to this
	int exposed;				// if set, we have sent this to netdata
	int hidden;					// if set, we set the hidden flag on the dimension
	int debug;

	struct target *target;	// the one that will be reported to netdata
	struct target *next;
} *target_root = NULL, *default_target = NULL;

long targets = 0;

// find or create a target
// there are targets that are just agregated to other target (the second argument)
struct target *get_target(const char *id, struct target *target)
{
	const char *nid = id;
	if(nid[0] == '-') nid++;

	struct target *w;
	for(w = target_root ; w ; w = w->next)
		if(strncmp(nid, w->id, MAX_NAME) == 0) return w;

	w = calloc(sizeof(struct target), 1);
	if(!w) {
		error("Cannot allocate %lu bytes of memory", (unsigned long)sizeof(struct target));
		return NULL;
	}

	strncpy(w->id, nid, MAX_NAME);
	strncpy(w->name, nid, MAX_NAME);
	strncpy(w->compare, nid, MAX_COMPARE_NAME);
	if(id[0] == '-') w->hidden = 1;

	w->target = target;

	w->next = target_root;
	target_root = w;

	if(debug) fprintf(stderr, "apps.plugin: adding hook for process '%s', compare '%s' on target '%s'\n", w->id, w->compare, w->target?w->target->id:"");

	return w;
}

// read the process groups file
int read_process_groups(const char *name)
{
	char buffer[4096+1];
	char filename[FILENAME_MAX + 1];

	snprintf(filename, FILENAME_MAX, "%s/apps_%s.conf", CONFIG_DIR, name);

	if(debug) fprintf(stderr, "apps.plugin: process groups file: '%s'\n", filename);
	FILE *fp = fopen(filename, "r");
	if(!fp) {
		error("Cannot open file '%s'", filename);
		return 1;
	}

	long line = 0;
	while(fgets(buffer, 4096, fp) != NULL) {
		int whidden = 0, wdebug = 0;
		line++;

		// if(debug) fprintf(stderr, "apps.plugin: \tread %s\n", buffer);

		char *s = buffer, *t, *p;
		s = trim(s);
		if(!s || !*s || *s == '#') continue;

		if(debug) fprintf(stderr, "apps.plugin: \tread %s\n", s);

		// the target name
		t = strsep(&s, ":");
		if(t) t = trim(t);
		if(!t || !*t) continue;

		while(t[0]) {
			int stop = 1;

			switch(t[0]) {
				case '-':
					stop = 0;
					whidden = 1;
					t++;
					break;

				case '+':
					stop = 0;
					wdebug = 1;
					t++;
					break;
			}

			if(stop) break;
		}

		if(debug) fprintf(stderr, "apps.plugin: \t\ttarget %s\n", t);

		struct target *w = NULL;
		long count = 0;
		int blen = 0;
		char buffer[4097] = "";
		buffer[4096] = '\0';

		// the process names
		while((p = strsep(&s, " "))) {
			p = trim(p);
			if(!p || !*p) continue;

			strncpy(&buffer[blen], p, 4096 - blen);
			blen = strlen(buffer);

			while(buffer[blen - 1] == '\\') {
				buffer[blen - 1] = ' ';

				if((p = strsep(&s, " ")))
					p = trim(p);

				if(!p || !*p) p = " ";
				strncpy(&buffer[blen], p, 4096 - blen);
				blen = strlen(buffer);
			}

			struct target *n = get_target(buffer, w);
			n->hidden = whidden;
			n->debug = wdebug;
			if(!w) w = n;

			buffer[0] = '\0';
			blen = 0;

			count++;
		}

		if(w) strncpy(w->name, t, MAX_NAME);
		if(!count) error("The line %ld on file '%s', for group '%s' does not state any process names.", line, filename, t);
	}
	fclose(fp);

	default_target = get_target("+p!o@w#e$i^r&7*5(-i)l-o_", NULL); // match nothing
	strncpy(default_target->name, "other", MAX_NAME);

	return 0;
}


// ----------------------------------------------------------------------------
// data to store for each pid
// see: man proc

struct pid_stat {
	int32_t pid;
	char comm[MAX_COMPARE_NAME + 1];
	// char state;
	int32_t ppid;
	// int32_t pgrp;
	// int32_t session;
	// int32_t tty_nr;
	// int32_t tpgid;
	// uint64_t flags;
	unsigned long long minflt;
	unsigned long long cminflt;
	unsigned long long majflt;
	unsigned long long cmajflt;
	unsigned long long utime;
	unsigned long long stime;
	unsigned long long cutime;
	unsigned long long cstime;
	// int64_t priority;
	// int64_t nice;
	int32_t num_threads;
	// int64_t itrealvalue;
	// unsigned long long starttime;
	// unsigned long long vsize;
	unsigned long long rss;
	// unsigned long long rsslim;
	// unsigned long long starcode;
	// unsigned long long endcode;
	// unsigned long long startstack;
	// unsigned long long kstkesp;
	// unsigned long long kstkeip;
	// uint64_t signal;
	// uint64_t blocked;
	// uint64_t sigignore;
	// uint64_t sigcatch;
	// uint64_t wchan;
	// uint64_t nswap;
	// uint64_t cnswap;
	// int32_t exit_signal;
	// int32_t processor;
	// uint32_t rt_priority;
	// uint32_t policy;
	// unsigned long long delayacct_blkio_ticks;
	// uint64_t guest_time;
	// int64_t cguest_time;

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

#ifdef INCLUDE_CHILDS
	unsigned long long old_utime;
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
#endif

	int *fds;					// array of fds it uses
	int fds_size;				// the size of the fds array

	int childs;					// number of processes directly referencing this
	int updated;				// 1 when update
	int merged;					// 1 when it has been merged to its parent
	int new_entry;
	struct target *target;
	struct pid_stat *parent;
	struct pid_stat *prev;
	struct pid_stat *next;
} *root_of_pids = NULL, **all_pids;

long all_pids_count = 0;

struct pid_stat *get_pid_entry(pid_t pid)
{
	if(all_pids[pid]) {
		all_pids[pid]->new_entry = 0;
		return all_pids[pid];
	}

	all_pids[pid] = calloc(sizeof(struct pid_stat), 1);
	if(!all_pids[pid]) {
		error("Cannot allocate %lu bytes of memory", (unsigned long)sizeof(struct pid_stat));
		return NULL;
	}

	all_pids[pid]->fds = calloc(sizeof(int), 100);
	if(!all_pids[pid]->fds)
		error("Cannot allocate %ld bytes of memory", (unsigned long)(sizeof(int) * 100));
	else all_pids[pid]->fds_size = 100;

	if(root_of_pids) root_of_pids->prev = all_pids[pid];
	all_pids[pid]->next = root_of_pids;
	root_of_pids = all_pids[pid];

	all_pids[pid]->pid = pid;
	all_pids[pid]->new_entry = 1;

	return all_pids[pid];
}

void del_pid_entry(pid_t pid)
{
	if(!all_pids[pid]) return;

	if(debug) fprintf(stderr, "apps.plugin: process %d %s exited, deleting it.\n", pid, all_pids[pid]->comm);

	if(root_of_pids == all_pids[pid]) root_of_pids = all_pids[pid]->next;
	if(all_pids[pid]->next) all_pids[pid]->next->prev = all_pids[pid]->prev;
	if(all_pids[pid]->prev) all_pids[pid]->prev->next = all_pids[pid]->next;

	if(all_pids[pid]->fds) free(all_pids[pid]->fds);
	free(all_pids[pid]);
	all_pids[pid] = NULL;
}


// ----------------------------------------------------------------------------
// update pids from proc

int read_proc_pid_stat(struct pid_stat *p) {
	char filename[FILENAME_MAX + 1];

	snprintf(filename, FILENAME_MAX, "%s/proc/%d/stat", host_prefix, p->pid);

	ff = procfile_reopen(ff, filename, NULL, PROCFILE_FLAG_NO_ERROR_ON_FILE_IO);
	if(!ff) return 1;

	ff = procfile_readall(ff);
	if(!ff) {
		// procfile_close(ff);
		return 1;
	}

	file_counter++;

	p->comm[0] = '\0';
	p->comm[MAX_COMPARE_NAME] = '\0';
	size_t blen = 0;

	char *s = procfile_lineword(ff, 0, 1);
	if(*s == '(') s++;
	size_t len = strlen(s);
	unsigned int i = 0;
	while(len && s[len - 1] != ')') {
		if(blen < MAX_COMPARE_NAME) {
			strncpy(&p->comm[blen], s, MAX_COMPARE_NAME - blen);
			blen = strlen(p->comm);
		}

		i++;
		s = procfile_lineword(ff, 0, 1+i);
		len = strlen(s);
	}
	if(len && s[len - 1] == ')') s[len - 1] = '\0';
	if(blen < MAX_COMPARE_NAME)
		strncpy(&p->comm[blen], s, MAX_COMPARE_NAME - blen);

	// p->pid			= atol(procfile_lineword(ff, 0, 0+i));
	// comm is at 1
	// p->state			= *(procfile_lineword(ff, 0, 2+i));
	p->ppid				= (int32_t) atol(procfile_lineword(ff, 0, 3 + i));
	// p->pgrp			= atol(procfile_lineword(ff, 0, 4+i));
	// p->session		= atol(procfile_lineword(ff, 0, 5+i));
	// p->tty_nr		= atol(procfile_lineword(ff, 0, 6+i));
	// p->tpgid			= atol(procfile_lineword(ff, 0, 7+i));
	// p->flags			= strtoull(procfile_lineword(ff, 0, 8+i), NULL, 10);
	p->minflt			= strtoull(procfile_lineword(ff, 0, 9+i), NULL, 10);
	p->cminflt			= strtoull(procfile_lineword(ff, 0, 10+i), NULL, 10);
	p->majflt			= strtoull(procfile_lineword(ff, 0, 11+i), NULL, 10);
	p->cmajflt			= strtoull(procfile_lineword(ff, 0, 12+i), NULL, 10);
	p->utime			= strtoull(procfile_lineword(ff, 0, 13+i), NULL, 10);
	p->stime			= strtoull(procfile_lineword(ff, 0, 14+i), NULL, 10);
	p->cutime			= strtoull(procfile_lineword(ff, 0, 15+i), NULL, 10);
	p->cstime			= strtoull(procfile_lineword(ff, 0, 16+i), NULL, 10);
	// p->priority		= strtoull(procfile_lineword(ff, 0, 17+i), NULL, 10);
	// p->nice			= strtoull(procfile_lineword(ff, 0, 18+i), NULL, 10);
	p->num_threads		= (int32_t) atol(procfile_lineword(ff, 0, 19 + i));
	// p->itrealvalue	= strtoull(procfile_lineword(ff, 0, 20+i), NULL, 10);
	// p->starttime		= strtoull(procfile_lineword(ff, 0, 21+i), NULL, 10);
	// p->vsize			= strtoull(procfile_lineword(ff, 0, 22+i), NULL, 10);
	p->rss				= strtoull(procfile_lineword(ff, 0, 23+i), NULL, 10);
	// p->rsslim		= strtoull(procfile_lineword(ff, 0, 24+i), NULL, 10);
	// p->starcode		= strtoull(procfile_lineword(ff, 0, 25+i), NULL, 10);
	// p->endcode		= strtoull(procfile_lineword(ff, 0, 26+i), NULL, 10);
	// p->startstack	= strtoull(procfile_lineword(ff, 0, 27+i), NULL, 10);
	// p->kstkesp		= strtoull(procfile_lineword(ff, 0, 28+i), NULL, 10);
	// p->kstkeip		= strtoull(procfile_lineword(ff, 0, 29+i), NULL, 10);
	// p->signal		= strtoull(procfile_lineword(ff, 0, 30+i), NULL, 10);
	// p->blocked		= strtoull(procfile_lineword(ff, 0, 31+i), NULL, 10);
	// p->sigignore		= strtoull(procfile_lineword(ff, 0, 32+i), NULL, 10);
	// p->sigcatch		= strtoull(procfile_lineword(ff, 0, 33+i), NULL, 10);
	// p->wchan			= strtoull(procfile_lineword(ff, 0, 34+i), NULL, 10);
	// p->nswap			= strtoull(procfile_lineword(ff, 0, 35+i), NULL, 10);
	// p->cnswap		= strtoull(procfile_lineword(ff, 0, 36+i), NULL, 10);
	// p->exit_signal	= atol(procfile_lineword(ff, 0, 37+i));
	// p->processor		= atol(procfile_lineword(ff, 0, 38+i));
	// p->rt_priority	= strtoul(procfile_lineword(ff, 0, 39+i), NULL, 10);
	// p->policy		= strtoul(procfile_lineword(ff, 0, 40+i), NULL, 10);
	// p->delayacct_blkio_ticks		= strtoull(procfile_lineword(ff, 0, 41+i), NULL, 10);
	// p->guest_time	= strtoull(procfile_lineword(ff, 0, 42+i), NULL, 10);
	// p->cguest_time	= strtoull(procfile_lineword(ff, 0, 43), NULL, 10);

	if(debug || (p->target && p->target->debug)) fprintf(stderr, "apps.plugin: VALUES: %s utime=%llu, stime=%llu, cutime=%llu, cstime=%llu, minflt=%llu, majflt=%llu, cminflt=%llu, cmajflt=%llu, threads=%d\n", p->comm, p->utime, p->stime, p->cutime, p->cstime, p->minflt, p->majflt, p->cminflt, p->cmajflt, p->num_threads);

	// procfile_close(ff);
	return 0;
}

int read_proc_pid_statm(struct pid_stat *p) {
	char filename[FILENAME_MAX + 1];

	snprintf(filename, FILENAME_MAX, "%s/proc/%d/statm", host_prefix, p->pid);

	ff = procfile_reopen(ff, filename, NULL, PROCFILE_FLAG_NO_ERROR_ON_FILE_IO);
	if(!ff) return 1;

	ff = procfile_readall(ff);
	if(!ff) {
		// procfile_close(ff);
		return 1;
	}

	file_counter++;

	p->statm_size			= strtoull(procfile_lineword(ff, 0, 0), NULL, 10);
	p->statm_resident		= strtoull(procfile_lineword(ff, 0, 1), NULL, 10);
	p->statm_share			= strtoull(procfile_lineword(ff, 0, 2), NULL, 10);
	p->statm_text			= strtoull(procfile_lineword(ff, 0, 3), NULL, 10);
	p->statm_lib			= strtoull(procfile_lineword(ff, 0, 4), NULL, 10);
	p->statm_data			= strtoull(procfile_lineword(ff, 0, 5), NULL, 10);
	p->statm_dirty			= strtoull(procfile_lineword(ff, 0, 6), NULL, 10);

	// procfile_close(ff);
	return 0;
}

int read_proc_pid_io(struct pid_stat *p) {
	char filename[FILENAME_MAX + 1];

	snprintf(filename, FILENAME_MAX, "%s/proc/%d/io", host_prefix, p->pid);

	ff = procfile_reopen(ff, filename, NULL, PROCFILE_FLAG_NO_ERROR_ON_FILE_IO);
	if(!ff) return 1;

	ff = procfile_readall(ff);
	if(!ff) {
		// procfile_close(ff);
		return 1;
	}

	file_counter++;

	p->io_logical_bytes_read 		= strtoull(procfile_lineword(ff, 0, 1), NULL, 10);
	p->io_logical_bytes_written 	= strtoull(procfile_lineword(ff, 1, 1), NULL, 10);
	p->io_read_calls 				= strtoull(procfile_lineword(ff, 2, 1), NULL, 10);
	p->io_write_calls 				= strtoull(procfile_lineword(ff, 3, 1), NULL, 10);
	p->io_storage_bytes_read 		= strtoull(procfile_lineword(ff, 4, 1), NULL, 10);
	p->io_storage_bytes_written 	= strtoull(procfile_lineword(ff, 5, 1), NULL, 10);
	p->io_cancelled_write_bytes		= strtoull(procfile_lineword(ff, 6, 1), NULL, 10);

	// procfile_close(ff);
	return 0;
}


// ----------------------------------------------------------------------------

#ifdef INCLUDE_CHILDS
// print a tree view of all processes
int walk_down(pid_t pid, int level) {
	struct pid_stat *p = NULL;
	char b[level+3];
	int i, ret = 0;

	for(i = 0; i < level; i++) b[i] = '\t';
	b[level] = '|';
	b[level+1] = '-';
	b[level+2] = '\0';

	for(p = root_of_pids; p ; p = p->next) {
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
#endif


// ----------------------------------------------------------------------------
// file descriptor
// this is used to keep a global list of all open files of the system
// it is needed in order to figure out the unique files a process tree has open

#define FILE_DESCRIPTORS_INCREASE_STEP 100

struct file_descriptor {
	avl avl;
	uint32_t magic;
	uint32_t hash;
	const char *name;
	int type;
	int count;
	int pos;
} *all_files = NULL;

int all_files_len = 0;
int all_files_size = 0;

int file_descriptor_compare(void* a, void* b) {
	if(((struct file_descriptor *)a)->magic != 0x0BADCAFE || ((struct file_descriptor *)b)->magic != 0x0BADCAFE)
		error("Corrupted index data detected. Please report this.");

	if(((struct file_descriptor *)a)->hash < ((struct file_descriptor *)b)->hash)
		return -1;
	else if(((struct file_descriptor *)a)->hash > ((struct file_descriptor *)b)->hash)
		return 1;
	else return strcmp(((struct file_descriptor *)a)->name, ((struct file_descriptor *)b)->name);
}
int file_descriptor_iterator(avl *a) { if(a) {}; return 0; }

avl_tree all_files_index = {
		NULL,
		file_descriptor_compare,
#ifdef AVL_LOCK_WITH_MUTEX
		PTHREAD_MUTEX_INITIALIZER
#else
		PTHREAD_RWLOCK_INITIALIZER
#endif
};

static struct file_descriptor *file_descriptor_find(const char *name, uint32_t hash) {
	struct file_descriptor *result = NULL, tmp;
	tmp.hash = (hash)?hash:simple_hash(name);
	tmp.name = name;
	tmp.count = 0;
	tmp.pos = 0;
	tmp.magic = 0x0BADCAFE;

	avl_search(&all_files_index, (avl *)&tmp, file_descriptor_iterator, (avl **)&result);
	return result;
}

#define file_descriptor_add(fd) avl_insert(&all_files_index, (avl *)(fd))
#define file_descriptor_remove(fd) avl_remove(&all_files_index, (avl *)(fd))

#define FILETYPE_OTHER 0
#define FILETYPE_FILE 1
#define FILETYPE_PIPE 2
#define FILETYPE_SOCKET 3
#define FILETYPE_INOTIFY 4
#define FILETYPE_EVENTFD 5
#define FILETYPE_EVENTPOLL 6
#define FILETYPE_TIMERFD 7
#define FILETYPE_SIGNALFD 8

void file_descriptor_not_used(int id)
{
	if(id > 0 && id < all_files_size) {
		if(all_files[id].magic != 0x0BADCAFE) {
			error("Ignoring request to remove empty file id %d.", id);
			return;
		}

		if(debug) fprintf(stderr, "apps.plugin: decreasing slot %d (count = %d).\n", id, all_files[id].count);

		if(all_files[id].count > 0) {
			all_files[id].count--;

			if(!all_files[id].count) {
				if(debug) fprintf(stderr, "apps.plugin:   >> slot %d is empty.\n", id);
				file_descriptor_remove(&all_files[id]);
				all_files[id].magic = 0x00000000;
				all_files_len--;
			}
		}
		else
			error("Request to decrease counter of fd %d (%s), while the use counter is 0", id, all_files[id].name);
	}
	else	error("Request to decrease counter of fd %d, which is outside the array size (1 to %d)", id, all_files_size);
}

int file_descriptor_find_or_add(const char *name)
{
	static int last_pos = 0;
	uint32_t hash = simple_hash(name);

	if(debug) fprintf(stderr, "apps.plugin: adding or finding name '%s' with hash %u\n", name, hash);

	struct file_descriptor *fd = file_descriptor_find(name, hash);
	if(fd) {
		// found
		if(debug) fprintf(stderr, "apps.plugin:   >> found on slot %d\n", fd->pos);
		fd->count++;
		return fd->pos;
	}
	// not found

	// check we have enough memory to add it
	if(!all_files || all_files_len == all_files_size) {
		void *old = all_files;
		int i;

		// there is no empty slot
		if(debug) fprintf(stderr, "apps.plugin: extending fd array to %d entries\n", all_files_size + FILE_DESCRIPTORS_INCREASE_STEP);
		all_files = realloc(all_files, (all_files_size + FILE_DESCRIPTORS_INCREASE_STEP) * sizeof(struct file_descriptor));

		// if the address changed, we have to rebuild the index
		// since all pointers are now invalid
		if(old && old != (void *)all_files) {
			if(debug) fprintf(stderr, "apps.plugin:   >> re-indexing.\n");
			all_files_index.root = NULL;
			for(i = 0; i < all_files_size; i++) {
				if(!all_files[i].count) continue;
				file_descriptor_add(&all_files[i]);
			}
			if(debug) fprintf(stderr, "apps.plugin:   >> re-indexing done.\n");
		}

		for(i = all_files_size; i < (all_files_size + FILE_DESCRIPTORS_INCREASE_STEP); i++) {
			all_files[i].count = 0;
			all_files[i].name = NULL;
			all_files[i].magic = 0x00000000;
			all_files[i].pos = i;
		}

		if(!all_files_size) all_files_len = 1;
		all_files_size += FILE_DESCRIPTORS_INCREASE_STEP;
	}

	if(debug) fprintf(stderr, "apps.plugin:   >> searching for empty slot.\n");

	// search for an empty slot
	int i, c;
	for(i = 0, c = last_pos ; i < all_files_size ; i++, c++) {
		if(c >= all_files_size) c = 0;
		if(c == 0) continue;

		if(!all_files[c].count) {
			if(debug) fprintf(stderr, "apps.plugin:   >> Examining slot %d.\n", c);

			if(all_files[c].magic == 0x0BADCAFE && all_files[c].name && file_descriptor_find(all_files[c].name, all_files[c].hash))
				error("fd on position %d is not cleared properly. It still has %s in it.\n", c, all_files[c].name);

			if(debug) fprintf(stderr, "apps.plugin:   >> %s fd position %d for %s (last name: %s)\n", all_files[c].name?"re-using":"using", c, name, all_files[c].name);
			if(all_files[c].name) free((void *)all_files[c].name);
			all_files[c].name = NULL;
			last_pos = c;
			break;
		}
	}
	if(i == all_files_size) {
		fatal("We should find an empty slot, but there isn't any");
		exit(1);
	}
	if(debug) fprintf(stderr, "apps.plugin:   >> updating slot %d.\n", c);

	all_files_len++;

	// else we have an empty slot in 'c'

	int type;
	if(name[0] == '/') type = FILETYPE_FILE;
	else if(strncmp(name, "pipe:", 5) == 0) type = FILETYPE_PIPE;
	else if(strncmp(name, "socket:", 7) == 0) type = FILETYPE_SOCKET;
	else if(strcmp(name, "anon_inode:inotify") == 0 || strcmp(name, "inotify") == 0) type = FILETYPE_INOTIFY;
	else if(strcmp(name, "anon_inode:[eventfd]") == 0) type = FILETYPE_EVENTFD;
	else if(strcmp(name, "anon_inode:[eventpoll]") == 0) type = FILETYPE_EVENTPOLL;
	else if(strcmp(name, "anon_inode:[timerfd]") == 0) type = FILETYPE_TIMERFD;
	else if(strcmp(name, "anon_inode:[signalfd]") == 0) type = FILETYPE_SIGNALFD;
	else if(strncmp(name, "anon_inode:", 11) == 0) {
		if(debug) fprintf(stderr, "apps.plugin: FIXME: unknown anonymous inode: %s\n", name);
		type = FILETYPE_OTHER;
	}
	else {
		if(debug) fprintf(stderr, "apps.plugin: FIXME: cannot understand linkname: %s\n", name);
		type = FILETYPE_OTHER;
	}

	all_files[c].name = strdup(name);
	all_files[c].hash = hash;
	all_files[c].type = type;
	all_files[c].pos  = c;
	all_files[c].count = 1;
	all_files[c].magic = 0x0BADCAFE;

	file_descriptor_add(&all_files[c]);

	if(debug) fprintf(stderr, "apps.plugin: using fd position %d (name: %s)\n", c, all_files[c].name);

	return c;
}


// 1. read all files in /proc
// 2. for each numeric directory:
//    i.   read /proc/pid/stat
//    ii.  read /proc/pid/statm
//    iii. read /proc/pid/io (requires root access)
//    iii. read the entries in directory /proc/pid/fd (requires root access)
//         for each entry:
//         a. find or create a struct file_descriptor
//         b. cleanup any old/unused file_descriptors

// after all these, some pids may be linked to targets, while others may not

// in case of errors, only 1 every 1000 errors is printed
// to avoid filling up all disk space
// if debug is enabled, all errors are printed

int update_from_proc(void)
{
	static long count_errors = 0;

	char filename[FILENAME_MAX+1];
	char dirname[FILENAME_MAX + 1];

	snprintf(dirname, FILENAME_MAX, "%s/proc", host_prefix);
	DIR *dir = opendir(dirname);
	if(!dir) return 0;

	struct dirent *file = NULL;
	struct pid_stat *p = NULL;

	// mark them all as un-updated
	all_pids_count = 0;
	for(p = root_of_pids; p ; p = p->next) {
		all_pids_count++;
		p->parent = NULL;
		p->updated = 0;
		p->childs = 0;
		p->merged = 0;
		p->new_entry = 0;
	}

	while((file = readdir(dir))) {
		char *endptr = file->d_name;
		pid_t pid = (pid_t) strtoul(file->d_name, &endptr, 10);
		if(pid <= 0 || pid > pid_max || endptr == file->d_name || *endptr != '\0') continue;

		p = get_pid_entry(pid);
		if(!p) continue;

		// --------------------------------------------------------------------
		// /proc/<pid>/stat

		if(read_proc_pid_stat(p)) {
			if(!count_errors++ || debug || (p->target && p->target->debug))
				error("Cannot process %s/proc/%d/stat", host_prefix, pid);

			continue;
		}
		if(p->ppid < 0 || p->ppid > pid_max) p->ppid = 0;


		// --------------------------------------------------------------------
		// /proc/<pid>/statm

		if(read_proc_pid_statm(p)) {
			if(!count_errors++ || debug || (p->target && p->target->debug))
				error("Cannot process %s/proc/%d/statm", host_prefix, pid);

			continue;
		}


		// --------------------------------------------------------------------
		// /proc/<pid>/io

		if(read_proc_pid_io(p)) {
			if(!count_errors++ || debug || (p->target && p->target->debug))
				error("Cannot process %s/proc/%d/io", host_prefix, pid);

			// on systems without /proc/X/io
			// allow proceeding without I/O information
			// continue;
		}

		// --------------------------------------------------------------------
		// link it

		// check if it is target
		// we do this only once, the first time this pid is loaded
		if(p->new_entry) {
			if(debug) fprintf(stderr, "apps.plugin: \tJust added %s\n", p->comm);

			struct target *w;
			for(w = target_root; w ; w = w->next) {
				// if(debug || (p->target && p->target->debug)) fprintf(stderr, "apps.plugin: \t\tcomparing '%s' with '%s'\n", w->compare, p->comm);

				if(strcmp(w->compare, p->comm) == 0) {
					if(w->target) p->target = w->target;
					else p->target = w;

					if(debug || (p->target && p->target->debug)) fprintf(stderr, "apps.plugin: \t\t%s linked to target %s\n", p->comm, p->target->name);
				}
			}
		}

		// --------------------------------------------------------------------
		// /proc/<pid>/fd

		snprintf(filename, FILENAME_MAX, "%s/proc/%s/fd", host_prefix, file->d_name);
		DIR *fds = opendir(filename);
		if(fds) {
			int c;
			struct dirent *de;
			char fdname[FILENAME_MAX + 1];
			char linkname[FILENAME_MAX + 1];

			// make the array negative
			for(c = 0 ; c < p->fds_size ; c++) p->fds[c] = -p->fds[c];

			while((de = readdir(fds))) {
				if(strcmp(de->d_name, ".") == 0 || strcmp(de->d_name, "..") == 0) continue;

				// check if the fds array is small
				int fdid = atoi(de->d_name);
				if(fdid < 0) continue;
				if(fdid >= p->fds_size) {
					// it is small, extend it
					if(debug) fprintf(stderr, "apps.plugin: extending fd memory slots for %s from %d to %d\n", p->comm, p->fds_size, fdid + 100);
					p->fds = realloc(p->fds, (fdid + 100) * sizeof(int));
					if(!p->fds) {
						error("Cannot re-allocate fds for %s", p->comm);
						break;
					}

					// and initialize it
					for(c = p->fds_size ; c < (fdid + 100) ; c++) p->fds[c] = 0;
					p->fds_size = fdid + 100;
				}

				if(p->fds[fdid] == 0) {
					// we don't know this fd, get it

					sprintf(fdname, "%s/proc/%s/fd/%s", host_prefix, file->d_name, de->d_name);
					ssize_t l = readlink(fdname, linkname, FILENAME_MAX);
					if(l == -1) {
						if(debug || (p->target && p->target->debug)) {
							if(!count_errors++ || debug || (p->target && p->target->debug))
								error("Cannot read link %s", fdname);
						}
						continue;
					}
					linkname[l] = '\0';
					file_counter++;

					// if another process already has this, we will get
					// the same id
					p->fds[fdid] = file_descriptor_find_or_add(linkname);
				}

				// else make it positive again, we need it
				// of course, the actual file may have changed, but we don't care so much
				// FIXME: we could compare the inode as returned by readdir direct structure
				else p->fds[fdid] = -p->fds[fdid];
			}
			closedir(fds);

			// remove all the negative file descriptors
			for(c = 0 ; c < p->fds_size ; c++) if(p->fds[c] < 0) {
				file_descriptor_not_used(-p->fds[c]);
				p->fds[c] = 0;
			}
		}

		// --------------------------------------------------------------------
		// done!

		// mark it as updated
		p->updated = 1;
	}
	if(count_errors > 1000) {
		error("%ld more errors encountered\n", count_errors - 1);
		count_errors = 0;
	}

	closedir(dir);

	return 1;
}


// ----------------------------------------------------------------------------
// update statistics on the targets

// 1. link all childs to their parents
// 2. go from bottom to top, marking as merged all childs to their parents
//    this step links all parents without a target to the child target, if any
// 3. link all top level processes (the ones not merged) to the default target
// 4. go from top to bottom, linking all childs without a target, to their parent target
//    after this step, all processes have a target
// [5. for each killed pid (updated = 0), remove its usage from its target]
// 6. zero all targets
// 7. concentrate all values on the targets
// 8. remove all killed processes
// 9. find the unique file count for each target

void update_statistics(void)
{
	int c;
	struct pid_stat *p = NULL;

	// link all parents and update childs count
	for(p = root_of_pids; p ; p = p->next) {
		if(p->ppid > 0 && p->ppid <= pid_max && all_pids[p->ppid]) {
			if(debug || (p->target && p->target->debug)) fprintf(stderr, "apps.plugin: \tparent of %d %s is %d %s\n", p->pid, p->comm, p->ppid, all_pids[p->ppid]->comm);

			p->parent = all_pids[p->ppid];
			p->parent->childs++;
		}
		else if(p->ppid != 0) error("pid %d %s states parent %d, but the later does not exist.", p->pid, p->comm, p->ppid);
	}

	// find all the procs with 0 childs and merge them to their parents
	// repeat, until nothing more can be done.
	int found = 1;
	while(found) {
		found = 0;
		for(p = root_of_pids; p ; p = p->next) {
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
					if(debug || (p->target && p->target->debug)) fprintf(stderr, "apps.plugin: \t\ttarget %s is inherited by %d %s from its child %d %s.\n", p->target->name, p->parent->pid, p->parent->comm, p->pid, p->comm);
				}

				found++;
			}
		}
		if(debug) fprintf(stderr, "apps.plugin: merged %d processes\n", found);
	}

	// give a default target on all top level processes
	// init goes always to default target
	if(all_pids[1]) all_pids[1]->target = default_target;

	for(p = root_of_pids; p ; p = p->next) {
		// if the process is not merged itself
		// then is is a top level process
		if(!p->merged && !p->target) p->target = default_target;

#ifdef INCLUDE_CHILDS
		// by the way, update the diffs
		// will be used later for substracting killed process times
		p->diff_cutime = p->utime - p->cutime;
		p->diff_cstime = p->stime - p->cstime;
		p->diff_cminflt = p->minflt - p->cminflt;
		p->diff_cmajflt = p->majflt - p->cmajflt;
#endif
	}

	// give a target to all merged child processes
	found = 1;
	while(found) {
		found = 0;
		for(p = root_of_pids; p ; p = p->next) {
			if(!p->target && p->merged && p->parent && p->parent->target) {
				p->target = p->parent->target;
				found++;
			}
		}
	}

#ifdef INCLUDE_CHILDS
	// for each killed process, remove its values from the parents
	// sums (we had already added them in a previous loop)
	for(p = root_of_pids; p ; p = p->next) {
		if(p->updated) continue;

		if(debug) fprintf(stderr, "apps.plugin: UNMERGING %d %s\n", p->pid, p->comm);

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
				if(debug) fprintf(stderr, "apps.plugin: \t cutime %llu from %d %s %s\n", x, t->pid, t->comm, t->target->name);
			}
			if(diff_stime && t->diff_cstime) {
				x = (t->diff_cstime < diff_stime)?t->diff_cstime:diff_stime;
				diff_stime -= x;
				t->diff_cstime -= x;
				t->fix_cstime += x;
				if(debug) fprintf(stderr, "apps.plugin: \t cstime %llu from %d %s %s\n", x, t->pid, t->comm, t->target->name);
			}
			if(diff_minflt && t->diff_cminflt) {
				x = (t->diff_cminflt < diff_minflt)?t->diff_cminflt:diff_minflt;
				diff_minflt -= x;
				t->diff_cminflt -= x;
				t->fix_cminflt += x;
				if(debug) fprintf(stderr, "apps.plugin: \t cminflt %llu from %d %s %s\n", x, t->pid, t->comm, t->target->name);
			}
			if(diff_majflt && t->diff_cmajflt) {
				x = (t->diff_cmajflt < diff_majflt)?t->diff_cmajflt:diff_majflt;
				diff_majflt -= x;
				t->diff_cmajflt -= x;
				t->fix_cmajflt += x;
				if(debug) fprintf(stderr, "apps.plugin: \t cmajflt %llu from %d %s %s\n", x, t->pid, t->comm, t->target->name);
			}
		}

		if(diff_utime) error("Cannot fix up utime %llu", diff_utime);
		if(diff_stime) error("Cannot fix up stime %llu", diff_stime);
		if(diff_minflt) error("Cannot fix up minflt %llu", diff_minflt);
		if(diff_majflt) error("Cannot fix up majflt %llu", diff_majflt);
	}
#endif

	// zero all the targets
	targets = 0;
	struct target *w;
	for (w = target_root; w ; w = w->next) {
		targets++;

		w->fds = calloc(sizeof(int), (size_t) all_files_size);
		if(!w->fds)
			error("Cannot allocate memory for fds in %s", w->name);

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

#ifdef INCLUDE_CHILDS
	if(debug) walk_down(0, 1);
#endif

	// concentrate everything on the targets
	for(p = root_of_pids; p ; p = p->next) {
		if(!p->target) {
			error("pid %d %s was left without a target!", p->pid, p->comm);
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

			//if(p->num_threads < 0)
			//	error("Negative threads number for pid '%s' (%d): %d", p->comm, p->pid, p->num_threads);

			//if(p->num_threads > 10000)
			//	error("Excessive threads number for pid '%s' (%d): %d", p->comm, p->pid, p->num_threads);

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

			for(c = 0; c < p->fds_size ;c++) {
				if(p->fds[c] == 0) continue;
				if(p->fds[c] < all_files_size) {
					if(p->target->fds) p->target->fds[p->fds[c]]++;
				}
				else
					error("Invalid fd number %d", p->fds[c]);
			}

			if(debug || p->target->debug) fprintf(stderr, "apps.plugin: \tAgregating %s pid %d on %s utime=%llu, stime=%llu, cutime=%llu, cstime=%llu, minflt=%llu, majflt=%llu, cminflt=%llu, cmajflt=%llu\n", p->comm, p->pid, p->target->name, p->utime, p->stime, p->cutime, p->cstime, p->minflt, p->majflt, p->cminflt, p->cmajflt);

/*			if(p->utime - p->old_utime > 100) fprintf(stderr, "BIG CHANGE: %d %s utime increased by %llu from %llu to %llu\n", p->pid, p->comm, p->utime - p->old_utime, p->old_utime, p->utime);
			if(p->cutime - p->old_cutime > 100) fprintf(stderr, "BIG CHANGE: %d %s cutime increased by %llu from %llu to %llu\n", p->pid, p->comm, p->cutime - p->old_cutime, p->old_cutime, p->cutime);
			if(p->stime - p->old_stime > 100) fprintf(stderr, "BIG CHANGE: %d %s stime increased by %llu from %llu to %llu\n", p->pid, p->comm, p->stime - p->old_stime, p->old_stime, p->stime);
			if(p->cstime - p->old_cstime > 100) fprintf(stderr, "BIG CHANGE: %d %s cstime increased by %llu from %llu to %llu\n", p->pid, p->comm, p->cstime - p->old_cstime, p->old_cstime, p->cstime);
			if(p->minflt - p->old_minflt > 5000) fprintf(stderr, "BIG CHANGE: %d %s minflt increased by %llu from %llu to %llu\n", p->pid, p->comm, p->minflt - p->old_minflt, p->old_minflt, p->minflt);
			if(p->majflt - p->old_majflt > 5000) fprintf(stderr, "BIG CHANGE: %d %s majflt increased by %llu from %llu to %llu\n", p->pid, p->comm, p->majflt - p->old_majflt, p->old_majflt, p->majflt);
			if(p->cminflt - p->old_cminflt > 15000) fprintf(stderr, "BIG CHANGE: %d %s cminflt increased by %llu from %llu to %llu\n", p->pid, p->comm, p->cminflt - p->old_cminflt, p->old_cminflt, p->cminflt);
			if(p->cmajflt - p->old_cmajflt > 15000) fprintf(stderr, "BIG CHANGE: %d %s cmajflt increased by %llu from %llu to %llu\n", p->pid, p->comm, p->cmajflt - p->old_cmajflt, p->old_cmajflt, p->cmajflt);
*/
#ifdef INCLUDE_CHILDS
			p->old_utime = p->utime;
			p->old_cutime = p->cutime;
			p->old_stime = p->stime;
			p->old_cstime = p->cstime;
			p->old_minflt = p->minflt;
			p->old_majflt = p->majflt;
			p->old_cminflt = p->cminflt;
			p->old_cmajflt = p->cmajflt;
#endif
		}
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

//	fprintf(stderr, "\n");
	// cleanup all un-updated processed (exited, killed, etc)
	for(p = root_of_pids; p ;) {
		if(!p->updated) {
//			fprintf(stderr, "\tEXITED %d %s [parent %d %s, target %s] utime=%llu, stime=%llu, cutime=%llu, cstime=%llu, minflt=%llu, majflt=%llu, cminflt=%llu, cmajflt=%llu\n", p->pid, p->comm, p->parent->pid, p->parent->comm, p->target->name,  p->utime, p->stime, p->cutime, p->cstime, p->minflt, p->majflt, p->cminflt, p->cmajflt);

			for(c = 0 ; c < p->fds_size ; c++) if(p->fds[c] > 0) {
				file_descriptor_not_used(p->fds[c]);
				p->fds[c] = 0;
			}

			pid_t r = p->pid;
			p = p->next;
			del_pid_entry(r);
		}
		else p = p->next;
	}

	for (w = target_root; w ; w = w->next) {
		w->openfiles = 0;
		w->openpipes = 0;
		w->opensockets = 0;
		w->openinotifies = 0;
		w->openeventfds = 0;
		w->opentimerfds = 0;
		w->opensignalfds = 0;
		w->openeventpolls = 0;
		w->openother = 0;

		for(c = 1; c < all_files_size ;c++) {
			if(w->fds && w->fds[c] > 0) switch(all_files[c].type) {
				case FILETYPE_FILE:
					w->openfiles++;
					break;

				case FILETYPE_PIPE:
					w->openpipes++;
					break;

				case FILETYPE_SOCKET:
					w->opensockets++;
					break;

				case FILETYPE_INOTIFY:
					w->openinotifies++;
					break;

				case FILETYPE_EVENTFD:
					w->openeventfds++;
					break;

				case FILETYPE_TIMERFD:
					w->opentimerfds++;
					break;

				case FILETYPE_SIGNALFD:
					w->opensignalfds++;
					break;

				case FILETYPE_EVENTPOLL:
					w->openeventpolls++;
					break;

				default:
					w->openother++;
			}
		}

		free(w->fds);
		w->fds = NULL;
	}
}

// ----------------------------------------------------------------------------
// update chart dimensions

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

	fprintf(stdout, "BEGIN apps.files %llu\n", usec);
	for (w = target_root; w ; w = w->next) {
		if(w->target || (!w->processes && !w->exposed)) continue;

		fprintf(stdout, "SET %s = %llu\n", w->name, w->openfiles);
	}
	fprintf(stdout, "END\n");

	fprintf(stdout, "BEGIN apps.sockets %llu\n", usec);
	for (w = target_root; w ; w = w->next) {
		if(w->target || (!w->processes && !w->exposed)) continue;

		fprintf(stdout, "SET %s = %llu\n", w->name, w->opensockets);
	}
	fprintf(stdout, "END\n");

	fprintf(stdout, "BEGIN apps.pipes %llu\n", usec);
	for (w = target_root; w ; w = w->next) {
		if(w->target || (!w->processes && !w->exposed)) continue;

		fprintf(stdout, "SET %s = %llu\n", w->name, w->openpipes);
	}
	fprintf(stdout, "END\n");

	fprintf(stdout, "BEGIN netdata.apps_cpu %llu\n", usec);
	fprintf(stdout, "SET user = %llu\n", cpuuser);
	fprintf(stdout, "SET system = %llu\n", cpusyst);
	fprintf(stdout, "END\n");

	fprintf(stdout, "BEGIN netdata.apps_files %llu\n", usec);
	fprintf(stdout, "SET files = %llu\n", file_counter);
	fprintf(stdout, "SET pids = %ld\n", all_pids_count);
	fprintf(stdout, "SET fds = %d\n", all_files_len);
	fprintf(stdout, "SET targets = %ld\n", targets);
	fprintf(stdout, "END\n");

	fflush(stdout);
}


// ----------------------------------------------------------------------------
// generate the charts

void show_charts(void)
{
	struct target *w;
	int newly_added = 0;

	for(w = target_root ; w ; w = w->next)
		if(!w->exposed && w->processes) {
			newly_added++;
			w->exposed = 1;
			if(debug || w->debug) fprintf(stderr, "apps.plugin: %s just added - regenerating charts.\n", w->name);
		}

	// nothing more to show
	if(!newly_added) return;

	// we have something new to show
	// update the charts
	fprintf(stdout, "CHART apps.cpu '' 'Apps CPU Time (%ld%% = %ld core%s)' 'cpu time %%' cpu apps.cpu stacked 20001 %d\n", (processors * 100), processors, (processors>1)?"s":"", update_every);
	for (w = target_root; w ; w = w->next) {
		if(w->target || (!w->processes && !w->exposed)) continue;

		fprintf(stdout, "DIMENSION %s '' incremental 100 %llu %s\n", w->name, Hertz, w->hidden ? "hidden,noreset" : "noreset");
	}

	fprintf(stdout, "CHART apps.mem '' 'Apps Dedicated Memory (w/o shared)' 'MB' mem apps.mem stacked 20003 %d\n", update_every);
	for (w = target_root; w ; w = w->next) {
		if(w->target || (!w->processes && !w->exposed)) continue;

		fprintf(stdout, "DIMENSION %s '' absolute %ld %ld noreset\n", w->name, sysconf(_SC_PAGESIZE), 1024L*1024L);
	}

	fprintf(stdout, "CHART apps.threads '' 'Apps Threads' 'threads' processes apps.threads stacked 20005 %d\n", update_every);
	for (w = target_root; w ; w = w->next) {
		if(w->target || (!w->processes && !w->exposed)) continue;

		fprintf(stdout, "DIMENSION %s '' absolute 1 1 noreset\n", w->name);
	}

	fprintf(stdout, "CHART apps.processes '' 'Apps Processes' 'processes' processes apps.processes stacked 20004 %d\n", update_every);
	for (w = target_root; w ; w = w->next) {
		if(w->target || (!w->processes && !w->exposed)) continue;

		fprintf(stdout, "DIMENSION %s '' absolute 1 1 noreset\n", w->name);
	}

	fprintf(stdout, "CHART apps.cpu_user '' 'Apps CPU User Time (%ld%% = %ld core%s)' 'cpu time %%' cpu apps.cpu_user stacked 20020 %d\n", (processors * 100), processors, (processors>1)?"s":"", update_every);
	for (w = target_root; w ; w = w->next) {
		if(w->target || (!w->processes && !w->exposed)) continue;

		fprintf(stdout, "DIMENSION %s '' incremental 100 %llu noreset\n", w->name, Hertz * processors);
	}

	fprintf(stdout, "CHART apps.cpu_system '' 'Apps CPU System Time (%ld%% = %ld core%s)' 'cpu time %%' cpu apps.cpu_system stacked 20021 %d\n", (processors * 100), processors, (processors>1)?"s":"", update_every);
	for (w = target_root; w ; w = w->next) {
		if(w->target || (!w->processes && !w->exposed)) continue;

		fprintf(stdout, "DIMENSION %s '' incremental 100 %llu noreset\n", w->name, Hertz * processors);
	}

	fprintf(stdout, "CHART apps.major_faults '' 'Apps Major Page Faults (swap read)' 'page faults/s' swap apps.major_faults stacked 20010 %d\n", update_every);
	for (w = target_root; w ; w = w->next) {
		if(w->target || (!w->processes && !w->exposed)) continue;

		fprintf(stdout, "DIMENSION %s '' incremental 1 1 noreset\n", w->name);
	}

	fprintf(stdout, "CHART apps.minor_faults '' 'Apps Minor Page Faults' 'page faults/s' mem apps.minor_faults stacked 20011 %d\n", update_every);
	for (w = target_root; w ; w = w->next) {
		if(w->target || (!w->processes && !w->exposed)) continue;

		fprintf(stdout, "DIMENSION %s '' incremental 1 1 noreset\n", w->name);
	}

	fprintf(stdout, "CHART apps.lreads '' 'Apps Disk Logical Reads' 'kilobytes/s' disk apps.lreads stacked 20042 %d\n", update_every);
	for (w = target_root; w ; w = w->next) {
		if(w->target || (!w->processes && !w->exposed)) continue;

		fprintf(stdout, "DIMENSION %s '' incremental 1 %d noreset\n", w->name, 1024);
	}

	fprintf(stdout, "CHART apps.lwrites '' 'Apps I/O Logical Writes' 'kilobytes/s' disk apps.lwrites stacked 20042 %d\n", update_every);
	for (w = target_root; w ; w = w->next) {
		if(w->target || (!w->processes && !w->exposed)) continue;

		fprintf(stdout, "DIMENSION %s '' incremental 1 %d noreset\n", w->name, 1024);
	}

	fprintf(stdout, "CHART apps.preads '' 'Apps Disk Reads' 'kilobytes/s' disk apps.preads stacked 20002 %d\n", update_every);
	for (w = target_root; w ; w = w->next) {
		if(w->target || (!w->processes && !w->exposed)) continue;

		fprintf(stdout, "DIMENSION %s '' incremental 1 %d noreset\n", w->name, 1024);
	}

	fprintf(stdout, "CHART apps.pwrites '' 'Apps Disk Writes' 'kilobytes/s' disk apps.pwrites stacked 20002 %d\n", update_every);
	for (w = target_root; w ; w = w->next) {
		if(w->target || (!w->processes && !w->exposed)) continue;

		fprintf(stdout, "DIMENSION %s '' incremental 1 %d noreset\n", w->name, 1024);
	}

	fprintf(stdout, "CHART apps.files '' 'Apps Open Files' 'open files' disk apps.files stacked 20050 %d\n", update_every);
	for (w = target_root; w ; w = w->next) {
		if(w->target || (!w->processes && !w->exposed)) continue;

		fprintf(stdout, "DIMENSION %s '' absolute 1 1 noreset\n", w->name);
	}

	fprintf(stdout, "CHART apps.sockets '' 'Apps Open Sockets' 'open sockets' net apps.sockets stacked 20051 %d\n", update_every);
	for (w = target_root; w ; w = w->next) {
		if(w->target || (!w->processes && !w->exposed)) continue;

		fprintf(stdout, "DIMENSION %s '' absolute 1 1 noreset\n", w->name);
	}

	fprintf(stdout, "CHART apps.pipes '' 'Apps Pipes' 'open pipes' processes apps.pipes stacked 20053 %d\n", update_every);
	for (w = target_root; w ; w = w->next) {
		if(w->target || (!w->processes && !w->exposed)) continue;

		fprintf(stdout, "DIMENSION %s '' absolute 1 1 noreset\n", w->name);
	}

	fprintf(stdout, "CHART netdata.apps_cpu '' 'Apps Plugin CPU' 'milliseconds/s' apps.plugin netdata.apps_cpu stacked 140000 %d\n", update_every);
	fprintf(stdout, "DIMENSION user '' incremental 1 %d\n", 1000);
	fprintf(stdout, "DIMENSION system '' incremental 1 %d\n", 1000);

	fprintf(stdout, "CHART netdata.apps_files '' 'Apps Plugin Files' 'files/s' apps.plugin netdata.apps_files line 140001 %d\n", update_every);
	fprintf(stdout, "DIMENSION files '' incremental 1 1\n");
	fprintf(stdout, "DIMENSION pids '' absolute 1 1\n");
	fprintf(stdout, "DIMENSION fds '' absolute 1 1\n");
	fprintf(stdout, "DIMENSION targets '' absolute 1 1\n");

	fflush(stdout);
}


// ----------------------------------------------------------------------------
// parse command line arguments

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
			debug_flags = 0xffffffff;
			continue;
		}

		if(!name) {
			name = argv[i];
			continue;
		}

		error("Cannot understand option %s", argv[i]);
		exit(1);
	}

	if(freq > 0) update_every = freq;
	if(!name) name = "groups";

	if(read_process_groups(name)) {
		error("Cannot read process groups %s", name);
		exit(1);
	}
}

int main(int argc, char **argv)
{
	// debug_flags = D_PROCFILE;

	// set the name for logging
	program_name = "apps.plugin";

	host_prefix = getenv("NETDATA_HOST_PREFIX");
	if(host_prefix == NULL) {
		info("NETDATA_HOST_PREFIX is not passed from netdata");
		host_prefix = "";
	}
	else info("Found NETDATA_HOST_PREFIX='%s'", host_prefix);

	info("starting...");

	procfile_adaptive_initial_allocation = 1;

	time_t started_t = time(NULL);
	time_t current_t;
	Hertz = get_hertz();
	pid_max = get_pid_max();
	processors = get_processors();

	parse_args(argc, argv);

	all_pids = calloc(sizeof(struct pid_stat *), (size_t) pid_max);
	if(!all_pids) {
		error("Cannot allocate %lu bytes of memory.", sizeof(struct pid_stat *) * pid_max);
		printf("DISABLE\n");
		exit(1);
	}

	unsigned long long counter = 1;
	unsigned long long usec = 0, susec = 0;
	struct timeval last, now;
	gettimeofday(&last, NULL);

	for(;1; counter++) {
		if(!update_from_proc()) {
			error("Cannot allocate %lu bytes of memory.", sizeof(struct pid_stat *) * pid_max);
			printf("DISABLE\n");
			exit(1);
		}

		update_statistics();
		show_charts();		// this is smart enough to show only newly added apps, when needed
		show_dimensions();

		if(debug) fprintf(stderr, "apps.plugin: done Loop No %llu\n", counter);
		fflush(NULL);

		gettimeofday(&now, NULL);
		usec = usecdiff(&now, &last) - susec;
		if(debug) fprintf(stderr, "apps.plugin: last loop took %llu usec (worked for %llu, sleeped for %llu).\n", usec + susec, usec, susec);

		// if the last loop took less than half the time
		// wait the rest of the time
		if(usec < (update_every * 1000000ULL / 2)) susec = (update_every * 1000000ULL) - usec;
		else susec = update_every * 1000000ULL / 2;

		usleep((useconds_t) susec);
		bcopy(&now, &last, sizeof(struct timeval));

		// restart once per day (14400 seconds)
		current_t = time(NULL);
		if(current_t - started_t > 14400) exit(0);
	}
}
