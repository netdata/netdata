#include <inttypes.h>
#include <stdio.h>
#include <stdlib.h>
#include <string.h>

#include "common.h"
#include "log.h"
#include "config.h"
#include "procfile.h"
#include "rrd.h"
#include "plugin_proc.h"

#define RRD_TYPE_DISK				"disk"
#define RRD_TYPE_DISK_LEN			strlen(RRD_TYPE_DISK)

#define MAX_PROC_DISKSTATS_LINE 4096

int do_proc_diskstats(int update_every, unsigned long long dt) {
	static procfile *ff = NULL;
	static char path_to_get_hw_sector_size[FILENAME_MAX + 1] = "";
	static int enable_new_disks = -1;
	static int do_io = -1, do_ops = -1, do_merged_ops = -1, do_iotime = -1, do_cur_ops = -1, do_util = -1, do_qsize = -1;

	if(enable_new_disks == -1)	enable_new_disks = config_get_boolean("plugin:proc:/proc/diskstats", "enable new disks detected at runtime", 1);

	if(do_io == -1)			do_io 			= config_get_boolean("plugin:proc:/proc/diskstats", "bandwidth for all disks", 1);
	if(do_ops == -1)		do_ops 			= config_get_boolean("plugin:proc:/proc/diskstats", "operations for all disks", 1);
	if(do_merged_ops == -1)	do_merged_ops 	= config_get_boolean("plugin:proc:/proc/diskstats", "merged operations for all disks", 1);
	if(do_iotime == -1)		do_iotime 		= config_get_boolean("plugin:proc:/proc/diskstats", "i/o time for all disks", 1);
	if(do_cur_ops == -1)	do_cur_ops 		= config_get_boolean("plugin:proc:/proc/diskstats", "current operations for all disks", 1);
	if(do_util == -1)		do_util 		= config_get_boolean("plugin:proc:/proc/diskstats", "utilization percentage for all disks", 1);
	if(do_qsize == -1)		do_qsize 		= config_get_boolean("plugin:proc:/proc/diskstats", "queue size for all disks", 1);

	if(!ff) {
		char filename[FILENAME_MAX + 1];
		snprintf(filename, FILENAME_MAX, "%s%s", global_host_prefix, "/proc/diskstats");
		ff = procfile_open(config_get("plugin:proc:/proc/diskstats", "filename to monitor", filename), " \t", PROCFILE_FLAG_DEFAULT);
	}
	if(!ff) return 1;

	if(!path_to_get_hw_sector_size[0]) {
		char filename[FILENAME_MAX + 1];
		snprintf(filename, FILENAME_MAX, "%s%s", global_host_prefix, "/sys/block/%s/queue/hw_sector_size");
		snprintf(path_to_get_hw_sector_size, FILENAME_MAX, "%s%s", global_host_prefix, config_get("plugin:proc:/proc/diskstats", "path to get h/w sector size", filename));
	}

	ff = procfile_readall(ff);
	if(!ff) return 0; // we return 0, so that we will retry to open it next time

	uint32_t lines = procfile_lines(ff), l;
	uint32_t words;

	for(l = 0; l < lines ;l++) {
		char *disk;
		unsigned long long 	major = 0, minor = 0,
							reads = 0,  reads_merged = 0,  readsectors = 0,  readms = 0,
							writes = 0, writes_merged = 0, writesectors = 0, writems = 0,
							currentios = 0, iosms = 0, wiosms = 0;

		unsigned long long 	last_reads = 0,  last_readsectors = 0,  last_readms = 0,
							last_writes = 0, last_writesectors = 0, last_writems = 0,
							last_iosms = 0;

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
		reads_merged 	= strtoull(procfile_lineword(ff, l, 4), NULL, 10); 	// rd_merges_or_rd_sec
		writes_merged 	= strtoull(procfile_lineword(ff, l, 8), NULL, 10); 	// wr_merges

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
		currentios 		= strtoull(procfile_lineword(ff, l, 11), NULL, 10);	// ios_pgr

		// # of milliseconds spent doing I/Os
		// This field increases so long as field currentios is nonzero.
		iosms 			= strtoull(procfile_lineword(ff, l, 12), NULL, 10);	// tot_ticks

		// weighted # of milliseconds spent doing I/Os
		// This field is incremented at each I/O start, I/O completion, I/O
		// merge, or read of these stats by the number of I/Os in progress
		// (field currentios) times the number of milliseconds spent doing I/O since the
		// last update of this field.  This can provide an easy measure of both
		// I/O completion time and the backlog that may be accumulating.
		wiosms 			= strtoull(procfile_lineword(ff, l, 13), NULL, 10);	// rq_ticks

		int def_enabled = 0;

		// remove slashes from disk names
		char *s;
		for(s = disk; *s ;s++) if(*s == '/') *s = '_';

		switch(major) {
			case 9: // MDs
			case 43: // network block
			case 144: // nfs
			case 145: // nfs
			case 146: // nfs
			case 199: // veritas
			case 201: // veritas
			case 251: // dm
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
			case 256: // flash
			case 257: // flash
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

			default:
				def_enabled = 0;
				break;
		}

		// check if it is enabled
		{
			char var_name[4096 + 1];
			snprintf(var_name, 4096, "disk %s", disk);
			if(!config_get_boolean("plugin:proc:/proc/diskstats", var_name, def_enabled)) continue;
		}

		RRDSET *st;

		// --------------------------------------------------------------------

		int sector_size = 512;
		if(do_io) {
			st = rrdset_find_bytype(RRD_TYPE_DISK, disk);
			if(!st) {
				char tf[FILENAME_MAX + 1], *t;
				char ssfilename[FILENAME_MAX + 1];

				strncpy(tf, disk, FILENAME_MAX);
				tf[FILENAME_MAX] = '\0';

				// replace all / with !
				while((t = strchr(tf, '/'))) *t = '!';

				snprintf(ssfilename, FILENAME_MAX, path_to_get_hw_sector_size, tf);
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

				st = rrdset_create(RRD_TYPE_DISK, disk, NULL, disk, "Disk I/O", "kilobytes/s", 2000, update_every, RRDSET_TYPE_AREA);

				rrddim_add(st, "reads", NULL, sector_size, 1024 * update_every, RRDDIM_INCREMENTAL);
				rrddim_add(st, "writes", NULL, sector_size * -1, 1024 * update_every, RRDDIM_INCREMENTAL);
			}
			else rrdset_next_usec(st, dt);

			last_readsectors  = rrddim_set(st, "reads", readsectors);
			last_writesectors = rrddim_set(st, "writes", writesectors);
			rrdset_done(st);
		}

		// --------------------------------------------------------------------

		if(do_ops) {
			st = rrdset_find_bytype("disk_ops", disk);
			if(!st) {
				st = rrdset_create("disk_ops", disk, NULL, disk, "Disk Operations", "operations/s", 2001, update_every, RRDSET_TYPE_LINE);
				st->isdetail = 1;

				rrddim_add(st, "reads", NULL, 1, 1 * update_every, RRDDIM_INCREMENTAL);
				rrddim_add(st, "writes", NULL, -1, 1 * update_every, RRDDIM_INCREMENTAL);
			}
			else rrdset_next_usec(st, dt);

			last_reads  = rrddim_set(st, "reads", reads);
			last_writes = rrddim_set(st, "writes", writes);
			rrdset_done(st);
		}

		// --------------------------------------------------------------------

		if(do_util) {
			st = rrdset_find_bytype("disk_util", disk);
			if(!st) {
				st = rrdset_create("disk_util", disk, NULL, disk, "Disk Utilization", "%", 2002, update_every, RRDSET_TYPE_AREA);
				st->isdetail = 1;

				rrddim_add(st, "utilization", NULL, 1, 10 * update_every, RRDDIM_INCREMENTAL);
			}
			else rrdset_next_usec(st, dt);

			last_iosms = rrddim_set(st, "utilization", iosms);
			rrdset_done(st);
		}

		// --------------------------------------------------------------------

		if(do_qsize) {
			st = rrdset_find_bytype("disk_qsize", disk);
			if(!st) {
				st = rrdset_create("disk_qsize", disk, NULL, disk, "Disk Average Queue Size", "queue size", 2003, update_every, RRDSET_TYPE_AREA);
				st->isdetail = 1;

				rrddim_add(st, "queue_size", NULL, 1, 1000 * update_every, RRDDIM_INCREMENTAL);
			}
			else rrdset_next_usec(st, dt);

			rrddim_set(st, "queue_size", wiosms);
			rrdset_done(st);
		}

		// --------------------------------------------------------------------

		if(do_cur_ops) {
			st = rrdset_find_bytype("disk_cur_ops", disk);
			if(!st) {
				st = rrdset_create("disk_cur_ops", disk, NULL, disk, "Current Disk I/O operations", "operations", 2004, update_every, RRDSET_TYPE_LINE);
				st->isdetail = 1;

				rrddim_add(st, "operations", NULL, 1, 1, RRDDIM_ABSOLUTE);
			}
			else rrdset_next_usec(st, dt);

			rrddim_set(st, "operations", currentios);
			rrdset_done(st);
		}

		// --------------------------------------------------------------------

		if(do_merged_ops) {
			st = rrdset_find_bytype("disk_merged_ops", disk);
			if(!st) {
				st = rrdset_create("disk_merged_ops", disk, NULL, disk, "Disk Merged Operations", "operations/s", 2021, update_every, RRDSET_TYPE_LINE);
				st->isdetail = 1;

				rrddim_add(st, "reads", NULL, 1, 1 * update_every, RRDDIM_INCREMENTAL);
				rrddim_add(st, "writes", NULL, -1, 1 * update_every, RRDDIM_INCREMENTAL);
			}
			else rrdset_next_usec(st, dt);

			rrddim_set(st, "reads", reads_merged);
			rrddim_set(st, "writes", writes_merged);
			rrdset_done(st);
		}

		// --------------------------------------------------------------------

		if(do_iotime) {
			st = rrdset_find_bytype("disk_iotime", disk);
			if(!st) {
				st = rrdset_create("disk_iotime", disk, NULL, disk, "Disk I/O Time", "milliseconds/s", 2022, update_every, RRDSET_TYPE_LINE);
				st->isdetail = 1;

				rrddim_add(st, "reads", NULL, 1, 1 * update_every, RRDDIM_INCREMENTAL);
				rrddim_add(st, "writes", NULL, -1, 1 * update_every, RRDDIM_INCREMENTAL);
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
			if(do_iotime && do_ops) {
				st = rrdset_find_bytype("disk_await", disk);
				if(!st) {
					st = rrdset_create("disk_await", disk, NULL, disk, "Average Wait Time", "ms per operation", 2005, update_every, RRDSET_TYPE_AREA);
					st->isdetail = 1;

					rrddim_add(st, "reads", NULL, 1, 1, RRDDIM_ABSOLUTE);
					rrddim_add(st, "writes", NULL, -1, 1, RRDDIM_ABSOLUTE);
				}
				else rrdset_next_usec(st, dt);

				rrddim_set(st, "reads", (reads - last_reads) ? (readms - last_readms) / (reads - last_reads) : 0);
				rrddim_set(st, "writes", (writes - last_writes) ? (writems - last_writems) / (writes - last_writes) : 0);
				rrdset_done(st);
			}

			if(do_io && do_ops) {
				st = rrdset_find_bytype("disk_avgsz", disk);
				if(!st) {
					st = rrdset_create("disk_avgsz", disk, NULL, disk, "Average Operation Size", "kilobytes", 2006, update_every, RRDSET_TYPE_AREA);
					st->isdetail = 1;

					rrddim_add(st, "reads", NULL, sector_size, 1024, RRDDIM_ABSOLUTE);
					rrddim_add(st, "writes", NULL, -sector_size, 1024, RRDDIM_ABSOLUTE);
				}
				else rrdset_next_usec(st, dt);

				rrddim_set(st, "reads", (reads - last_reads) ? (readsectors - last_readsectors) / (reads - last_reads) : 0);
				rrddim_set(st, "writes", (writes - last_writes) ? (writesectors - last_writesectors) / (writes - last_writes) : 0);
				rrdset_done(st);
			}

			if(do_util && do_ops) {
				st = rrdset_find_bytype("disk_svctm", disk);
				if(!st) {
					st = rrdset_create("disk_svctm", disk, NULL, disk, "Average Service Time", "ms per operation", 2007, update_every, RRDSET_TYPE_AREA);
					st->isdetail = 1;

					rrddim_add(st, "svctm", NULL, -1, 1, RRDDIM_ABSOLUTE);
				}
				else rrdset_next_usec(st, dt);

				rrddim_set(st, "svctm", ((reads - last_reads) + (writes - last_writes)) ? (iosms - last_iosms) / ((reads - last_reads) + (writes - last_writes)) : 0);
				rrdset_done(st);
			}
		}

		// TODO
		// the differential (readms + writems) / (reads + writes) = average wait time
		// the differential (readms ) / (reads ) = the average wait time for reads
		// the differential (writems) / (writes) = the average wait time for writes

		// similarly for bytes to find the average read and write size

		// svctm = % of utilization per request (reads + writes)
	}
	
	return 0;
}
