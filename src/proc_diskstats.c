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

	static int enable_new_disks = -1;
	static int do_io = -1, do_ops = -1, do_merged_ops = -1, do_iotime = -1, do_cur_ops = -1;

	if(enable_new_disks == -1)	enable_new_disks = config_get_boolean("plugin:proc:/proc/diskstats", "enable new disks detected at runtime", 1);

	if(do_io == -1)			do_io 			= config_get_boolean("plugin:proc:/proc/diskstats", "bandwidth for all disks", 1);
	if(do_ops == -1)		do_ops 			= config_get_boolean("plugin:proc:/proc/diskstats", "operations for all disks", 1);
	if(do_merged_ops == -1)	do_merged_ops 	= config_get_boolean("plugin:proc:/proc/diskstats", "merged operations for all disks", 1);
	if(do_iotime == -1)		do_iotime 		= config_get_boolean("plugin:proc:/proc/diskstats", "i/o time for all disks", 1);
	if(do_cur_ops == -1)	do_cur_ops 		= config_get_boolean("plugin:proc:/proc/diskstats", "current operations for all disks", 1);

	if(dt) {};

	if(!ff) ff = procfile_open("/proc/diskstats", " \t", PROCFILE_FLAG_DEFAULT);
	if(!ff) return 1;

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

		words = procfile_linewords(ff, l);
		if(words < 14) continue;

		major 			= strtoull(procfile_lineword(ff, l, 0), NULL, 10);
		minor 			= strtoull(procfile_lineword(ff, l, 1), NULL, 10);
		disk 			= procfile_lineword(ff, l, 2);
		reads 			= strtoull(procfile_lineword(ff, l, 3), NULL, 10);
		reads_merged 	= strtoull(procfile_lineword(ff, l, 4), NULL, 10);
		readsectors 	= strtoull(procfile_lineword(ff, l, 5), NULL, 10);
		readms 			= strtoull(procfile_lineword(ff, l, 6), NULL, 10);
		writes 			= strtoull(procfile_lineword(ff, l, 7), NULL, 10);
		writes_merged 	= strtoull(procfile_lineword(ff, l, 8), NULL, 10);
		writesectors 	= strtoull(procfile_lineword(ff, l, 9), NULL, 10);
		writems 		= strtoull(procfile_lineword(ff, l, 10), NULL, 10);
		currentios 		= strtoull(procfile_lineword(ff, l, 11), NULL, 10);
		iosms 			= strtoull(procfile_lineword(ff, l, 12), NULL, 10);
		wiosms 			= strtoull(procfile_lineword(ff, l, 13), NULL, 10);

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

		if(do_io) {
			st = rrdset_find_bytype(RRD_TYPE_DISK, disk);
			if(!st) {
				char tf[FILENAME_MAX + 1], *t;
				char ssfilename[FILENAME_MAX + 1];
				int sector_size = 512;

				strncpy(tf, disk, FILENAME_MAX);
				tf[FILENAME_MAX] = '\0';

				// replace all / with !
				while((t = strchr(tf, '/'))) *t = '!';

				snprintf(ssfilename, FILENAME_MAX, "/sys/block/%s/queue/hw_sector_size", tf);
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
			else rrdset_next(st);

			rrddim_set(st, "reads", readsectors);
			rrddim_set(st, "writes", writesectors);
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
			else rrdset_next(st);

			rrddim_set(st, "reads", reads);
			rrddim_set(st, "writes", writes);
			rrdset_done(st);
		}
		
		// --------------------------------------------------------------------

		if(do_merged_ops) {
			st = rrdset_find_bytype("disk_merged_ops", disk);
			if(!st) {
				st = rrdset_create("disk_merged_ops", disk, NULL, disk, "Merged Disk Operations", "operations/s", 2010, update_every, RRDSET_TYPE_LINE);
				st->isdetail = 1;

				rrddim_add(st, "reads", NULL, 1, 1 * update_every, RRDDIM_INCREMENTAL);
				rrddim_add(st, "writes", NULL, -1, 1 * update_every, RRDDIM_INCREMENTAL);
			}
			else rrdset_next(st);

			rrddim_set(st, "reads", reads_merged);
			rrddim_set(st, "writes", writes_merged);
			rrdset_done(st);
		}

		// --------------------------------------------------------------------

		if(do_iotime) {
			st = rrdset_find_bytype("disk_iotime", disk);
			if(!st) {
				st = rrdset_create("disk_iotime", disk, NULL, disk, "Disk I/O Time", "milliseconds/s", 2005, update_every, RRDSET_TYPE_LINE);
				st->isdetail = 1;

				rrddim_add(st, "reads", NULL, 1, 1 * update_every, RRDDIM_INCREMENTAL);
				rrddim_add(st, "writes", NULL, -1, 1 * update_every, RRDDIM_INCREMENTAL);
				rrddim_add(st, "latency", NULL, 1, 1 * update_every, RRDDIM_INCREMENTAL);
				rrddim_add(st, "weighted", NULL, 1, 1 * update_every, RRDDIM_INCREMENTAL);
			}
			else rrdset_next(st);

			rrddim_set(st, "reads", readms);
			rrddim_set(st, "writes", writems);
			rrddim_set(st, "latency", iosms);
			rrddim_set(st, "weighted", wiosms);
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
			else rrdset_next(st);

			rrddim_set(st, "operations", currentios);
			rrdset_done(st);
		}
	}
	
	return 0;
}
