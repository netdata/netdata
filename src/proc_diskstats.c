#ifdef HAVE_CONFIG_H
#include <config.h>
#endif
#include <stdio.h>
#include <stdlib.h>
#include <string.h>
#include <sys/types.h>
#include <sys/stat.h>
#include <fcntl.h>

#include "common.h"
#include "log.h"
#include "appconfig.h"
#include "procfile.h"
#include "rrd.h"
#include "plugin_proc.h"

#include "proc_self_mountinfo.h"

#define RRD_TYPE_DISK "disk"

struct disk {
	unsigned long major;
	unsigned long minor;
	int partition_id;       // -1 = this is not a partition
	char *family;
	struct disk *next;
} *disk_root = NULL;

struct disk *get_disk(unsigned long major, unsigned long minor) {
	static char path_find_block_device_partition[FILENAME_MAX + 1] = "";
	static struct mountinfo *mountinfo_root = NULL;
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

	if(unlikely(!path_find_block_device_partition[0])) {
		char filename[FILENAME_MAX + 1];
		snprintfz(filename, FILENAME_MAX, "%s%s", global_host_prefix, "/sys/dev/block/%lu:%lu/partition");
		snprintfz(path_find_block_device_partition, FILENAME_MAX, "%s", config_get("plugin:proc:/proc/diskstats", "path to get block device partition", filename));
	}

	// not found
	// create a new disk structure
	d = (struct disk *)malloc(sizeof(struct disk));
	if(!d) fatal("Cannot allocate memory for struct disk in proc_diskstats.");

	d->major = major;
	d->minor = minor;
	d->partition_id = -1;
	d->next = NULL;

	// append it to the list
	if(!disk_root)
		disk_root = d;
	else {
		struct disk *last;
		for(last = disk_root; last->next ;last = last->next);
		last->next = d;
	}

	// find if it is a partition
	// by reading /sys/dev/block/MAJOR:MINOR/partition
	char buffer[FILENAME_MAX + 1];
	snprintfz(buffer, FILENAME_MAX, path_find_block_device_partition, major, minor);

	int fd = open(buffer, O_RDONLY, 0666);
	if(likely(fd != -1)) {
		// we opened it
		int bytes = read(fd, buffer, FILENAME_MAX);
		close(fd);

		if(bytes > 0)
			d->partition_id = strtoul(buffer, NULL, 10);
	}
	// if the /partition file does not exist, it is a disk, not a partition

	// ------------------------------------------------------------------------
	// check if we can find its mount point

	// mountinfo_find() can be called with NULL mountinfo_root
	struct mountinfo *mi = mountinfo_find(mountinfo_root, d->major, d->minor);
	if(unlikely(!mi)) {
		// mountinfo_free() can be called with NULL mountinfo_root
		mountinfo_free(mountinfo_root);

		// re-read mountinfo in case something changed
		mountinfo_root = mountinfo_read();

		// search again for this disk
		mi = mountinfo_find(mountinfo_root, d->major, d->minor);
	}

	if(mi)
		d->family = strdup(mi->mount_point);
		// no need to check for NULL
	else
		d->family = NULL;

	return d;
}

int do_proc_diskstats(int update_every, unsigned long long dt) {
	static procfile *ff = NULL;
	static char path_to_get_hw_sector_size[FILENAME_MAX + 1] = "";
	static int enable_new_disks = -1;
	static int do_io = -1, do_ops = -1, do_mops = -1, do_iotime = -1, do_qops = -1, do_util = -1, do_backlog = -1;

	if(enable_new_disks == -1)	enable_new_disks = config_get_boolean_ondemand("plugin:proc:/proc/diskstats", "enable new disks detected at runtime", CONFIG_ONDEMAND_ONDEMAND);

	if(do_io == -1)		do_io 		= config_get_boolean_ondemand("plugin:proc:/proc/diskstats", "bandwidth for all disks", CONFIG_ONDEMAND_ONDEMAND);
	if(do_ops == -1)	do_ops 		= config_get_boolean_ondemand("plugin:proc:/proc/diskstats", "operations for all disks", CONFIG_ONDEMAND_ONDEMAND);
	if(do_mops == -1)	do_mops 	= config_get_boolean_ondemand("plugin:proc:/proc/diskstats", "merged operations for all disks", CONFIG_ONDEMAND_ONDEMAND);
	if(do_iotime == -1)	do_iotime 	= config_get_boolean_ondemand("plugin:proc:/proc/diskstats", "i/o time for all disks", CONFIG_ONDEMAND_ONDEMAND);
	if(do_qops == -1)	do_qops 	= config_get_boolean_ondemand("plugin:proc:/proc/diskstats", "queued operations for all disks", CONFIG_ONDEMAND_ONDEMAND);
	if(do_util == -1)	do_util 	= config_get_boolean_ondemand("plugin:proc:/proc/diskstats", "utilization percentage for all disks", CONFIG_ONDEMAND_ONDEMAND);
	if(do_backlog == -1)do_backlog 	= config_get_boolean_ondemand("plugin:proc:/proc/diskstats", "backlog for all disks", CONFIG_ONDEMAND_ONDEMAND);

	if(!ff) {
		char filename[FILENAME_MAX + 1];
		snprintfz(filename, FILENAME_MAX, "%s%s", global_host_prefix, "/proc/diskstats");
		ff = procfile_open(config_get("plugin:proc:/proc/diskstats", "filename to monitor", filename), " \t", PROCFILE_FLAG_DEFAULT);
	}
	if(!ff) return 1;

	if(!path_to_get_hw_sector_size[0]) {
		char filename[FILENAME_MAX + 1];
		snprintfz(filename, FILENAME_MAX, "%s%s", global_host_prefix, "/sys/block/%s/queue/hw_sector_size");
		snprintfz(path_to_get_hw_sector_size, FILENAME_MAX, "%s", config_get("plugin:proc:/proc/diskstats", "path to get h/w sector size", filename));
	}

	ff = procfile_readall(ff);
	if(!ff) return 0; // we return 0, so that we will retry to open it next time

	uint32_t lines = procfile_lines(ff), l;
	uint32_t words;

	for(l = 0; l < lines ;l++) {
		char *disk;
		unsigned long long 	major = 0, minor = 0,
							reads = 0,  mreads = 0,  readsectors = 0,  readms = 0,
							writes = 0, mwrites = 0, writesectors = 0, writems = 0,
							queued_ios = 0, busy_ms = 0, backlog_ms = 0;

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

		int def_enabled = 0;

		// remove slashes from disk names
		char *s;
		for(s = disk; *s ;s++) if(*s == '/') *s = '_';

		struct disk *d = get_disk(major, minor);
		if(d->partition_id == -1)
			def_enabled = enable_new_disks;
		else
			def_enabled = 0;

		char *family = d->family;
		if(!family) family = disk;

/*
		switch(major) {
			case 9: // MDs
			case 43: // network block
			case 144: // nfs
			case 145: // nfs
			case 146: // nfs
			case 199: // veritas
			case 201: // veritas
			case 251: // dm
			case 253: // virtio
				def_enabled = enable_new_disks;
				break;

			case 48: // RAID
			case 49: // RAID
			case 50: // RAID
			case 51: // RAID
			case 52: // RAID
			case 53: // RAID
			case 54: // RAID
			case 55: // RAID
			case 112: // RAID
			case 136: // RAID
			case 137: // RAID
			case 138: // RAID
			case 139: // RAID
			case 140: // RAID
			case 141: // RAID
			case 142: // RAID
			case 143: // RAID
			case 179: // MMC
			case 180: // USB
				if(minor % 8) def_enabled = 0; // partitions
				else def_enabled = enable_new_disks;
				break;

			case 8: // scsi disks
			case 65: // scsi disks
			case 66: // scsi disks
			case 67: // scsi disks
			case 68: // scsi disks
			case 69: // scsi disks
			case 70: // scsi disks
			case 71: // scsi disks
			case 72: // scsi disks
			case 73: // scsi disks
			case 74: // scsi disks
			case 75: // scsi disks
			case 76: // scsi disks
			case 77: // scsi disks
			case 78: // scsi disks
			case 79: // scsi disks
			case 80: // i2o
			case 81: // i2o
			case 82: // i2o
			case 83: // i2o
			case 84: // i2o
			case 85: // i2o
			case 86: // i2o
			case 87: // i2o
			case 101: // hyperdisk
			case 102: // compressed
			case 104: // scsi
			case 105: // scsi
			case 106: // scsi
			case 107: // scsi
			case 108: // scsi
			case 109: // scsi
			case 110: // scsi
			case 111: // scsi
			case 114: // bios raid
			case 116: // ram board
			case 128: // scsi
			case 129: // scsi
			case 130: // scsi
			case 131: // scsi
			case 132: // scsi
			case 133: // scsi
			case 134: // scsi
			case 135: // scsi
			case 153: // raid
			case 202: // xen
			case 254: // virtio3
			case 256: // flash
			case 257: // flash
			case 259: // nvme0n1 issue #119
				if(minor % 16) def_enabled = 0; // partitions
				else def_enabled = enable_new_disks;
				break;

			case 160: // raid
			case 161: // raid
				if(minor % 32) def_enabled = 0; // partitions
				else def_enabled = enable_new_disks;
				break;

			case 3: // ide
			case 13: // 8bit ide
			case 22: // ide
			case 33: // ide
			case 34: // ide
			case 56: // ide
			case 57: // ide
			case 88: // ide
			case 89: // ide
			case 90: // ide
			case 91: // ide
				if(minor % 64) def_enabled = 0; // partitions
				else def_enabled = enable_new_disks;
				break;

			case 252: // zram
				def_enabled = 0;
				break;

			default:
				def_enabled = 0;
				break;
		}
*/

		int ddo_io = do_io, ddo_ops = do_ops, ddo_mops = do_mops, ddo_iotime = do_iotime, ddo_qops = do_qops, ddo_util = do_util, ddo_backlog = do_backlog;

		// check which charts are enabled for this disk
		{
			char var_name[4096 + 1];
			snprintfz(var_name, 4096, "plugin:proc:/proc/diskstats:%s", disk);
			def_enabled = config_get_boolean_ondemand(var_name, "enabled", def_enabled);
			if(def_enabled == CONFIG_ONDEMAND_NO) continue;
			if(def_enabled == CONFIG_ONDEMAND_ONDEMAND && !reads && !writes) continue;


			ddo_io 		= config_get_boolean_ondemand(var_name, "bandwidth", ddo_io);
			ddo_ops 	= config_get_boolean_ondemand(var_name, "operations", ddo_ops);
			ddo_mops 	= config_get_boolean_ondemand(var_name, "merged operations", ddo_mops);
			ddo_iotime 	= config_get_boolean_ondemand(var_name, "i/o time", ddo_iotime);
			ddo_qops 	= config_get_boolean_ondemand(var_name, "queued operations", ddo_qops);
			ddo_util 	= config_get_boolean_ondemand(var_name, "utilization percentage", ddo_util);
			ddo_backlog = config_get_boolean_ondemand(var_name, "backlog", ddo_backlog);

			// by default, do not add charts that do not have values
			if(ddo_io == CONFIG_ONDEMAND_ONDEMAND && !reads && !writes) ddo_io = 0;
			if(ddo_mops == CONFIG_ONDEMAND_ONDEMAND && mreads == 0 && mwrites == 0) ddo_mops = 0;
			if(ddo_iotime == CONFIG_ONDEMAND_ONDEMAND && readms == 0 && writems == 0) ddo_iotime = 0;
			if(ddo_util == CONFIG_ONDEMAND_ONDEMAND && busy_ms == 0) ddo_util = 0;
			if(ddo_backlog == CONFIG_ONDEMAND_ONDEMAND && backlog_ms == 0) ddo_backlog = 0;
			if(ddo_qops == CONFIG_ONDEMAND_ONDEMAND && backlog_ms == 0) ddo_qops = 0;

			// for absolute values, we need to switch the setting to 'yes'
			// to allow it refresh from now on
			if(ddo_qops == CONFIG_ONDEMAND_ONDEMAND) config_set(var_name, "queued operations", "yes");
		}

		RRDSET *st;

		// --------------------------------------------------------------------

		int sector_size = 512;
		if(ddo_io) {
			st = rrdset_find_bytype(RRD_TYPE_DISK, disk);
			if(!st) {
				char tf[FILENAME_MAX + 1], *t;
				char ssfilename[FILENAME_MAX + 1];

				strncpyz(tf, disk, FILENAME_MAX);

				// replace all / with !
				while((t = strchr(tf, '/'))) *t = '!';

				snprintfz(ssfilename, FILENAME_MAX, path_to_get_hw_sector_size, tf);
				FILE *fpss = fopen(ssfilename, "r");
				if(fpss) {
					char ssbuffer[1025];
					char *tmp = fgets(ssbuffer, 1024, fpss);

					if(tmp) {
						sector_size = atoi(tmp);
						if(sector_size <= 0) {
							error("Invalid sector size %d for device %s in %s. Assuming 512.", sector_size, disk, ssfilename);
							sector_size = 512;
						}
					}
					else error("Cannot read data for sector size for device %s from %s. Assuming 512.", disk, ssfilename);

					fclose(fpss);
				}
				else error("Cannot read sector size for device %s from %s. Assuming 512.", disk, ssfilename);

				st = rrdset_create(RRD_TYPE_DISK, disk, NULL, family, "disk.io", "Disk I/O Bandwidth", "kilobytes/s", 2000, update_every, RRDSET_TYPE_AREA);

				rrddim_add(st, "reads", NULL, sector_size, 1024, RRDDIM_INCREMENTAL);
				rrddim_add(st, "writes", NULL, sector_size * -1, 1024, RRDDIM_INCREMENTAL);
			}
			else rrdset_next_usec(st, dt);

			last_readsectors  = rrddim_set(st, "reads", readsectors);
			last_writesectors = rrddim_set(st, "writes", writesectors);
			rrdset_done(st);
		}

		// --------------------------------------------------------------------

		if(ddo_ops) {
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

		if(ddo_qops) {
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

		if(ddo_backlog) {
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

		if(ddo_util) {
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

		if(ddo_mops) {
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

		if(ddo_iotime) {
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
			if(ddo_iotime && ddo_ops) {
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

			if(ddo_io && ddo_ops) {
				st = rrdset_find_bytype("disk_avgsz", disk);
				if(!st) {
					st = rrdset_create("disk_avgsz", disk, NULL, family, "disk.avgsz", "Average Completed I/O Operation Bandwidth", "kilobytes per operation", 2006, update_every, RRDSET_TYPE_AREA);
					st->isdetail = 1;

					rrddim_add(st, "reads", NULL, sector_size, 1024, RRDDIM_ABSOLUTE);
					rrddim_add(st, "writes", NULL, -sector_size, 1024, RRDDIM_ABSOLUTE);
				}
				else rrdset_next_usec(st, dt);

				rrddim_set(st, "reads", (reads - last_reads) ? (readsectors - last_readsectors) / (reads - last_reads) : 0);
				rrddim_set(st, "writes", (writes - last_writes) ? (writesectors - last_writesectors) / (writes - last_writes) : 0);
				rrdset_done(st);
			}

			if(ddo_util && ddo_ops) {
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
	}

	return 0;
}
