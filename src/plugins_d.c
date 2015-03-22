#include <sys/types.h>
#include <dirent.h>
#include <pthread.h>
#include <string.h>
#include <stdio.h>
#include <stdlib.h>
#include <signal.h>

#include "common.h"
#include "config.h"
#include "log.h"
#include "rrd.h"
#include "popen.h"
#include "plugins_d.h"

struct plugind *pluginsd_root = NULL;

void *pluginsd_worker_thread(void *arg)
{
	struct plugind *cd = (struct plugind *)arg;
	char line[PLUGINSD_LINE_MAX + 1];

#ifdef DETACH_PLUGINS_FROM_NETDATA
	unsigned long long usec = 0, susec = 0;
	struct timeval last = {0, 0} , now = {0, 0};
#endif

	while(1) {
		FILE *fp = mypopen(cd->cmd, &cd->pid);
		if(!fp) {
			error("Cannot popen(\"%s\", \"r\").", cd->cmd);
			break;
		}

		RRDSET *st = NULL;

		unsigned long long count = 0;
		while(fgets(line, PLUGINSD_LINE_MAX, fp) != NULL) {
			char *p = trim(line);
			debug(D_PLUGINSD, "PLUGINSD: %s: %s", cd->filename, line);

			char *s = qstrsep(&p);

			if(!s || !*s) continue;
			else if(!strcmp(s, "SET")) {
				char *t;
				while((t = strchr(p, '='))) *t = ' ';
				
				char *dimension = qstrsep(&p);
				char *value = qstrsep(&p);

				if(!dimension || !*dimension || !value) {
					error("PLUGINSD: '%s' is requesting a SET on chart '%s', like this: 'SET %s = %s'. Disabling it.", cd->fullfilename, st->id, dimension?dimension:"", value?value:"");
					cd->enabled = 0;
					kill(cd->pid, SIGTERM);
					break;
				}

				if(!st) {
					error("PLUGINSD: '%s' is requesting a SET on dimension %s with value %s, without a BEGIN. Disabling it.", cd->fullfilename, dimension, value);
					cd->enabled = 0;
					kill(cd->pid, SIGTERM);
					break;
				}

				if(st->debug) debug(D_PLUGINSD, "PLUGINSD: '%s' is setting dimension %s/%s to %s", cd->fullfilename, st->id, dimension, value);
				rrddim_set(st, dimension, atoll(value));

				count++;
			}
			else if(!strcmp(s, "BEGIN")) {
				char *id = qstrsep(&p);
				char *microseconds_txt = qstrsep(&p);

				if(!id) {
					error("PLUGINSD: '%s' is requesting a BEGIN without a chart id. Disabling it.", cd->fullfilename);
					cd->enabled = 0;
					kill(cd->pid, SIGTERM);
					break;
				}

				st = rrdset_find(id);
				if(!st) {
					error("PLUGINSD: '%s' is requesting a BEGIN on chart '%s', which does not exist. Disabling it.", cd->fullfilename, id);
					cd->enabled = 0;
					kill(cd->pid, SIGTERM);
					break;
				}

				if(st->counter_done) {
					unsigned long long microseconds = 0;
					if(microseconds_txt && *microseconds_txt) microseconds = strtoull(microseconds_txt, NULL, 10);
					if(microseconds) rrdset_next_usec(st, microseconds);
					else rrdset_next_plugins(st);
				}
			}
			else if(!strcmp(s, "END")) {
				if(!st) {
					error("PLUGINSD: '%s' is requesting an END, without a BEGIN. Disabling it.", cd->fullfilename);
					cd->enabled = 0;
					kill(cd->pid, SIGTERM);
					break;
				}

				if(st->debug) debug(D_PLUGINSD, "PLUGINSD: '%s' is requesting a END on chart %s", cd->fullfilename, st->id);

				rrdset_done(st);
				st = NULL;
			}
			else if(!strcmp(s, "FLUSH")) {
				debug(D_PLUGINSD, "PLUGINSD: '%s' is requesting a FLUSH", cd->fullfilename);
				st = NULL;
			}
			else if(!strcmp(s, "CHART")) {
				st = NULL;

				char *type = qstrsep(&p);
				char *id = NULL;
				if(type) {
					id = strchr(type, '.');
					if(id) { *id = '\0'; id++; }
				}
				char *name = qstrsep(&p);
				char *title = qstrsep(&p);
				char *units = qstrsep(&p);
				char *family = qstrsep(&p);
				char *category = qstrsep(&p);
				char *chart = qstrsep(&p);
				char *priority_s = qstrsep(&p);
				char *update_every_s = qstrsep(&p);

				if(!type || !*type || !id || !*id) {
					error("PLUGINSD: '%s' is requesting a CHART, without a type.id. Disabling it.", cd->fullfilename);
					cd->enabled = 0;
					kill(cd->pid, SIGTERM);
					break;
				}

				int priority = 1000;
				if(priority_s) priority = atoi(priority_s);

				int update_every = cd->update_every;
				if(update_every_s) update_every = atoi(update_every_s);
				if(!update_every) update_every = cd->update_every;

				int chart_type = RRDSET_TYPE_LINE;
				if(chart) chart_type = rrdset_type_id(chart);

				if(!name || !*name) name = NULL;
				if(!family || !*family) family = id;
				if(!category || !*category) category = type;

				st = rrdset_find_bytype(type, id);
				if(!st) {
					debug(D_PLUGINSD, "PLUGINSD: Creating chart type='%s', id='%s', name='%s', family='%s', category='%s', chart='%s', priority=%d, update_every=%d"
						, type, id
						, name?name:""
						, family?family:""
						, category?category:""
						, rrdset_type_name(chart_type)
						, priority
						, update_every
						);

					st = rrdset_create(type, id, name, family, title, units, priority, update_every, chart_type);
					cd->update_every = update_every;

					if(strcmp(category, "none") == 0) st->isdetail = 1;
				}
				else debug(D_PLUGINSD, "PLUGINSD: Chart '%s' already exists. Not adding it again.", st->id);
			}
			else if(!strcmp(s, "DIMENSION")) {
				char *id = qstrsep(&p);
				char *name = qstrsep(&p);
				char *algorithm = qstrsep(&p);
				char *multiplier_s = qstrsep(&p);
				char *divisor_s = qstrsep(&p);
				char *hidden = qstrsep(&p);

				if(!id || !*id) {
					error("PLUGINSD: '%s' is requesting a DIMENSION, without an id. Disabling it.", cd->fullfilename);
					cd->enabled = 0;
					kill(cd->pid, SIGTERM);
					break;
				}

				if(!st) {
					error("PLUGINSD: '%s' is requesting a DIMENSION, without a CHART. Disabling it.", cd->fullfilename);
					cd->enabled = 0;
					kill(cd->pid, SIGTERM);
					break;
				}

				long multiplier = 1;
				if(multiplier_s && *multiplier_s) multiplier = atol(multiplier_s);
				if(!multiplier) multiplier = 1;

				long divisor = 1;
				if(divisor_s && *divisor_s) divisor = atol(divisor_s);
				if(!divisor) divisor = 1;

				if(!algorithm || !*algorithm) algorithm = "absolute";

				if(st->debug) debug(D_PLUGINSD, "PLUGINSD: Creating dimension in chart %s, id='%s', name='%s', algorithm='%s', multiplier=%ld, divisor=%ld, hidden='%s'"
					, st->id
					, id
					, name?name:""
					, rrddim_algorithm_name(rrddim_algorithm_id(algorithm))
					, multiplier
					, divisor
					, hidden?hidden:""
					);

				RRDDIM *rd = rrddim_find(st, id);
				if(!rd) {
					rd = rrddim_add(st, id, name, multiplier, divisor, rrddim_algorithm_id(algorithm));
					if(hidden && strcmp(hidden, "hidden") == 0) rd->hidden = 1;
				}
				else if(st->debug) debug(D_PLUGINSD, "PLUGINSD: dimension %s/%s already exists. Not adding it again.", st->id, id);
			}
			else if(!strcmp(s, "DISABLE")) {
				error("PLUGINSD: '%s' called DISABLE. Disabling it.", cd->fullfilename);
				cd->enabled = 0;
				kill(cd->pid, SIGTERM);
				break;
			}
#ifdef DETACH_PLUGINS_FROM_NETDATA
			else if(!strcmp(s, "MYPID")) {
				char *pid_s = qstrsep(&p);
				pid_t pid = atol(pid_s);

				if(pid) cd->pid = pid;
				debug(D_PLUGINSD, "PLUGINSD: %s is on pid %d", cd->id, cd->pid);
			}
			else if(!strcmp(s, "STOPPING_WAKE_ME_UP_PLEASE")) {
				error("PLUGINSD: '%s' (pid %d) called STOPPING_WAKE_ME_UP_PLEASE.", cd->fullfilename, cd->pid);

				gettimeofday(&now, NULL);
				if(!usec && !susec) {
					// our first run
					susec = cd->rrd_update_every * 1000000ULL;
				}
				else {
					// second+ run
					usec = usecdiff(&now, &last) - susec;
					error("PLUGINSD: %s last loop took %llu usec (worked for %llu, sleeped for %llu).\n", cd->fullfilename, usec + susec, usec, susec);
					if(usec < (rrd_update_every * 1000000ULL / 2ULL)) susec = (rrd_update_every * 1000000ULL) - usec;
					else susec = rrd_update_every * 1000000ULL / 2ULL;
				}

				error("PLUGINSD: %s sleeping for %llu. Will kill with SIGCONT pid %d to wake it up.\n", cd->fullfilename, susec, cd->pid);
				usleep(susec);
				kill(cd->pid, SIGCONT);
				bcopy(&now, &last, sizeof(struct timeval));
				break;
			}
#endif
			else {
				error("PLUGINSD: '%s' is sending command '%s' which is not known by netdata. Disabling it.", cd->fullfilename, s);
				cd->enabled = 0;
				kill(cd->pid, SIGTERM);
				break;
			}
		}

		// fgets() failed or loop broke
		mypclose(fp);

		if(!count && cd->enabled) {
			error("PLUGINSD: '%s' does not generate usefull output. Disabling it.", cd->fullfilename);
			cd->enabled = 0;
			kill(cd->pid, SIGTERM);
		}

		if(cd->enabled) sleep(cd->update_every);
		else break;
	}

	cd->obsolete = 1;
	return NULL;
}

void *pluginsd_main(void *ptr)
{
	if(ptr) { ; }

	if(pthread_setcanceltype(PTHREAD_CANCEL_DEFERRED, NULL) != 0)
		error("Cannot set pthread cancel type to DEFERRED.");

	if(pthread_setcancelstate(PTHREAD_CANCEL_ENABLE, NULL) != 0)
		error("Cannot set pthread cancel state to ENABLE.");

	char *dir_name = config_get("plugins", "plugins directory", PLUGINS_DIR);
	int automatic_run = config_get_boolean("plugins", "enable running new plugins", 0);
	int scan_frequency = config_get_number("plugins", "check for new plugins every", 60);
	DIR *dir = NULL;
	struct dirent *file = NULL;
	struct plugind *cd;

	// enable the apps plugin by default
	config_get_boolean("plugins", "apps", 1);

	if(scan_frequency < 1) scan_frequency = 1;

	while(1) {
		dir = opendir(dir_name);
		if(!dir) {
			error("Cannot open directory '%s'.", dir_name);
			return NULL;
		}

		while((file = readdir(dir))) {
			debug(D_PLUGINSD, "PLUGINSD: Examining file '%s'", file->d_name);

			if(strcmp(file->d_name, ".") == 0 || strcmp(file->d_name, "..") == 0) continue;

			int len = strlen(file->d_name);
			if(len <= (int)PLUGINSD_FILE_SUFFIX_LEN) continue;
			if(strcmp(PLUGINSD_FILE_SUFFIX, &file->d_name[len - (int)PLUGINSD_FILE_SUFFIX_LEN]) != 0) {
				debug(D_PLUGINSD, "PLUGINSD: File '%s' does not end in '%s'.", file->d_name, PLUGINSD_FILE_SUFFIX);
				continue;
			}

			char pluginname[CONFIG_MAX_NAME + 1];
			snprintf(pluginname, CONFIG_MAX_NAME, "%.*s", (int)(len - PLUGINSD_FILE_SUFFIX_LEN), file->d_name);
			int enabled = config_get_boolean("plugins", pluginname, automatic_run);

			if(!enabled) {
				debug(D_PLUGINSD, "PLUGINSD: plugin '%s' is not enabled", file->d_name);
				continue;
			}

			// check if it runs already
			for(cd = pluginsd_root ; cd ; cd = cd->next) {
				if(strcmp(cd->filename, file->d_name) == 0) break;
			}
			if(cd && !cd->obsolete) {
				debug(D_PLUGINSD, "PLUGINSD: plugin '%s' is already running", cd->filename);
				continue;
			}

			// it is not running
			// allocate a new one, or use the obsolete one
			if(!cd) {
				cd = calloc(sizeof(struct plugind), 1);
				if(!cd) fatal("Cannot allocate memory for plugin.");

				snprintf(cd->id, CONFIG_MAX_NAME, "plugin:%s", pluginname);
				
				strncpy(cd->filename, file->d_name, FILENAME_MAX);
				snprintf(cd->fullfilename, FILENAME_MAX, "%s/%s", dir_name, cd->filename);

				cd->enabled = enabled;
				cd->update_every = config_get_number(cd->id, "update every", rrd_update_every);
				cd->started_t = time(NULL);

				char *def = "";
				snprintf(cd->cmd, PLUGINSD_CMD_MAX, "exec %s %d %s", cd->fullfilename, cd->update_every, config_get(cd->id, "command options", def));

				// link it
				if(pluginsd_root) cd->next = pluginsd_root;
				pluginsd_root = cd;
			}
			cd->obsolete = 0;

			if(!cd->enabled) continue;

			// spawn a new thread for it
			if(pthread_create(&cd->thread, NULL, pluginsd_worker_thread, cd) != 0) {
				error("CHARTS.D: failed to create new thread for chart.d %s.", cd->filename);
				cd->obsolete = 1;
			}
			else if(pthread_detach(cd->thread) != 0)
				error("CHARTS.D: Cannot request detach of newly created thread for chart.d %s.", cd->filename);
		}

		closedir(dir);
		sleep(scan_frequency);
	}

	return NULL;
}


