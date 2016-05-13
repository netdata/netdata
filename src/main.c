#ifdef HAVE_CONFIG_H
#include <config.h>
#endif
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
#include <sys/time.h>
#include <sys/resource.h>
#include <sys/mman.h>
#include <sys/prctl.h>

#include "common.h"
#include "log.h"
#include "daemon.h"
#include "web_server.h"
#include "popen.h"
#include "appconfig.h"
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
#include "registry.h"

#include "main.h"

extern void *cgroups_main(void *ptr);

volatile sig_atomic_t netdata_exit = 0;

void netdata_cleanup_and_exit(int ret)
{
	netdata_exit = 1;
	rrdset_save_all();
	// kill_childs();

	// let it log a few more error messages
	error_log_limit_reset();

	if(pidfd != -1) {
		if(ftruncate(pidfd, 0) != 0)
			error("Cannot truncate pidfile '%s'.", pidfile);

		close(pidfd);
		pidfd = -1;
	}

	if(pidfile[0]) {
		if(unlink(pidfile) != 0)
			error("Cannot unlink pidfile '%s'.", pidfile);
	}

	info("NetData exiting. Bye bye...");
	exit(ret);
}

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
	{"cgroups",		"plugins",	"cgroups",		1, NULL, NULL,	cgroups_main},

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

int killpid(pid_t pid, int sig)
{
	int ret = -1;
	debug(D_EXIT, "Request to kill pid %d", pid);

	errno = 0;
	if(kill(pid, 0) == -1) {
		switch(errno) {
			case ESRCH:
				error("Request to kill pid %d, but it is not running.", pid);
				break;

			case EPERM:
				error("Request to kill pid %d, but I do not have enough permissions.", pid);
				break;

			default:
				error("Request to kill pid %d, but I received an error.", pid);
				break;
		}
	}
	else {
		errno = 0;
		ret = kill(pid, sig);
		if(ret == -1) {
			switch(errno) {
				case ESRCH:
					error("Cannot kill pid %d, but it is not running.", pid);
					break;

				case EPERM:
					error("Cannot kill pid %d, but I do not have enough permissions.", pid);
					break;

				default:
					error("Cannot kill pid %d, but I received an error.", pid);
					break;
			}
		}
	}

	return ret;
}

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
		info("Killing tc-qos-helper procees");
		if(killpid(tc_child_pid, SIGTERM) != -1)
			waitid(P_PID, (id_t) tc_child_pid, &info, WEXITED);
	}
	tc_child_pid = 0;

	struct plugind *cd;
	for(cd = pluginsd_root ; cd ; cd = cd->next) {
		debug(D_EXIT, "Stopping %s plugin thread", cd->id);
		pthread_cancel(cd->thread);
		pthread_join(cd->thread, NULL);

		if(cd->pid && !cd->obsolete) {
			debug(D_EXIT, "killing %s plugin process", cd->id);
			if(killpid(cd->pid, SIGTERM) != -1)
				waitid(P_PID, (id_t) cd->pid, &info, WEXITED);
		}
	}

	// if, for any reason there is any child exited
	// catch it here
	waitid(P_PID, 0, &info, WEXITED|WNOHANG);

	debug(D_EXIT, "All threads/childs stopped.");
}


int main(int argc, char **argv)
{
	int i;
	int config_loaded = 0;
	int dont_fork = 0;
	size_t wanted_stacksize = 0, stacksize = 0;
	pthread_attr_t attr;

	// global initialization
	get_HZ();

	// set the name for logging
	program_name = "netdata";

	// parse  the arguments
	for(i = 1; i < argc ; i++) {
		if(strcmp(argv[i], "-c") == 0 && (i+1) < argc) {
			if(load_config(argv[i+1], 1) != 1) {
				error("Cannot load configuration file %s.", argv[i+1]);
				exit(1);
			}
			else {
				debug(D_OPTIONS, "Configuration loaded from %s.", argv[i+1]);
				config_loaded = 1;
			}
			i++;
		}
		else if(strcmp(argv[i], "-df") == 0 && (i+1) < argc) { config_set("global", "debug flags",  argv[i+1]); debug_flags = strtoull(argv[i+1], NULL, 0); i++; }
		else if(strcmp(argv[i], "-p")  == 0 && (i+1) < argc) { config_set("global", "port",         argv[i+1]); i++; }
		else if(strcmp(argv[i], "-u")  == 0 && (i+1) < argc) { config_set("global", "run as user",  argv[i+1]); i++; }
		else if(strcmp(argv[i], "-l")  == 0 && (i+1) < argc) { config_set("global", "history",      argv[i+1]); i++; }
		else if(strcmp(argv[i], "-t")  == 0 && (i+1) < argc) { config_set("global", "update every", argv[i+1]); i++; }
		else if(strcmp(argv[i], "-ch") == 0 && (i+1) < argc) { config_set("global", "host access prefix", argv[i+1]); i++; }
		else if(strcmp(argv[i], "-stacksize") == 0 && (i+1) < argc) { config_set("global", "pthread stack size", argv[i+1]); i++; }
		else if(strcmp(argv[i], "-nodaemon") == 0 || strcmp(argv[i], "-nd") == 0) dont_fork = 1;
		else if(strcmp(argv[i], "-pidfile") == 0 && (i+1) < argc) {
			i++;
			strncpyz(pidfile, argv[i], FILENAME_MAX);
		}
		else if(strcmp(argv[i], "--unittest")  == 0) {
			rrd_update_every = 1;
			if(run_all_mockup_tests()) exit(1);
			if(unit_test_storage()) exit(1);
			fprintf(stderr, "\n\nALL TESTS PASSED\n\n");
			exit(0);
		}
		else {
			fprintf(stderr, "Cannot understand option '%s'.\n", argv[i]);
			fprintf(stderr, "\nUSAGE: %s [-d] [-l LINES_TO_SAVE] [-u UPDATE_TIMER] [-p LISTEN_PORT] [-df debug flags].\n\n", argv[0]);
			fprintf(stderr, "  -c CONFIG FILE the configuration file to load. Default: %s.\n", CONFIG_DIR "/" CONFIG_FILENAME);
			fprintf(stderr, "  -l LINES_TO_SAVE can be from 5 to %d lines in JSON data. Default: %d.\n", RRD_HISTORY_ENTRIES_MAX, RRD_DEFAULT_HISTORY_ENTRIES);
			fprintf(stderr, "  -t UPDATE_TIMER can be from 1 to %d seconds. Default: %d.\n", UPDATE_EVERY_MAX, UPDATE_EVERY);
			fprintf(stderr, "  -p LISTEN_PORT can be from 1 to %d. Default: %d.\n", 65535, LISTEN_PORT);
			fprintf(stderr, "  -u USERNAME can be any system username to run as. Default: none.\n");
			fprintf(stderr, "  -ch path to access host /proc and /sys when running in a container. Default: empty.\n");
			fprintf(stderr, "  -nd or -nodeamon to disable forking in the background. Default: unset.\n");
			fprintf(stderr, "  -df FLAGS debug options. Default: 0x%08llx.\n", debug_flags);
			fprintf(stderr, "  -stacksize BYTES to overwrite the pthread stack size.\n");
			fprintf(stderr, "  -pidfile FILENAME to save a pid while running.\n");
			exit(1);
		}
	}

	if(!config_loaded) load_config(NULL, 0);

	// prepare configuration environment variables for the plugins
	setenv("NETDATA_CONFIG_DIR" , config_get("global", "config directory"   , CONFIG_DIR) , 1);
	setenv("NETDATA_PLUGINS_DIR", config_get("global", "plugins directory"  , PLUGINS_DIR), 1);
	setenv("NETDATA_WEB_DIR"    , config_get("global", "web files directory", WEB_DIR)    , 1);
	setenv("NETDATA_CACHE_DIR"  , config_get("global", "cache directory"    , CACHE_DIR)  , 1);
	setenv("NETDATA_LIB_DIR"    , config_get("global", "lib directory"      , VARLIB_DIR) , 1);
	setenv("NETDATA_LOG_DIR"    , config_get("global", "log directory"      , LOG_DIR)    , 1);
	setenv("NETDATA_HOST_PREFIX", config_get("global", "host access prefix" , "")         , 1);
	setenv("HOME"               , config_get("global", "home directory"     , CACHE_DIR)  , 1);

	// avoid extended to stat(/etc/localtime)
	// http://stackoverflow.com/questions/4554271/how-to-avoid-excessive-stat-etc-localtime-calls-in-strftime-on-linux
	setenv("TZ", ":/etc/localtime", 0);

	// cd to /tmp to avoid any plugins writing files at random places
	if(chdir("/tmp")) error("netdata: ERROR: Cannot cd to /tmp");

	char *input_log_file = NULL;
	char *output_log_file = NULL;
	char *error_log_file = NULL;
	char *access_log_file = NULL;
	char *user = NULL;
	{
		char buffer[1024];

		// --------------------------------------------------------------------

		sprintf(buffer, "0x%08llx", 0ULL);
		char *flags = config_get("global", "debug flags", buffer);
		setenv("NETDATA_DEBUG_FLAGS", flags, 1);

		debug_flags = strtoull(flags, NULL, 0);
		debug(D_OPTIONS, "Debug flags set to '0x%8llx'.", debug_flags);

		if(debug_flags != 0) {
			struct rlimit rl = { RLIM_INFINITY, RLIM_INFINITY };
			if(setrlimit(RLIMIT_CORE, &rl) != 0)
				info("Cannot request unlimited core dumps for debugging... Proceeding anyway...");
			prctl(PR_SET_DUMPABLE, 1, 0, 0, 0);
		}

		// --------------------------------------------------------------------

#ifdef MADV_MERGEABLE
		enable_ksm = config_get_boolean("global", "memory deduplication (ksm)", enable_ksm);
#else
#warning "Kernel memory deduplication (KSM) is not available"
#endif

		// --------------------------------------------------------------------


		global_host_prefix = config_get("global", "host access prefix", "");
		setenv("NETDATA_HOST_PREFIX", global_host_prefix, 1);

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

		error_log_file = config_get("global", "error log", LOG_DIR "/error.log");
		if(strcmp(error_log_file, "syslog") == 0) {
			error_log_syslog = 1;
			error_log_file = NULL;
		}
		else if(strcmp(error_log_file, "none") == 0) {
			error_log_syslog = 0;
			error_log_file = NULL;
			// optimization - do not even generate debug log entries
		}
		else error_log_syslog = 0;

		error_log_throttle_period = config_get_number("global", "errors flood protection period", error_log_throttle_period);
		setenv("NETDATA_ERRORS_THROTTLE_PERIOD", config_get("global", "errors flood protection period"    , ""), 1);

		error_log_errors_per_period = config_get_number("global", "errors to trigger flood protection", error_log_errors_per_period);
		setenv("NETDATA_ERRORS_PER_PERIOD"     , config_get("global", "errors to trigger flood protection", ""), 1);

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

		rrd_default_history_entries = (int) config_get_number("global", "history", RRD_DEFAULT_HISTORY_ENTRIES);
		if(rrd_default_history_entries < 5 || rrd_default_history_entries > RRD_HISTORY_ENTRIES_MAX) {
			info("Invalid save lines %d given. Defaulting to %d.", rrd_default_history_entries, RRD_DEFAULT_HISTORY_ENTRIES);
			rrd_default_history_entries = RRD_DEFAULT_HISTORY_ENTRIES;
		}
		else {
			debug(D_OPTIONS, "save lines set to %d.", rrd_default_history_entries);
		}

		// --------------------------------------------------------------------

		rrd_update_every = (int) config_get_number("global", "update every", UPDATE_EVERY);
		if(rrd_update_every < 1 || rrd_update_every > 600) {
			info("Invalid update timer %d given. Defaulting to %d.", rrd_update_every, UPDATE_EVERY_MAX);
			rrd_update_every = UPDATE_EVERY;
		}
		else debug(D_OPTIONS, "update timer set to %d.", rrd_update_every);

		// let the plugins know the min update_every
		{
			char buf[51];
			snprintfz(buf, 50, "%d", rrd_update_every);
			setenv("NETDATA_UPDATE_EVERY", buf, 1);
		}

		// --------------------------------------------------------------------

		// block signals while initializing threads.
		// this causes the threads to block signals.
		sigset_t sigset;
		sigfillset(&sigset);

		if(pthread_sigmask(SIG_BLOCK, &sigset, NULL) == -1) {
			error("Could not block signals for threads");
		}

		// Catch signals which we want to use to quit savely
		struct sigaction sa;
		sigemptyset(&sa.sa_mask);
		sigaddset(&sa.sa_mask, SIGHUP);
		sigaddset(&sa.sa_mask, SIGINT);
		sigaddset(&sa.sa_mask, SIGTERM);
		sa.sa_handler = sig_handler;
		sa.sa_flags = 0;
		if(sigaction(SIGHUP, &sa, NULL) == -1) {
			error("Failed to change signal handler for SIGHUP");
		}
		if(sigaction(SIGINT, &sa, NULL) == -1) {
			error("Failed to change signal handler for SIGINT");
		}
		if(sigaction(SIGTERM, &sa, NULL) == -1) {
			error("Failed to change signal handler for SIGTERM");
		}
		// Ignore SIGPIPE completely.
		// INFO: If we add signals here we have to unblock them
		// at popen.c when running a external plugin.
		sa.sa_handler = SIG_IGN;
		if(sigaction(SIGPIPE, &sa, NULL) == -1) {
			error("Failed to change signal handler for SIGTERM");
		}

		// --------------------------------------------------------------------

		i = pthread_attr_init(&attr);
		if(i != 0)
			fatal("pthread_attr_init() failed with code %d.", i);

		i = pthread_attr_getstacksize(&attr, &stacksize);
		if(i != 0)
			fatal("pthread_attr_getstacksize() failed with code %d.", i);
		else
			debug(D_OPTIONS, "initial pthread stack size is %zu bytes", stacksize);

		wanted_stacksize = config_get_number("global", "pthread stack size", stacksize);

		// --------------------------------------------------------------------

		for (i = 0; static_threads[i].name != NULL ; i++) {
			struct netdata_static_thread *st = &static_threads[i];

			if(st->config_name) st->enabled = config_get_boolean(st->config_section, st->config_name, st->enabled);
			if(st->enabled && st->init_routine) st->init_routine();
		}

		// --------------------------------------------------------------------

		// get the user we should run
		// IMPORTANT: this is required before web_files_uid()
		user = config_get("global", "run as user"    , (getuid() == 0)?NETDATA_USER:"");

		// IMPORTANT: these have to run once, while single threaded
		web_files_uid(); // IMPORTANT: web_files_uid() before web_files_gid()
		web_files_gid();

		// --------------------------------------------------------------------

		listen_backlog = (int) config_get_number("global", "http port listen backlog", LISTEN_BACKLOG);

		listen_port = (int) config_get_number("global", "port", LISTEN_PORT);
		if(listen_port < 1 || listen_port > 65535) {
			info("Invalid listen port %d given. Defaulting to %d.", listen_port, LISTEN_PORT);
			listen_port = LISTEN_PORT;
		}
		else debug(D_OPTIONS, "Listen port set to %d.", listen_port);

		int ip = 0;
		char *ipv = config_get("global", "ip version", "any");
		if(!strcmp(ipv, "any") || !strcmp(ipv, "both") || !strcmp(ipv, "all")) ip = 0;
		else if(!strcmp(ipv, "ipv4") || !strcmp(ipv, "IPV4") || !strcmp(ipv, "IPv4") || !strcmp(ipv, "4")) ip = 4;
		else if(!strcmp(ipv, "ipv6") || !strcmp(ipv, "IPV6") || !strcmp(ipv, "IPv6") || !strcmp(ipv, "6")) ip = 6;
		else info("Cannot understand ip version '%s'. Assuming 'any'.", ipv);

		if(ip == 0 || ip == 6) listen_fd = create_listen_socket6(config_get("global", "bind socket to IP", "*"), listen_port, listen_backlog);
		if(listen_fd < 0) {
			listen_fd = create_listen_socket4(config_get("global", "bind socket to IP", "*"), listen_port, listen_backlog);
			if(listen_fd >= 0 && ip != 4) info("Managed to open an IPv4 socket on port %d.", listen_port);
		}

		if(listen_fd < 0) fatal("Cannot listen socket.");
	}

	// never become a problem
	if(nice(20) == -1) error("Cannot lower my CPU priority.");

	if(become_daemon(dont_fork, 0, user, input_log_file, output_log_file, error_log_file, access_log_file, &access_fd, &stdaccess) == -1)
		fatal("Cannot demonize myself.");

#ifdef NETDATA_INTERNAL_CHECKS
	if(debug_flags != 0) {
		struct rlimit rl = { RLIM_INFINITY, RLIM_INFINITY };
		if(setrlimit(RLIMIT_CORE, &rl) != 0)
			info("Cannot request unlimited core dumps for debugging... Proceeding anyway...");
		prctl(PR_SET_DUMPABLE, 1, 0, 0, 0);
	}
#endif /* NETDATA_INTERNAL_CHECKS */

	if(output_log_syslog || error_log_syslog || access_log_syslog)
		openlog("netdata", LOG_PID, LOG_DAEMON);

	info("NetData started on pid %d", getpid());


	// ------------------------------------------------------------------------
	// get default pthread stack size

	if(stacksize < wanted_stacksize) {
		i = pthread_attr_setstacksize(&attr, wanted_stacksize);
		if(i != 0)
			fatal("pthread_attr_setstacksize() to %zu bytes, failed with code %d.", wanted_stacksize, i);
		else
			info("Successfully set pthread stacksize to %zu bytes", wanted_stacksize);
	}

	// --------------------------------------------------------------------
	// initialize the registry

	registry_init();

	// ------------------------------------------------------------------------
	// spawn the threads

	for (i = 0; static_threads[i].name != NULL ; i++) {
		struct netdata_static_thread *st = &static_threads[i];

		if(st->enabled) {
			st->thread = malloc(sizeof(pthread_t));
			if(!st->thread)
				fatal("Cannot allocate pthread_t memory");

			info("Starting thread %s.", st->name);

			if(pthread_create(st->thread, &attr, st->start_routine, NULL))
				error("failed to create new thread for %s.", st->name);

			else if(pthread_detach(*st->thread))
				error("Cannot request detach of newly created %s thread.", st->name);
		}
		else info("Not starting thread %s.", st->name);
	}

	// ------------------------------------------------------------------------
	// block signals while initializing threads.
	sigset_t sigset;
	sigfillset(&sigset);

	if(pthread_sigmask(SIG_UNBLOCK, &sigset, NULL) == -1) {
		error("Could not unblock signals for threads");
	}

	// Handle flags set in the signal handler.
	while(1) {
		pause();
		if(netdata_exit) {
			info("Exit main loop of netdata.");
			netdata_cleanup_and_exit(0);
			exit(0);
		}
	}
}
