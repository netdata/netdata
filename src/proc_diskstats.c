#ifdef HAVE_CONFIG_H
#include <config.h>
#endif
#include <dirent.h>
#include <fcntl.h>
#include <stdio.h>
#include <stdlib.h>
#include <string.h>
#include <sys/stat.h>
#include <sys/statvfs.h>
#include <sys/types.h>
#include <unistd.h>


#include "common.h"
#include "log.h"
#include "appconfig.h"
#include "procfile.h"
#include "rrd.h"
#include "plugin_proc.h"

#include "proc_self_mountinfo.h"

#define RRD_TYPE_DISK "disk"

#define DISK_TYPE_PHYSICAL  1
#define DISK_TYPE_PARTITION 2
#define DISK_TYPE_CONTAINER 3

static struct disk {
	char *disk;				// the name of the disk (sda, sdb, etc)
	unsigned long major;
	unsigned long minor;
	int sector_size;
	int type;
	char *mount_point;

	// disk options caching
	int configured;
	int do_io;
	int do_ops;
	int do_mops;
	int do_iotime;
	int do_qops;
	int do_util;
	int do_backlog;
	int do_space;
	int do_inodes;

	struct disk *next;
} *disk_root = NULL;

static struct mountinfo *disk_mountinfo_root = NULL;

static struct disk *get_disk(unsigned long major, unsigned long minor, char *disk) {
	static char path_to_get_hw_sector_size[FILENAME_MAX + 1] = "";
	static char path_to_get_hw_sector_size_partitions[FILENAME_MAX + 1] = "";
	static char path_find_block_device[FILENAME_MAX + 1] = "";
	struct disk *d;

	// search for it in our RAM list.
	// this is sequential, but since we just walk through
	// and the number of disks / partitions in a system
	// should not be that many, it should be acceptable
	for(d = disk_root; d ; d = d->next)
		if(unlikely(d->major == major && d->minor == minor))
			break;

	// if we found it, return it
	if(likely(d))
		return d;

	// not found
	// create a new disk structure
	d = (struct disk *)malloc(sizeof(struct disk));
	if(!d) fatal("Cannot allocate memory for struct disk in proc_diskstats.");

	d->disk = strdup(disk);
	d->major = major;
	d->minor = minor;
	d->type = DISK_TYPE_PHYSICAL; // Default type. Changed later if not correct.
	d->configured = 0;
	d->sector_size = 512; // the default, will be changed below
	d->next = NULL;

	// append it to the list
	if(!disk_root)
		disk_root = d;
	else {
		struct disk *last;
		for(last = disk_root; last->next ;last = last->next);
		last->next = d;
	}

	// ------------------------------------------------------------------------
	// find the type of the device

	char buffer[FILENAME_MAX + 1];

	// get the default path for finding info about the block device
	if(unlikely(!path_find_block_device[0])) {
		snprintfz(buffer, FILENAME_MAX, "%s%s", global_host_prefix, "/sys/dev/block/%lu:%lu/%s");
		snprintfz(path_find_block_device, FILENAME_MAX, "%s", config_get("plugin:proc:/proc/diskstats", "path to get block device infos", buffer));
	}

	// find if it is a partition
	// by checking if /sys/dev/block/MAJOR:MINOR/partition is readable.
	snprintfz(buffer, FILENAME_MAX, path_find_block_device, major, minor, "partition");
	if(access(buffer, R_OK) == 0) {
		d->type = DISK_TYPE_PARTITION;
	} else {
		// find if it is a container
		// by checking if /sys/dev/block/MAJOR:MINOR/slaves has entries
		snprintfz(buffer, FILENAME_MAX, path_find_block_device, major, minor, "slaves/");
		DIR *dirp = opendir(buffer);	
		if (dirp != NULL) {
			struct dirent *dp;
			while( (dp = readdir(dirp)) ) {
				// . and .. are also files in empty folders.
				if(strcmp(dp->d_name, ".") == 0 || strcmp(dp->d_name, "..") == 0) {
					continue;
				}

				d->type = DISK_TYPE_CONTAINER;

				// Stop the loop after we found one file.
				break;
			}
			if(closedir(dirp) == -1)
				error("Unable to close dir %s", buffer);
		}
	}

	// ------------------------------------------------------------------------
	// check if we can find its mount point

	// mountinfo_find() can be called with NULL disk_mountinfo_root
	struct mountinfo *mi = mountinfo_find(disk_mountinfo_root, d->major, d->minor);
	if(unlikely(!mi)) {
		// mountinfo_free() can be called with NULL disk_mountinfo_root
		mountinfo_free(disk_mountinfo_root);

		// re-read mountinfo in case something changed
		disk_mountinfo_root = mountinfo_read();

		// search again for this disk
		mi = mountinfo_find(disk_mountinfo_root, d->major, d->minor);
	}

	if(mi)
		d->mount_point = strdup(mi->mount_point);
		// no need to check for NULL
	else
		d->mount_point = NULL;

	// ------------------------------------------------------------------------
	// find the disk sector size

	if(!path_to_get_hw_sector_size[0]) {
		snprintfz(buffer, FILENAME_MAX, "%s%s", global_host_prefix, "/sys/block/%s/queue/hw_sector_size");
		snprintfz(path_to_get_hw_sector_size, FILENAME_MAX, "%s", config_get("plugin:proc:/proc/diskstats", "path to get h/w sector size", buffer));
	}
	if(!path_to_get_hw_sector_size_partitions[0]) {
		snprintfz(buffer, FILENAME_MAX, "%s%s", global_host_prefix, "/sys/dev/block/%lu:%lu/subsystem/%s/../queue/hw_sector_size");
		snprintfz(path_to_get_hw_sector_size_partitions, FILENAME_MAX, "%s", config_get("plugin:proc:/proc/diskstats", "path to get h/w sector size for partitions", buffer));
	}

	{
		char tf[FILENAME_MAX + 1], *t;
		strncpyz(tf, d->disk, FILENAME_MAX);

		// replace all / with !
		for(t = tf; *t ;t++)
			if(*t == '/') *t = '!';

		if(d->type == DISK_TYPE_PARTITION)
			snprintfz(buffer, FILENAME_MAX, path_to_get_hw_sector_size_partitions, d->major, d->minor, tf);
		else
			snprintfz(buffer, FILENAME_MAX, path_to_get_hw_sector_size, tf);

		FILE *fpss = fopen(buffer, "r");
		if(fpss) {
			char buffer2[1024 + 1];
			char *tmp = fgets(buffer2, 1024, fpss);

			if(tmp) {
				d->sector_size = atoi(tmp);
				if(d->sector_size <= 0) {
					error("Invalid sector size %d for device %s in %s. Assuming 512.", d->sector_size, d->disk, buffer);
					d->sector_size = 512;
				}
			}
			else error("Cannot read data for sector size for device %s from %s. Assuming 512.", d->disk, buffer);

			fclose(fpss);
		}
		else error("Cannot read sector size for device %s from %s. Assuming 512.", d->disk, buffer);
	}

	return d;
}

static inline int select_positive_option(int option1, int option2) {
	if(option1 == CONFIG_ONDEMAND_YES || option2 == CONFIG_ONDEMAND_YES)
		return CONFIG_ONDEMAND_YES;
	else if(option1 == CONFIG_ONDEMAND_ONDEMAND || option2 == CONFIG_ONDEMAND_ONDEMAND)
		return CONFIG_ONDEMAND_ONDEMAND;

	return CONFIG_ONDEMAND_NO;
}

int do_proc_diskstats(int update_every, unsigned long long dt) {
	static procfile *ff = NULL;
	static struct statvfs buff_statvfs;
	static struct stat buff_stat;
	static int 	global_enable_new_disks_detected_at_runtime = CONFIG_ONDEMAND_YES,
				global_enable_performance_for_physical_disks = CONFIG_ONDEMAND_ONDEMAND,
				global_enable_performance_for_virtual_disks = CONFIG_ONDEMAND_NO,
				global_enable_performance_for_partitions = CONFIG_ONDEMAND_NO,
				global_enable_performance_for_mountpoints = CONFIG_ONDEMAND_NO,
				global_enable_performance_for_virtual_mountpoints = CONFIG_ONDEMAND_ONDEMAND,
				global_enable_space_for_mountpoints = CONFIG_ONDEMAND_ONDEMAND,
				global_do_io = CONFIG_ONDEMAND_ONDEMAND,
				global_do_ops = CONFIG_ONDEMAND_ONDEMAND,
				global_do_mops = CONFIG_ONDEMAND_ONDEMAND,
				global_do_iotime = CONFIG_ONDEMAND_ONDEMAND,
				global_do_qops = CONFIG_ONDEMAND_ONDEMAND,
				global_do_util = CONFIG_ONDEMAND_ONDEMAND,
				global_do_backlog = CONFIG_ONDEMAND_ONDEMAND,
				global_do_space = CONFIG_ONDEMAND_ONDEMAND,
				global_do_inodes = CONFIG_ONDEMAND_ONDEMAND,
				globals_initialized = 0;

	if(unlikely(!globals_initialized)) {
		global_enable_new_disks_detected_at_runtime = config_get_boolean("plugin:proc:/proc/diskstats", "enable new disks detected at runtime", global_enable_new_disks_detected_at_runtime);

		global_enable_performance_for_physical_disks = config_get_boolean_ondemand("plugin:proc:/proc/diskstats", "performance metrics for physical disks", global_enable_performance_for_physical_disks);
		global_enable_performance_for_virtual_disks = config_get_boolean_ondemand("plugin:proc:/proc/diskstats", "performance metrics for virtual disks", global_enable_performance_for_virtual_disks);
		global_enable_performance_for_partitions = config_get_boolean_ondemand("plugin:proc:/proc/diskstats", "performance metrics for partitions", global_enable_performance_for_partitions);
		global_enable_performance_for_mountpoints = config_get_boolean_ondemand("plugin:proc:/proc/diskstats", "performance metrics for mounted filesystems", global_enable_performance_for_mountpoints);
		global_enable_performance_for_virtual_mountpoints = config_get_boolean_ondemand("plugin:proc:/proc/diskstats", "performance metrics for mounted virtual disks", global_enable_performance_for_virtual_mountpoints);
		global_enable_space_for_mountpoints = config_get_boolean_ondemand("plugin:proc:/proc/diskstats", "space metrics for mounted filesystems", global_enable_space_for_mountpoints);

		global_do_io 	  = config_get_boolean_ondemand("plugin:proc:/proc/diskstats", "bandwidth for all disks", global_do_io);
		global_do_ops 	  = config_get_boolean_ondemand("plugin:proc:/proc/diskstats", "operations for all disks", global_do_ops);
		global_do_mops    = config_get_boolean_ondemand("plugin:proc:/proc/diskstats", "merged operations for all disks", global_do_mops);
		global_do_iotime  = config_get_boolean_ondemand("plugin:proc:/proc/diskstats", "i/o time for all disks", global_do_iotime);
		global_do_qops    = config_get_boolean_ondemand("plugin:proc:/proc/diskstats", "queued operations for all disks", global_do_qops);
		global_do_util    = config_get_boolean_ondemand("plugin:proc:/proc/diskstats", "utilization percentage for all disks", global_do_util);
		global_do_backlog = config_get_boolean_ondemand("plugin:proc:/proc/diskstats", "backlog for all disks", global_do_backlog);
		global_do_space   = config_get_boolean_ondemand("plugin:proc:/proc/diskstats", "space usage for all disks", global_do_space);
		global_do_inodes  = config_get_boolean_ondemand("plugin:proc:/proc/diskstats", "inodes usage for all disks", global_do_inodes);

		globals_initialized = 1;
	}

	if(!ff) {
		char filename[FILENAME_MAX + 1];
		snprintfz(filename, FILENAME_MAX, "%s%s", global_host_prefix, "/proc/diskstats");
		ff = procfile_open(config_get("plugin:proc:/proc/diskstats", "filename to monitor", filename), " \t", PROCFILE_FLAG_DEFAULT);
	}
	if(!ff) return 1;

	ff = procfile_readall(ff);
	if(!ff) return 0; // we return 0, so that we will retry to open it next time

	uint32_t lines = procfile_lines(ff), l;
	uint32_t words;

	for(l = 0; l < lines ;l++) {
		// --------------------------------------------------------------------------
		// Read parameters

		char *disk;
		unsigned long long 	major = 0, minor = 0,
							reads = 0,  mreads = 0,  readsectors = 0,  readms = 0,
							writes = 0, mwrites = 0, writesectors = 0, writems = 0,
							queued_ios = 0, busy_ms = 0, backlog_ms = 0,
							space_avail = 0, space_avail_root = 0, space_used = 0,
							inodes_avail = 0, inodes_avail_root = 0, inodes_used = 0;

		unsigned long long 	last_reads = 0,  last_readsectors = 0,  last_readms = 0,
							last_writes = 0, last_writesectors = 0, last_writems = 0,
							last_busy_ms = 0;

		words = procfile_linewords(ff, l);
		if(words < 14) continue;

		major 			= strtoull(procfile_lineword(ff, l, 0), NULL, 10);
		minor 			= strtoull(procfile_lineword(ff, l, 1), NULL, 10);
		disk 			= procfile_lineword(ff, l, 2);

		// # of reads completed # of writes completed
		// This is the total number of reads or writes completed successfully.
		reads 			= strtoull(procfile_lineword(ff, l, 3), NULL, 10); 	// rd_ios
		writes 			= strtoull(procfile_lineword(ff, l, 7), NULL, 10); 	// wr_ios

		// # of reads merged # of writes merged
		// Reads and writes which are adjacent to each other may be merged for
	    // efficiency.  Thus two 4K reads may become one 8K read before it is
	    // ultimately handed to the disk, and so it will be counted (and queued)
		mreads		 	= strtoull(procfile_lineword(ff, l, 4), NULL, 10); 	// rd_merges_or_rd_sec
		mwrites 		= strtoull(procfile_lineword(ff, l, 8), NULL, 10); 	// wr_merges

		// # of sectors read # of sectors written
		// This is the total number of sectors read or written successfully.
		readsectors 	= strtoull(procfile_lineword(ff, l, 5), NULL, 10); 	// rd_sec_or_wr_ios
		writesectors 	= strtoull(procfile_lineword(ff, l, 9), NULL, 10); 	// wr_sec

		// # of milliseconds spent reading # of milliseconds spent writing
		// This is the total number of milliseconds spent by all reads or writes (as
		// measured from __make_request() to end_that_request_last()).
		readms 			= strtoull(procfile_lineword(ff, l, 6), NULL, 10); 	// rd_ticks_or_wr_sec
		writems 		= strtoull(procfile_lineword(ff, l, 10), NULL, 10);	// wr_ticks

		// # of I/Os currently in progress
		// The only field that should go to zero. Incremented as requests are
		// given to appropriate struct request_queue and decremented as they finish.
		queued_ios 		= strtoull(procfile_lineword(ff, l, 11), NULL, 10);	// ios_pgr

		// # of milliseconds spent doing I/Os
		// This field increases so long as field queued_ios is nonzero.
		busy_ms 		= strtoull(procfile_lineword(ff, l, 12), NULL, 10);	// tot_ticks

		// weighted # of milliseconds spent doing I/Os
		// This field is incremented at each I/O start, I/O completion, I/O
		// merge, or read of these stats by the number of I/Os in progress
		// (field queued_ios) times the number of milliseconds spent doing I/O since the
		// last update of this field.  This can provide an easy measure of both
		// I/O completion time and the backlog that may be accumulating.
		backlog_ms 		= strtoull(procfile_lineword(ff, l, 13), NULL, 10);	// rq_ticks


		// --------------------------------------------------------------------------
		// remove slashes from disk names
		char *s;
		for(s = disk; *s ;s++)
			if(*s == '/') *s = '_';

		// --------------------------------------------------------------------------
		// get a disk structure for the disk

		struct disk *d = get_disk(major, minor, disk);


		// --------------------------------------------------------------------------
		// Set its family based on mount point

		char *family = d->mount_point;
		if(!family) family = disk;


		// --------------------------------------------------------------------------
		// Check the configuration for the device

		if(unlikely(!d->configured)) {
			char var_name[4096 + 1];
			snprintfz(var_name, 4096, "plugin:proc:/proc/diskstats:%s", disk);

			int def_enable = config_get_boolean_ondemand(var_name, "enable", global_enable_new_disks_detected_at_runtime);
			if(def_enable == CONFIG_ONDEMAND_NO) {
				// the user does not want any metrics for this disk
				d->do_io = CONFIG_ONDEMAND_NO;
				d->do_ops = CONFIG_ONDEMAND_NO;
				d->do_mops = CONFIG_ONDEMAND_NO;
				d->do_iotime = CONFIG_ONDEMAND_NO;
				d->do_qops = CONFIG_ONDEMAND_NO;
				d->do_util = CONFIG_ONDEMAND_NO;
				d->do_backlog = CONFIG_ONDEMAND_NO;
				d->do_space = CONFIG_ONDEMAND_NO;
				d->do_inodes = CONFIG_ONDEMAND_NO;
			}
			else {
				// this disk is enabled
				// check its direct settings

				int def_performance = CONFIG_ONDEMAND_ONDEMAND;
				int def_space = (d->mount_point)?CONFIG_ONDEMAND_ONDEMAND:CONFIG_ONDEMAND_NO;

				// since this is 'on demand' we can figure the performance settings
				// based on the type of disk

				switch(d->type) {
					case DISK_TYPE_PHYSICAL:
						def_performance = global_enable_performance_for_physical_disks;
						break;

					case DISK_TYPE_PARTITION:
						def_performance = global_enable_performance_for_partitions;
						break;

					case DISK_TYPE_CONTAINER:
						def_performance = global_enable_performance_for_virtual_disks;

						if(d->mount_point)
							def_performance = select_positive_option(def_performance, global_enable_performance_for_virtual_mountpoints);

						break;
				}

				if(d->mount_point)
					def_performance = select_positive_option(def_performance, global_enable_performance_for_mountpoints);

				// ------------------------------------------------------------
				// now we have def_performance and def_space
				// to work further

				// def_performance
				// check the user configuration (this will also show our 'on demand' decision)
				def_performance = config_get_boolean_ondemand(var_name, "enable performance metrics", def_performance);

				int ddo_io = CONFIG_ONDEMAND_NO,
					ddo_ops = CONFIG_ONDEMAND_NO,
					ddo_mops = CONFIG_ONDEMAND_NO,
					ddo_iotime = CONFIG_ONDEMAND_NO,
					ddo_qops = CONFIG_ONDEMAND_NO,
					ddo_util = CONFIG_ONDEMAND_NO,
					ddo_backlog = CONFIG_ONDEMAND_NO;

				// we enable individual performance charts only when def_performance is not disabled
				if(def_performance != CONFIG_ONDEMAND_NO) {
					ddo_io = global_do_io,
					ddo_ops = global_do_ops,
					ddo_mops = global_do_mops,
					ddo_iotime = global_do_iotime,
					ddo_qops = global_do_qops,
					ddo_util = global_do_util,
					ddo_backlog = global_do_backlog;
				}

				d->do_io      = config_get_boolean_ondemand(var_name, "bandwidth", ddo_io);
				d->do_ops     = config_get_boolean_ondemand(var_name, "operations", ddo_ops);
				d->do_mops    = config_get_boolean_ondemand(var_name, "merged operations", ddo_mops);
				d->do_iotime  = config_get_boolean_ondemand(var_name, "i/o time", ddo_iotime);
				d->do_qops    = config_get_boolean_ondemand(var_name, "queued operations", ddo_qops);
				d->do_util    = config_get_boolean_ondemand(var_name, "utilization percentage", ddo_util);
				d->do_backlog = config_get_boolean_ondemand(var_name, "backlog", ddo_backlog);

				// def_space
				if(d->mount_point) {
					// check the user configuration (this will also show our 'on demand' decision)
					def_space = config_get_boolean_ondemand(var_name, "enable space metrics", def_space);

					int ddo_space = def_space,
						ddo_inodes = def_space;

					d->do_space = config_get_boolean_ondemand(var_name, "space usage", ddo_space);
					d->do_inodes = config_get_boolean_ondemand(var_name, "inodes usage", ddo_inodes);
				}
				else {
					// don't show settings for this disk
					d->do_space = CONFIG_ONDEMAND_NO;
					d->do_inodes = CONFIG_ONDEMAND_NO;
				}
			}

			d->configured = 1;
		}

		RRDSET *st;

		// --------------------------------------------------------------------------
		// Do performance metrics

		if(d->do_io == CONFIG_ONDEMAND_YES || (d->do_io == CONFIG_ONDEMAND_ONDEMAND && (readsectors || writesectors))) {
			d->do_io = CONFIG_ONDEMAND_YES;

			st = rrdset_find_bytype(RRD_TYPE_DISK, disk);
			if(!st) {
				st = rrdset_create(RRD_TYPE_DISK, disk, NULL, family, "disk.io", "Disk I/O Bandwidth", "kilobytes/s", 2000, update_every, RRDSET_TYPE_AREA);

				rrddim_add(st, "reads", NULL, d->sector_size, 1024, RRDDIM_INCREMENTAL);
				rrddim_add(st, "writes", NULL, d->sector_size * -1, 1024, RRDDIM_INCREMENTAL);
			}
			else rrdset_next_usec(st, dt);

			last_readsectors  = rrddim_set(st, "reads", readsectors);
			last_writesectors = rrddim_set(st, "writes", writesectors);
			rrdset_done(st);
		}

		// --------------------------------------------------------------------

		if(d->do_ops == CONFIG_ONDEMAND_YES || (d->do_ops == CONFIG_ONDEMAND_ONDEMAND && (reads || writes))) {
			d->do_ops = CONFIG_ONDEMAND_YES;

			st = rrdset_find_bytype("disk_ops", disk);
			if(!st) {
				st = rrdset_create("disk_ops", disk, NULL, family, "disk.ops", "Disk Completed I/O Operations", "operations/s", 2001, update_every, RRDSET_TYPE_LINE);
				st->isdetail = 1;

				rrddim_add(st, "reads", NULL, 1, 1, RRDDIM_INCREMENTAL);
				rrddim_add(st, "writes", NULL, -1, 1, RRDDIM_INCREMENTAL);
			}
			else rrdset_next_usec(st, dt);

			last_reads  = rrddim_set(st, "reads", reads);
			last_writes = rrddim_set(st, "writes", writes);
			rrdset_done(st);
		}

		// --------------------------------------------------------------------

		if(d->do_qops == CONFIG_ONDEMAND_YES || (d->do_qops == CONFIG_ONDEMAND_ONDEMAND && queued_ios)) {
			d->do_qops = CONFIG_ONDEMAND_YES;

			st = rrdset_find_bytype("disk_qops", disk);
			if(!st) {
				st = rrdset_create("disk_qops", disk, NULL, family, "disk.qops", "Disk Current I/O Operations", "operations", 2002, update_every, RRDSET_TYPE_LINE);
				st->isdetail = 1;

				rrddim_add(st, "operations", NULL, 1, 1, RRDDIM_ABSOLUTE);
			}
			else rrdset_next_usec(st, dt);

			rrddim_set(st, "operations", queued_ios);
			rrdset_done(st);
		}

		// --------------------------------------------------------------------

		if(d->do_backlog == CONFIG_ONDEMAND_YES || (d->do_backlog == CONFIG_ONDEMAND_ONDEMAND && backlog_ms)) {
			d->do_backlog = CONFIG_ONDEMAND_YES;

			st = rrdset_find_bytype("disk_backlog", disk);
			if(!st) {
				st = rrdset_create("disk_backlog", disk, NULL, family, "disk.backlog", "Disk Backlog", "backlog (ms)", 2003, update_every, RRDSET_TYPE_AREA);
				st->isdetail = 1;

				rrddim_add(st, "backlog", NULL, 1, 10, RRDDIM_INCREMENTAL);
			}
			else rrdset_next_usec(st, dt);

			rrddim_set(st, "backlog", backlog_ms);
			rrdset_done(st);
		}

		// --------------------------------------------------------------------

		if(d->do_util == CONFIG_ONDEMAND_YES || (d->do_util == CONFIG_ONDEMAND_ONDEMAND && busy_ms)) {
			d->do_util = CONFIG_ONDEMAND_YES;

			st = rrdset_find_bytype("disk_util", disk);
			if(!st) {
				st = rrdset_create("disk_util", disk, NULL, family, "disk.util", "Disk Utilization Time", "% of time working", 2004, update_every, RRDSET_TYPE_AREA);
				st->isdetail = 1;

				rrddim_add(st, "utilization", NULL, 1, 10, RRDDIM_INCREMENTAL);
			}
			else rrdset_next_usec(st, dt);

			last_busy_ms = rrddim_set(st, "utilization", busy_ms);
			rrdset_done(st);
		}

		// --------------------------------------------------------------------

		if(d->do_mops == CONFIG_ONDEMAND_YES || (d->do_mops == CONFIG_ONDEMAND_ONDEMAND && (mreads || mwrites))) {
			d->do_mops = CONFIG_ONDEMAND_YES;

			st = rrdset_find_bytype("disk_mops", disk);
			if(!st) {
				st = rrdset_create("disk_mops", disk, NULL, family, "disk.mops", "Disk Merged Operations", "merged operations/s", 2021, update_every, RRDSET_TYPE_LINE);
				st->isdetail = 1;

				rrddim_add(st, "reads", NULL, 1, 1, RRDDIM_INCREMENTAL);
				rrddim_add(st, "writes", NULL, -1, 1, RRDDIM_INCREMENTAL);
			}
			else rrdset_next_usec(st, dt);

			rrddim_set(st, "reads", mreads);
			rrddim_set(st, "writes", mwrites);
			rrdset_done(st);
		}

		// --------------------------------------------------------------------

		if(d->do_iotime == CONFIG_ONDEMAND_YES || (d->do_iotime == CONFIG_ONDEMAND_ONDEMAND && (readms || writems))) {
			d->do_iotime = CONFIG_ONDEMAND_YES;

			st = rrdset_find_bytype("disk_iotime", disk);
			if(!st) {
				st = rrdset_create("disk_iotime", disk, NULL, family, "disk.iotime", "Disk Total I/O Time", "milliseconds/s", 2022, update_every, RRDSET_TYPE_LINE);
				st->isdetail = 1;

				rrddim_add(st, "reads", NULL, 1, 1, RRDDIM_INCREMENTAL);
				rrddim_add(st, "writes", NULL, -1, 1, RRDDIM_INCREMENTAL);
			}
			else rrdset_next_usec(st, dt);

			last_readms  = rrddim_set(st, "reads", readms);
			last_writems = rrddim_set(st, "writes", writems);
			rrdset_done(st);
		}

		// --------------------------------------------------------------------
		// calculate differential charts
		// only if this is not the first time we run

		if(dt) {
			if( (d->do_iotime == CONFIG_ONDEMAND_YES || (d->do_iotime == CONFIG_ONDEMAND_ONDEMAND && (readms || writems))) &&
				(d->do_ops    == CONFIG_ONDEMAND_YES || (d->do_ops    == CONFIG_ONDEMAND_ONDEMAND && (reads || writes)))) {
				st = rrdset_find_bytype("disk_await", disk);
				if(!st) {
					st = rrdset_create("disk_await", disk, NULL, family, "disk.await", "Average Completed I/O Operation Time", "ms per operation", 2005, update_every, RRDSET_TYPE_LINE);
					st->isdetail = 1;

					rrddim_add(st, "reads", NULL, 1, 1, RRDDIM_ABSOLUTE);
					rrddim_add(st, "writes", NULL, -1, 1, RRDDIM_ABSOLUTE);
				}
				else rrdset_next_usec(st, dt);

				rrddim_set(st, "reads", (reads - last_reads) ? (readms - last_readms) / (reads - last_reads) : 0);
				rrddim_set(st, "writes", (writes - last_writes) ? (writems - last_writems) / (writes - last_writes) : 0);
				rrdset_done(st);
			}

			if( (d->do_io  == CONFIG_ONDEMAND_YES || (d->do_io  == CONFIG_ONDEMAND_ONDEMAND && (readsectors || writesectors))) &&
				(d->do_ops == CONFIG_ONDEMAND_YES || (d->do_ops == CONFIG_ONDEMAND_ONDEMAND && (reads || writes)))) {
				st = rrdset_find_bytype("disk_avgsz", disk);
				if(!st) {
					st = rrdset_create("disk_avgsz", disk, NULL, family, "disk.avgsz", "Average Completed I/O Operation Bandwidth", "kilobytes per operation", 2006, update_every, RRDSET_TYPE_AREA);
					st->isdetail = 1;

					rrddim_add(st, "reads", NULL, d->sector_size, 1024, RRDDIM_ABSOLUTE);
					rrddim_add(st, "writes", NULL, d->sector_size * -1, 1024, RRDDIM_ABSOLUTE);
				}
				else rrdset_next_usec(st, dt);

				rrddim_set(st, "reads", (reads - last_reads) ? (readsectors - last_readsectors) / (reads - last_reads) : 0);
				rrddim_set(st, "writes", (writes - last_writes) ? (writesectors - last_writesectors) / (writes - last_writes) : 0);
				rrdset_done(st);
			}

			if( (d->do_util == CONFIG_ONDEMAND_YES || (d->do_util == CONFIG_ONDEMAND_ONDEMAND && busy_ms)) &&
				(d->do_ops  == CONFIG_ONDEMAND_YES || (d->do_ops  == CONFIG_ONDEMAND_ONDEMAND && (reads || writes)))) {
				st = rrdset_find_bytype("disk_svctm", disk);
				if(!st) {
					st = rrdset_create("disk_svctm", disk, NULL, family, "disk.svctm", "Average Service Time", "ms per operation", 2007, update_every, RRDSET_TYPE_LINE);
					st->isdetail = 1;

					rrddim_add(st, "svctm", NULL, 1, 1, RRDDIM_ABSOLUTE);
				}
				else rrdset_next_usec(st, dt);

				rrddim_set(st, "svctm", ((reads - last_reads) + (writes - last_writes)) ? (busy_ms - last_busy_ms) / ((reads - last_reads) + (writes - last_writes)) : 0);
				rrdset_done(st);
			}
		}

		// --------------------------------------------------------------------------
		// space metrics

		if(d->mount_point && (d->do_space || d->do_inodes) ) {
			// collect space metrics using statvfs

			if (statvfs(d->mount_point, &buff_statvfs) < 0)
				error("Failed statvfs() for '%s' (disk '%s')", d->mount_point, d->disk);
			else {
				space_avail = buff_statvfs.f_bavail * buff_statvfs.f_bsize;
				space_avail_root = (buff_statvfs.f_bfree - buff_statvfs.f_bavail) * buff_statvfs.f_bsize;
				space_used = (buff_statvfs.f_blocks - buff_statvfs.f_bfree) * buff_statvfs.f_bsize;

				inodes_avail = buff_statvfs.f_favail;
				inodes_avail_root = buff_statvfs.f_ffree - buff_statvfs.f_favail;
				inodes_used = buff_statvfs.f_files - buff_statvfs.f_ffree;

				// verify we collected the metrics for the right disk.
				// if not the mountpoint has changed.

				if(stat(d->mount_point, &buff_stat) == -1)
					error("Failed to stat() for '%s' (disk '%s')", d->mount_point, d->disk);
				else {
					if(major(buff_stat.st_dev) == major && minor(buff_stat.st_dev) == minor) {

						// --------------------------------------------------------------------------

						if(d->do_space == CONFIG_ONDEMAND_YES || (d->do_space == CONFIG_ONDEMAND_ONDEMAND && (space_avail || space_avail_root || space_used))) {
							st = rrdset_find_bytype("disk_space", disk);
							if(!st) {
								st = rrdset_create("disk_space", disk, NULL, family, "disk.space", "Disk Space Usage", "GB", 2023, update_every, RRDSET_TYPE_STACKED);
								st->isdetail = 1;

								rrddim_add(st, "avail", NULL, 1, 1024*1024*1024, RRDDIM_ABSOLUTE);
								rrddim_add(st, "reserved_for_root", "reserved for root", 1, 1024*1024*1024, RRDDIM_ABSOLUTE);
								rrddim_add(st, "used" , NULL, 1, 1024*1024*1024, RRDDIM_ABSOLUTE);
							}
							else rrdset_next_usec(st, dt);

							rrddim_set(st, "avail", space_avail);
							rrddim_set(st, "reserved_for_root", space_avail_root);
							rrddim_set(st, "used", space_used);
							rrdset_done(st);
						}

						// --------------------------------------------------------------------------

						if(d->do_inodes == CONFIG_ONDEMAND_YES || (d->do_inodes == CONFIG_ONDEMAND_ONDEMAND && (inodes_avail || inodes_avail_root || inodes_used))) {
							st = rrdset_find_bytype("disk_inodes", disk);
							if(!st) {
								st = rrdset_create("disk_inodes", disk, NULL, family, "disk.inodes", "Disk Inodes Usage", "Inodes", 2024, update_every, RRDSET_TYPE_STACKED);
								st->isdetail = 1;

								rrddim_add(st, "avail", NULL, 1, 1, RRDDIM_ABSOLUTE);
								rrddim_add(st, "reserved_for_root", "reserved for root", 1, 1, RRDDIM_ABSOLUTE);
								rrddim_add(st, "used" , NULL, 1, 1, RRDDIM_ABSOLUTE);
							}
							else rrdset_next_usec(st, dt);

							rrddim_set(st, "avail", inodes_avail);
							rrddim_set(st, "reserved_for_root", inodes_avail_root);
							rrddim_set(st, "used", inodes_used);
							rrdset_done(st);
						}
					}
				}
			}
		}
	}

	return 0;
}
