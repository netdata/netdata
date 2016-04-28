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

#include "proc_self_mountinfo.h"

// find the mount info with the given major:minor
// in the supplied linked list of mountinfo structures
struct mountinfo *mountinfo_find(struct mountinfo *root, unsigned long major, unsigned long minor) {
	struct mountinfo *mi;

	for(mi = root; mi ; mi = mi->next)
		if(mi->major == major && mi->minor == minor)
			return mi;

	return NULL;
}

// free a linked list of mountinfo structures
void mountinfo_free(struct mountinfo *mi) {
	if(unlikely(!mi))
		return;

	if(likely(mi->next))
		mountinfo_free(mi->next);

	free(mi->root);
	free(mi->mount_point);
	free(mi->mount_options);

	if(mi->optional_fields_count) {
		int i;
		for(i = 0; i < mi->optional_fields_count ; i++)
			free(mi->optional_fields[i]);
	}
	free(mi->optional_fields);

	free(mi->filesystem);
	free(mi->mount_source);
	free(mi->super_options);
	free(mi);
}

// read the whole mountinfo into a linked list
struct mountinfo *mountinfo_read() {
	procfile *ff = NULL;

	char filename[FILENAME_MAX + 1];
	snprintf(filename, FILENAME_MAX, "%s/proc/self/mountinfo", global_host_prefix);
	ff = procfile_open(filename, " \t", PROCFILE_FLAG_DEFAULT);
	if(!ff) {
		snprintf(filename, FILENAME_MAX, "%s/proc/1/mountinfo", global_host_prefix);
		ff = procfile_open(filename, " \t", PROCFILE_FLAG_DEFAULT);
		if(!ff) return NULL;
	}

	ff = procfile_readall(ff);
	if(!ff) return NULL;

	struct mountinfo *root = NULL, *last = NULL, *mi = NULL;

	unsigned long l, lines = procfile_lines(ff);
	error("MOUNTINFO: file has %u lines", lines);
	for(l = 0; l < lines ;l++) {
		if(procfile_linewords(ff, l) < 5)
			continue;

		mi = malloc(sizeof(struct mountinfo));
		if(unlikely(!mi)) fatal("Cannot allocate memory for mountinfo");

		if(unlikely(!root))
			root = last = mi;
		else
			last->next = mi;

		last = mi;
		mi->next = NULL;

		unsigned long w = 0;
		mi->id = strtoul(procfile_lineword(ff, l, w), NULL, 10); w++;
		mi->parentid = strtoul(procfile_lineword(ff, l, w), NULL, 10); w++;

		char *major = procfile_lineword(ff, l, w), *minor; w++;
		for(minor = major; *minor && *minor != ':' ;minor++) ;
		*minor = '\0';
		minor++;

		mi->major = strtoul(major, NULL, 10);
		mi->minor = strtoul(minor, NULL, 10);

		mi->root = strdup(procfile_lineword(ff, l, w)); w++;
		if(unlikely(!mi->root)) fatal("Cannot allocate memory");

		mi->mount_point = strdup(procfile_lineword(ff, l, w)); w++;
		if(unlikely(!mi->mount_point)) fatal("Cannot allocate memory");

		mi->mount_options = strdup(procfile_lineword(ff, l, w)); w++;
		if(unlikely(!mi->mount_options)) fatal("Cannot allocate memory");

		// count the optional fields
		unsigned long wo = w;
		mi->optional_fields_count = 0;
		char *s = procfile_lineword(ff, l, w);
		while(*s && *s != '-') {
			w++;
			s = procfile_lineword(ff, l, w);
			mi->optional_fields_count++;
		}

		if(unlikely(mi->optional_fields_count)) {
			// we have some optional fields
			// read them into a new array of pointers;

			mi->optional_fields = malloc(mi->optional_fields_count * sizeof(char *));
			if(unlikely(!mi->optional_fields))
				fatal("Cannot allocate memory for %d mountinfo optional fields", mi->optional_fields_count);

			int i;
			for(i = 0; i < mi->optional_fields_count ; i++) {
				mi->optional_fields[wo] = strdup(procfile_lineword(ff, l, w));
				if(!mi->optional_fields[wo]) fatal("Cannot allocate memory");
				wo++;
			}
		}
		else
			mi->optional_fields = NULL;

		if(likely(*s == '-')) {
			mi->filesystem = strdup(procfile_lineword(ff, l, w)); w++;
			if(!mi->filesystem) fatal("Cannot allocate memory");

			mi->mount_source = strdup(procfile_lineword(ff, l, w)); w++;
			if(!mi->mount_source) fatal("Cannot allocate memory");

			mi->super_options = strdup(procfile_lineword(ff, l, w)); w++;
			if(!mi->super_options) fatal("Cannot allocate memory");
		}
		else {
			mi->filesystem = NULL;
			mi->mount_source = NULL;
			mi->super_options = NULL;
		}

		//info("MOUNTINFO: %u %u %u:%u root '%s', mount point '%s', mount options '%s', filesystem '%s', mount source '%s', super options '%s'",
		//     mi->id,
		//     mi->parentid,
		//     mi->major,
		//     mi->minor,
		//     mi->root,
		//     mi->mount_point,
		//     mi->mount_options,
		//     mi->filesystem,
		//     mi->mount_source,
		//     mi->super_options
		//);
	}

	procfile_close(ff);
	return root;
}
