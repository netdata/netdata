#include <stdio.h>
#include <stdlib.h>
#include <unistd.h>
#include <signal.h>
#include <syslog.h>
#include <pthread.h>
#include <string.h>
#include <errno.h>
#include <sys/types.h>
#include <sys/wait.h>

#include "log.h"
#include "daemon.h"
#include "web_server.h"
#include "popen.h"
#include "config.h"
#include "web_client.h"
#include "rrd.h"
#include "rrd2json.h"

#include "unit_test.h"

#include "plugins_d.h"
#include "plugin_idlejitter.h"
#include "plugin_tc.h"
#include "plugin_checks.h"
#include "plugin_proc.h"
#include "plugin_nfacct.h"

#include "main.h"

struct netdata_static_thread {
	char *name;

	char *config_section;
	char *config_name;

	int enabled;

	pthread_t *thread;

	void (*init_routine) (void);
	void *(*start_routine) (void *);
};

struct netdata_static_thread static_threads[] = {
	{"tc",			"plugins",	"tc",			1, NULL, NULL,	tc_main},
	{"idlejitter",	"plugins",	"idlejitter",	1, NULL, NULL,	cpuidlejitter_main},
	{"proc",		"plugins",	"proc",			1, NULL, NULL,	proc_main},

#ifdef INTERNAL_PLUGIN_NFACCT
	// nfacct requires root access
	// so, we build it as an external plugin with setuid to root
	{"nfacct",		"plugins",	"nfacct",		1, NULL, NULL, 	nfacct_main},
#endif

	{"plugins.d",	NULL,		NULL,			1, NULL, NULL,	pluginsd_main},
	{"check",		"plugins",	"checks",		0, NULL, NULL,	checks_main},
	{"web",			NULL,		NULL,			1, NULL, NULL,	socket_listen_main},
	{NULL,			NULL,		NULL,			0, NULL, NULL,	NULL}
};

void kill_childs()
{
	siginfo_t info;

	struct web_client *w;
	for(w = web_clients; w ; w = w->next) {
		debug(D_EXIT, "Stopping web client %s", w->client_ip);
		pthread_cancel(w->thread);
		pthread_join(w->thread, NULL);
	}

	int i;	
	for (i = 0; static_threads[i].name != NULL ; i++) {
		if(static_threads[i].thread) {
			debug(D_EXIT, "Stopping %s thread", static_threads[i].name);
			pthread_cancel(*static_threads[i].thread);
			pthread_join(*static_threads[i].thread, NULL);
			static_threads[i].thread = NULL;
		}
	}

	if(tc_child_pid) {
		if(kill(tc_child_pid, 0) != -1) {
			info("Killing tc-qos-helper procees");
			kill(tc_child_pid, SIGTERM);
			waitid(tc_child_pid, 0, &info, WEXITED);
		}
	}
	tc_child_pid = 0;

	struct plugind *cd;
	for(cd = pluginsd_root ; cd ; cd = cd->next) {
		debug(D_EXIT, "Stopping %s plugin thread", cd->id);
		pthread_cancel(cd->thread);
		pthread_join(cd->thread, NULL);

		if(cd->pid && !cd->obsolete) {
			if(kill(cd->pid, 0) != -1) {
				debug(D_EXIT, "killing %s plugin process", cd->id);
				kill(cd->pid, SIGTERM);
				waitid(cd->pid, 0, &info, WEXITED);
			}
		}
	}

	debug(D_EXIT, "All threads/childs stopped.");
}


int main(int argc, char **argv)
{
	int i;
	int config_loaded = 0;

	// set the name for logging
	program_name = "netdata";

	// parse  the arguments
	for(i = 1; i < argc ; i++) {
		if(strcmp(argv[i], "-c") == 0 && (i+1) < argc) {
			if(load_config(argv[i+1], 1) != 1) {
				fprintf(stderr, "Cannot load configuration file %s. Reason: %s\n", argv[i+1], strerror(errno));
				exit(1);
			}
			else {
				debug(D_OPTIONS, "Configuration loaded from %s.", argv[i+1]);
				config_loaded = 1;
			}
			i++;
		}
		else if(strcmp(argv[i], "-df") == 0 && (i+1) < argc) { config_set("global", "debug flags",  argv[i+1]); i++; }
		else if(strcmp(argv[i], "-p")  == 0 && (i+1) < argc) { config_set("global", "port",         argv[i+1]); i++; }
		else if(strcmp(argv[i], "-u")  == 0 && (i+1) < argc) { config_set("global", "run as user",  argv[i+1]); i++; }
		else if(strcmp(argv[i], "-l")  == 0 && (i+1) < argc) { config_set("global", "history",      argv[i+1]); i++; }
		else if(strcmp(argv[i], "-t")  == 0 && (i+1) < argc) { config_set("global", "update every", argv[i+1]); i++; }
		else if(strcmp(argv[i], "--unittest")  == 0) {
			if(unit_test_storage()) exit(1);
			exit(0);
			rrd_update_every = 1;
			if(unit_test(1000000, 0)) exit(1);
			if(unit_test(1000000, 500000)) exit(1);
			if(unit_test(1000000, 100000)) exit(1);
			if(unit_test(1000000, 900000)) exit(1);
			if(unit_test(2000000, 0)) exit(1);
			if(unit_test(2000000, 500000)) exit(1);
			if(unit_test(2000000, 100000)) exit(1);
			if(unit_test(2000000, 900000)) exit(1);
			if(unit_test(500000, 500000)) exit(1);
			//if(unit_test(500000, 100000)) exit(1); // FIXME: the expected numbers are wrong
			//if(unit_test(500000, 900000)) exit(1); // FIXME: the expected numbers are wrong
			if(unit_test(500000, 0)) exit(1);
			fprintf(stderr, "\n\nALL TESTS PASSED\n\n");
			exit(0);
		}
		else {
			fprintf(stderr, "Cannot understand option '%s'.\n", argv[i]);
			fprintf(stderr, "\nUSAGE: %s [-d] [-l LINES_TO_SAVE] [-u UPDATE_TIMER] [-p LISTEN_PORT] [-dl debug log file] [-df debug flags].\n\n", argv[0]);
			fprintf(stderr, "  -c CONFIG FILE the configuration file to load. Default: %s.\n", CONFIG_DIR "/" CONFIG_FILENAME);
			fprintf(stderr, "  -l LINES_TO_SAVE can be from 5 to %d lines in JSON data. Default: %d.\n", RRD_HISTORY_ENTRIES_MAX, RRD_DEFAULT_HISTORY_ENTRIES);
			fprintf(stderr, "  -t UPDATE_TIMER can be from 1 to %d seconds. Default: %d.\n", UPDATE_EVERY_MAX, UPDATE_EVERY);
			fprintf(stderr, "  -p LISTEN_PORT can be from 1 to %d. Default: %d.\n", 65535, LISTEN_PORT);
			fprintf(stderr, "  -u USERNAME can be any system username to run as. Default: none.\n");
			fprintf(stderr, "  -df FLAGS debug options. Default: 0x%8llx.\n", debug_flags);
			exit(1);
		}
	}

	if(!config_loaded) load_config(NULL, 0);

	char *input_log_file = NULL;
	char *output_log_file = NULL;
	char *error_log_file = NULL;
	char *access_log_file = NULL;
	{
		char buffer[1024];

		// --------------------------------------------------------------------

		sprintf(buffer, "0x%08llx", 0ULL);
		char *flags = config_get("global", "debug flags", buffer);
		debug_flags = strtoull(flags, NULL, 0);
		debug(D_OPTIONS, "Debug flags set to '0x%8llx'.", debug_flags);

		// --------------------------------------------------------------------

		output_log_file = config_get("global", "debug log", LOG_DIR "/debug.log");
		if(strcmp(output_log_file, "syslog") == 0) {
			output_log_syslog = 1;
			output_log_file = NULL;
		}
		else if(strcmp(output_log_file, "none") == 0) {
			output_log_syslog = 0;
			output_log_file = NULL;
		}
		else output_log_syslog = 0;

		// --------------------------------------------------------------------

		silent = 0;
		error_log_file = config_get("global", "error log", LOG_DIR "/error.log");
		if(strcmp(error_log_file, "syslog") == 0) {
			error_log_syslog = 1;
			error_log_file = NULL;
		}
		else if(strcmp(error_log_file, "none") == 0) {
			error_log_syslog = 0;
			error_log_file = NULL;
			silent = 1; // optimization - do not even generate debug log entries
		}
		else error_log_syslog = 0;

		// --------------------------------------------------------------------

		access_log_file = config_get("global", "access log", LOG_DIR "/access.log");
		if(strcmp(access_log_file, "syslog") == 0) {
			access_log_syslog = 1;
			access_log_file = NULL;
		}
		else if(strcmp(access_log_file, "none") == 0) {
			access_log_syslog = 0;
			access_log_file = NULL;
		}
		else access_log_syslog = 0;

		// --------------------------------------------------------------------

		rrd_memory_mode = rrd_memory_mode_id(config_get("global", "memory mode", rrd_memory_mode_name(rrd_memory_mode)));

		// --------------------------------------------------------------------

		if(gethostname(buffer, HOSTNAME_MAX) == -1)
			error("WARNING: Cannot get machine hostname.");
		hostname = config_get("global", "hostname", buffer);
		debug(D_OPTIONS, "hostname set to '%s'", hostname);

		// --------------------------------------------------------------------

		rrd_default_history_entries = config_get_number("global", "history", RRD_DEFAULT_HISTORY_ENTRIES);
		if(rrd_default_history_entries < 5 || rrd_default_history_entries > RRD_HISTORY_ENTRIES_MAX) {
			fprintf(stderr, "Invalid save lines %d given. Defaulting to %d.\n", rrd_default_history_entries, RRD_DEFAULT_HISTORY_ENTRIES);
			rrd_default_history_entries = RRD_DEFAULT_HISTORY_ENTRIES;
		}
		else {
			debug(D_OPTIONS, "save lines set to %d.", rrd_default_history_entries);
		}

		// --------------------------------------------------------------------

		rrd_update_every = config_get_number("global", "update every", UPDATE_EVERY);
		if(rrd_update_every < 1 || rrd_update_every > 600) {
			fprintf(stderr, "Invalid update timer %d given. Defaulting to %d.\n", rrd_update_every, UPDATE_EVERY_MAX);
			rrd_update_every = UPDATE_EVERY;
		}
		else debug(D_OPTIONS, "update timer set to %d.", rrd_update_every);

		// --------------------------------------------------------------------
		
		for (i = 0; static_threads[i].name != NULL ; i++) {
			struct netdata_static_thread *st = &static_threads[i];

			if(st->config_name) st->enabled = config_get_boolean(st->config_section, st->config_name, st->enabled);
			if(st->enabled && st->init_routine) st->init_routine();
		}

		// --------------------------------------------------------------------

		prepare_rundir();
		char *user = config_get("global", "run as user", (getuid() == 0)?"nobody":"");
		if(*user) {
			if(become_user(user) != 0) {
				fprintf(stderr, "Cannot become user %s.\n", user);
				exit(1);
			}
			else debug(D_OPTIONS, "Successfully became user %s.", user);
		}

		// --------------------------------------------------------------------

		listen_backlog = config_get_number("global", "http port listen backlog", LISTEN_BACKLOG);

		listen_port = config_get_number("global", "port", LISTEN_PORT);
		if(listen_port < 1 || listen_port > 65535) {
			fprintf(stderr, "Invalid listen port %d given. Defaulting to %d.\n", listen_port, LISTEN_PORT);
			listen_port = LISTEN_PORT;
		}
		else debug(D_OPTIONS, "Listen port set to %d.", listen_port);

		int ip = 0;
		char *ipv = config_get("global", "ip version", "any");
		if(!strcmp(ipv, "any") || !strcmp(ipv, "both") || !strcmp(ipv, "all")) ip = 0;
		else if(!strcmp(ipv, "ipv4") || !strcmp(ipv, "IPV4") || !strcmp(ipv, "IPv4") || !strcmp(ipv, "4")) ip = 4;
		else if(!strcmp(ipv, "ipv6") || !strcmp(ipv, "IPV6") || !strcmp(ipv, "IPv6") || !strcmp(ipv, "6")) ip = 6;
		else fprintf(stderr, "Cannot understand ip version '%s'. Assumming 'any'.", ipv);

		if(ip == 0 || ip == 6) listen_fd = create_listen_socket6(listen_port, listen_backlog);
		if(listen_fd < 0) {
			listen_fd = create_listen_socket4(listen_port, listen_backlog);
			if(listen_fd >= 0 && ip != 4) fprintf(stderr, "Managed to open an IPv4 socket on port %d.", listen_port);
		}

		if(listen_fd < 0) fatal("Cannot listen socket.");
	}

	// never become a problem
	if(nice(20) == -1) fprintf(stderr, "Cannot lower my CPU priority. Error: %s.\n", strerror(errno));

#ifndef NETDATA_NO_DAEMON
	if(become_daemon(0, input_log_file, output_log_file, error_log_file, access_log_file, &access_fd, &stdaccess) == -1) {
		fprintf(stderr, "Cannot demonize myself (%s).", strerror(errno));
		exit(1);
	}
#endif

	if(output_log_syslog || error_log_syslog || access_log_syslog)
		openlog("netdata", LOG_PID, LOG_DAEMON);

	info("NetData started on pid %d", getpid());


	// catch all signals
	for (i = 1 ; i < 65 ;i++) {
		switch(i) {
			case SIGSEGV:
			case SIGFPE:
			case SIGCHLD:
				signal(i, SIG_DFL);
				break;

			default:
				signal(i,  sig_handler);
				break;
		}
	}

	for (i = 0; static_threads[i].name != NULL ; i++) {
		struct netdata_static_thread *st = &static_threads[i];

		if(st->enabled) {
			st->thread = malloc(sizeof(pthread_t));

			info("Starting thread %s.", st->name);

			if(pthread_create(st->thread, NULL, st->start_routine, NULL))
				error("failed to create new thread for %s.", st->name);

			else if(pthread_detach(*st->thread))
				error("Cannot request detach of newly created %s thread.", st->name);
		}
		else info("Not starting thread %s.", st->name);
	}

	// for future use - the main thread
	while(1) sleep(60);

	exit(0);
}
