#include <inttypes.h>
#include <stdio.h>
#include <stdlib.h>
#include <string.h>
#include <unistd.h>

#include "common.h"
#include "log.h"
#include "config.h"
#include "procfile.h"
#include "rrd.h"
#include "plugin_proc.h"

#define MAX_PROC_VMSTAT_LINE 4096
#define MAX_PROC_VMSTAT_NAME 1024

int do_proc_vmstat(int update_every, unsigned long long dt) {
	static procfile *ff = NULL;
	static int do_swapio = -1, do_io = -1, do_pgfaults = -1;

	if(do_swapio == -1)	do_swapio = config_get_boolean("plugin:proc:/proc/vmstat", "swap i/o", 1);
	if(do_io == -1)		do_io = config_get_boolean("plugin:proc:/proc/vmstat", "disk i/o", 1);
	if(do_pgfaults == -1)	do_pgfaults = config_get_boolean("plugin:proc:/proc/vmstat", "memory page faults", 1);

	if(dt) {};

	if(!ff) ff = procfile_open("/proc/vmstat", " \t:", PROCFILE_FLAG_DEFAULT);
	if(!ff) return 1;

	ff = procfile_readall(ff);
	if(!ff) return 0; // we return 0, so that we will retry to open it next time

	uint32_t lines = procfile_lines(ff), l;
	uint32_t words;

	unsigned long long nr_free_pages = 0, nr_inactive_anon = 0, nr_active_anon = 0, nr_inactive_file = 0, nr_active_file = 0, nr_unevictable = 0, nr_mlock = 0,
		nr_anon_pages = 0, nr_mapped = 0, nr_file_pages = 0, nr_dirty = 0, nr_writeback = 0, nr_slab_reclaimable = 0, nr_slab_unreclaimable = 0, nr_page_table_pages = 0,
		nr_kernel_stack = 0, nr_unstable = 0, nr_bounce = 0, nr_vmscan_write = 0, nr_vmscan_immediate_reclaim = 0, nr_writeback_temp = 0, nr_isolated_anon = 0, nr_isolated_file = 0,
		nr_shmem = 0, nr_dirtied = 0, nr_written = 0, nr_anon_transparent_hugepages = 0, nr_dirty_threshold = 0, nr_dirty_background_threshold = 0,
		pgpgin = 0, pgpgout = 0, pswpin = 0, pswpout = 0, pgalloc_dma = 0, pgalloc_dma32 = 0, pgalloc_normal = 0, pgalloc_movable = 0, pgfree = 0, pgactivate = 0, pgdeactivate = 0,
		pgfault = 0, pgmajfault = 0, pgrefill_dma = 0, pgrefill_dma32 = 0, pgrefill_normal = 0, pgrefill_movable = 0, pgsteal_kswapd_dma = 0, pgsteal_kswapd_dma32 = 0,
		pgsteal_kswapd_normal = 0, pgsteal_kswapd_movable = 0, pgsteal_direct_dma = 0, pgsteal_direct_dma32 = 0, pgsteal_direct_normal = 0, pgsteal_direct_movable = 0, 
		pgscan_kswapd_dma = 0, pgscan_kswapd_dma32 = 0, pgscan_kswapd_normal = 0, pgscan_kswapd_movable = 0, pgscan_direct_dma = 0, pgscan_direct_dma32 = 0, pgscan_direct_normal = 0,
		pgscan_direct_movable = 0, pginodesteal = 0, slabs_scanned = 0, kswapd_inodesteal = 0, kswapd_low_wmark_hit_quickly = 0, kswapd_high_wmark_hit_quickly = 0,
		kswapd_skip_congestion_wait = 0, pageoutrun = 0, allocstall = 0, pgrotated = 0, compact_blocks_moved = 0, compact_pages_moved = 0, compact_pagemigrate_failed = 0,
		compact_stall = 0, compact_fail = 0, compact_success = 0, htlb_buddy_alloc_success = 0, htlb_buddy_alloc_fail = 0, unevictable_pgs_culled = 0, unevictable_pgs_scanned = 0,
		unevictable_pgs_rescued = 0, unevictable_pgs_mlocked = 0, unevictable_pgs_munlocked = 0, unevictable_pgs_cleared = 0, unevictable_pgs_stranded = 0, unevictable_pgs_mlockfreed = 0,
		thp_fault_alloc = 0, thp_fault_fallback = 0, thp_collapse_alloc = 0, thp_collapse_alloc_failed = 0, thp_split = 0;

	for(l = 0; l < lines ;l++) {
		words = procfile_linewords(ff, l);
		if(words < 2) {
			if(words) error("Cannot read /proc/vmstat line %d. Expected 2 params, read %d.", l, words);
			continue;
		}

		char *name = procfile_lineword(ff, l, 0);
		unsigned long long value = strtoull(procfile_lineword(ff, l, 1), NULL, 10);

		     if(!nr_free_pages && strcmp(name, "nr_free_pages") == 0) nr_free_pages = value;
		else if(!nr_inactive_anon && strcmp(name, "nr_inactive_anon") == 0) nr_inactive_anon = value;
		else if(!nr_active_anon && strcmp(name, "nr_active_anon") == 0) nr_active_anon = value;
		else if(!nr_inactive_file && strcmp(name, "nr_inactive_file") == 0) nr_inactive_file = value;
		else if(!nr_active_file && strcmp(name, "nr_active_file") == 0) nr_active_file = value;
		else if(!nr_unevictable && strcmp(name, "nr_unevictable") == 0) nr_unevictable = value;
		else if(!nr_mlock && strcmp(name, "nr_mlock") == 0) nr_mlock = value;
		else if(!nr_anon_pages && strcmp(name, "nr_anon_pages") == 0) nr_anon_pages = value;
		else if(!nr_mapped && strcmp(name, "nr_mapped") == 0) nr_mapped = value;
		else if(!nr_file_pages && strcmp(name, "nr_file_pages") == 0) nr_file_pages = value;
		else if(!nr_dirty && strcmp(name, "nr_dirty") == 0) nr_dirty = value;
		else if(!nr_writeback && strcmp(name, "nr_writeback") == 0) nr_writeback = value;
		else if(!nr_slab_reclaimable && strcmp(name, "nr_slab_reclaimable") == 0) nr_slab_reclaimable = value;
		else if(!nr_slab_unreclaimable && strcmp(name, "nr_slab_unreclaimable") == 0) nr_slab_unreclaimable = value;
		else if(!nr_page_table_pages && strcmp(name, "nr_page_table_pages") == 0) nr_page_table_pages = value;
		else if(!nr_kernel_stack && strcmp(name, "nr_kernel_stack") == 0) nr_kernel_stack = value;
		else if(!nr_unstable && strcmp(name, "nr_unstable") == 0) nr_unstable = value;
		else if(!nr_bounce && strcmp(name, "nr_bounce") == 0) nr_bounce = value;
		else if(!nr_vmscan_write && strcmp(name, "nr_vmscan_write") == 0) nr_vmscan_write = value;
		else if(!nr_vmscan_immediate_reclaim && strcmp(name, "nr_vmscan_immediate_reclaim") == 0) nr_vmscan_immediate_reclaim = value;
		else if(!nr_writeback_temp && strcmp(name, "nr_writeback_temp") == 0) nr_writeback_temp = value;
		else if(!nr_isolated_anon && strcmp(name, "nr_isolated_anon") == 0) nr_isolated_anon = value;
		else if(!nr_isolated_file && strcmp(name, "nr_isolated_file") == 0) nr_isolated_file = value;
		else if(!nr_shmem && strcmp(name, "nr_shmem") == 0) nr_shmem = value;
		else if(!nr_dirtied && strcmp(name, "nr_dirtied") == 0) nr_dirtied = value;
		else if(!nr_written && strcmp(name, "nr_written") == 0) nr_written = value;
		else if(!nr_anon_transparent_hugepages && strcmp(name, "nr_anon_transparent_hugepages") == 0) nr_anon_transparent_hugepages = value;
		else if(!nr_dirty_threshold && strcmp(name, "nr_dirty_threshold") == 0) nr_dirty_threshold = value;
		else if(!nr_dirty_background_threshold && strcmp(name, "nr_dirty_background_threshold") == 0) nr_dirty_background_threshold = value;
		else if(!pgpgin && strcmp(name, "pgpgin") == 0) pgpgin = value;
		else if(!pgpgout && strcmp(name, "pgpgout") == 0) pgpgout = value;
		else if(!pswpin && strcmp(name, "pswpin") == 0) pswpin = value;
		else if(!pswpout && strcmp(name, "pswpout") == 0) pswpout = value;
		else if(!pgalloc_dma && strcmp(name, "pgalloc_dma") == 0) pgalloc_dma = value;
		else if(!pgalloc_dma32 && strcmp(name, "pgalloc_dma32") == 0) pgalloc_dma32 = value;
		else if(!pgalloc_normal && strcmp(name, "pgalloc_normal") == 0) pgalloc_normal = value;
		else if(!pgalloc_movable && strcmp(name, "pgalloc_movable") == 0) pgalloc_movable = value;
		else if(!pgfree && strcmp(name, "pgfree") == 0) pgfree = value;
		else if(!pgactivate && strcmp(name, "pgactivate") == 0) pgactivate = value;
		else if(!pgdeactivate && strcmp(name, "pgdeactivate") == 0) pgdeactivate = value;
		else if(!pgfault && strcmp(name, "pgfault") == 0) pgfault = value;
		else if(!pgmajfault && strcmp(name, "pgmajfault") == 0) pgmajfault = value;
		else if(!pgrefill_dma && strcmp(name, "pgrefill_dma") == 0) pgrefill_dma = value;
		else if(!pgrefill_dma32 && strcmp(name, "pgrefill_dma32") == 0) pgrefill_dma32 = value;
		else if(!pgrefill_normal && strcmp(name, "pgrefill_normal") == 0) pgrefill_normal = value;
		else if(!pgrefill_movable && strcmp(name, "pgrefill_movable") == 0) pgrefill_movable = value;
		else if(!pgsteal_kswapd_dma && strcmp(name, "pgsteal_kswapd_dma") == 0) pgsteal_kswapd_dma = value;
		else if(!pgsteal_kswapd_dma32 && strcmp(name, "pgsteal_kswapd_dma32") == 0) pgsteal_kswapd_dma32 = value;
		else if(!pgsteal_kswapd_normal && strcmp(name, "pgsteal_kswapd_normal") == 0) pgsteal_kswapd_normal = value;
		else if(!pgsteal_kswapd_movable && strcmp(name, "pgsteal_kswapd_movable") == 0) pgsteal_kswapd_movable = value;
		else if(!pgsteal_direct_dma && strcmp(name, "pgsteal_direct_dma") == 0) pgsteal_direct_dma = value;
		else if(!pgsteal_direct_dma32 && strcmp(name, "pgsteal_direct_dma32") == 0) pgsteal_direct_dma32 = value;
		else if(!pgsteal_direct_normal && strcmp(name, "pgsteal_direct_normal") == 0) pgsteal_direct_normal = value;
		else if(!pgsteal_direct_movable && strcmp(name, "pgsteal_direct_movable") == 0) pgsteal_direct_movable = value;
		else if(!pgscan_kswapd_dma && strcmp(name, "pgscan_kswapd_dma") == 0) pgscan_kswapd_dma = value;
		else if(!pgscan_kswapd_dma32 && strcmp(name, "pgscan_kswapd_dma32") == 0) pgscan_kswapd_dma32 = value;
		else if(!pgscan_kswapd_normal && strcmp(name, "pgscan_kswapd_normal") == 0) pgscan_kswapd_normal = value;
		else if(!pgscan_kswapd_movable && strcmp(name, "pgscan_kswapd_movable") == 0) pgscan_kswapd_movable = value;
		else if(!pgscan_direct_dma && strcmp(name, "pgscan_direct_dma") == 0) pgscan_direct_dma = value;
		else if(!pgscan_direct_dma32 && strcmp(name, "pgscan_direct_dma32") == 0) pgscan_direct_dma32 = value;
		else if(!pgscan_direct_normal && strcmp(name, "pgscan_direct_normal") == 0) pgscan_direct_normal = value;
		else if(!pgscan_direct_movable && strcmp(name, "pgscan_direct_movable") == 0) pgscan_direct_movable = value;
		else if(!pginodesteal && strcmp(name, "pginodesteal") == 0) pginodesteal = value;
		else if(!slabs_scanned && strcmp(name, "slabs_scanned") == 0) slabs_scanned = value;
		else if(!kswapd_inodesteal && strcmp(name, "kswapd_inodesteal") == 0) kswapd_inodesteal = value;
		else if(!kswapd_low_wmark_hit_quickly && strcmp(name, "kswapd_low_wmark_hit_quickly") == 0) kswapd_low_wmark_hit_quickly = value;
		else if(!kswapd_high_wmark_hit_quickly && strcmp(name, "kswapd_high_wmark_hit_quickly") == 0) kswapd_high_wmark_hit_quickly = value;
		else if(!kswapd_skip_congestion_wait && strcmp(name, "kswapd_skip_congestion_wait") == 0) kswapd_skip_congestion_wait = value;
		else if(!pageoutrun && strcmp(name, "pageoutrun") == 0) pageoutrun = value;
		else if(!allocstall && strcmp(name, "allocstall") == 0) allocstall = value;
		else if(!pgrotated && strcmp(name, "pgrotated") == 0) pgrotated = value;
		else if(!compact_blocks_moved && strcmp(name, "compact_blocks_moved") == 0) compact_blocks_moved = value;
		else if(!compact_pages_moved && strcmp(name, "compact_pages_moved") == 0) compact_pages_moved = value;
		else if(!compact_pagemigrate_failed && strcmp(name, "compact_pagemigrate_failed") == 0) compact_pagemigrate_failed = value;
		else if(!compact_stall && strcmp(name, "compact_stall") == 0) compact_stall = value;
		else if(!compact_fail && strcmp(name, "compact_fail") == 0) compact_fail = value;
		else if(!compact_success && strcmp(name, "compact_success") == 0) compact_success = value;
		else if(!htlb_buddy_alloc_success && strcmp(name, "htlb_buddy_alloc_success") == 0) htlb_buddy_alloc_success = value;
		else if(!htlb_buddy_alloc_fail && strcmp(name, "htlb_buddy_alloc_fail") == 0) htlb_buddy_alloc_fail = value;
		else if(!unevictable_pgs_culled && strcmp(name, "unevictable_pgs_culled") == 0) unevictable_pgs_culled = value;
		else if(!unevictable_pgs_scanned && strcmp(name, "unevictable_pgs_scanned") == 0) unevictable_pgs_scanned = value;
		else if(!unevictable_pgs_rescued && strcmp(name, "unevictable_pgs_rescued") == 0) unevictable_pgs_rescued = value;
		else if(!unevictable_pgs_mlocked && strcmp(name, "unevictable_pgs_mlocked") == 0) unevictable_pgs_mlocked = value;
		else if(!unevictable_pgs_munlocked && strcmp(name, "unevictable_pgs_munlocked") == 0) unevictable_pgs_munlocked = value;
		else if(!unevictable_pgs_cleared && strcmp(name, "unevictable_pgs_cleared") == 0) unevictable_pgs_cleared = value;
		else if(!unevictable_pgs_stranded && strcmp(name, "unevictable_pgs_stranded") == 0) unevictable_pgs_stranded = value;
		else if(!unevictable_pgs_mlockfreed && strcmp(name, "unevictable_pgs_mlockfreed") == 0) unevictable_pgs_mlockfreed = value;
		else if(!thp_fault_alloc && strcmp(name, "thp_fault_alloc") == 0) thp_fault_alloc = value;
		else if(!thp_fault_fallback && strcmp(name, "thp_fault_fallback") == 0) thp_fault_fallback = value;
		else if(!thp_collapse_alloc && strcmp(name, "thp_collapse_alloc") == 0) thp_collapse_alloc = value;
		else if(!thp_collapse_alloc_failed && strcmp(name, "thp_collapse_alloc_failed") == 0) thp_collapse_alloc_failed = value;
		else if(!thp_split && strcmp(name, "thp_split") == 0) thp_split = value;
	}

	RRDSET *st;

	// --------------------------------------------------------------------
	
	if(do_swapio) {
		st = rrdset_find("system.swapio");
		if(!st) {
			st = rrdset_create("system", "swapio", NULL, "mem", "Swap I/O", "kilobytes/s", 250, update_every, RRDSET_TYPE_AREA);

			rrddim_add(st, "in",  NULL, sysconf(_SC_PAGESIZE), 1024 * update_every, RRDDIM_INCREMENTAL);
			rrddim_add(st, "out", NULL, -sysconf(_SC_PAGESIZE), 1024 * update_every, RRDDIM_INCREMENTAL);
		}
		else rrdset_next(st);

		rrddim_set(st, "in", pswpin);
		rrddim_set(st, "out", pswpout);
		rrdset_done(st);
	}

	// --------------------------------------------------------------------
	
	if(do_io) {
		st = rrdset_find("system.io");
		if(!st) {
			st = rrdset_create("system", "io", NULL, "disk", "Disk I/O", "kilobytes/s", 150, update_every, RRDSET_TYPE_AREA);

			rrddim_add(st, "in",  NULL,  1, 1 * update_every, RRDDIM_INCREMENTAL);
			rrddim_add(st, "out", NULL, -1, 1 * update_every, RRDDIM_INCREMENTAL);
		}
		else rrdset_next(st);

		rrddim_set(st, "in", pgpgin);
		rrddim_set(st, "out", pgpgout);
		rrdset_done(st);
	}

	// --------------------------------------------------------------------
	
	if(do_pgfaults) {
		st = rrdset_find("system.pgfaults");
		if(!st) {
			st = rrdset_create("system", "pgfaults", NULL, "mem", "Memory Page Faults", "page faults/s", 500, update_every, RRDSET_TYPE_LINE);
			st->isdetail = 1;

			rrddim_add(st, "minor",  NULL,  1, 1 * update_every, RRDDIM_INCREMENTAL);
			rrddim_add(st, "major", NULL, -1, 1 * update_every, RRDDIM_INCREMENTAL);
		}
		else rrdset_next(st);

		rrddim_set(st, "minor", pgfault);
		rrddim_set(st, "major", pgmajfault);
		rrdset_done(st);
	}

	return 0;
}

