#ifdef HAVE_CONFIG_H
#include <config.h>
#endif
#include <stdio.h>
#include <stdlib.h>
#include <string.h>
#include <sys/types.h>
#include <sys/resource.h>

#include "avl.h"
#include "log.h"
#include "common.h"
#include "appconfig.h"
#include "rrd.h"
#include "popen.h"
#include "plugin_tc.h"
#include "main.h"
#include "../config.h"

#define RRD_TYPE_TC					"tc"
#define RRD_TYPE_TC_LEN				strlen(RRD_TYPE_TC)

// ----------------------------------------------------------------------------
// /sbin/tc processor
// this requires the script plugins.d/tc-qos-helper.sh

#define TC_LINE_MAX 1024

struct tc_class {
	avl avl;

	char *id;
	uint32_t hash;

	char *name;

	char *leafid;
	uint32_t leaf_hash;

	char *parentid;
	uint32_t parent_hash;

	char hasparent;
	char isleaf;
	unsigned long long bytes;
	unsigned long long packets;
	unsigned long long dropped;
	unsigned long long overlimits;
	unsigned long long requeues;
	unsigned long long lended;
	unsigned long long borrowed;
	unsigned long long giants;
	unsigned long long tokens;
	unsigned long long ctokens;

	char updated;	// updated bytes
	char seen;		// seen in the tc list (even without bytes)

	struct tc_class *next;
	struct tc_class *prev;
};

struct tc_device {
	avl avl;

	char *id;
	uint32_t hash;

	char *name;
	char *family;

	avl_tree classes_index;

	struct tc_class *classes;

	struct tc_device *next;
	struct tc_device *prev;
};


struct tc_device *tc_device_root = NULL;

// ----------------------------------------------------------------------------
// tc_device index

static int tc_device_compare(void* a, void* b) {
	if(((struct tc_device *)a)->hash < ((struct tc_device *)b)->hash) return -1;
	else if(((struct tc_device *)a)->hash > ((struct tc_device *)b)->hash) return 1;
	else return strcmp(((struct tc_device *)a)->id, ((struct tc_device *)b)->id);
}

avl_tree tc_device_root_index = {
		NULL,
		tc_device_compare
};

#define tc_device_index_add(st) avl_insert(&tc_device_root_index, (avl *)(st))
#define tc_device_index_del(st) avl_remove(&tc_device_root_index, (avl *)(st))

static inline struct tc_device *tc_device_index_find(const char *id, uint32_t hash) {
	struct tc_device tmp;
	tmp.id = (char *)id;
	tmp.hash = (hash)?hash:simple_hash(tmp.id);

	return (struct tc_device *)avl_search(&(tc_device_root_index), (avl *)&tmp);
}


// ----------------------------------------------------------------------------
// tc_class index

static int tc_class_compare(void* a, void* b) {
	if(((struct tc_class *)a)->hash < ((struct tc_class *)b)->hash) return -1;
	else if(((struct tc_class *)a)->hash > ((struct tc_class *)b)->hash) return 1;
	else return strcmp(((struct tc_class *)a)->id, ((struct tc_class *)b)->id);
}

#define tc_class_index_add(st, rd) avl_insert(&((st)->classes_index), (avl *)(rd))
#define tc_class_index_del(st, rd) avl_remove(&((st)->classes_index), (avl *)(rd))

static inline struct tc_class *tc_class_index_find(struct tc_device *st, const char *id, uint32_t hash) {
	struct tc_class tmp;
	tmp.id = (char *)id;
	tmp.hash = (hash)?hash:simple_hash(tmp.id);

	return (struct tc_class *)avl_search(&(st->classes_index), (avl *) &tmp);
}

// ----------------------------------------------------------------------------

static inline void tc_class_free(struct tc_device *n, struct tc_class *c) {
	debug(D_TC_LOOP, "Removing from device '%s' class '%s', parentid '%s', leafid '%s', seen=%d", n->id, c->id, c->parentid?c->parentid:"", c->leafid?c->leafid:"", c->seen);

	if(c->next) c->next->prev = c->prev;
	if(c->prev) c->prev->next = c->next;
	if(n->classes == c) {
		if(c->next) n->classes = c->next;
		else n->classes = c->prev;
	}

	tc_class_index_del(n, c);

	if(c->id) free(c->id);
	if(c->name) free(c->name);
	if(c->leafid) free(c->leafid);
	if(c->parentid) free(c->parentid);

	free(c);
}

static inline void tc_device_classes_cleanup(struct tc_device *d) {
	static int cleanup_every = 999;

	if(cleanup_every > 0) {
		cleanup_every = (int) -config_get_number("plugin:tc", "cleanup unused classes every", 60);
		if(cleanup_every > 0) cleanup_every = -cleanup_every;
		if(cleanup_every == 0) cleanup_every = -1;
	}

	struct tc_class *c = d->classes;
	while(c) {
		if(c->seen < cleanup_every) {
			struct tc_class *nc = c->next;
			tc_class_free(d, c);
			c = nc;
		}
		else c = c->next;

		if(c) {
			c->updated = 0;
			c->seen--;
		}
	}
}

static inline void tc_device_commit(struct tc_device *d)
{
	static int enable_new_interfaces = -1;

	if(enable_new_interfaces == -1)	enable_new_interfaces = config_get_boolean("plugin:tc", "enable new interfaces detected at runtime", 1);

	// we only need to add leaf classes
	struct tc_class *c, *x;

	// set all classes
	for(c = d->classes ; c ; c = c->next) {
		c->isleaf = 1;
		c->hasparent = 0;
	}

	// mark the classes as leafs and parents
	for(c = d->classes ; c ; c = c->next) {
		if(!c->updated) continue;

		for(x = d->classes ; x ; x = x->next) {
			if(!x->updated) continue;

			if(c == x) continue;

			if(x->parentid && (
				(               c->hash      == x->parent_hash && strcmp(c->id,     x->parentid) == 0) ||
				(c->leafid   && c->leaf_hash == x->parent_hash && strcmp(c->leafid, x->parentid) == 0))) {
				// debug(D_TC_LOOP, "TC: In device '%s', class '%s' (leafid: '%s') has as leaf class '%s' (parentid: '%s').", d->name?d->name:d->id, c->name?c->name:c->id, c->leafid?c->leafid:c->id, x->name?x->name:x->id, x->parentid?x->parentid:x->id);
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

	// we need at least a class
	for(c = d->classes ; c ; c = c->next) {
		// debug(D_TC_LOOP, "TC: Device '%s', class '%s', isLeaf=%d, HasParent=%d, Seen=%d", d->name?d->name:d->id, c->name?c->name:c->id, c->isleaf, c->hasparent, c->seen);
		if(!c->updated) continue;
		if(c->isleaf && c->hasparent) break;
	}
	if(!c) {
		debug(D_TC_LOOP, "TC: Ignoring TC device '%s'. No leaf classes.", d->name?d->name:d->id);
		tc_device_classes_cleanup(d);
		return;
	}

	char var_name[CONFIG_MAX_NAME + 1];
	snprintfz(var_name, CONFIG_MAX_NAME, "qos for %s", d->id);
	if(config_get_boolean("plugin:tc", var_name, enable_new_interfaces)) {
		RRDSET *st = rrdset_find_bytype(RRD_TYPE_TC, d->id);
		if(!st) {
			debug(D_TC_LOOP, "TC: Creating new chart for device '%s'", d->name?d->name:d->id);

			st = rrdset_create(RRD_TYPE_TC, d->id, d->name?d->name:d->id, d->family?d->family:d->id, RRD_TYPE_TC ".qos", "Class Usage", "kilobits/s", 7000, rrd_update_every, RRDSET_TYPE_STACKED);

			for(c = d->classes ; c ; c = c->next) {
				if(!c->updated) continue;

				if(c->isleaf && c->hasparent)
					rrddim_add(st, c->id, c->name?c->name:c->id, 8, 1024, RRDDIM_INCREMENTAL);
			}
		}
		else {
			debug(D_TC_LOOP, "TC: Updating chart for device '%s'", d->name?d->name:d->id);
			rrdset_next_plugins(st);

			if(d->name && strcmp(d->id, d->name) != 0) rrdset_set_name(st, d->name);
		}

		for(c = d->classes ; c ; c = c->next) {
			if(!c->updated) continue;

			if(c->isleaf && c->hasparent) {
				RRDDIM *rd = rrddim_find(st, c->id);

				if(!rd) {
					debug(D_TC_LOOP, "TC: Adding to chart '%s', dimension '%s'", st->id, c->id, c->name);

					// new class, we have to add it
					rd = rrddim_add(st, c->id, c->name?c->name:c->id, 8, 1024, RRDDIM_INCREMENTAL);
				}
				else debug(D_TC_LOOP, "TC: Updating chart '%s', dimension '%s'", st->id, c->id);

				rrddim_set_by_pointer(st, rd, c->bytes);

				// if it has a name, different to the id
				if(c->name) {
					// update the rrd dimension with the new name
					debug(D_TC_LOOP, "TC: Setting chart '%s', dimension '%s' name to '%s'", st->id, rd->id, c->name);
					rrddim_set_name(st, rd, c->name);

					free(c->name);
					c->name = NULL;
				}
			}
		}
		rrdset_done(st);
	}

	tc_device_classes_cleanup(d);
}

static inline void tc_device_set_class_name(struct tc_device *d, char *id, char *name)
{
	struct tc_class *c = tc_class_index_find(d, id, 0);
	if(c) {
		if(c->name) free(c->name);
		c->name = NULL;

		if(name && *name && strcmp(c->id, name) != 0) {
			debug(D_TC_LOOP, "TC: Setting device '%s', class '%s' name to '%s'", d->id, id, name);
			c->name = strdup(name);
		}
	}
}

static inline void tc_device_set_device_name(struct tc_device *d, char *name) {
	if(d->name) free(d->name);
	d->name = NULL;

	if(name && *name && strcmp(d->id, name) != 0) {
		debug(D_TC_LOOP, "TC: Setting device '%s' name to '%s'", d->id, name);
		d->name = strdup(name);
	}
}

static inline void tc_device_set_device_family(struct tc_device *d, char *family) {
	if(d->family) free(d->family);
	d->family = NULL;

	if(family && *family && strcmp(d->id, family) != 0) {
		debug(D_TC_LOOP, "TC: Setting device '%s' family to '%s'", d->id, family);
		d->family = strdup(family);
	}
	// no need for null termination - it is already null
}

static inline struct tc_device *tc_device_create(char *id)
{
	struct tc_device *d = tc_device_index_find(id, 0);

	if(!d) {
		debug(D_TC_LOOP, "TC: Creating device '%s'", id);

		d = calloc(1, sizeof(struct tc_device));
		if(!d) {
			fatal("Cannot allocate memory for tc_device %s", id);
			return NULL;
		}

		d->id = strdup(id);
		d->hash = simple_hash(d->id);

		avl_init(&d->classes_index, tc_class_compare);
		tc_device_index_add(d);

		if(!tc_device_root) {
			tc_device_root = d;
		}
		else {
			d->next = tc_device_root;
			tc_device_root->prev = d;
			tc_device_root = d;
		}
	}

	return(d);
}

static inline struct tc_class *tc_class_add(struct tc_device *n, char *id, char *parentid, char *leafid)
{
	struct tc_class *c = tc_class_index_find(n, id, 0);

	if(!c) {
		debug(D_TC_LOOP, "TC: Creating in device '%s', class id '%s', parentid '%s', leafid '%s'", n->id, id, parentid?parentid:"", leafid?leafid:"");

		c = calloc(1, sizeof(struct tc_class));
		if(!c) {
			fatal("Cannot allocate memory for tc class");
			return NULL;
		}

		if(n->classes) n->classes->prev = c;
		c->next = n->classes;
		n->classes = c;

		c->id = strdup(id);
		if(!c->id) {
			free(c);
			return NULL;
		}
		c->hash = simple_hash(c->id);

		if(parentid && *parentid) {
			c->parentid = strdup(parentid);
			c->parent_hash = simple_hash(c->parentid);
		}

		if(leafid && *leafid) {
			c->leafid = strdup(leafid);
			c->leaf_hash = simple_hash(c->leafid);
		}

		tc_class_index_add(n, c);
	}

	c->seen = 1;

	return(c);
}

static inline void tc_device_free(struct tc_device *n)
{
	if(n->next) n->next->prev = n->prev;
	if(n->prev) n->prev->next = n->next;
	if(tc_device_root == n) {
		if(n->next) tc_device_root = n->next;
		else tc_device_root = n->prev;
	}

	tc_device_index_del(n);

	while(n->classes) tc_class_free(n, n->classes);

	if(n->id) free(n->id);
	if(n->name) free(n->name);
	if(n->family) free(n->family);

	free(n);
}

static inline void tc_device_free_all()
{
	while(tc_device_root)
		tc_device_free(tc_device_root);
}

#define MAX_WORDS 20

static inline int tc_space(char c) {
	switch(c) {
	case ' ':
	case '\t':
	case '\r':
	case '\n':
		return 1;

	default:
		return 0;
	}
}

static inline void tc_split_words(char *str, char **words, int max_words) {
	char *s = str;
	int i = 0;

	// skip all white space
	while(tc_space(*s)) s++;

	// store the first word
	words[i++] = s;

	// while we have something
	while(*s) {
		// if it is a space
		if(tc_space(*s)) {

			// terminate the word
			*s++ = '\0';

			// skip all white space
			while(tc_space(*s)) s++;

			// if we reached the end, stop
			if(!*s) break;

			// store the next word
			if(i < max_words) words[i++] = s;
			else break;
		}
		else s++;
	}

	// terminate the words
	while(i < max_words) words[i++] = NULL;
}

pid_t tc_child_pid = 0;
void *tc_main(void *ptr)
{
	if(ptr) { ; }

	info("TC thread created with task id %d", gettid());

	if(pthread_setcanceltype(PTHREAD_CANCEL_DEFERRED, NULL) != 0)
		error("Cannot set pthread cancel type to DEFERRED.");

	if(pthread_setcancelstate(PTHREAD_CANCEL_ENABLE, NULL) != 0)
		error("Cannot set pthread cancel state to ENABLE.");

	struct rusage thread;
	RRDSET *stcpu = NULL, *sttime = NULL;

	char buffer[TC_LINE_MAX+1] = "";
	char *words[MAX_WORDS] = { NULL };

	uint32_t BEGIN_HASH = simple_hash("BEGIN");
	uint32_t END_HASH = simple_hash("END");
	uint32_t CLASS_HASH = simple_hash("class");
	uint32_t SENT_HASH = simple_hash("Sent");
	uint32_t LENDED_HASH = simple_hash("lended:");
	uint32_t TOKENS_HASH = simple_hash("tokens:");
	uint32_t SETDEVICENAME_HASH = simple_hash("SETDEVICENAME");
	uint32_t SETDEVICEGROUP_HASH = simple_hash("SETDEVICEGROUP");
	uint32_t SETCLASSNAME_HASH = simple_hash("SETCLASSNAME");
	uint32_t WORKTIME_HASH = simple_hash("WORKTIME");
#ifdef DETACH_PLUGINS_FROM_NETDATA
	uint32_t MYPID_HASH = simple_hash("MYPID");
#endif
	uint32_t first_hash;

	for(;1;) {
		if(unlikely(netdata_exit)) break;

		FILE *fp;
		struct tc_device *device = NULL;
		struct tc_class *class = NULL;

		snprintfz(buffer, TC_LINE_MAX, "exec %s %d", config_get("plugin:tc", "script to run to get tc values", PLUGINS_DIR "/tc-qos-helper.sh"), rrd_update_every);
		debug(D_TC_LOOP, "executing '%s'", buffer);
		// fp = popen(buffer, "r");
		fp = mypopen(buffer, &tc_child_pid);
		if(!fp) {
			error("TC: Cannot popen(\"%s\", \"r\").", buffer);
			pthread_exit(NULL);
			return NULL;
		}

		while(fgets(buffer, TC_LINE_MAX, fp) != NULL) {
			if(unlikely(netdata_exit)) break;

			buffer[TC_LINE_MAX] = '\0';
			// debug(D_TC_LOOP, "TC: read '%s'", buffer);

			tc_split_words(buffer, words, MAX_WORDS);
			if(!words[0] || !*words[0]) {
				// debug(D_TC_LOOP, "empty line");
				continue;
			}
			// else debug(D_TC_LOOP, "First word is '%s'", words[0]);

			first_hash = simple_hash(words[0]);

			if(device && first_hash == CLASS_HASH && strcmp(words[0], "class") == 0) {
				// debug(D_TC_LOOP, "CLASS line on class id='%s', parent='%s', parentid='%s', leaf='%s', leafid='%s'", words[2], words[3], words[4], words[5], words[6]);

				// clear the last class
				class = NULL;

				// words[1] : class type
				// words[2] : N:XX
				// words[3] : parent or root
				if(words[1] && words[2] && words[3] && (strcmp(words[3], "parent") == 0 || strcmp(words[3], "root") == 0)) {
					//char *type     = words[1];  // the class: htb, fq_codel, etc

					// we are only interested for HTB classes
					//if(strcmp(type, "htb") != 0) continue;

					char *id       = words[2];	// the class major:minor
					char *parent   = words[3];	// 'parent' or 'root'
					char *parentid = words[4];	// the parent's id
					char *leaf     = words[5];	// 'leaf'
					char *leafid   = words[6];	// leafid

					if(strcmp(parent, "root") == 0) {
						parentid = NULL;
						leafid = NULL;
					}
					else if(!leaf || strcmp(leaf, "leaf") != 0)
						leafid = NULL;

					char leafbuf[20 + 1] = "";
					if(leafid && leafid[strlen(leafid) - 1] == ':') {
						strncpyz(leafbuf, leafid, 20 - 1);
						strcat(leafbuf, "1");
						leafid = leafbuf;
					}

					class = tc_class_add(device, id, parentid, leafid);
				}
			}
			else if(first_hash == END_HASH && strcmp(words[0], "END") == 0) {
				// debug(D_TC_LOOP, "END line");

				if(device) {
					if(pthread_setcancelstate(PTHREAD_CANCEL_DISABLE, NULL) != 0)
						error("Cannot set pthread cancel state to DISABLE.");

					tc_device_commit(device);
					// tc_device_free(device);
					device = NULL;
					class = NULL;

					if(pthread_setcancelstate(PTHREAD_CANCEL_ENABLE, NULL) != 0)
						error("Cannot set pthread cancel state to ENABLE.");
				}
			}
			else if(first_hash == BEGIN_HASH && strcmp(words[0], "BEGIN") == 0) {
				// debug(D_TC_LOOP, "BEGIN line on device '%s'", words[1]);

				if(device) {
					// tc_device_free(device);
					device = NULL;
					class = NULL;
				}

				if(words[1] && *words[1]) {
					device = tc_device_create(words[1]);
					class = NULL;
				}
			}
			else if(device && class && first_hash == SENT_HASH && strcmp(words[0], "Sent") == 0) {
				// debug(D_TC_LOOP, "SENT line '%s'", words[1]);
				if(words[1] && *words[1]) {
					class->bytes = strtoull(words[1], NULL, 10);
					class->updated = 1;
				}

				if(words[3] && *words[3])
					class->packets = strtoull(words[3], NULL, 10);

				if(words[6] && *words[6])
					class->dropped = strtoull(words[6], NULL, 10);

				if(words[8] && *words[8])
					class->overlimits = strtoull(words[8], NULL, 10);

				if(words[10] && *words[10])
					class->requeues = strtoull(words[8], NULL, 10);
			}
			else if(device && class && class->updated && first_hash == LENDED_HASH && strcmp(words[0], "lended:") == 0) {
				// debug(D_TC_LOOP, "LENDED line '%s'", words[1]);
				if(words[1] && *words[1])
					class->lended = strtoull(words[1], NULL, 10);

				if(words[3] && *words[3])
					class->borrowed = strtoull(words[3], NULL, 10);

				if(words[5] && *words[5])
					class->giants = strtoull(words[5], NULL, 10);
			}
			else if(device && class && class->updated && first_hash == TOKENS_HASH && strcmp(words[0], "tokens:") == 0) {
				// debug(D_TC_LOOP, "TOKENS line '%s'", words[1]);
				if(words[1] && *words[1])
					class->tokens = strtoull(words[1], NULL, 10);

				if(words[3] && *words[3])
					class->ctokens = strtoull(words[3], NULL, 10);
			}
			else if(device && first_hash == SETDEVICENAME_HASH && strcmp(words[0], "SETDEVICENAME") == 0) {
				// debug(D_TC_LOOP, "SETDEVICENAME line '%s'", words[1]);
				if(words[1] && *words[1]) tc_device_set_device_name(device, words[1]);
			}
			else if(device && first_hash == SETDEVICEGROUP_HASH && strcmp(words[0], "SETDEVICEGROUP") == 0) {
				// debug(D_TC_LOOP, "SETDEVICEGROUP line '%s'", words[1]);
				if(words[1] && *words[1]) tc_device_set_device_family(device, words[1]);
			}
			else if(device && first_hash == SETCLASSNAME_HASH && strcmp(words[0], "SETCLASSNAME") == 0) {
				// debug(D_TC_LOOP, "SETCLASSNAME line '%s' '%s'", words[1], words[2]);
				char *id    = words[1];
				char *path  = words[2];
				if(id && *id && path && *path) tc_device_set_class_name(device, id, path);
			}
			else if(first_hash == WORKTIME_HASH && strcmp(words[0], "WORKTIME") == 0) {
				// debug(D_TC_LOOP, "WORKTIME line '%s' '%s'", words[1], words[2]);
				getrusage(RUSAGE_THREAD, &thread);

				if(!stcpu) stcpu = rrdset_find("netdata.plugin_tc_cpu");
				if(!stcpu) {
					stcpu = rrdset_create("netdata", "plugin_tc_cpu", NULL, "tc.helper", NULL, "NetData TC CPU usage", "milliseconds/s", 135000, rrd_update_every, RRDSET_TYPE_STACKED);
					rrddim_add(stcpu, "user",  NULL,  1, 1000, RRDDIM_INCREMENTAL);
					rrddim_add(stcpu, "system", NULL, 1, 1000, RRDDIM_INCREMENTAL);
				}
				else rrdset_next(stcpu);

				rrddim_set(stcpu, "user"  , thread.ru_utime.tv_sec * 1000000ULL + thread.ru_utime.tv_usec);
				rrddim_set(stcpu, "system", thread.ru_stime.tv_sec * 1000000ULL + thread.ru_stime.tv_usec);
				rrdset_done(stcpu);

				if(!sttime) stcpu = rrdset_find("netdata.plugin_tc_time");
				if(!sttime) {
					sttime = rrdset_create("netdata", "plugin_tc_time", NULL, "tc.helper", NULL, "NetData TC script execution", "milliseconds/run", 135001, rrd_update_every, RRDSET_TYPE_AREA);
					rrddim_add(sttime, "run_time",  "run time",  1, 1, RRDDIM_ABSOLUTE);
				}
				else rrdset_next(sttime);

				rrddim_set(sttime, "run_time", atoll(words[1]));
				rrdset_done(sttime);

			}
#ifdef DETACH_PLUGINS_FROM_NETDATA
			else if(first_hash == MYPID_HASH && (strcmp(words[0], "MYPID") == 0)) {
				// debug(D_TC_LOOP, "MYPID line '%s'", words[1]);
				char *id = words[1];
				pid_t pid = atol(id);

				if(pid) tc_child_pid = pid;

				debug(D_TC_LOOP, "TC: Child PID is %d.", tc_child_pid);
			}
#endif
			//else {
			//	debug(D_TC_LOOP, "IGNORED line");
			//}
		}
		// fgets() failed or loop broke
		int code = mypclose(fp, tc_child_pid);
		tc_child_pid = 0;

		if(device) {
			// tc_device_free(device);
			device = NULL;
			class = NULL;
		}

		if(netdata_exit) {
			tc_device_free_all();
			pthread_exit(NULL);
			return NULL;
		}

		if(code == 1 || code == 127) {
			// 1 = DISABLE
			// 127 = cannot even run it
			error("TC: tc-qos-helper.sh exited with code %d. Disabling it.", code);

			tc_device_free_all();
			pthread_exit(NULL);
			return NULL;
		}

		sleep((unsigned int) rrd_update_every);
	}

	pthread_exit(NULL);
	return NULL;
}
