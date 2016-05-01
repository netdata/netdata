#ifdef HAVE_CONFIG_H
#include <config.h>
#endif
#include <stdio.h>
#include <stdlib.h>
#include <string.h>

#include "common.h"
#include "log.h"
#include "appconfig.h"
#include "procfile.h"
#include "rrd.h"
#include "plugin_proc.h"

#define MAX_PROC_MEMINFO_LINE 4096
#define MAX_PROC_MEMINFO_NAME 1024

int do_proc_meminfo(int update_every, unsigned long long dt) {
	static procfile *ff = NULL;

	static int do_ram = -1, do_swap = -1, do_hwcorrupt = -1, do_committed = -1, do_writeback = -1, do_kernel = -1, do_slab = -1;

	if(do_ram == -1)		do_ram 			= config_get_boolean("plugin:proc:/proc/meminfo", "system ram", 1);
	if(do_swap == -1)		do_swap 		= config_get_boolean("plugin:proc:/proc/meminfo", "system swap", 1);
	if(do_hwcorrupt == -1)	do_hwcorrupt 	= config_get_boolean_ondemand("plugin:proc:/proc/meminfo", "hardware corrupted ECC", CONFIG_ONDEMAND_ONDEMAND);
	if(do_committed == -1)	do_committed 	= config_get_boolean("plugin:proc:/proc/meminfo", "committed memory", 1);
	if(do_writeback == -1)	do_writeback 	= config_get_boolean("plugin:proc:/proc/meminfo", "writeback memory", 1);
	if(do_kernel == -1)		do_kernel 		= config_get_boolean("plugin:proc:/proc/meminfo", "kernel memory", 1);
	if(do_slab == -1)		do_slab 		= config_get_boolean("plugin:proc:/proc/meminfo", "slab memory", 1);

	if(dt) {};

	if(!ff) {
		char filename[FILENAME_MAX + 1];
		snprintfz(filename, FILENAME_MAX, "%s%s", global_host_prefix, "/proc/meminfo");
		ff = procfile_open(config_get("plugin:proc:/proc/meminfo", "filename to monitor", filename), " \t:", PROCFILE_FLAG_DEFAULT);
	}
	if(!ff) return 1;

	ff = procfile_readall(ff);
	if(!ff) return 0; // we return 0, so that we will retry to open it next time

	uint32_t lines = procfile_lines(ff), l;
	uint32_t words;

	int hwcorrupted = 0;

	unsigned long long MemTotal = 0, MemFree = 0, Buffers = 0, Cached = 0, SwapCached = 0,
		Active = 0, Inactive = 0, ActiveAnon = 0, InactiveAnon = 0, ActiveFile = 0, InactiveFile = 0,
		Unevictable = 0, Mlocked = 0, SwapTotal = 0, SwapFree = 0, Dirty = 0, Writeback = 0, AnonPages = 0,
		Mapped = 0, Shmem = 0, Slab = 0, SReclaimable = 0, SUnreclaim = 0, KernelStack = 0, PageTables = 0,
		NFS_Unstable = 0, Bounce = 0, WritebackTmp = 0, CommitLimit = 0, Committed_AS = 0,
		VmallocTotal = 0, VmallocUsed = 0, VmallocChunk = 0,
		AnonHugePages = 0, HugePages_Total = 0, HugePages_Free = 0, HugePages_Rsvd = 0, HugePages_Surp = 0, Hugepagesize = 0,
		DirectMap4k = 0, DirectMap2M = 0, HardwareCorrupted = 0;

	for(l = 0; l < lines ;l++) {
		words = procfile_linewords(ff, l);
		if(words < 2) continue;

		char *name = procfile_lineword(ff, l, 0);
		unsigned long long value = strtoull(procfile_lineword(ff, l, 1), NULL, 10);

		     if(!MemTotal && strcmp(name, "MemTotal") == 0) MemTotal = value;
		else if(!MemFree && strcmp(name, "MemFree") == 0) MemFree = value;
		else if(!Buffers && strcmp(name, "Buffers") == 0) Buffers = value;
		else if(!Cached && strcmp(name, "Cached") == 0) Cached = value;
		else if(!SwapCached && strcmp(name, "SwapCached") == 0) SwapCached = value;
		else if(!Active && strcmp(name, "Active") == 0) Active = value;
		else if(!Inactive && strcmp(name, "Inactive") == 0) Inactive = value;
		else if(!ActiveAnon && strcmp(name, "ActiveAnon") == 0) ActiveAnon = value;
		else if(!InactiveAnon && strcmp(name, "InactiveAnon") == 0) InactiveAnon = value;
		else if(!ActiveFile && strcmp(name, "ActiveFile") == 0) ActiveFile = value;
		else if(!InactiveFile && strcmp(name, "InactiveFile") == 0) InactiveFile = value;
		else if(!Unevictable && strcmp(name, "Unevictable") == 0) Unevictable = value;
		else if(!Mlocked && strcmp(name, "Mlocked") == 0) Mlocked = value;
		else if(!SwapTotal && strcmp(name, "SwapTotal") == 0) SwapTotal = value;
		else if(!SwapFree && strcmp(name, "SwapFree") == 0) SwapFree = value;
		else if(!Dirty && strcmp(name, "Dirty") == 0) Dirty = value;
		else if(!Writeback && strcmp(name, "Writeback") == 0) Writeback = value;
		else if(!AnonPages && strcmp(name, "AnonPages") == 0) AnonPages = value;
		else if(!Mapped && strcmp(name, "Mapped") == 0) Mapped = value;
		else if(!Shmem && strcmp(name, "Shmem") == 0) Shmem = value;
		else if(!Slab && strcmp(name, "Slab") == 0) Slab = value;
		else if(!SReclaimable && strcmp(name, "SReclaimable") == 0) SReclaimable = value;
		else if(!SUnreclaim && strcmp(name, "SUnreclaim") == 0) SUnreclaim = value;
		else if(!KernelStack && strcmp(name, "KernelStack") == 0) KernelStack = value;
		else if(!PageTables && strcmp(name, "PageTables") == 0) PageTables = value;
		else if(!NFS_Unstable && strcmp(name, "NFS_Unstable") == 0) NFS_Unstable = value;
		else if(!Bounce && strcmp(name, "Bounce") == 0) Bounce = value;
		else if(!WritebackTmp && strcmp(name, "WritebackTmp") == 0) WritebackTmp = value;
		else if(!CommitLimit && strcmp(name, "CommitLimit") == 0) CommitLimit = value;
		else if(!Committed_AS && strcmp(name, "Committed_AS") == 0) Committed_AS = value;
		else if(!VmallocTotal && strcmp(name, "VmallocTotal") == 0) VmallocTotal = value;
		else if(!VmallocUsed && strcmp(name, "VmallocUsed") == 0) VmallocUsed = value;
		else if(!VmallocChunk && strcmp(name, "VmallocChunk") == 0) VmallocChunk = value;
		else if(!HardwareCorrupted && strcmp(name, "HardwareCorrupted") == 0) { HardwareCorrupted = value; hwcorrupted = 1; }
		else if(!AnonHugePages && strcmp(name, "AnonHugePages") == 0) AnonHugePages = value;
		else if(!HugePages_Total && strcmp(name, "HugePages_Total") == 0) HugePages_Total = value;
		else if(!HugePages_Free && strcmp(name, "HugePages_Free") == 0) HugePages_Free = value;
		else if(!HugePages_Rsvd && strcmp(name, "HugePages_Rsvd") == 0) HugePages_Rsvd = value;
		else if(!HugePages_Surp && strcmp(name, "HugePages_Surp") == 0) HugePages_Surp = value;
		else if(!Hugepagesize && strcmp(name, "Hugepagesize") == 0) Hugepagesize = value;
		else if(!DirectMap4k && strcmp(name, "DirectMap4k") == 0) DirectMap4k = value;
		else if(!DirectMap2M && strcmp(name, "DirectMap2M") == 0) DirectMap2M = value;
	}

	RRDSET *st;

	// --------------------------------------------------------------------

	// http://stackoverflow.com/questions/3019748/how-to-reliably-measure-available-memory-in-linux
	unsigned long long MemUsed = MemTotal - MemFree - Cached - Buffers;

	if(do_ram) {
		st = rrdset_find("system.ram");
		if(!st) {
			st = rrdset_create("system", "ram", NULL, "ram", NULL, "System RAM", "MB", 200, update_every, RRDSET_TYPE_STACKED);

			rrddim_add(st, "buffers", NULL, 1, 1024, RRDDIM_ABSOLUTE);
			rrddim_add(st, "used",    NULL, 1, 1024, RRDDIM_ABSOLUTE);
			rrddim_add(st, "cached",  NULL, 1, 1024, RRDDIM_ABSOLUTE);
			rrddim_add(st, "free",    NULL, 1, 1024, RRDDIM_ABSOLUTE);
		}
		else rrdset_next(st);

		rrddim_set(st, "used", MemUsed);
		rrddim_set(st, "free", MemFree);
		rrddim_set(st, "cached", Cached);
		rrddim_set(st, "buffers", Buffers);
		rrdset_done(st);
	}

	// --------------------------------------------------------------------

	unsigned long long SwapUsed = SwapTotal - SwapFree;

	if(do_swap) {
		st = rrdset_find("system.swap");
		if(!st) {
			st = rrdset_create("system", "swap", NULL, "swap", NULL, "System Swap", "MB", 201, update_every, RRDSET_TYPE_STACKED);
			st->isdetail = 1;

			rrddim_add(st, "free",    NULL, 1, 1024, RRDDIM_ABSOLUTE);
			rrddim_add(st, "used",    NULL, 1, 1024, RRDDIM_ABSOLUTE);
		}
		else rrdset_next(st);

		rrddim_set(st, "used", SwapUsed);
		rrddim_set(st, "free", SwapFree);
		rrdset_done(st);
	}

	// --------------------------------------------------------------------

	if(hwcorrupted && do_hwcorrupt && HardwareCorrupted > 0) {
		do_hwcorrupt = CONFIG_ONDEMAND_YES;

		st = rrdset_find("mem.hwcorrupt");
		if(!st) {
			st = rrdset_create("mem", "hwcorrupt", NULL, "errors", NULL, "Hardware Corrupted ECC", "MB", 9000, update_every, RRDSET_TYPE_LINE);
			st->isdetail = 1;

			rrddim_add(st, "HardwareCorrupted", NULL, 1, 1024, RRDDIM_ABSOLUTE);
		}
		else rrdset_next(st);

		rrddim_set(st, "HardwareCorrupted", HardwareCorrupted);
		rrdset_done(st);
	}

	// --------------------------------------------------------------------

	if(do_committed) {
		st = rrdset_find("mem.committed");
		if(!st) {
			st = rrdset_create("mem", "committed", NULL, "system", NULL, "Committed (Allocated) Memory", "MB", 5000, update_every, RRDSET_TYPE_AREA);
			st->isdetail = 1;

			rrddim_add(st, "Committed_AS", NULL, 1, 1024, RRDDIM_ABSOLUTE);
		}
		else rrdset_next(st);

		rrddim_set(st, "Committed_AS", Committed_AS);
		rrdset_done(st);
	}

	// --------------------------------------------------------------------

	if(do_writeback) {
		st = rrdset_find("mem.writeback");
		if(!st) {
			st = rrdset_create("mem", "writeback", NULL, "kernel", NULL, "Writeback Memory", "MB", 4000, update_every, RRDSET_TYPE_LINE);
			st->isdetail = 1;

			rrddim_add(st, "Dirty", NULL, 1, 1024, RRDDIM_ABSOLUTE);
			rrddim_add(st, "Writeback", NULL, 1, 1024, RRDDIM_ABSOLUTE);
			rrddim_add(st, "FuseWriteback", NULL, 1, 1024, RRDDIM_ABSOLUTE);
			rrddim_add(st, "NfsWriteback", NULL, 1, 1024, RRDDIM_ABSOLUTE);
			rrddim_add(st, "Bounce", NULL, 1, 1024, RRDDIM_ABSOLUTE);
		}
		else rrdset_next(st);

		rrddim_set(st, "Dirty", Dirty);
		rrddim_set(st, "Writeback", Writeback);
		rrddim_set(st, "FuseWriteback", WritebackTmp);
		rrddim_set(st, "NfsWriteback", NFS_Unstable);
		rrddim_set(st, "Bounce", Bounce);
		rrdset_done(st);
	}

	// --------------------------------------------------------------------

	if(do_kernel) {
		st = rrdset_find("mem.kernel");
		if(!st) {
			st = rrdset_create("mem", "kernel", NULL, "kernel", NULL, "Memory Used by Kernel", "MB", 6000, update_every, RRDSET_TYPE_STACKED);
			st->isdetail = 1;

			rrddim_add(st, "Slab", NULL, 1, 1024, RRDDIM_ABSOLUTE);
			rrddim_add(st, "KernelStack", NULL, 1, 1024, RRDDIM_ABSOLUTE);
			rrddim_add(st, "PageTables", NULL, 1, 1024, RRDDIM_ABSOLUTE);
			rrddim_add(st, "VmallocUsed", NULL, 1, 1024, RRDDIM_ABSOLUTE);
		}
		else rrdset_next(st);

		rrddim_set(st, "KernelStack", KernelStack);
		rrddim_set(st, "Slab", Slab);
		rrddim_set(st, "PageTables", PageTables);
		rrddim_set(st, "VmallocUsed", VmallocUsed);
		rrdset_done(st);
	}

	// --------------------------------------------------------------------

	if(do_slab) {
		st = rrdset_find("mem.slab");
		if(!st) {
			st = rrdset_create("mem", "slab", NULL, "slab", NULL, "Reclaimable Kernel Memory", "MB", 6500, update_every, RRDSET_TYPE_STACKED);
			st->isdetail = 1;

			rrddim_add(st, "reclaimable", NULL, 1, 1024, RRDDIM_ABSOLUTE);
			rrddim_add(st, "unreclaimable", NULL, 1, 1024, RRDDIM_ABSOLUTE);
		}
		else rrdset_next(st);

		rrddim_set(st, "reclaimable", SReclaimable);
		rrddim_set(st, "unreclaimable", SUnreclaim);
		rrdset_done(st);
	}

	return 0;
}

