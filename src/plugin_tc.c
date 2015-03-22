#include <inttypes.h>
#include <stdio.h>
#include <stdlib.h>
#include <string.h>

#include "log.h"
#include "config.h"
#include "rrd.h"
#include "popen.h"
#include "plugin_tc.h"

#define RRD_TYPE_TC					"tc"
#define RRD_TYPE_TC_LEN				strlen(RRD_TYPE_TC)

// ----------------------------------------------------------------------------
// /sbin/tc processor
// this requires the script plugins.d/tc-qos-helper.sh

#define TC_LINE_MAX 1024

struct tc_class {
	char id[RRD_ID_LENGTH_MAX + 1];
	char name[RRD_ID_LENGTH_MAX + 1];

	char leafid[RRD_ID_LENGTH_MAX + 1];
	char parentid[RRD_ID_LENGTH_MAX + 1];

	int hasparent;
	int isleaf;
	unsigned long long bytes;

	struct tc_class *next;
};

struct tc_device {
	char id[RRD_ID_LENGTH_MAX + 1];
	char name[RRD_ID_LENGTH_MAX + 1];
	char family[RRD_ID_LENGTH_MAX + 1];

	struct tc_class *classes;
};

void tc_device_commit(struct tc_device *d)
{
	static int enable_new_interfaces = -1;

	if(enable_new_interfaces == -1)	enable_new_interfaces = config_get_boolean("plugin:tc", "enable new interfaces detected at runtime", 1);
	
	// we only need to add leaf classes
	struct tc_class *c, *x;

	for ( c = d->classes ; c ; c = c->next)
		c->isleaf = 1;

	for ( c = d->classes ; c ; c = c->next) {
		for ( x = d->classes ; x ; x = x->next) {
			if(x->parentid[0] && (strcmp(c->id, x->parentid) == 0 || strcmp(c->leafid, x->parentid) == 0)) {
				// debug(D_TC_LOOP, "TC: In device '%s', class '%s' (leafid: '%s') has leaf the class '%s' (parentid: '%s').", d->name, c->name, c->leafid, x->name, x->parentid);
				c->isleaf = 0;
				x->hasparent = 1;
			}
		}
	}
	
	// debugging:
	/*
	for ( c = d->classes ; c ; c = c->next) {
		if(c->isleaf && c->hasparent) debug(D_TC_LOOP, "TC: Device %s, class %s, OK", d->name, c->id);
		else debug(D_TC_LOOP, "TC: Device %s, class %s, IGNORE (isleaf: %d, hasparent: %d, parent: %s)", d->name, c->id, c->isleaf, c->hasparent, c->parentid);
	}
	*/

	for ( c = d->classes ; c ; c = c->next) {
		if(c->isleaf && c->hasparent) break;
	}
	if(!c) {
		debug(D_TC_LOOP, "TC: Ignoring TC device '%s'. No leaf classes.", d->name);
		return;
	}

	char var_name[4096 + 1];
	snprintf(var_name, 4096, "qos for %s", d->id);
	if(config_get_boolean("plugin:tc", var_name, enable_new_interfaces)) {
		RRDSET *st = rrdset_find_bytype(RRD_TYPE_TC, d->id);
		if(!st) {
			debug(D_TC_LOOP, "TC: Committing new TC device '%s'", d->name);

			st = rrdset_create(RRD_TYPE_TC, d->id, d->name, d->family, "Class Usage", "kilobits/s", 1000, rrd_update_every, RRDSET_TYPE_STACKED);

			for ( c = d->classes ; c ; c = c->next) {
				if(c->isleaf && c->hasparent)
					rrddim_add(st, c->id, c->name, 8, 1024 * rrd_update_every, RRDDIM_INCREMENTAL);
			}
		}
		else {
			rrdset_next_plugins(st);

			if(strcmp(d->id, d->name) != 0) rrdset_set_name(st, d->name);
		}

		for ( c = d->classes ; c ; c = c->next) {
			if(c->isleaf && c->hasparent) {
				if(rrddim_set(st, c->id, c->bytes) != 0) {
					
					// new class, we have to add it
					rrddim_add(st, c->id, c->name, 8, 1024 * rrd_update_every, RRDDIM_INCREMENTAL);
					rrddim_set(st, c->id, c->bytes);
				}

				// if it has a name, different to the id
				if(strcmp(c->id, c->name) != 0) {
					// update the rrd dimension with the new name
					RRDDIM *rd;
					for(rd = st->dimensions ; rd ; rd = rd->next) {
						if(strcmp(rd->id, c->id) == 0) { rrddim_set_name(st, rd, c->name); break; }
					}
				}
			}
		}
		rrdset_done(st);
	}
}

void tc_device_set_class_name(struct tc_device *d, char *id, char *name)
{
	struct tc_class *c;
	for ( c = d->classes ; c ; c = c->next) {
		if(strcmp(c->id, id) == 0) {
			strncpy(c->name, name, RRD_ID_LENGTH_MAX);
			// no need for null termination - it is already null
			break;
		}
	}
}

void tc_device_set_device_name(struct tc_device *d, char *name)
{
	strncpy(d->name, name, RRD_ID_LENGTH_MAX);
	// no need for null termination - it is already null
}

void tc_device_set_device_family(struct tc_device *d, char *name)
{
	strncpy(d->family, name, RRD_ID_LENGTH_MAX);
	// no need for null termination - it is already null
}

struct tc_device *tc_device_create(char *name)
{
	struct tc_device *d;

	d = calloc(1, sizeof(struct tc_device));
	if(!d) {
		fatal("Cannot allocate memory for tc_device %s", name);
		return NULL;
	}

	strncpy(d->id, name, RRD_ID_LENGTH_MAX);
	strcpy(d->name, d->id);
	strcpy(d->family, d->id);

	// no need for null termination on the strings, because of calloc()

	return(d);
}

struct tc_class *tc_class_add(struct tc_device *n, char *id, char *parentid, char *leafid)
{
	struct tc_class *c;

	c = calloc(1, sizeof(struct tc_class));
	if(!c) {
		fatal("Cannot allocate memory for tc class");
		return NULL;
	}

	c->next = n->classes;
	n->classes = c;

	strncpy(c->id, id, RRD_ID_LENGTH_MAX);
	strcpy(c->name, c->id);
	if(parentid) strncpy(c->parentid, parentid, RRD_ID_LENGTH_MAX);
	if(leafid) strncpy(c->leafid, leafid, RRD_ID_LENGTH_MAX);

	// no need for null termination on the strings, because of calloc()

	return(c);
}

void tc_class_free(struct tc_class *c)
{
	if(c->next) tc_class_free(c->next);
	free(c);
}

void tc_device_free(struct tc_device *n)
{
	if(n->classes) tc_class_free(n->classes);
	free(n);
}

pid_t tc_child_pid = 0;
void *tc_main(void *ptr)
{
	if(ptr) { ; }

	if(pthread_setcanceltype(PTHREAD_CANCEL_DEFERRED, NULL) != 0)
		error("Cannot set pthread cancel type to DEFERRED.");

	if(pthread_setcancelstate(PTHREAD_CANCEL_ENABLE, NULL) != 0)
		error("Cannot set pthread cancel state to ENABLE.");

	char buffer[TC_LINE_MAX+1] = "";

	for(;1;) {
		FILE *fp;
		struct tc_device *device = NULL;
		struct tc_class *class = NULL;

		snprintf(buffer, TC_LINE_MAX, "exec %s %d", config_get("plugin:tc", "script to run to get tc values", PLUGINS_DIR "/tc-qos-helper.sh"), rrd_update_every);
		debug(D_TC_LOOP, "executing '%s'", buffer);
		// fp = popen(buffer, "r");
		fp = mypopen(buffer, &tc_child_pid);
		if(!fp) {
			error("TC: Cannot popen(\"%s\", \"r\").", buffer);
			return NULL;
		}

		while(fgets(buffer, TC_LINE_MAX, fp) != NULL) {
			buffer[TC_LINE_MAX] = '\0';
			char *b = buffer, *p;
			// debug(D_TC_LOOP, "TC: read '%s'", buffer);

			p = strsep(&b, " \n");
			while (p && (*p == ' ' || *p == '\0')) p = strsep(&b, " \n");
			if(!p) continue;

			if(strcmp(p, "END") == 0) {
				if(device) {
					if(pthread_setcancelstate(PTHREAD_CANCEL_DISABLE, NULL) != 0)
						error("Cannot set pthread cancel state to DISABLE.");

					tc_device_commit(device);
					tc_device_free(device);
					device = NULL;
					class = NULL;

					if(pthread_setcancelstate(PTHREAD_CANCEL_ENABLE, NULL) != 0)
						error("Cannot set pthread cancel state to ENABLE.");
				}
			}
			else if(strcmp(p, "BEGIN") == 0) {
				if(device) {
					tc_device_free(device);
					device = NULL;
					class = NULL;
				}

				p = strsep(&b, " \n");
				if(p && *p) {
					device = tc_device_create(p);
					class = NULL;
				}
			}
			else if(device && (strcmp(p, "class") == 0)) {
				p = strsep(&b, " \n"); // the class: htb, fq_codel, etc
				char *id       = strsep(&b, " \n"); // the class major:minor
				char *parent   = strsep(&b, " \n"); // 'parent' or 'root'
				char *parentid = strsep(&b, " \n"); // the parent's id
				char *leaf     = strsep(&b, " \n"); // 'leaf'
				char *leafid   = strsep(&b, " \n"); // leafid

				if(id && *id
					&& parent && *parent
					&& parentid && *parentid
					&& (
						(strcmp(parent, "parent") == 0 && parentid && *parentid)
						|| strcmp(parent, "root") == 0
					)) {

					if(strcmp(parent, "root") == 0) {
						parentid = NULL;
						leafid = NULL;
					}
					else if(!leaf || strcmp(leaf, "leaf") != 0)
						leafid = NULL;

					char leafbuf[20 + 1] = "";
					if(leafid && leafid[strlen(leafid) - 1] == ':') {
						strncpy(leafbuf, leafid, 20 - 1);
						strcat(leafbuf, "1");
						leafid = leafbuf;
					}

					class = tc_class_add(device, id, parentid, leafid);
				}
			}
			else if(device && class && (strcmp(p, "Sent") == 0)) {
				p = strsep(&b, " \n");
				if(p && *p) class->bytes = atoll(p);
			}
			else if(device && (strcmp(p, "SETDEVICENAME") == 0)) {
				char *name = strsep(&b, " \n");
				if(name && *name) tc_device_set_device_name(device, name);
			}
			else if(device && (strcmp(p, "SETDEVICEGROUP") == 0)) {
				char *name = strsep(&b, " \n");
				if(name && *name) tc_device_set_device_family(device, name);
			}
			else if(device && (strcmp(p, "SETCLASSNAME") == 0)) {
				char *id    = strsep(&b, " \n");
				char *path  = strsep(&b, " \n");
				if(id && *id && path && *path) tc_device_set_class_name(device, id, path);
			}
#ifdef DETACH_PLUGINS_FROM_NETDATA
			else if((strcmp(p, "MYPID") == 0)) {
				char *id = strsep(&b, " \n");
				pid_t pid = atol(id);

				if(pid) tc_child_pid = pid;

				debug(D_TC_LOOP, "TC: Child PID is %d.", tc_child_pid);
			}
#endif
		}
		mypclose(fp);

		if(device) {
			tc_device_free(device);
			device = NULL;
			class = NULL;
		}

		sleep(rrd_update_every);
	}

	return NULL;
}

