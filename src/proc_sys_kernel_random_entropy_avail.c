#ifdef HAVE_CONFIG_H
#include <config.h>
#endif
#include <stdio.h>
#include <stdlib.h>

#include "common.h"
#include "appconfig.h"
#include "procfile.h"
#include "rrd.h"
#include "plugin_proc.h"

int do_proc_sys_kernel_random_entropy_avail(int update_every, unsigned long long dt) {
	static procfile *ff = NULL;

	if(dt) {} ;

	if(!ff) {
		char filename[FILENAME_MAX + 1];
		snprintfz(filename, FILENAME_MAX, "%s%s", global_host_prefix, "/proc/sys/kernel/random/entropy_avail");
		ff = procfile_open(config_get("plugin:proc:/proc/sys/kernel/random/entropy_avail", "filename to monitor", filename), "", PROCFILE_FLAG_DEFAULT);
	}
	if(!ff) return 1;

	ff = procfile_readall(ff);
	if(!ff) return 0; // we return 0, so that we will retry to open it next time

	unsigned long long entropy = strtoull(procfile_lineword(ff, 0, 0), NULL, 10);

	RRDSET *st = rrdset_find_bytype("system", "entropy");
	if(!st) {
		st = rrdset_create("system", "entropy", NULL, "entropy", NULL, "Available Entropy", "entropy", 1000, update_every, RRDSET_TYPE_LINE);
		rrddim_add(st, "entropy", NULL, 1, 1, RRDDIM_ABSOLUTE);
	}
	else rrdset_next(st);

	rrddim_set(st, "entropy", entropy);
	rrdset_done(st);

	return 0;
}
