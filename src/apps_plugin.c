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
#include <sys/stat.h>

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

#include <sys/types.h>
#include <pwd.h>
#include <grp.h>

#include "avl.h"

#include "common.h"
#include "log.h"
#include "procfile.h"
#include "../config.h"

#ifdef NETDATA_INTERNAL_CHECKS
#include <sys/prctl.h>
#endif

#define MAX_COMPARE_NAME 100
#define MAX_NAME 100
#define MAX_CMDLINE 1024

long processors = 1;
long pid_max = 32768;
int debug = 0;

int update_every = 1;
unsigned long long file_counter = 0;
int proc_pid_cmdline_is_needed = 0;

char *host_prefix = "";
char *config_dir = CONFIG_DIR;

#ifdef NETDATA_INTERNAL_CHECKS
// ----------------------------------------------------------------------------
// memory debugger
// do not use in production systems - it mis-aligns allocated memory

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

#endif /* NETDATA_INTERNAL_CHECKS */

// ----------------------------------------------------------------------------
// system functions
// to retrieve settings of the system

long get_system_cpus(void) {
	procfile *ff = NULL;

	int processors = 0;

	char filename[FILENAME_MAX + 1];
	snprintfz(filename, FILENAME_MAX, "%s/proc/stat", host_prefix);

	ff = procfile_open(filename, NULL, PROCFILE_FLAG_DEFAULT);
	if(!ff) return 1;

	ff = procfile_readall(ff);
	if(!ff) {
		procfile_close(ff);
		return 1;
	}

	unsigned int i;
	for(i = 0; i < procfile_lines(ff); i++) {
		if(!procfile_linewords(ff, i)) continue;

		if(strncmp(procfile_lineword(ff, i, 0), "cpu", 3) == 0) processors++;
	}
	processors--;
	if(processors < 1) processors = 1;

	procfile_close(ff);
	return processors;
}

long get_system_pid_max(void) {
	procfile *ff = NULL;
	long mpid = 32768;

	char filename[FILENAME_MAX + 1];
	snprintfz(filename, FILENAME_MAX, "%s/proc/sys/kernel/pid_max", host_prefix);
	ff = procfile_open(filename, NULL, PROCFILE_FLAG_DEFAULT);
	if(!ff) return mpid;

	ff = procfile_readall(ff);
	if(!ff) {
		procfile_close(ff);
		return mpid;
	}

	mpid = atol(procfile_lineword(ff, 0, 0));
	if(!mpid) mpid = 32768;

	procfile_close(ff);
	return mpid;
}

// ----------------------------------------------------------------------------
// target
// target is the structure that process data are aggregated

struct target {
	char compare[MAX_COMPARE_NAME + 1];
	uint32_t comparehash;
	size_t comparelen;

	char id[MAX_NAME + 1];
	uint32_t idhash;

	char name[MAX_NAME + 1];

	uid_t uid;
	gid_t gid;

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
	int ends_with;
	int starts_with;            // if set, the compare string matches only the
								// beginning of the command

	struct target *target;		// the one that will be reported to netdata
	struct target *next;
};


// ----------------------------------------------------------------------------
// apps_groups.conf
// aggregate all processes in groups, to have a limited number of dimensions

struct target *apps_groups_root_target = NULL;
struct target *apps_groups_default_target = NULL;
long apps_groups_targets = 0;

struct target *users_root_target = NULL;
struct target *groups_root_target = NULL;

struct target *get_users_target(uid_t uid)
{
	struct target *w;
	for(w = users_root_target ; w ; w = w->next)
		if(w->uid == uid) return w;

	w = calloc(sizeof(struct target), 1);
	if(unlikely(!w)) {
		error("Cannot allocate %lu bytes of memory", (unsigned long)sizeof(struct target));
		return NULL;
	}

	snprintfz(w->compare, MAX_COMPARE_NAME, "%d", uid);
	w->comparehash = simple_hash(w->compare);
	w->comparelen = strlen(w->compare);

	snprintfz(w->id, MAX_NAME, "%d", uid);
	w->idhash = simple_hash(w->id);

	struct passwd *pw = getpwuid(uid);
	if(!pw)
		snprintfz(w->name, MAX_NAME, "%d", uid);
	else
		snprintfz(w->name, MAX_NAME, "%s", pw->pw_name);

	netdata_fix_chart_name(w->name);

	w->uid = uid;

	w->next = users_root_target;
	users_root_target = w;

	if(unlikely(debug))
		fprintf(stderr, "apps.plugin: added uid %d ('%s') target\n", w->uid, w->name);

	return w;
}

struct target *get_groups_target(gid_t gid)
{
	struct target *w;
	for(w = groups_root_target ; w ; w = w->next)
		if(w->gid == gid) return w;

	w = calloc(sizeof(struct target), 1);
	if(unlikely(!w)) {
		error("Cannot allocate %lu bytes of memory", (unsigned long)sizeof(struct target));
		return NULL;
	}

	snprintfz(w->compare, MAX_COMPARE_NAME, "%d", gid);
	w->comparehash = simple_hash(w->compare);
	w->comparelen = strlen(w->compare);

	snprintfz(w->id, MAX_NAME, "%d", gid);
	w->idhash = simple_hash(w->id);

	struct group *gr = getgrgid(gid);
	if(!gr)
		snprintfz(w->name, MAX_NAME, "%d", gid);
	else
		snprintfz(w->name, MAX_NAME, "%s", gr->gr_name);

	netdata_fix_chart_name(w->name);

	w->gid = gid;

	w->next = groups_root_target;
	groups_root_target = w;

	if(unlikely(debug))
		fprintf(stderr, "apps.plugin: added gid %d ('%s') target\n", w->gid, w->name);

	return w;
}

// find or create a new target
// there are targets that are just aggregated to other target (the second argument)
struct target *get_apps_groups_target(const char *id, struct target *target)
{
	int tdebug = 0, thidden = 0, ends_with = 0;
	const char *nid = id;

	while(nid[0] == '-' || nid[0] == '+' || nid[0] == '*') {
		if(nid[0] == '-') thidden = 1;
		if(nid[0] == '+') tdebug = 1;
		if(nid[0] == '*') ends_with = 1;
		nid++;
	}
	uint32_t hash = simple_hash(id);

	struct target *w;
	for(w = apps_groups_root_target ; w ; w = w->next) {
		if(w->idhash == hash && strncmp(nid, w->id, MAX_NAME) == 0)
			return w;
	}

	w = calloc(sizeof(struct target), 1);
	if(unlikely(!w)) {
		error("Cannot allocate %lu bytes of memory", (unsigned long)sizeof(struct target));
		return NULL;
	}

	strncpyz(w->id, nid, MAX_NAME);
	w->idhash = simple_hash(w->id);

	strncpyz(w->name, nid, MAX_NAME);

	strncpyz(w->compare, nid, MAX_COMPARE_NAME);
	int len = strlen(w->compare);
	if(w->compare[len - 1] == '*') {
		w->compare[len - 1] = '\0';
		w->starts_with = 1;
	}
	w->ends_with = ends_with;

	if(w->starts_with && w->ends_with)
		proc_pid_cmdline_is_needed = 1;

	w->comparehash = simple_hash(w->compare);
	w->comparelen = strlen(w->compare);

	w->hidden = thidden;
	w->debug = tdebug;
	w->target = target;

	w->next = apps_groups_root_target;
	apps_groups_root_target = w;

	if(unlikely(debug))
		fprintf(stderr, "apps.plugin: ADDING TARGET ID '%s', process name '%s' (%s), aggregated on target '%s', options: %s %s\n"
		        , w->id
				, w->compare, (w->starts_with && w->ends_with)?"substring":((w->starts_with)?"prefix":((w->ends_with)?"suffix":"exact"))
				, w->target?w->target->id:w->id
				, (w->hidden)?"hidden":"-"
				, (w->debug)?"debug":"-"
		);

	return w;
}

// read the apps_groups.conf file
int read_apps_groups_conf(const char *name)
{
	char filename[FILENAME_MAX + 1];

	snprintfz(filename, FILENAME_MAX, "%s/apps_%s.conf", config_dir, name);

	if(unlikely(debug))
		fprintf(stderr, "apps.plugin: process groups file: '%s'\n", filename);

	// ----------------------------------------

	procfile *ff = procfile_open(filename, " :\t", PROCFILE_FLAG_DEFAULT);
	if(!ff) return 1;

	procfile_set_quotes(ff, "'\"");

	ff = procfile_readall(ff);
	if(!ff) {
		procfile_close(ff);
		return 1;
	}

	unsigned long line, lines = procfile_lines(ff);

	for(line = 0; line < lines ;line++) {
		unsigned long word, words = procfile_linewords(ff, line);
		struct target *w = NULL;

		char *t = procfile_lineword(ff, line, 0);
		if(!t || !*t) continue;

		for(word = 0; word < words ;word++) {
			char *s = procfile_lineword(ff, line, word);
			if(!s || !*s) continue;
			if(*s == '#') break;

			if(t == s) continue;

			struct target *n = get_apps_groups_target(s, w);
			if(!n) {
				error("Cannot create target '%s' (line %d, word %d)", s, line, word);
				continue;
			}

			if(!w) w = n;
		}

		if(w) {
			int tdebug = 0, thidden = 0;

			while(t[0] == '-' || t[0] == '+') {
				if(t[0] == '-') thidden = 1;
				if(t[0] == '+') tdebug = 1;
				t++;
			}

			strncpyz(w->name, t, MAX_NAME);
			w->hidden = thidden;
			w->debug = tdebug;

			if(unlikely(debug))
				fprintf(stderr, "apps.plugin: AGGREGATION TARGET NAME '%s' on ID '%s', process name '%s' (%s), aggregated on target '%s', options: %s %s\n"
						, w->name
						, w->id
						, w->compare, (w->starts_with && w->ends_with)?"substring":((w->starts_with)?"prefix":((w->ends_with)?"suffix":"exact"))
						, w->target?w->target->id:w->id
						, (w->hidden)?"hidden":"-"
						, (w->debug)?"debug":"-"
				);
		}
	}

	procfile_close(ff);

	apps_groups_default_target = get_apps_groups_target("p+!o@w#e$i^r&7*5(-i)l-o_", NULL); // match nothing
	if(!apps_groups_default_target)
		error("Cannot create default target");
	else
		strncpyz(apps_groups_default_target->name, "other", MAX_NAME);

	return 0;
}


// ----------------------------------------------------------------------------
// data to store for each pid
// see: man proc

struct pid_stat {
	int32_t pid;
	char comm[MAX_COMPARE_NAME + 1];
	char cmdline[MAX_CMDLINE + 1];

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

	uid_t uid;
	gid_t gid;

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

	// we need the last values
	// for all incremental counters
	// so that when a process switches users/groups
	// we will subtract these values from the old
	// target
	unsigned long long last_minflt;
	unsigned long long last_cminflt;
	unsigned long long last_majflt;
	unsigned long long last_cmajflt;
	unsigned long long last_utime;
	unsigned long long last_stime;
	unsigned long long last_cutime;
	unsigned long long last_cstime;

	unsigned long long last_io_logical_bytes_read;
	unsigned long long last_io_logical_bytes_written;
	unsigned long long last_io_read_calls;
	unsigned long long last_io_write_calls;
	unsigned long long last_io_storage_bytes_read;
	unsigned long long last_io_storage_bytes_written;
	unsigned long long last_io_cancelled_write_bytes;

#ifdef AGGREGATE_CHILDREN_TO_PARENTS
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
#endif /* AGGREGATE_CHILDREN_TO_PARENTS */

	int *fds;						// array of fds it uses
	int fds_size;					// the size of the fds array

	int children_count;				// number of processes directly referencing this
	int updated;					// 1 when update
	int merged;						// 1 when it has been merged to its parent
	int new_entry;

	struct target *target;			// app_groups.conf targets
	struct target *user_target;		// uid based targets
	struct target *group_target;	// gid based targets

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

int read_proc_pid_cmdline(struct pid_stat *p) {
	char filename[FILENAME_MAX + 1];
	snprintfz(filename, FILENAME_MAX, "%s/proc/%d/cmdline", host_prefix, p->pid);

	int fd = open(filename, O_RDONLY, 0666);
	if(unlikely(fd == -1)) return 1;

	int i, bytes = read(fd, p->cmdline, MAX_CMDLINE);
	close(fd);

	if(bytes <= 0) {
		// copy the command to the command line
		strncpyz(p->cmdline, p->comm, MAX_CMDLINE);
		return 0;
	}

	p->cmdline[bytes] = '\0';
	for(i = 0; i < bytes ; i++)
		if(!p->cmdline[i]) p->cmdline[i] = ' ';

	if(unlikely(debug))
		fprintf(stderr, "Read file '%s' contents: %s\n", filename, p->cmdline);

	return 0;
}

int read_proc_pid_ownership(struct pid_stat *p) {
	char filename[FILENAME_MAX + 1];

	snprintfz(filename, FILENAME_MAX, "%s/proc/%d", host_prefix, p->pid);

	// ----------------------------------------
	// read uid and gid

	struct stat st;
	if(stat(filename, &st) != 0)
		return 1;

	p->uid = st.st_uid;
	p->gid = st.st_gid;

	return 0;
}

int read_proc_pid_stat(struct pid_stat *p) {
	static procfile *ff = NULL;

	char filename[FILENAME_MAX + 1];

	snprintfz(filename, FILENAME_MAX, "%s/proc/%d/stat", host_prefix, p->pid);

	// ----------------------------------------

	int set_quotes = (!ff)?1:0;

	ff = procfile_reopen(ff, filename, NULL, PROCFILE_FLAG_NO_ERROR_ON_FILE_IO);
	if(!ff) return 1;

	// if(set_quotes) procfile_set_quotes(ff, "()");
	if(set_quotes) procfile_set_open_close(ff, "(", ")");

	ff = procfile_readall(ff);
	if(!ff) {
		// procfile_close(ff);
		return 1;
	}

	file_counter++;

	// parse the process name
	unsigned int i = 0;
	strncpyz(p->comm, procfile_lineword(ff, 0, 1), MAX_COMPARE_NAME);

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

	if(debug || (p->target && p->target->debug))
		fprintf(stderr, "apps.plugin: READ PROC/PID/STAT: %s/proc/%d/stat, process: '%s' VALUES: utime=%llu, stime=%llu, cutime=%llu, cstime=%llu, minflt=%llu, majflt=%llu, cminflt=%llu, cmajflt=%llu, threads=%d\n", host_prefix, p->pid, p->comm, p->utime, p->stime, p->cutime, p->cstime, p->minflt, p->majflt, p->cminflt, p->cmajflt, p->num_threads);

	// procfile_close(ff);
	return 0;
}

int read_proc_pid_statm(struct pid_stat *p) {
	static procfile *ff = NULL;

	char filename[FILENAME_MAX + 1];

	snprintfz(filename, FILENAME_MAX, "%s/proc/%d/statm", host_prefix, p->pid);

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
	static procfile *ff = NULL;

	char filename[FILENAME_MAX + 1];

	snprintfz(filename, FILENAME_MAX, "%s/proc/%d/io", host_prefix, p->pid);

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
// file descriptor
// this is used to keep a global list of all open files of the system
// it is needed in order to calculate the unique files processes have open

#define FILE_DESCRIPTORS_INCREASE_STEP 100

struct file_descriptor {
	avl avl;
#ifdef NETDATA_INTERNAL_CHECKS
	uint32_t magic;
#endif /* NETDATA_INTERNAL_CHECKS */
	uint32_t hash;
	const char *name;
	int type;
	int count;
	int pos;
} *all_files = NULL;

int all_files_len = 0;
int all_files_size = 0;

int file_descriptor_compare(void* a, void* b) {
#ifdef NETDATA_INTERNAL_CHECKS
	if(((struct file_descriptor *)a)->magic != 0x0BADCAFE || ((struct file_descriptor *)b)->magic != 0x0BADCAFE)
		error("Corrupted index data detected. Please report this.");
#endif /* NETDATA_INTERNAL_CHECKS */

	if(((struct file_descriptor *)a)->hash < ((struct file_descriptor *)b)->hash)
		return -1;

	else if(((struct file_descriptor *)a)->hash > ((struct file_descriptor *)b)->hash)
		return 1;

	else
		return strcmp(((struct file_descriptor *)a)->name, ((struct file_descriptor *)b)->name);
}

int file_descriptor_iterator(avl *a) { if(a) {}; return 0; }

avl_tree all_files_index = {
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

	return (struct file_descriptor *)avl_search(&all_files_index, (avl *) &tmp);
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

#ifdef NETDATA_INTERNAL_CHECKS
		if(all_files[id].magic != 0x0BADCAFE) {
			error("Ignoring request to remove empty file id %d.", id);
			return;
		}
#endif /* NETDATA_INTERNAL_CHECKS */

		if(debug) fprintf(stderr, "apps.plugin: decreasing slot %d (count = %d).\n", id, all_files[id].count);

		if(all_files[id].count > 0) {
			all_files[id].count--;

			if(!all_files[id].count) {
				if(debug) fprintf(stderr, "apps.plugin:   >> slot %d is empty.\n", id);
				file_descriptor_remove(&all_files[id]);
#ifdef NETDATA_INTERNAL_CHECKS
				all_files[id].magic = 0x00000000;
#endif /* NETDATA_INTERNAL_CHECKS */
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
#ifdef NETDATA_INTERNAL_CHECKS
			all_files[i].magic = 0x00000000;
#endif /* NETDATA_INTERNAL_CHECKS */
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

#ifdef NETDATA_INTERNAL_CHECKS
			if(all_files[c].magic == 0x0BADCAFE && all_files[c].name && file_descriptor_find(all_files[c].name, all_files[c].hash))
				error("fd on position %d is not cleared properly. It still has %s in it.\n", c, all_files[c].name);
#endif /* NETDATA_INTERNAL_CHECKS */

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
#ifdef NETDATA_INTERNAL_CHECKS
	all_files[c].magic = 0x0BADCAFE;
#endif /* NETDATA_INTERNAL_CHECKS */
	file_descriptor_add(&all_files[c]);

	if(debug) fprintf(stderr, "apps.plugin: using fd position %d (name: %s)\n", c, all_files[c].name);

	return c;
}

int read_pid_file_descriptors(struct pid_stat *p) {
	char dirname[FILENAME_MAX+1];

	snprintfz(dirname, FILENAME_MAX, "%s/proc/%d/fd", host_prefix, p->pid);
	DIR *fds = opendir(dirname);
	if(fds) {
		int c;
		struct dirent *de;
		char fdname[FILENAME_MAX + 1];
		char linkname[FILENAME_MAX + 1];

		// make the array negative
		for(c = 0 ; c < p->fds_size ; c++)
			p->fds[c] = -p->fds[c];

		while((de = readdir(fds))) {
			if(strcmp(de->d_name, ".") == 0 || strcmp(de->d_name, "..") == 0)
				continue;

			// check if the fds array is small
			int fdid = atoi(de->d_name);
			if(fdid < 0) continue;
			if(fdid >= p->fds_size) {
				// it is small, extend it
				if(debug) fprintf(stderr, "apps.plugin: extending fd memory slots for %s from %d to %d\n", p->comm, p->fds_size, fdid + 100);
				p->fds = realloc(p->fds, (fdid + 100) * sizeof(int));
				if(!p->fds) {
					fatal("Cannot re-allocate fds for %s", p->comm);
					break;
				}

				// and initialize it
				for(c = p->fds_size ; c < (fdid + 100) ; c++) p->fds[c] = 0;
				p->fds_size = fdid + 100;
			}

			if(p->fds[fdid] == 0) {
				// we don't know this fd, get it

				sprintf(fdname, "%s/proc/%d/fd/%s", host_prefix, p->pid, de->d_name);
				ssize_t l = readlink(fdname, linkname, FILENAME_MAX);
				if(l == -1) {
					if(debug || (p->target && p->target->debug)) {
						if(debug || (p->target && p->target->debug))
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
	else return 1;

	return 0;
}

// ----------------------------------------------------------------------------

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

int collect_data_for_all_processes_from_proc(void)
{
	char dirname[FILENAME_MAX + 1];

	snprintfz(dirname, FILENAME_MAX, "%s/proc", host_prefix);
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
		p->children_count = 0;
		p->merged = 0;
		p->new_entry = 0;

        p->last_minflt  = p->minflt;
        p->last_cminflt  = p->cminflt;
        p->last_majflt  = p->majflt;
        p->last_cmajflt  = p->cmajflt;
        p->last_utime  = p->utime;
        p->last_stime  = p->stime;
        p->last_cutime  = p->cutime;
        p->last_cstime  = p->cstime;

        p->last_io_logical_bytes_read  = p->io_logical_bytes_read;
        p->last_io_logical_bytes_written  = p->io_logical_bytes_written;
        p->last_io_read_calls  = p->io_read_calls;
        p->last_io_write_calls  = p->io_write_calls;
        p->last_io_storage_bytes_read  = p->io_storage_bytes_read;
        p->last_io_storage_bytes_written  = p->io_storage_bytes_written;
        p->last_io_cancelled_write_bytes  = p->io_cancelled_write_bytes;
	}

	while((file = readdir(dir))) {
		char *endptr = file->d_name;
		pid_t pid = (pid_t) strtoul(file->d_name, &endptr, 10);

		// make sure we read a valid number
		if(unlikely(pid <= 0 || pid > pid_max || endptr == file->d_name || *endptr != '\0'))
			continue;

		p = get_pid_entry(pid);
		if(unlikely(!p)) continue;


		// --------------------------------------------------------------------
		// /proc/<pid>/stat

		if(unlikely(read_proc_pid_stat(p))) {
				error("Cannot process %s/proc/%d/stat", host_prefix, pid);

			// there is no reason to proceed if we cannot get its status
			continue;
		}

		// check its parent pid
		if(unlikely(p->ppid < 0 || p->ppid > pid_max)) {
				error("Pid %d states invalid parent pid %d. Using 0.", pid, p->ppid);

			p->ppid = 0;
		}

		// --------------------------------------------------------------------
		// /proc/<pid>/cmdline

		if(proc_pid_cmdline_is_needed) {
			if(unlikely(read_proc_pid_cmdline(p))) {
					error("Cannot process %s/proc/%d/cmdline", host_prefix, pid);
			}
		}

		// --------------------------------------------------------------------
		// /proc/<pid>/statm

		if(unlikely(read_proc_pid_statm(p))) {
				error("Cannot process %s/proc/%d/statm", host_prefix, pid);

			// there is no reason to proceed if we cannot get its memory status
			continue;
		}


		// --------------------------------------------------------------------
		// /proc/<pid>/io

		if(unlikely(read_proc_pid_io(p))) {
				error("Cannot process %s/proc/%d/io", host_prefix, pid);

			// on systems without /proc/X/io
			// allow proceeding without I/O information
			// continue;
		}

		// --------------------------------------------------------------------
		// <pid> ownership

		if(unlikely(read_proc_pid_ownership(p))) {
				error("Cannot stat %s/proc/%d", host_prefix, pid);
		}

		// --------------------------------------------------------------------
		// link it

		// check if it is target
		// we do this only once, the first time this pid is loaded
		if(unlikely(p->new_entry)) {
			if(debug) fprintf(stderr, "apps.plugin: \tJust added %s\n", p->comm);
			uint32_t hash = simple_hash(p->comm);
			size_t pclen = strlen(p->comm);

			struct target *w;
			for(w = apps_groups_root_target; w ; w = w->next) {
				// if(debug || (p->target && p->target->debug)) fprintf(stderr, "apps.plugin: \t\tcomparing '%s' with '%s'\n", w->compare, p->comm);

				// find it - 4 cases:
				// 1. the target is not a pattern
				// 2. the target has the prefix
				// 3. the target has the suffix
				// 4. the target is something inside cmdline
				if(	(!w->starts_with && !w->ends_with && w->comparehash == hash && !strcmp(w->compare, p->comm))
				       || (w->starts_with && !w->ends_with && !strncmp(w->compare, p->comm, w->comparelen))
				       || (!w->starts_with && w->ends_with && pclen >= w->comparelen && !strcmp(w->compare, &p->comm[pclen - w->comparelen]))
				       || (proc_pid_cmdline_is_needed && w->starts_with && w->ends_with && strstr(p->cmdline, w->compare))
						) {
					if(w->target) p->target = w->target;
					else p->target = w;

					if(debug || (p->target && p->target->debug))
						fprintf(stderr, "apps.plugin: \t\t%s linked to target %s\n", p->comm, p->target->name);
				}
			}
		}

		// --------------------------------------------------------------------
		// /proc/<pid>/fd

		if(unlikely(read_pid_file_descriptors(p))) {
				error("Cannot process entries in %s/proc/%d/fd", host_prefix, pid);
		}

		// --------------------------------------------------------------------
		// done!

		// mark it as updated
		p->updated = 1;
	}

	closedir(dir);

	return 1;
}


// ----------------------------------------------------------------------------

#ifdef AGGREGATE_CHILDREN_TO_PARENTS
// print a tree view of all processes
int debug_childrens_aggregations(pid_t pid, int level) {
	struct pid_stat *p = NULL;
	char b[level+3];
	int i, ret = 0;

	for(i = 0; i < level; i++) b[i] = '\t';
	b[level] = '|';
	b[level+1] = '-';
	b[level+2] = '\0';

	for(p = root_of_pids; p ; p = p->next) {
		if(p->ppid == pid) {
			ret += debug_childrens_aggregations(p->pid, level+1);
		}
	}

	p = all_pids[pid];
	if(p) {
		if(!p->updated) ret += 1;
		if(ret) fprintf(stderr, "%s %s %d [%s, %s] c=%d u=%llu+%llu, s=%llu+%llu, cu=%llu+%llu, cs=%llu+%llu, n=%llu+%llu, j=%llu+%llu, cn=%llu+%llu, cj=%llu+%llu\n"
			, b, p->comm, p->pid, p->updated?"OK":"KILLED", p->target->name, p->children_count
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
#endif /* AGGREGATE_CHILDREN_TO_PARENTS */



// ----------------------------------------------------------------------------
// update statistics on the targets

// 1. link all childs to their parents
// 2. go from bottom to top, marking as merged all childs to their parents
//    this step links all parents without a target to the child target, if any
// 3. link all top level processes (the ones not merged) to the default target
// 4. go from top to bottom, linking all childs without a target, to their parent target
//    after this step, all processes have a target
// [5. for each killed pid (updated = 0), remove its usage from its target]
// 6. zero all apps_groups_targets
// 7. concentrate all values on the apps_groups_targets
// 8. remove all killed processes
// 9. find the unique file count for each target
// check: update_apps_groups_statistics()

void link_all_processes_to_their_parents(void) {
	struct pid_stat *p = NULL;

	// link all children to their parents
	// and update children count on parents
	for(p = root_of_pids; p ; p = p->next) {
		// for each process found running

		if(p->ppid > 0
				&& p->ppid <= pid_max
				&& all_pids[p->ppid]
			) {
			// for valid processes

			if(debug || (p->target && p->target->debug))
				fprintf(stderr, "apps.plugin: \tparent of %d (%s) is %d (%s)\n", p->pid, p->comm, p->ppid, all_pids[p->ppid]->comm);

			p->parent = all_pids[p->ppid];
			p->parent->children_count++;
		}
		else if(p->ppid != 0)
			error("pid %d %s states parent %d, but the later does not exist.", p->pid, p->comm, p->ppid);
	}
}

#ifdef AGGREGATE_CHILDREN_TO_PARENTS
void aggregate_children_to_parents(void) {
	struct pid_stat *p = NULL;

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
}
#endif /* AGGREGATE_CHILDREN_TO_PARENTS */

void cleanup_non_existing_pids(void) {
	int c;
	struct pid_stat *p = NULL;

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
}

void apply_apps_groups_targets_inheritance(void) {
	struct pid_stat *p = NULL;

	// children that do not have a target
	// inherit their target from their parent
	int found = 1;
	while(found) {
		found = 0;
		for(p = root_of_pids; p ; p = p->next) {
			// if this process does not have a target
			// and it has a parent
			// and its parent has a target
			// then, set the parent's target to this process
			if(unlikely(!p->target && p->parent && p->parent->target)) {
				p->target = p->parent->target;
				found++;

				if(debug || (p->target && p->target->debug))
					fprintf(stderr, "apps.plugin: \t\tTARGET INHERITANCE: %s is inherited by %d (%s) from its parent %d (%s).\n", p->target->name, p->pid, p->comm, p->parent->pid, p->parent->comm);
			}
		}
	}


	// find all the procs with 0 childs and merge them to their parents
	// repeat, until nothing more can be done.
	found = 1;
	while(found) {
		found = 0;
		for(p = root_of_pids; p ; p = p->next) {
			// if this process does not have any children
			// and is not already merged
			// and has a parent
			// and its parent has children
			// and the target of this process and its parent is the same, or the parent does not have a target
			// and its parent is not init
			// then, mark them as merged.
			if(unlikely(
					!p->children_count
					&& !p->merged
					&& p->parent
					&& p->parent->children_count
					&& (p->target == p->parent->target || !p->parent->target)
					&& p->ppid != 1
				)) {
				p->parent->children_count--;
				p->merged = 1;

				// the parent inherits the child's target, if it does not have a target itself
				if(unlikely(p->target && !p->parent->target)) {
					p->parent->target = p->target;

					if(debug || (p->target && p->target->debug))
						fprintf(stderr, "apps.plugin: \t\tTARGET INHERITANCE: %s is inherited by %d (%s) from its child %d (%s).\n", p->target->name, p->parent->pid, p->parent->comm, p->pid, p->comm);
				}

				found++;
			}
		}

		if(debug)
			fprintf(stderr, "apps.plugin: merged %d processes\n", found);
	}

	// init goes always to default target
	if(all_pids[1])
		all_pids[1]->target = apps_groups_default_target;

	// give a default target on all top level processes
	for(p = root_of_pids; p ; p = p->next) {
		// if the process is not merged itself
		// then is is a top level process
		if(!p->merged && !p->target)
			p->target = apps_groups_default_target;

#ifdef AGGREGATE_CHILDREN_TO_PARENTS
		// by the way, update the diffs
		// will be used later for subtracting killed process times
		p->diff_cutime = p->utime - p->cutime;
		p->diff_cstime = p->stime - p->cstime;
		p->diff_cminflt = p->minflt - p->cminflt;
		p->diff_cmajflt = p->majflt - p->cmajflt;
#endif /* AGGREGATE_CHILDREN_TO_PARENTS */
	}

	// give a target to all merged child processes
	found = 1;
	while(found) {
		found = 0;
		for(p = root_of_pids; p ; p = p->next) {
			if(unlikely(!p->target && p->merged && p->parent && p->parent->target)) {
				p->target = p->parent->target;
				found++;

				if(debug || (p->target && p->target->debug))
					fprintf(stderr, "apps.plugin: \t\tTARGET INHERITANCE: %s is inherited by %d (%s) from its parent %d (%s) at phase 2.\n", p->target->name, p->pid, p->comm, p->parent->pid, p->parent->comm);
			}
		}
	}
}

long zero_all_targets(struct target *root) {
	struct target *w;
	long count = 0;

	for (w = root; w ; w = w->next) {
		count++;

		if(w->fds) free(w->fds);
		w->fds = NULL;

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

	return count;
}

void aggregate_pid_on_target(struct target *w, struct pid_stat *p, struct target *o) {
	if(unlikely(!w->fds)) {
		w->fds = calloc(sizeof(int), (size_t) all_files_size);
		if(unlikely(!w->fds))
			error("Cannot allocate memory for fds in %s", w->name);
	}

	if(likely(p->updated)) {
		w->cutime += p->cutime; // - p->fix_cutime;
		w->cstime += p->cstime; // - p->fix_cstime;
		w->cminflt += p->cminflt; // - p->fix_cminflt;
		w->cmajflt += p->cmajflt; // - p->fix_cmajflt;

		w->utime += p->utime; //+ (p->pid != 1)?(p->cutime - p->fix_cutime):0;
		w->stime += p->stime; //+ (p->pid != 1)?(p->cstime - p->fix_cstime):0;
		w->minflt += p->minflt; //+ (p->pid != 1)?(p->cminflt - p->fix_cminflt):0;
		w->majflt += p->majflt; //+ (p->pid != 1)?(p->cmajflt - p->fix_cmajflt):0;

		//if(p->num_threads < 0)
		//	error("Negative threads number for pid '%s' (%d): %d", p->comm, p->pid, p->num_threads);

		//if(p->num_threads > 10000)
		//	error("Excessive threads number for pid '%s' (%d): %d", p->comm, p->pid, p->num_threads);

		w->num_threads += p->num_threads;
		w->rss += p->rss;

		w->statm_size += p->statm_size;
		w->statm_resident += p->statm_resident;
		w->statm_share += p->statm_share;
		w->statm_text += p->statm_text;
		w->statm_lib += p->statm_lib;
		w->statm_data += p->statm_data;
		w->statm_dirty += p->statm_dirty;

		w->io_logical_bytes_read += p->io_logical_bytes_read;
		w->io_logical_bytes_written += p->io_logical_bytes_written;
		w->io_read_calls += p->io_read_calls;
		w->io_write_calls += p->io_write_calls;
		w->io_storage_bytes_read += p->io_storage_bytes_read;
		w->io_storage_bytes_written += p->io_storage_bytes_written;
		w->io_cancelled_write_bytes += p->io_cancelled_write_bytes;

		w->processes++;

		if(likely(w->fds)) {
			int c;
			for(c = 0; c < p->fds_size ;c++) {
				if(p->fds[c] == 0) continue;

				if(likely(p->fds[c] < all_files_size)) {
					if(w->fds) w->fds[p->fds[c]]++;
				}
				else
					error("Invalid fd number %d", p->fds[c]);
			}
		}

		if(unlikely(debug || w->debug))
			fprintf(stderr, "apps.plugin: \tAgregating %s pid %d on %s utime=%llu, stime=%llu, cutime=%llu, cstime=%llu, minflt=%llu, majflt=%llu, cminflt=%llu, cmajflt=%llu\n", p->comm, p->pid, w->name, p->utime, p->stime, p->cutime, p->cstime, p->minflt, p->majflt, p->cminflt, p->cmajflt);

/*		if(p->utime - p->old_utime > 100) fprintf(stderr, "BIG CHANGE: %d %s utime increased by %llu from %llu to %llu\n", p->pid, p->comm, p->utime - p->old_utime, p->old_utime, p->utime);
		if(p->cutime - p->old_cutime > 100) fprintf(stderr, "BIG CHANGE: %d %s cutime increased by %llu from %llu to %llu\n", p->pid, p->comm, p->cutime - p->old_cutime, p->old_cutime, p->cutime);
		if(p->stime - p->old_stime > 100) fprintf(stderr, "BIG CHANGE: %d %s stime increased by %llu from %llu to %llu\n", p->pid, p->comm, p->stime - p->old_stime, p->old_stime, p->stime);
		if(p->cstime - p->old_cstime > 100) fprintf(stderr, "BIG CHANGE: %d %s cstime increased by %llu from %llu to %llu\n", p->pid, p->comm, p->cstime - p->old_cstime, p->old_cstime, p->cstime);
		if(p->minflt - p->old_minflt > 5000) fprintf(stderr, "BIG CHANGE: %d %s minflt increased by %llu from %llu to %llu\n", p->pid, p->comm, p->minflt - p->old_minflt, p->old_minflt, p->minflt);
		if(p->majflt - p->old_majflt > 5000) fprintf(stderr, "BIG CHANGE: %d %s majflt increased by %llu from %llu to %llu\n", p->pid, p->comm, p->majflt - p->old_majflt, p->old_majflt, p->majflt);
		if(p->cminflt - p->old_cminflt > 15000) fprintf(stderr, "BIG CHANGE: %d %s cminflt increased by %llu from %llu to %llu\n", p->pid, p->comm, p->cminflt - p->old_cminflt, p->old_cminflt, p->cminflt);
		if(p->cmajflt - p->old_cmajflt > 15000) fprintf(stderr, "BIG CHANGE: %d %s cmajflt increased by %llu from %llu to %llu\n", p->pid, p->comm, p->cmajflt - p->old_cmajflt, p->old_cmajflt, p->cmajflt);
*/
#ifdef AGGREGATE_CHILDREN_TO_PARENTS
		p->old_utime = p->utime;
		p->old_cutime = p->cutime;
		p->old_stime = p->stime;
		p->old_cstime = p->cstime;
		p->old_minflt = p->minflt;
		p->old_majflt = p->majflt;
		p->old_cminflt = p->cminflt;
		p->old_cmajflt = p->cmajflt;
#endif /* AGGREGATE_CHILDREN_TO_PARENTS */

		if(o) {
			// since the process switched target
			// for all incremental values
			// we have to subtract its OLD values from the new target
			// and add its OLD values to the old target

			// IMPORTANT
			// We add/subtract the last/OLD values we added to the target

			w->fix_cutime -= p->last_cutime;
			w->fix_cstime -= p->last_cstime;
			w->fix_cminflt -= p->last_cminflt;
			w->fix_cmajflt -= p->last_cmajflt;

			w->fix_utime -= p->last_utime;
			w->fix_stime -= p->last_stime;
			w->fix_minflt -= p->last_minflt;
			w->fix_majflt -= p->last_majflt;


			w->fix_io_logical_bytes_read -= p->last_io_logical_bytes_read;
			w->fix_io_logical_bytes_written -= p->last_io_logical_bytes_written;
			w->fix_io_read_calls -= p->last_io_read_calls;
			w->fix_io_write_calls -= p->last_io_write_calls;
			w->fix_io_storage_bytes_read -= p->last_io_storage_bytes_read;
			w->fix_io_storage_bytes_written -= p->last_io_storage_bytes_written;
			w->fix_io_cancelled_write_bytes -= p->last_io_cancelled_write_bytes;

			// ---

			o->fix_cutime += p->last_cutime;
			o->fix_cstime += p->last_cstime;
			o->fix_cminflt += p->last_cminflt;
			o->fix_cmajflt += p->last_cmajflt;

			o->fix_utime += p->last_utime;
			o->fix_stime += p->last_stime;
			o->fix_minflt += p->last_minflt;
			o->fix_majflt += p->last_majflt;

			o->fix_io_logical_bytes_read += p->last_io_logical_bytes_read;
			o->fix_io_logical_bytes_written += p->last_io_logical_bytes_written;
			o->fix_io_read_calls += p->last_io_read_calls;
			o->fix_io_write_calls += p->last_io_write_calls;
			o->fix_io_storage_bytes_read += p->last_io_storage_bytes_read;
			o->fix_io_storage_bytes_written += p->last_io_storage_bytes_written;
			o->fix_io_cancelled_write_bytes += p->last_io_cancelled_write_bytes;
		}
	}
	else {
		// if(o) fprintf(stderr, "apps.plugin: \t\tpid %d (%s) is not updated by OLD target %s (%s) is present.\n", p->pid, p->comm, o->id, o->name);

		// since the process has exited, the user
		// will see a drop in our charts, because the incremental
		// values of this process will not be there

		// add them to the fix_* values and they will be added to
		// the reported values, so that the report goes steady
		w->fix_minflt += p->minflt;
		w->fix_majflt += p->majflt;
		w->fix_utime += p->utime;
		w->fix_stime += p->stime;
		w->fix_cminflt += p->cminflt;
		w->fix_cmajflt += p->cmajflt;
		w->fix_cutime += p->cutime;
		w->fix_cstime += p->cstime;

		w->fix_io_logical_bytes_read += p->io_logical_bytes_read;
		w->fix_io_logical_bytes_written += p->io_logical_bytes_written;
		w->fix_io_read_calls += p->io_read_calls;
		w->fix_io_write_calls += p->io_write_calls;
		w->fix_io_storage_bytes_read += p->io_storage_bytes_read;
		w->fix_io_storage_bytes_written += p->io_storage_bytes_written;
		w->fix_io_cancelled_write_bytes += p->io_cancelled_write_bytes;
	}

}

void count_targets_fds(struct target *root) {
	int c;
	struct target *w;

	for (w = root; w ; w = w->next) {
		if(!w->fds) continue;

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
			if(w->fds[c] > 0)
				switch(all_files[c].type) {
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

void calculate_netdata_statistics(void)
{
	link_all_processes_to_their_parents();
	apply_apps_groups_targets_inheritance();

#ifdef AGGREGATE_CHILDREN_TO_PARENTS
	aggregate_children_to_parents();
#endif /* AGGREGATE_CHILDREN_TO_PARENTS */

	zero_all_targets(users_root_target);
	zero_all_targets(groups_root_target);
	apps_groups_targets = zero_all_targets(apps_groups_root_target);

#ifdef AGGREGATE_CHILDREN_TO_PARENTS
	if(debug)
		debug_childrens_aggregations(0, 1);
#endif /* AGGREGATE_CHILDREN_TO_PARENTS */

	// this has to be done, before the cleanup
	struct pid_stat *p = NULL;
	struct target *w = NULL, *o = NULL;

	// concentrate everything on the apps_groups_targets
	for(p = root_of_pids; p ; p = p->next) {

		// --------------------------------------------------------------------
		// apps_groups targets
		if(likely(p->target))
			aggregate_pid_on_target(p->target, p, NULL);
		else
			error("pid %d %s was left without a target!", p->pid, p->comm);


		// --------------------------------------------------------------------
		// user targets
		o = p->user_target;
		if(likely(p->user_target && p->user_target->uid == p->uid))
			w = p->user_target;
		else {
			if(unlikely(debug && p->user_target))
					fprintf(stderr, "apps.plugin: \t\tpid %d (%s) switched user from %d (%s) to %d.\n", p->pid, p->comm, p->user_target->uid, p->user_target->name, p->uid);

			w = p->user_target = get_users_target(p->uid);
		}

		if(likely(w))
			aggregate_pid_on_target(w, p, o);
		else
			error("pid %d %s was left without a user target!", p->pid, p->comm);


		// --------------------------------------------------------------------
		// group targets
		o = p->group_target;
		if(likely(p->group_target && p->group_target->gid == p->gid))
			w = p->group_target;
		else {
			if(unlikely(debug && p->group_target))
					fprintf(stderr, "apps.plugin: \t\tpid %d (%s) switched group from %d (%s) to %d.\n", p->pid, p->comm, p->group_target->gid, p->group_target->name, p->gid);

			w = p->group_target = get_groups_target(p->gid);
		}

		if(likely(w))
			aggregate_pid_on_target(w, p, o);
		else
			error("pid %d %s was left without a group target!", p->pid, p->comm);

	}

	count_targets_fds(apps_groups_root_target);
	count_targets_fds(users_root_target);
	count_targets_fds(groups_root_target);

	cleanup_non_existing_pids();
}

// ----------------------------------------------------------------------------
// update chart dimensions

unsigned long long send_resource_usage_to_netdata() {
	static struct timeval last = { 0, 0 };
	static struct rusage me_last;

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

	fprintf(stdout, "BEGIN netdata.apps_cpu %llu\n", usec);
	fprintf(stdout, "SET user = %llu\n", cpuuser);
	fprintf(stdout, "SET system = %llu\n", cpusyst);
	fprintf(stdout, "END\n");

	fprintf(stdout, "BEGIN netdata.apps_files %llu\n", usec);
	fprintf(stdout, "SET files = %llu\n", file_counter);
	fprintf(stdout, "SET pids = %ld\n", all_pids_count);
	fprintf(stdout, "SET fds = %d\n", all_files_len);
	fprintf(stdout, "SET targets = %ld\n", apps_groups_targets);
	fprintf(stdout, "END\n");

	return usec;
}

void send_collected_data_to_netdata(struct target *root, const char *type, unsigned long long usec)
{
	struct target *w;

	fprintf(stdout, "BEGIN %s.cpu %llu\n", type, usec);
	for (w = root; w ; w = w->next) {
		if(w->target || (!w->processes && !w->exposed)) continue;

		fprintf(stdout, "SET %s = %llu\n", w->name, w->utime + w->stime + w->fix_utime + w->fix_stime);
	}
	fprintf(stdout, "END\n");

	fprintf(stdout, "BEGIN %s.cpu_user %llu\n", type, usec);
	for (w = root; w ; w = w->next) {
		if(w->target || (!w->processes && !w->exposed)) continue;

		fprintf(stdout, "SET %s = %llu\n", w->name, w->utime + w->fix_utime);
	}
	fprintf(stdout, "END\n");

	fprintf(stdout, "BEGIN %s.cpu_system %llu\n", type, usec);
	for (w = root; w ; w = w->next) {
		if(w->target || (!w->processes && !w->exposed)) continue;

		fprintf(stdout, "SET %s = %llu\n", w->name, w->stime + w->fix_stime);
	}
	fprintf(stdout, "END\n");

	fprintf(stdout, "BEGIN %s.threads %llu\n", type, usec);
	for (w = root; w ; w = w->next) {
		if(w->target || (!w->processes && !w->exposed)) continue;

		fprintf(stdout, "SET %s = %llu\n", w->name, w->num_threads);
	}
	fprintf(stdout, "END\n");

	fprintf(stdout, "BEGIN %s.processes %llu\n", type, usec);
	for (w = root; w ; w = w->next) {
		if(w->target || (!w->processes && !w->exposed)) continue;

		fprintf(stdout, "SET %s = %lu\n", w->name, w->processes);
	}
	fprintf(stdout, "END\n");

	fprintf(stdout, "BEGIN %s.mem %llu\n", type, usec);
	for (w = root; w ; w = w->next) {
		if(w->target || (!w->processes && !w->exposed)) continue;

		fprintf(stdout, "SET %s = %lld\n", w->name, (long long)w->statm_resident - (long long)w->statm_share);
	}
	fprintf(stdout, "END\n");

	fprintf(stdout, "BEGIN %s.minor_faults %llu\n", type, usec);
	for (w = root; w ; w = w->next) {
		if(w->target || (!w->processes && !w->exposed)) continue;

		fprintf(stdout, "SET %s = %llu\n", w->name, w->minflt + w->fix_minflt);
	}
	fprintf(stdout, "END\n");

	fprintf(stdout, "BEGIN %s.major_faults %llu\n", type, usec);
	for (w = root; w ; w = w->next) {
		if(w->target || (!w->processes && !w->exposed)) continue;

		fprintf(stdout, "SET %s = %llu\n", w->name, w->majflt + w->fix_majflt);
	}
	fprintf(stdout, "END\n");

	fprintf(stdout, "BEGIN %s.lreads %llu\n", type, usec);
	for (w = root; w ; w = w->next) {
		if(w->target || (!w->processes && !w->exposed)) continue;

		fprintf(stdout, "SET %s = %llu\n", w->name, w->io_logical_bytes_read + w->fix_io_logical_bytes_read);
	}
	fprintf(stdout, "END\n");

	fprintf(stdout, "BEGIN %s.lwrites %llu\n", type, usec);
	for (w = root; w ; w = w->next) {
		if(w->target || (!w->processes && !w->exposed)) continue;

		fprintf(stdout, "SET %s = %llu\n", w->name, w->io_logical_bytes_written + w->fix_io_logical_bytes_written);
	}
	fprintf(stdout, "END\n");

	fprintf(stdout, "BEGIN %s.preads %llu\n", type, usec);
	for (w = root; w ; w = w->next) {
		if(w->target || (!w->processes && !w->exposed)) continue;

		fprintf(stdout, "SET %s = %llu\n", w->name, w->io_storage_bytes_read + w->fix_io_storage_bytes_read);
	}
	fprintf(stdout, "END\n");

	fprintf(stdout, "BEGIN %s.pwrites %llu\n", type, usec);
	for (w = root; w ; w = w->next) {
		if(w->target || (!w->processes && !w->exposed)) continue;

		fprintf(stdout, "SET %s = %llu\n", w->name, w->io_storage_bytes_written + w->fix_io_storage_bytes_written);
	}
	fprintf(stdout, "END\n");

	fprintf(stdout, "BEGIN %s.files %llu\n", type, usec);
	for (w = root; w ; w = w->next) {
		if(w->target || (!w->processes && !w->exposed)) continue;

		fprintf(stdout, "SET %s = %llu\n", w->name, w->openfiles);
	}
	fprintf(stdout, "END\n");

	fprintf(stdout, "BEGIN %s.sockets %llu\n", type, usec);
	for (w = root; w ; w = w->next) {
		if(w->target || (!w->processes && !w->exposed)) continue;

		fprintf(stdout, "SET %s = %llu\n", w->name, w->opensockets);
	}
	fprintf(stdout, "END\n");

	fprintf(stdout, "BEGIN %s.pipes %llu\n", type, usec);
	for (w = root; w ; w = w->next) {
		if(w->target || (!w->processes && !w->exposed)) continue;

		fprintf(stdout, "SET %s = %llu\n", w->name, w->openpipes);
	}
	fprintf(stdout, "END\n");

	fflush(stdout);
}


// ----------------------------------------------------------------------------
// generate the charts

void send_charts_updates_to_netdata(struct target *root, const char *type, const char *title)
{
	struct target *w;
	int newly_added = 0;

	for(w = root ; w ; w = w->next)
		if(!w->exposed && w->processes) {
			newly_added++;
			w->exposed = 1;
			if(debug || w->debug) fprintf(stderr, "apps.plugin: %s just added - regenerating charts.\n", w->name);
		}

	// nothing more to show
	if(!newly_added) return;

	// we have something new to show
	// update the charts
	fprintf(stdout, "CHART %s.cpu '' '%s CPU Time (%ld%% = %ld core%s)' 'cpu time %%' cpu %s.cpu stacked 20001 %d\n", type, title, (processors * 100), processors, (processors>1)?"s":"", type, update_every);
	for (w = root; w ; w = w->next) {
		if(w->target || (!w->processes && !w->exposed)) continue;

		fprintf(stdout, "DIMENSION %s '' incremental 100 %u %s\n", w->name, hz, w->hidden ? "hidden,noreset" : "noreset");
	}

	fprintf(stdout, "CHART %s.mem '' '%s Dedicated Memory (w/o shared)' 'MB' mem %s.mem stacked 20003 %d\n", type, title, type, update_every);
	for (w = root; w ; w = w->next) {
		if(w->target || (!w->processes && !w->exposed)) continue;

		fprintf(stdout, "DIMENSION %s '' absolute %ld %ld noreset\n", w->name, sysconf(_SC_PAGESIZE), 1024L*1024L);
	}

	fprintf(stdout, "CHART %s.threads '' '%s Threads' 'threads' processes %s.threads stacked 20005 %d\n", type, title, type, update_every);
	for (w = root; w ; w = w->next) {
		if(w->target || (!w->processes && !w->exposed)) continue;

		fprintf(stdout, "DIMENSION %s '' absolute 1 1 noreset\n", w->name);
	}

	fprintf(stdout, "CHART %s.processes '' '%s Processes' 'processes' processes %s.processes stacked 20004 %d\n", type, title, type, update_every);
	for (w = root; w ; w = w->next) {
		if(w->target || (!w->processes && !w->exposed)) continue;

		fprintf(stdout, "DIMENSION %s '' absolute 1 1 noreset\n", w->name);
	}

	fprintf(stdout, "CHART %s.cpu_user '' '%s CPU User Time (%ld%% = %ld core%s)' 'cpu time %%' cpu %s.cpu_user stacked 20020 %d\n", type, title, (processors * 100), processors, (processors>1)?"s":"", type, update_every);
	for (w = root; w ; w = w->next) {
		if(w->target || (!w->processes && !w->exposed)) continue;

		fprintf(stdout, "DIMENSION %s '' incremental 100 %ld noreset\n", w->name, hz * processors);
	}

	fprintf(stdout, "CHART %s.cpu_system '' '%s CPU System Time (%ld%% = %ld core%s)' 'cpu time %%' cpu %s.cpu_system stacked 20021 %d\n", type, title, (processors * 100), processors, (processors>1)?"s":"", type, update_every);
	for (w = root; w ; w = w->next) {
		if(w->target || (!w->processes && !w->exposed)) continue;

		fprintf(stdout, "DIMENSION %s '' incremental 100 %ld noreset\n", w->name, hz * processors);
	}

	fprintf(stdout, "CHART %s.major_faults '' '%s Major Page Faults (swap read)' 'page faults/s' swap %s.major_faults stacked 20010 %d\n", type, title, type, update_every);
	for (w = root; w ; w = w->next) {
		if(w->target || (!w->processes && !w->exposed)) continue;

		fprintf(stdout, "DIMENSION %s '' incremental 1 1 noreset\n", w->name);
	}

	fprintf(stdout, "CHART %s.minor_faults '' '%s Minor Page Faults' 'page faults/s' mem %s.minor_faults stacked 20011 %d\n", type, title, type, update_every);
	for (w = root; w ; w = w->next) {
		if(w->target || (!w->processes && !w->exposed)) continue;

		fprintf(stdout, "DIMENSION %s '' incremental 1 1 noreset\n", w->name);
	}

	fprintf(stdout, "CHART %s.lreads '' '%s Disk Logical Reads' 'kilobytes/s' disk %s.lreads stacked 20042 %d\n", type, title, type, update_every);
	for (w = root; w ; w = w->next) {
		if(w->target || (!w->processes && !w->exposed)) continue;

		fprintf(stdout, "DIMENSION %s '' incremental 1 %d noreset\n", w->name, 1024);
	}

	fprintf(stdout, "CHART %s.lwrites '' '%s I/O Logical Writes' 'kilobytes/s' disk %s.lwrites stacked 20042 %d\n", type, title, type, update_every);
	for (w = root; w ; w = w->next) {
		if(w->target || (!w->processes && !w->exposed)) continue;

		fprintf(stdout, "DIMENSION %s '' incremental 1 %d noreset\n", w->name, 1024);
	}

	fprintf(stdout, "CHART %s.preads '' '%s Disk Reads' 'kilobytes/s' disk %s.preads stacked 20002 %d\n", type, title, type, update_every);
	for (w = root; w ; w = w->next) {
		if(w->target || (!w->processes && !w->exposed)) continue;

		fprintf(stdout, "DIMENSION %s '' incremental 1 %d noreset\n", w->name, 1024);
	}

	fprintf(stdout, "CHART %s.pwrites '' '%s Disk Writes' 'kilobytes/s' disk %s.pwrites stacked 20002 %d\n", type, title, type, update_every);
	for (w = root; w ; w = w->next) {
		if(w->target || (!w->processes && !w->exposed)) continue;

		fprintf(stdout, "DIMENSION %s '' incremental 1 %d noreset\n", w->name, 1024);
	}

	fprintf(stdout, "CHART %s.files '' '%s Open Files' 'open files' disk %s.files stacked 20050 %d\n", type, title, type, update_every);
	for (w = root; w ; w = w->next) {
		if(w->target || (!w->processes && !w->exposed)) continue;

		fprintf(stdout, "DIMENSION %s '' absolute 1 1 noreset\n", w->name);
	}

	fprintf(stdout, "CHART %s.sockets '' '%s Open Sockets' 'open sockets' net %s.sockets stacked 20051 %d\n", type, title, type, update_every);
	for (w = root; w ; w = w->next) {
		if(w->target || (!w->processes && !w->exposed)) continue;

		fprintf(stdout, "DIMENSION %s '' absolute 1 1 noreset\n", w->name);
	}

	fprintf(stdout, "CHART %s.pipes '' '%s Pipes' 'open pipes' processes %s.pipes stacked 20053 %d\n", type, title, type, update_every);
	for (w = root; w ; w = w->next) {
		if(w->target || (!w->processes && !w->exposed)) continue;

		fprintf(stdout, "DIMENSION %s '' absolute 1 1 noreset\n", w->name);
	}
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

	if(read_apps_groups_conf(name)) {
		error("Cannot read process groups %s", name);
		exit(1);
	}
}

int main(int argc, char **argv)
{
	// debug_flags = D_PROCFILE;

	// set the name for logging
	program_name = "apps.plugin";

	// disable syslog for apps.plugin
	error_log_syslog = 0;

	// set errors flood protection to 100 logs per hour
	error_log_errors_per_period = 100;
	error_log_throttle_period = 3600;

	host_prefix = getenv("NETDATA_HOST_PREFIX");
	if(host_prefix == NULL) {
		info("NETDATA_HOST_PREFIX is not passed from netdata");
		host_prefix = "";
	}
	else info("Found NETDATA_HOST_PREFIX='%s'", host_prefix);

	config_dir = getenv("NETDATA_CONFIG_DIR");
	if(config_dir == NULL) {
		info("NETDATA_CONFIG_DIR is not passed from netdata");
		config_dir = CONFIG_DIR;
	}
	else info("Found NETDATA_CONFIG_DIR='%s'", config_dir);

#ifdef NETDATA_INTERNAL_CHECKS
	if(debug_flags != 0) {
		struct rlimit rl = { RLIM_INFINITY, RLIM_INFINITY };
		if(setrlimit(RLIMIT_CORE, &rl) != 0)
			info("Cannot request unlimited core dumps for debugging... Proceeding anyway...");
		prctl(PR_SET_DUMPABLE, 1, 0, 0, 0);
	}
#endif /* NETDATA_INTERNAL_CHECKS */

	info("starting...");

	procfile_adaptive_initial_allocation = 1;

	time_t started_t = time(NULL);
	time_t current_t;
	get_HZ();
	pid_max = get_system_pid_max();
	processors = get_system_cpus();

	parse_args(argc, argv);

	all_pids = calloc(sizeof(struct pid_stat *), (size_t) pid_max);
	if(!all_pids) {
		error("Cannot allocate %lu bytes of memory.", sizeof(struct pid_stat *) * pid_max);
		printf("DISABLE\n");
		exit(1);
	}

	fprintf(stdout, "CHART netdata.apps_cpu '' 'Apps Plugin CPU' 'milliseconds/s' apps.plugin netdata.apps_cpu stacked 140000 %1$d\n"
	                "DIMENSION user '' incremental 1 1000\n"
	                "DIMENSION system '' incremental 1 1000\n"
	                "CHART netdata.apps_files '' 'Apps Plugin Files' 'files/s' apps.plugin netdata.apps_files line 140001 %1$d\n"
	                "DIMENSION files '' incremental 1 1\n"  
                  "DIMENSION pids '' absolute 1 1\n"  
	                "DIMENSION fds '' absolute 1 1\n"  
	                "DIMENSION targets '' absolute 1 1\n", update_every);

#ifndef PROFILING_MODE
	unsigned long long sunext = (time(NULL) - (time(NULL) % update_every) + update_every) * 1000000ULL;
	unsigned long long sunow;
#endif /* PROFILING_MODE */

	unsigned long long counter = 1;
	for(;1; counter++) {
#ifndef PROFILING_MODE
		// delay until it is our time to run
		while((sunow = timems()) < sunext)
			usleep((useconds_t)(sunext - sunow));

		// find the next time we need to run
		while(timems() > sunext)
			sunext += update_every * 1000000ULL;
#endif /* PROFILING_MODE */

		if(!collect_data_for_all_processes_from_proc()) {
			error("Cannot collect /proc data for running processes. Disabling apps.plugin...");
			printf("DISABLE\n");
			exit(1);
		}

		calculate_netdata_statistics();

		unsigned long long dt = send_resource_usage_to_netdata();

		// this is smart enough to show only newly added apps, when needed
		send_charts_updates_to_netdata(apps_groups_root_target, "apps", "Apps");
		send_charts_updates_to_netdata(users_root_target, "users", "Users");
		send_charts_updates_to_netdata(groups_root_target, "groups", "User Groups");

		send_collected_data_to_netdata(apps_groups_root_target, "apps", dt);
		send_collected_data_to_netdata(users_root_target, "users", dt);
		send_collected_data_to_netdata(groups_root_target, "groups", dt);

		if(debug) fprintf(stderr, "apps.plugin: done Loop No %llu\n", counter);

		current_t = time(NULL);

#ifndef PROFILING_MODE
		// restart check (14400 seconds)
		if(current_t - started_t > 14400) exit(0);
#else
		if(current_t - started_t > 10) exit(0);
#endif /* PROFILING_MODE */
	}
}
