// enable strcasestr()
#define _GNU_SOURCE

#include <stdio.h>
#include <stdlib.h>
#include <string.h>
#include <time.h>
#include <unistd.h>
#include <pwd.h>
#include <sys/types.h>
#include <sys/time.h>
#include <sys/wait.h>

#include <sys/socket.h>
#include <sys/select.h>
#include <sys/resource.h>
#include <sys/stat.h>
#include <sys/sendfile.h>

#include <arpa/inet.h>
#include <netinet/in.h>
#include <netinet/tcp.h>

#include <errno.h>
#include <stdarg.h>
#include <locale.h>
#include <signal.h>
#include <ctype.h>
#include <time.h>
#include <fcntl.h>
#include <syslog.h>

#include <pthread.h>
#include <zlib.h>
#include <malloc.h>
#include <inttypes.h>
#include <dirent.h>
#include <sys/mman.h>

// enabling this will detach the plugins from netdata
// each plugin will have its own process group
// #define DETACH_PLUGINS_FROM_NETDATA

#define RRD_TYPE_NET				"net"
#define RRD_TYPE_NET_LEN			strlen(RRD_TYPE_NET)

#define RRD_TYPE_TC					"tc"
#define RRD_TYPE_TC_LEN				strlen(RRD_TYPE_TC)

#define RRD_TYPE_DISK				"disk"
#define RRD_TYPE_DISK_LEN			strlen(RRD_TYPE_DISK)

#define RRD_TYPE_NET_SNMP			"ipv4"
#define RRD_TYPE_NET_SNMP_LEN		strlen(RRD_TYPE_NET_SNMP)

#define RRD_TYPE_NET_STAT_CONNTRACK 	"conntrack"
#define RRD_TYPE_NET_STAT_CONNTRACK_LEN	strlen(RRD_TYPE_NET_STAT_CONNTRACK)

#define RRD_TYPE_NET_IPVS 			"ipvs"
#define RRD_TYPE_NET_IPVS_LEN		strlen(RRD_TYPE_NET_IPVS)

#define RRD_TYPE_STAT 				"cpu"
#define RRD_TYPE_STAT_LEN			strlen(RRD_TYPE_STAT)

#define WEB_PATH_FILE				"file"
#define WEB_PATH_DATA				"data"
#define WEB_PATH_DATASOURCE			"datasource"
#define WEB_PATH_GRAPH				"graph"

// type of JSON generations
#define DATASOURCE_JSON 0
#define DATASOURCE_GOOGLE_JSON 1
#define DATASOURCE_GOOGLE_JSONP 2

// internal defaults
#define UPDATE_EVERY 1
#define UPDATE_EVERY_MAX 3600
#define LISTEN_PORT 19999
#define HISTORY 3600
#define HISTORY_MAX (86400*10)
#define SAVE_PATH "/tmp"

#define D_WEB_BUFFER 		0x00000001
#define D_WEB_CLIENT 		0x00000002
#define D_LISTENER   		0x00000004
#define D_WEB_DATA   		0x00000008
#define D_OPTIONS    		0x00000010
#define D_PROCNETDEV_LOOP   0x00000020
#define D_RRD_STATS 		0x00000040
#define D_WEB_CLIENT_ACCESS	0x00000080
#define D_TC_LOOP           0x00000100
#define D_DEFLATE           0x00000200
#define D_CONFIG            0x00000400
#define D_PLUGINSD          0x00000800
#define D_CHILDS            0x00001000

#define CT_APPLICATION_JSON				1
#define CT_TEXT_PLAIN					2
#define CT_TEXT_HTML					3
#define CT_APPLICATION_X_JAVASCRIPT		4
#define CT_TEXT_CSS						5
#define CT_TEXT_XML						6
#define CT_APPLICATION_XML				7
#define CT_TEXT_XSL						8
#define CT_APPLICATION_OCTET_STREAM		9
#define CT_APPLICATION_X_FONT_TRUETYPE	10
#define CT_APPLICATION_X_FONT_OPENTYPE	11
#define CT_APPLICATION_FONT_WOFF		12
#define CT_APPLICATION_VND_MS_FONTOBJ	13
#define CT_IMAGE_SVG_XML				14


#define DEBUG (D_WEB_CLIENT_ACCESS|D_LISTENER|D_RRD_STATS)
//#define DEBUG 0xffffffff
//#define DEBUG (0)

#define HOSTNAME_MAX 1024
char *hostname;

#define EXIT_FAILURE 1
#define LISTEN_BACKLOG 100

#define INITIAL_WEB_DATA_LENGTH 65536
#define WEB_DATA_LENGTH_INCREASE_STEP 65536
#define ZLIB_CHUNK 	16384

#define MAX_HTTP_HEADER_SIZE 16384

#define MAX_PROC_NET_SNMP_LINE 4096
#define MAX_PROC_NET_SNMP_NAME 1024

#define MAX_PROC_NET_STAT_CONNTRACK_LINE 4096
#define MAX_PROC_NET_STAT_CONNTRACK_NAME 1024

#define MAX_PROC_NET_IPVS_LINE 4096
#define MAX_PROC_NET_IPVS_NAME 1024

#define MAX_PROC_STAT_LINE 4096
#define MAX_PROC_STAT_NAME 1024


int silent = 0;
int save_history = HISTORY;
int update_every = UPDATE_EVERY;
int listen_port = LISTEN_PORT;


// ----------------------------------------------------------------------------
// global statistics

struct global_statistics {

	unsigned long long connected_clients;

	unsigned long long web_requests;

	unsigned long long bytes_received;
	unsigned long long bytes_sent;

} global_statistics = { 0ULL, 0ULL, 0ULL, 0ULL };

pthread_mutex_t global_statistics_mutex = PTHREAD_MUTEX_INITIALIZER;

void global_statistics_lock(void)
{
	pthread_mutex_lock(&global_statistics_mutex);
}
void global_statistics_unlock(void)
{
	pthread_mutex_unlock(&global_statistics_mutex);
}

// ----------------------------------------------------------------------------
// LOG

unsigned long long debug_flags = DEBUG;

int access_fd = -1;
FILE *stdaccess = NULL;

int access_log_syslog = 1;
int error_log_syslog = 1;
int output_log_syslog = 1;	// debug log


void log_date(FILE *out)
{
		char outstr[200];
		time_t t;
		struct tm *tmp;

		t = time(NULL);
		tmp = localtime(&t);

		if (tmp == NULL) return;
		if (strftime(outstr, sizeof(outstr), "%y-%m-%d %H:%M:%S", tmp) == 0) return;

		fprintf(out, "%s: ", outstr);
}

int debug_variable;
//#define debug(args...) debug_int(__FILE__, __FUNCTION__, __LINE__, ##args)
#define debug(type, args...) do { if(!silent && debug_flags & type) debug_int(__FILE__, __FUNCTION__, __LINE__, ##args); } while(0)

void debug_int( const char *file, const char *function, const unsigned long line, const char *fmt, ... )
{
	if(file) { ; }
	va_list args;

	log_date(stdout);
	va_start( args, fmt );
	fprintf(stdout, "DEBUG (%04lu@%-15.15s): ", line, function);
	vfprintf( stdout, fmt, args );
	va_end( args );
	fprintf(stdout, "\n");

	if(output_log_syslog) {
		va_start( args, fmt );
		vsyslog(LOG_ERR,  fmt, args );
		va_end( args );
	}
}

#define info(args...)  info_int(__FILE__, __FUNCTION__, __LINE__, ##args)

void info_int( const char *file, const char *function, const unsigned long line, const char *fmt, ... )
{
	if(file) { ; }
	va_list args;

	log_date(stderr);

	va_start( args, fmt );
	if(debug_flags) fprintf(stderr, "INFO (%04lu@%-15.15s): ", line, function);
	else            fprintf(stderr, "INFO: ");
	vfprintf( stderr, fmt, args );
	va_end( args );

	fprintf(stderr, "\n");

	if(error_log_syslog) {
		va_start( args, fmt );
		vsyslog(LOG_INFO,  fmt, args );
		va_end( args );
	}
}

#define error(args...)  error_int(__FILE__, __FUNCTION__, __LINE__, ##args)

void error_int( const char *file, const char *function, const unsigned long line, const char *fmt, ... )
{
	if(file) { ; }
	va_list args;

	log_date(stderr);

	va_start( args, fmt );
	if(debug_flags) fprintf(stderr, "ERROR (%04lu@%-15.15s): ", line, function);
	else            fprintf(stderr, "ERROR: ");
	vfprintf( stderr, fmt, args );
	va_end( args );

	if(errno) {
			fprintf(stderr, " (errno %d, %s)\n", errno, strerror(errno));
			errno = 0;
	}
	else fprintf(stderr, "\n");

	if(error_log_syslog) {
		va_start( args, fmt );
		vsyslog(LOG_ERR,  fmt, args );
		va_end( args );
	}
}

#define fatal(args...)  fatal_int(__FILE__, __FUNCTION__, __LINE__, ##args)

void fatal_int( const char *file, const char *function, const unsigned long line, const char *fmt, ... )
{
	if(file) { ; }
	va_list args;

	log_date(stderr);

	va_start( args, fmt );
	if(debug_flags) fprintf(stderr, "FATAL (%04lu@%-15.15s): ", line, function);
	else            fprintf(stderr, "FATAL: ");
	vfprintf( stderr, fmt, args );
	va_end( args );

	perror(" # ");
	fprintf(stderr, "\n");

	if(error_log_syslog) {
		va_start( args, fmt );
		vsyslog(LOG_CRIT,  fmt, args );
		va_end( args );
	}

	exit(EXIT_FAILURE);
}

void log_access( const char *fmt, ... )
{
	va_list args;

	if(stdaccess) {
		log_date(stdaccess);

		va_start( args, fmt );
		vfprintf( stdaccess, fmt, args );
		va_end( args );
		fprintf( stdaccess, "\n");
		fflush( stdaccess );
	}

	if(access_log_syslog) {
		va_start( args, fmt );
		vsyslog(LOG_INFO,  fmt, args );
		va_end( args );
	}
}


// ----------------------------------------------------------------------------
// helpers

unsigned long long usecdiff(struct timeval *now, struct timeval *last) {
		return ((((now->tv_sec * 1000000ULL) + now->tv_usec) - ((last->tv_sec * 1000000ULL) + last->tv_usec)));
}

unsigned long simple_hash(const char *name)
{
	int i, len = strlen(name);
	unsigned long hash = 0;

	for(i = 0; i < len ;i++) hash += (i * name[i]) + i + name[i];

	return hash;
}

#define PIPE_READ 0
#define PIPE_WRITE 1

FILE *mypopen(const char *command, pid_t *pidptr)
{
	int pipefd[2];

	if(pipe(pipefd) == -1) return NULL;

	int pid = fork();
	if(pid == -1) {
		close(pipefd[PIPE_READ]);
		close(pipefd[PIPE_WRITE]);
		return NULL;
	}
	if(pid != 0) {
		// the parent
		*pidptr = pid;
		close(pipefd[PIPE_WRITE]);
		FILE *fp = fdopen(pipefd[PIPE_READ], "r");
		return(fp);
	}
	// the child

	// close all files
	int i;
	for(i = sysconf(_SC_OPEN_MAX); i > 0; i--)
		if(i != STDIN_FILENO && i != STDERR_FILENO && i != pipefd[PIPE_WRITE]) close(i);

	// move the pipe to stdout
	if(pipefd[PIPE_WRITE] != STDOUT_FILENO) {
		dup2(pipefd[PIPE_WRITE], STDOUT_FILENO);
		close(pipefd[PIPE_WRITE]);
	}

#ifdef DETACH_PLUGINS_FROM_NETDATA
	// this was an attempt to detach the child and use the suspend mode charts.d
	// unfortunatelly it does not work as expected.

	// fork again to become session leader
	pid = fork();
	if(pid == -1) fprintf(stderr, "Cannot fork again on pid %d\n", getpid());
	if(pid != 0) {
		// the parent
		exit(0);
	}

	// set a new process group id for just this child
	if( setpgid(0, 0) != 0 )
		fprintf(stderr, "Cannot set a new process group for pid %d (%s)\n", getpid(), strerror(errno));

	if( getpgid(0) != getpid() )
		fprintf(stderr, "Process group set is incorrect. Expected %d, found %d\n", getpid(), getpgid(0));

	if( setsid() != 0 )
		fprintf(stderr, "Cannot set session id for pid %d (%s)\n", getpid(), strerror(errno));

	fprintf(stdout, "MYPID %d\n", getpid());
	fflush(NULL);
#endif
	
	// ignore all signals
	for (i = 1 ; i < 65 ;i++) if(i != SIGSEGV) signal(i, SIG_DFL);

	fprintf(stderr, "executing command: '%s' on pid %d.\n", command, getpid());
 	execl("/bin/sh", "sh", "-c", command, NULL);
	exit(1);
}

void mypclose(FILE *fp)
{
	// this is a very poor implementation of pclose()
	// the caller should catch SIGCHLD and waitpid() on the exited child
	// otherwise the child will be a zombie forever

	fclose(fp);
}


// ----------------------------------------------------------------------------
// URL encode / decode
// code from: http://www.geekhideout.com/urlcode.shtml

/* Converts a hex character to its integer value */
char from_hex(char ch) {
	return (char)(isdigit(ch) ? ch - '0' : tolower(ch) - 'a' + 10);
}

/* Converts an integer value to its hex character*/
char to_hex(char code) {
	static char hex[] = "0123456789abcdef";
	return hex[code & 15];
}

/* Returns a url-encoded version of str */
/* IMPORTANT: be sure to free() the returned string after use */
char *url_encode(char *str) {
	char *pstr = str,
		*buf = malloc(strlen(str) * 3 + 1),
		*pbuf = buf;

	while (*pstr) {
		if (isalnum(*pstr) || *pstr == '-' || *pstr == '_' || *pstr == '.' || *pstr == '~')
			*pbuf++ = *pstr;

		else if (*pstr == ' ')
			*pbuf++ = '+';

		else
			*pbuf++ = '%', *pbuf++ = to_hex(*pstr >> 4), *pbuf++ = to_hex(*pstr & 15);

		pstr++;
	}

	*pbuf = '\0';

	return buf;
}

/* Returns a url-decoded version of str */
/* IMPORTANT: be sure to free() the returned string after use */
char *url_decode(char *str) {
	char *pstr = str,
		*buf = malloc(strlen(str) + 1),
		*pbuf = buf;

	if(!buf) fatal("Cannot allocate memory.");

	while (*pstr) {
		if (*pstr == '%') {
			if (pstr[1] && pstr[2]) {
				*pbuf++ = from_hex(pstr[1]) << 4 | from_hex(pstr[2]);
				pstr += 2;
			}
		}
		else if (*pstr == '+')
			*pbuf++ = ' ';

		else
			*pbuf++ = *pstr;
		
		pstr++;
	}
	
	*pbuf = '\0';
	
	return buf;
}


// ----------------------------------------------------------------------------
// socket

int listen_fd = -1;

int create_listen_socket(int port)
{
		int sock=-1;
		int sockopt=1;
		struct sockaddr_in name;

		debug(D_LISTENER, "Creating new listening socket on port %d", port);

		sock = socket(AF_INET, SOCK_STREAM, 0);
		if (sock < 0)
				fatal("socket() failed, errno=%d", errno);

		/* avoid "address already in use" */
		setsockopt(sock, SOL_SOCKET, SO_REUSEADDR, (void*)&sockopt, sizeof(sockopt));

		memset(&name, 0, sizeof(struct sockaddr_in));
		name.sin_family = AF_INET;
		name.sin_port = htons (port);
		name.sin_addr.s_addr = htonl (INADDR_ANY);
		if (bind (sock, (struct sockaddr *) &name, sizeof (name)) < 0)
			fatal("bind() failed, errno=%d", errno);

		if (listen(sock, LISTEN_BACKLOG) < 0)
			fatal("listen() failed, errno=%d", errno);

		debug(D_LISTENER, "Listening Port %d created", port);
		return sock;
}


// ----------------------------------------------------------------------------
// CONFIG

#define CONFIG_MAX_NAME 100
#define CONFIG_MAX_VALUE 1024
#define CONFIG_FILENAME "netdata.conf"
#define CONFIG_FILE_LINE_MAX 4096

pthread_rwlock_t config_rwlock = PTHREAD_RWLOCK_INITIALIZER;

struct config_value {
	char name[CONFIG_MAX_NAME + 1];
	char value[CONFIG_MAX_VALUE + 1];

	unsigned long hash;		// a simple hash to speed up searching
							// we first compare hashes, and only if the hashes are equal we do string comparisons

	int loaded;				// loaded from the user config
	int used;				// has been accessed from the program
	int changed;			// changed from the internal default

	struct config_value *next;
};

struct config {
	char name[CONFIG_MAX_NAME + 1];

	unsigned long hash;		// a simple hash to speed up searching
							// we first compare hashes, and only if the hashes are equal we do string comparisons

	struct config_value *values;

	struct config *next;
} *config_root = NULL;

struct config_value *config_value_create(struct config *co, const char *name, const char *value)
{
	debug(D_CONFIG, "Creating config entry for name '%s', value '%s', in section '%s'.", name, value, co->name);

	struct config_value *cv = calloc(1, sizeof(struct config_value));
	if(!cv) fatal("Cannot allocate config_value");

	strncpy(cv->name,  name,  CONFIG_MAX_NAME);
	strncpy(cv->value, value, CONFIG_MAX_VALUE);
	cv->hash = simple_hash(cv->name);

	// no need for string termination, due to calloc()

	struct config_value *cv2 = co->values;
	if(cv2) {
		while (cv2->next) cv2 = cv2->next;
		cv2->next = cv;
	}
	else co->values = cv;

	return cv;
}

struct config *config_create(const char *section)
{
	debug(D_CONFIG, "Creating section '%s'.", section);

	struct config *co = calloc(1, sizeof(struct config));
	if(!co) fatal("Cannot allocate config");

	strncpy(co->name, section, CONFIG_MAX_NAME);
	co->hash = simple_hash(co->name);

	// no need for string termination, due to calloc()

	struct config *co2 = config_root;
	if(co2) {
		while (co2->next) co2 = co2->next;
		co2->next = co;
	}
	else config_root = co;

	return co;
}

struct config *config_find_section(const char *section)
{
	struct config *co;
	unsigned long hash = simple_hash(section);

	for(co = config_root; co ; co = co->next)
		if(hash == co->hash)
			if(strcmp(co->name, section) == 0)
				break;

	return co;
}

char *trim(char *s)
{
	// skip leading spaces
	while(*s && isspace(*s)) s++;
	if(!*s || *s == '#') return NULL;

	// skip tailing spaces
	int c = strlen(s) - 1;
	while(c >= 0 && isspace(s[c])) {
		s[c] = '\0';
		c--;
	}
	if(c < 0) return NULL;
	if(!*s) return NULL;
	return s;
}

int load_config(char *filename, int overwrite_used)
{
	int line = 0;
	struct config *co = NULL;

	pthread_rwlock_wrlock(&config_rwlock);

	char buffer[CONFIG_FILE_LINE_MAX + 1], *s;

	if(!filename) filename = CONFIG_DIR "/" CONFIG_FILENAME;
	FILE *fp = fopen(filename, "r");
	if(!fp) {
		error("Cannot open file '%s'", CONFIG_DIR "/" CONFIG_FILENAME);
		pthread_rwlock_unlock(&config_rwlock);
		return 0;
	}

	while(fgets(buffer, CONFIG_FILE_LINE_MAX, fp) != NULL) {
		buffer[CONFIG_FILE_LINE_MAX] = '\0';
		line++;

		s = trim(buffer);
		if(!s) {
			debug(D_CONFIG, "Ignoring line %d, it is empty.", line);
			continue;
		}

		int len = strlen(s);
		if(*s == '[' && s[len - 1] == ']') {
			// new section
			s[len - 1] = '\0';
			s++;

			co = config_find_section(s);
			if(!co) co = config_create(s);

			continue;
		}

		if(!co) {
			// line outside a section
			error("Ignoring line %d ('%s'), it is outsize all sections.", line, s);
			continue;
		}

		char *name = s;
		char *value = strchr(s, '=');
		if(!value) {
			error("Ignoring line %d ('%s'), there is no = in it.", line, s);
			continue;
		}
		*value = '\0';
		value++;

		name = trim(name);
		value = trim(value);

		if(!name) {
			error("Ignoring line %d, name is empty.", line);
			continue;
		}
		if(!value) {
			debug(D_CONFIG, "Ignoring line %d, value is empty.", line);
			continue;
		}

		struct config_value *cv;
		for(cv = co->values; cv ; cv = cv->next)
			if(strcmp(cv->name, name) == 0) break;

		if(!cv) cv = config_value_create(co, name, value);
		else {
			if((cv->used && overwrite_used) || !cv->used) {
				debug(D_CONFIG, "Overwriting '%s/%s'.", line, co->name, cv->name);
				strncpy(cv->value, value, CONFIG_MAX_VALUE);
				// termination is already there
			}
			else
				debug(D_CONFIG, "Ignoring line %d, '%s/%s' is already present and used.", line, co->name, cv->name);
		}
		cv->loaded = 1;
	}

	fclose(fp);

	pthread_rwlock_unlock(&config_rwlock);
	return 1;
}

char *config_get(const char *section, const char *name, const char *default_value)
{
	struct config_value *cv;

	debug(D_CONFIG, "request to get config in section '%s', name '%s', default_value '%s'", section, name, default_value);

	pthread_rwlock_rdlock(&config_rwlock);

	struct config *co = config_find_section(section);
	if(!co) co = config_create(section);

	unsigned long hash = simple_hash(name);
	for(cv = co->values; cv ; cv = cv->next)
		if(hash == cv->hash)
			if(strcmp(cv->name, name) == 0)
				break;

	if(!cv) cv = config_value_create(co, name, default_value);
	cv->used = 1;

	if(cv->loaded || cv->changed) {
		// this is a loaded value from the config file
		// if it is different that the default, mark it
		if(strcmp(cv->value, default_value) != 0) cv->changed = 1;
	}
	else {
		// this is not loaded from the config
		// copy the default value to it
		strncpy(cv->value, default_value, CONFIG_MAX_VALUE);
	}

	pthread_rwlock_unlock(&config_rwlock);
	return(cv->value);
}

long long config_get_number(const char *section, const char *name, long long value)
{
	char buffer[100], *s;
	sprintf(buffer, "%lld", value);

	s = config_get(section, name, buffer);
	return strtoll(s, NULL, 0);
}

int config_get_boolean(const char *section, const char *name, int value)
{
	char *s;
	if(value) s = "yes";
	else s = "no";

	s = config_get(section, name, s);

	if(strcmp(s, "yes") == 0 || strcmp(s, "true") == 0 || strcmp(s, "1") == 0) {
		strcpy(s, "yes");
		return 1;
	}
	else {
		strcpy(s, "no");
		return 0;
	}
}

const char *config_set(const char *section, const char *name, const char *value)
{
	struct config_value *cv;

	debug(D_CONFIG, "request to set config in section '%s', name '%s', value '%s'", section, name, value);

	pthread_rwlock_wrlock(&config_rwlock);

	struct config *co = config_find_section(section);
	if(!co) co = config_create(section);

	unsigned long hash = simple_hash(name);
	for(cv = co->values; cv ; cv = cv->next)
		if(hash == cv->hash)
			if(strcmp(cv->name, name) == 0)
				break;

	if(!cv) cv = config_value_create(co, name, value);
	cv->used = 1;

	if(strcmp(cv->value, value) != 0) cv->changed = 1;

	strncpy(cv->value, value, CONFIG_MAX_VALUE);
	// termination is already there

	pthread_rwlock_unlock(&config_rwlock);

	return value;
}

long long config_set_number(const char *section, const char *name, long long value)
{
	char buffer[100];
	sprintf(buffer, "%lld", value);

	config_set(section, name, buffer);

	return value;
}

int config_set_boolean(const char *section, const char *name, int value)
{
	char *s;
	if(value) s = "yes";
	else s = "no";

	config_set(section, name, s);

	return value;
}


// ----------------------------------------------------------------------------
// chart types

#define CHART_TYPE_LINE	0
#define CHART_TYPE_AREA 1
#define CHART_TYPE_STACKED 2

int chart_type_id(const char *name)
{
	if(strcmp(name, "area") == 0) return CHART_TYPE_AREA;
	if(strcmp(name, "stacked") == 0) return CHART_TYPE_STACKED;
	if(strcmp(name, "line") == 0) return CHART_TYPE_LINE;
	return CHART_TYPE_LINE;
}

const char *chart_type_name(int chart_type)
{
	static char *line = "line";
	static char *area = "area";
	static char *stacked = "stacked";

	switch(chart_type) {
		case CHART_TYPE_LINE:
			return line;

		case CHART_TYPE_AREA:
			return area;

		case CHART_TYPE_STACKED:
			return stacked;
	}
	return line;
}


// ----------------------------------------------------------------------------
// algorithms types

#define RRD_DIMENSION_ABSOLUTE					0
#define RRD_DIMENSION_INCREMENTAL				1
#define RRD_DIMENSION_PCENT_OVER_DIFF_TOTAL 	2
#define RRD_DIMENSION_PCENT_OVER_ROW_TOTAL 		3

int algorithm_id(const char *name)
{
	if(strcmp(name, "absolute") == 0) return RRD_DIMENSION_ABSOLUTE;
	if(strcmp(name, "incremental") == 0) return RRD_DIMENSION_INCREMENTAL;
	if(strcmp(name, "percentage-of-absolute-row") == 0) return RRD_DIMENSION_PCENT_OVER_ROW_TOTAL;
	if(strcmp(name, "percentage-of-incremental-row") == 0) return RRD_DIMENSION_PCENT_OVER_DIFF_TOTAL;
	return RRD_DIMENSION_ABSOLUTE;
}

const char *algorithm_name(int chart_type)
{
	static char *absolute = "absolute";
	static char *incremental = "incremental";
	static char *percentage_of_absolute_row = "percentage-of-absolute-row";
	static char *percentage_of_incremental_row = "percentage-of-incremental-row";

	switch(chart_type) {
		case RRD_DIMENSION_ABSOLUTE:
			return absolute;

		case RRD_DIMENSION_INCREMENTAL:
			return incremental;

		case RRD_DIMENSION_PCENT_OVER_ROW_TOTAL:
			return percentage_of_absolute_row;

		case RRD_DIMENSION_PCENT_OVER_DIFF_TOTAL:
			return percentage_of_incremental_row;
	}
	return absolute;
}


// ----------------------------------------------------------------------------
// FAST NUMBER TO STRING

static void strreverse(char* begin, char* end)
{
    char aux;
    while (end > begin)
        aux = *end, *end-- = *begin, *begin++ = aux;
}


// ----------------------------------------------------------------------------
// mmap() wrapper

#define NETDATA_MEMORY_MODE_RAM_NAME "ram"
#define NETDATA_MEMORY_MODE_MAP_NAME "map"
#define NETDATA_MEMORY_MODE_SAVE_NAME "save"

#define NETDATA_MEMORY_MODE_RAM 0
#define NETDATA_MEMORY_MODE_MAP 1
#define NETDATA_MEMORY_MODE_SAVE 2

int memory_mode = NETDATA_MEMORY_MODE_SAVE;

const char *memory_mode_name(int id)
{
	static const char *ram = NETDATA_MEMORY_MODE_RAM_NAME;
	static const char *map = NETDATA_MEMORY_MODE_MAP_NAME;
	static const char *save = NETDATA_MEMORY_MODE_SAVE_NAME;
	const char *s = save;

	switch(id) {
		case NETDATA_MEMORY_MODE_RAM:
			s = ram;
			;;

		case NETDATA_MEMORY_MODE_MAP:
			s = map;
			;;

		case NETDATA_MEMORY_MODE_SAVE:
		default:
			s = save;
			;;
	}

	return(s);
}

int memory_mode_id(const char *name)
{
	if(!strcmp(name, NETDATA_MEMORY_MODE_RAM_NAME))
		return NETDATA_MEMORY_MODE_RAM;
	else if(!strcmp(name, NETDATA_MEMORY_MODE_MAP_NAME))
		return NETDATA_MEMORY_MODE_MAP;

	return NETDATA_MEMORY_MODE_SAVE;
}

void *mymmap(const char *filename, unsigned long size)
{
	if(memory_mode == NETDATA_MEMORY_MODE_RAM) return NULL;

	int fd, flags;
	void *mem = NULL;

	if(memory_mode == NETDATA_MEMORY_MODE_MAP)
		flags = MAP_SHARED;
	else
		flags = MAP_PRIVATE;

	errno = 0;
	fd = open(filename, O_RDWR|O_CREAT|O_NOATIME, 0664);
	if(fd != -1) {
		if(lseek(fd, size, SEEK_SET) == (long)size) {
			if(write(fd, "", 1) == 1) {

				if(ftruncate(fd, size))
					error("Cannot truncate file '%s' to size %ld. Will use the larger file.", filename, size);

				mem = mmap(NULL, size, PROT_READ|PROT_WRITE, flags, fd, 0);
				if(mem) {
					if(madvise(mem, size, MADV_SEQUENTIAL|MADV_DONTFORK|MADV_WILLNEED) != 0)
						error("Cannot advise the kernel about the memory usage of file '%s'.", filename);
				}
			}
			else error("Cannot write to file '%s' at position %ld.", filename, size);
		}
		else error("Cannot seek file '%s' to size %ld.", filename, size);

		close(fd);
	}
	else error("Cannot create/open file '%s'.", filename);

	return mem;
}

int savememory(const char *filename, void *mem, unsigned long size)
{
	char tmpfilename[FILENAME_MAX + 1];

	snprintf(tmpfilename, FILENAME_MAX, "%s.%ld.tmp", filename, (long)getpid());

	int fd = open(tmpfilename, O_RDWR|O_CREAT|O_NOATIME, 0664);
	if(fd < 0) {
		error("Cannot create/open file '%s'.", filename);
		return -1;
	}

	if(write(fd, mem, size) != (long)size) {
		error("Cannot write to file '%s' %ld bytes.", filename, (long)size);
		close(fd);
		return -1;
	}

	close(fd);

	int ret = 0;
	if(rename(tmpfilename, filename)) {
		error("Cannot rename '%s' to '%s'", tmpfilename, filename);
		ret = -1;
	}

	return ret;
}


// ----------------------------------------------------------------------------
// RRD STATS

#define RRD_STATS_NAME_MAX 1024

typedef long double calculated_number;
#define CALCULATED_NUMBER_FORMAT "%0.3Lf"
//typedef long long calculated_number;
//#define CALCULATED_NUMBER_FORMAT "%lld"

typedef long long collected_number;
#define COLLECTED_NUMBER_FORMAT "%lld"

typedef long long total_number;
#define TOTAL_NUMBER_FORMAT "%lld"

typedef int32_t storage_number;
typedef uint32_t ustorage_number;
#define STORAGE_NUMBER_FORMAT "%d"

#define RRD_STATS_MAGIC     "NETDATA CACHE STATS FILE V007"
#define RRD_DIMENSION_MAGIC "NETDATA CACHE DIMENSION FILE V005"

struct rrd_dimension {
	char magic[sizeof(RRD_DIMENSION_MAGIC) + 1];// our magic
	char id[RRD_STATS_NAME_MAX + 1];			// the id of this dimension (for internal identification)
	char *name;									// the name of this dimension (as presented to user)
	char cache_file[FILENAME_MAX+1];
	
	unsigned long hash;							// a simple hash on the id, to speed up searching
												// we first compare hashes, and only if the hashes are equal we do string comparisons

	long entries;								// how many entries this dimension has
												// this should be the same to the entries of the data set
	long current_entry;							// the entry that is currently being updated

	int hidden;									// if set to non zero, this dimension will not be sent to the client
	int mapped;									// 1 if the file is mapped
	unsigned long memsize;						// the memory allocated for this dimension

	int algorithm;
	long multiplier;
	long divisor;

	struct timeval last_collected;				// when was this dimension last updated

	calculated_number calculated_value;
	calculated_number last_calculated_value;

	collected_number collected_value;
	collected_number last_collected_value;

	struct rrd_dimension *next;					// linking of dimensions within the same data set

	storage_number values[];					// the array of values - THIS HAS TO BE THE LAST MEMBER
};
typedef struct rrd_dimension RRD_DIMENSION;

struct rrd_stats {
	char magic[sizeof(RRD_STATS_MAGIC) + 1];// our magic

	char id[RRD_STATS_NAME_MAX + 1];			// id of the data set
	char *name;									// name of the data set
	char *cache_dir;							// the directory to store dimension maps
	char cache_file[FILENAME_MAX+1];

	char *type;									// the type of graph RRD_TYPE_* (a category, for determining graphing options)
	char *family;								// the family of this data set (for grouping them together)
	char *title;								// title shown to user
	char *units;								// units of measurement

	pthread_rwlock_t rwlock;
	unsigned long counter;						// the number of times we added values to this rrd
	unsigned long counter_done;					// the number of times we added values to this rrd

	int mapped;									// if set to 1, this is memory mapped
	unsigned long memsize;						// how much mem we have allocated for this (without dimensions)

	unsigned long hash_name;					// a simple hash on the name
	unsigned long hash;							// a simple hash on the id, to speed up searching
												// we first compare hashes, and only if the hashes are equal we do string comparisons

	long priority;

	long entries;								// total number of entries in the data set
	long current_entry;							// the entry that is currently being updated
												// it goes around in a round-robin fashion

	int update_every;							// every how many seconds is this updated?
	unsigned long long first_entry_t;			// the timestamp (in microseconds) of the oldest entry in the db
	struct timeval last_updated;				// when this data set was last updated
	struct timeval last_collected;
	unsigned long long usec_since_last_update;

	total_number absolute_total;
	total_number last_absolute_total;

	int chart_type;
	int debug;
	int enabled;
	int isdetail;								// if set, the data set should be considered as a detail of another
												// (the master data set should be the one that has the same family and is not detail)

	RRD_DIMENSION *dimensions;					// the actual data for every dimension

	struct rrd_stats *next;						// linking of rrd stats
};
typedef struct rrd_stats RRD_STATS;

RRD_STATS *root = NULL;
pthread_rwlock_t root_rwlock = PTHREAD_RWLOCK_INITIALIZER;

char *rrd_stats_strncpy_name(char *to, const char *from, int length)
{
	int i;
	for(i = 0; i < length && from[i] ;i++) {
		if(from[i] == '.' || isalpha(from[i]) || isdigit(from[i])) to[i] = from[i];
		else to[i] = '_';
	}
	if(i < length) to[i] = '\0';
	to[length - 1] = '\0';

	return to;
}

void rrd_stats_set_name(RRD_STATS *st, const char *name)
{
	char b[CONFIG_MAX_VALUE + 1];
	char n[RRD_STATS_NAME_MAX + 1];

	snprintf(n, RRD_STATS_NAME_MAX, "%s.%s", st->type, name);
	rrd_stats_strncpy_name(b, n, CONFIG_MAX_VALUE);
	st->name = config_get(st->id, "name", b);
	st->hash_name = simple_hash(st->name);
}

char *rrd_stats_cache_dir(const char *id)
{
	char *ret = NULL;

	static char *cache_dir = NULL;
	if(!cache_dir) cache_dir = config_get("global", "database directory", "cache");

	char b[FILENAME_MAX + 1];
	char n[FILENAME_MAX + 1];
	rrd_stats_strncpy_name(b, id, FILENAME_MAX);

	snprintf(n, FILENAME_MAX, "%s/%s", cache_dir, b);
	ret = config_get(id, "database directory", n);

	if(memory_mode == NETDATA_MEMORY_MODE_MAP || memory_mode == NETDATA_MEMORY_MODE_SAVE) {
		int r = mkdir(ret, 0775);
		if(r != 0 && errno != EEXIST)
			error("Cannot create directory '%s'", ret);
	}

	return ret;
}

RRD_STATS *rrd_stats_create(const char *type, const char *id, const char *name, const char *family, const char *title, const char *units, long priority, int update_every, int chart_type)
{
	if(!id || !id[0]) {
		fatal("Cannot create rrd stats without an id.");
		return NULL;
	}

	char fullid[RRD_STATS_NAME_MAX + 1];
	char fullfilename[FILENAME_MAX + 1];
	RRD_STATS *st;

	snprintf(fullid, RRD_STATS_NAME_MAX, "%s.%s", type, id);

	long entries = config_get_number(fullid, "history", save_history);
	if(entries < 5) entries = config_set_number(fullid, "history", 5);
	if(entries > HISTORY_MAX) entries = config_set_number(fullid, "history", HISTORY_MAX);

	int enabled = config_get_boolean(fullid, "enabled", 1);
	if(!enabled) entries = 5;

	unsigned long size = sizeof(RRD_STATS);
	char *cache_dir = rrd_stats_cache_dir(fullid);

	debug(D_RRD_STATS, "Creating RRD_STATS for '%s.%s'.", type, id);

	snprintf(fullfilename, FILENAME_MAX, "%s/main.db", cache_dir);
	st = (RRD_STATS *)mymmap(fullfilename, size);
	if(st) {
		if(strcmp(st->magic, RRD_STATS_MAGIC) != 0) {
			errno = 0;
			error("File %s does not have our version. Clearing it.", fullfilename);
			bzero(st, size);
		}
		else if(strcmp(st->id, fullid) != 0) {
			errno = 0;
			error("File %s does not have our id. Unmapping it.", fullfilename);
			munmap(st, size);
			st = NULL;
		}
		else if(st->memsize != size || st->entries != entries) {
			errno = 0;
			error("File %s does not have the desired size. Clearing it.", fullfilename);
			bzero(st, size);
		}
		else if(st->update_every != update_every) {
			errno = 0;
			error("File %s does not have the desired update frequency. Clearing it.", fullfilename);
			bzero(st, size);
		}
	}

	if(st) {
		st->name = NULL;
		st->type = NULL;
		st->family = NULL;
		st->title = NULL;
		st->units = NULL;
		st->dimensions = NULL;
		st->next = NULL;
		st->mapped = memory_mode;
	}
	else {
		st = calloc(1, size);
		if(!st) {
			fatal("Cannot allocate memory for RRD_STATS %s.%s", type, id);
			return NULL;
		}
		st->mapped = NETDATA_MEMORY_MODE_RAM;
	}
	st->memsize = size;
	st->entries = entries;
	st->update_every = update_every;

	strcpy(st->cache_file, fullfilename);
	strcpy(st->magic, RRD_STATS_MAGIC);

	strcpy(st->id, fullid);
	st->hash = simple_hash(st->id);

	st->cache_dir = cache_dir;

	st->family     = config_get(st->id, "family", family?family:st->id);
	st->units      = config_get(st->id, "units", units?units:"");
	st->type       = config_get(st->id, "type", type);
	st->chart_type = chart_type_id(config_get(st->id, "chart type", chart_type_name(chart_type)));

	if(name && *name) rrd_stats_set_name(st, name);
	else rrd_stats_set_name(st, id);

	{
		char varvalue[CONFIG_MAX_VALUE + 1];
		snprintf(varvalue, CONFIG_MAX_VALUE, "%s (%s)", title?title:"", st->name);
		st->title = config_get(st->id, "title", varvalue);
	}

	st->priority = config_get_number(st->id, "priority", priority);
	st->enabled = enabled;

	st->debug = 0;

	st->last_collected.tv_sec = 0;
	st->last_collected.tv_usec = 0;

	pthread_rwlock_init(&st->rwlock, NULL);
	pthread_rwlock_wrlock(&root_rwlock);

	st->next = root;
	root = st;

	pthread_rwlock_unlock(&root_rwlock);

	return(st);
}

RRD_DIMENSION *rrd_stats_dimension_add(RRD_STATS *st, const char *id, const char *name, long multiplier, long divisor, int algorithm)
{
	char filename[FILENAME_MAX + 1];
	char fullfilename[FILENAME_MAX + 1];

	char varname[CONFIG_MAX_NAME + 1];
	RRD_DIMENSION *rd;
	unsigned long size = sizeof(RRD_DIMENSION) + (st->entries * sizeof(storage_number));

	debug(D_RRD_STATS, "Adding dimension '%s/%s'.", st->id, id);

	rrd_stats_strncpy_name(filename, id, FILENAME_MAX);
	snprintf(fullfilename, FILENAME_MAX, "%s/%s.db", st->cache_dir, filename);
	rd = (RRD_DIMENSION *)mymmap(fullfilename, size);
	if(rd) {
		if(strcmp(rd->magic, RRD_DIMENSION_MAGIC) != 0) {
			errno = 0;
			error("File %s does not have our version. Clearing it.", fullfilename);
			bzero(rd, size);
		}
		else if(rd->memsize != size) {
			errno = 0;
			error("File %s does not have the desired size. Clearing it.", fullfilename);
			bzero(rd, size);
		}
		else if(rd->multiplier != multiplier) {
			errno = 0;
			error("File %s does not have the same multiplier. Clearing it.", fullfilename);
			bzero(rd, size);
		}
		else if(rd->divisor != divisor) {
			errno = 0;
			error("File %s does not have the same divisor. Clearing it.", fullfilename);
			bzero(rd, size);
		}
		else if(rd->algorithm != algorithm) {
			errno = 0;
			error("File %s does not have the same algorithm. Clearing it.", fullfilename);
			bzero(rd, size);
		}
		else if(strcmp(rd->id, id) != 0) {
			errno = 0;
			error("File %s does not have our dimension id. Unmapping it.", fullfilename);
			munmap(rd, size);
			rd = NULL;
		}
	}

	if(rd) {
		// we have a file mapped for rd
		rd->mapped = memory_mode;
		rd->hidden = 0;
		rd->next = NULL;
		rd->name = NULL;
	}
	else {
		// if we didn't manage to get a mmap'd dimension, just create one

		rd = calloc(1, size);
		if(!rd) {
			fatal("Cannot allocate RRD_DIMENSION %s/%s.", st->id, id);
			return NULL;
		}

		rd->mapped = NETDATA_MEMORY_MODE_RAM;
	}
	rd->memsize = size;

	strcpy(rd->magic, RRD_DIMENSION_MAGIC);
	strcpy(rd->cache_file, fullfilename);
	strncpy(rd->id, id, RRD_STATS_NAME_MAX);
	rd->hash = simple_hash(rd->id);

	snprintf(varname, CONFIG_MAX_NAME, "dim %s name", rd->id);
	rd->name = config_get(st->id, varname, (name && *name)?name:rd->id);

	snprintf(varname, CONFIG_MAX_NAME, "dim %s algorithm", rd->id);
	rd->algorithm = algorithm_id(config_get(st->id, varname, algorithm_name(algorithm)));

	snprintf(varname, CONFIG_MAX_NAME, "dim %s multiplier", rd->id);
	rd->multiplier = config_get_number(st->id, varname, multiplier);

	snprintf(varname, CONFIG_MAX_NAME, "dim %s divisor", rd->id);
	rd->divisor = config_get_number(st->id, varname, divisor);
	if(!rd->divisor) rd->divisor = 1;

	rd->entries = st->entries;
	
	// append this dimension
	if(!st->dimensions)
		st->dimensions = rd;
	else {
		RRD_DIMENSION *td = st->dimensions;
		for(; td->next; td = td->next) ;
		td->next = rd;
	}

	return(rd);
}

void rrd_stats_dimension_set_name(RRD_STATS *st, RRD_DIMENSION *rd, const char *name)
{
	char varname[CONFIG_MAX_NAME + 1];
	snprintf(varname, CONFIG_MAX_NAME, "dim %s name", rd->id);
	config_get(st->id, varname, name);
}

void rrd_stats_dimension_free(RRD_DIMENSION *rd)
{
	if(rd->next) rrd_stats_dimension_free(rd->next);
	// free(rd->annotations);
	if(rd->mapped == NETDATA_MEMORY_MODE_SAVE) {
		debug(D_RRD_STATS, "Saving dimension '%s' to '%s'.", rd->name, rd->cache_file);
		savememory(rd->cache_file, rd, rd->memsize);

		debug(D_RRD_STATS, "Unmapping dimension '%s'.", rd->name);
		munmap(rd, rd->memsize);
	}
	else if(rd->mapped == NETDATA_MEMORY_MODE_MAP) {
		debug(D_RRD_STATS, "Unmapping dimension '%s'.", rd->name);
		munmap(rd, rd->memsize);
	}
	else {
		debug(D_RRD_STATS, "Removing dimension '%s'.", rd->name);
		free(rd);
	}
}

void rrd_stats_free_all(void)
{
	pthread_rwlock_wrlock(&root_rwlock);
	RRD_STATS *st;
	for(st = root; st ;) {
		RRD_STATS *next = st->next;

		pthread_rwlock_wrlock(&st->rwlock);

		if(st->dimensions) rrd_stats_dimension_free(st->dimensions);
		st->dimensions = NULL;

		// we leave it locked, or other threads may crash...
		// pthread_rwlock_unlock(&st->rwlock);

		if(st->mapped == NETDATA_MEMORY_MODE_SAVE) {
			debug(D_RRD_STATS, "Saving stats '%s' to '%s'.", st->name, st->cache_file);
			savememory(st->cache_file, st, st->memsize);

			debug(D_RRD_STATS, "Unmapping stats '%s'.", st->name);
			munmap(st, st->memsize);
		}
		else if(st->mapped == NETDATA_MEMORY_MODE_MAP) {
			debug(D_RRD_STATS, "Unmapping stats '%s'.", st->name);
			munmap(st, st->memsize);
		}
		else
			free(st);

		st = next;
	}
	root = NULL;
	pthread_rwlock_unlock(&root_rwlock);
}

RRD_STATS *rrd_stats_find(const char *id)
{
	unsigned long hash = simple_hash(id);

	pthread_rwlock_rdlock(&root_rwlock);
	RRD_STATS *st = root;
	for ( ; st ; st = st->next )
		if(hash == st->hash)
			if(strcmp(st->id, id) == 0)
				break;
	pthread_rwlock_unlock(&root_rwlock);

	return(st);
}

RRD_STATS *rrd_stats_find_bytype(const char *type, const char *id)
{
	char buf[RRD_STATS_NAME_MAX + 1];

	strncpy(buf, type, RRD_STATS_NAME_MAX - 1);
	buf[RRD_STATS_NAME_MAX - 1] = '\0';
	strcat(buf, ".");
	int len = strlen(buf);
	strncpy(&buf[len], id, RRD_STATS_NAME_MAX - len);
	buf[RRD_STATS_NAME_MAX] = '\0';

	return(rrd_stats_find(buf));
}

RRD_STATS *rrd_stats_find_byname(const char *name)
{
	char b[CONFIG_MAX_VALUE + 1];

	rrd_stats_strncpy_name(b, name, CONFIG_MAX_VALUE);
	unsigned long hash = simple_hash(b);

	pthread_rwlock_rdlock(&root_rwlock);
	RRD_STATS *st = root;
	for ( ; st ; st = st->next ) {
		if(hash == st->hash_name && strcmp(st->name, b) == 0) break;
	}
	pthread_rwlock_unlock(&root_rwlock);

	return(st);
}

RRD_DIMENSION *rrd_stats_dimension_find(RRD_STATS *st, const char *id)
{
	unsigned long hash = simple_hash(id);

	RRD_DIMENSION *rd = st->dimensions;

	for ( ; rd ; rd = rd->next )
		if(hash == rd->hash)
			if(strcmp(rd->id, id) == 0)
				break;

	return(rd);
}

int rrd_stats_dimension_hide(RRD_STATS *st, const char *id)
{
	RRD_DIMENSION *rd = rrd_stats_dimension_find(st, id);
	if(!rd) {
		error("Cannot find dimension with id '%s' on stats '%s' (%s).", id, st->name, st->id);
		return 1;
	}

	rd->hidden = 1;
	return 0;
}

void rrd_stats_dimension_set_by_pointer(RRD_STATS *st, RRD_DIMENSION *rd, collected_number value)
{
	if(!st->last_collected.tv_sec) gettimeofday(&st->last_collected, NULL);

	rd->last_collected.tv_sec = st->last_collected.tv_sec;
	rd->last_collected.tv_usec = st->last_collected.tv_usec;

	rd->collected_value = value;
}

int rrd_stats_dimension_set(RRD_STATS *st, char *id, collected_number value)
{
	RRD_DIMENSION *rd = rrd_stats_dimension_find(st, id);
	if(!rd) {
		error("Cannot find dimension with id '%s' on stats '%s' (%s).", id, st->name, st->id);
		return 1;
	}

	rrd_stats_dimension_set_by_pointer(st, rd, value);
	return 0;
}

void rrd_stats_next_internal(RRD_STATS *st)
{
	// a read lock is OK here
	pthread_rwlock_rdlock(&st->rwlock);

	RRD_DIMENSION *rd;
	for( rd = st->dimensions; rd ; rd = rd->next ) {
		rd->last_collected_value = rd->collected_value;
		rd->collected_value = 0;
	}

	pthread_rwlock_unlock(&st->rwlock);
}

void rrd_stats_next_timeval(RRD_STATS *st, struct timeval *now)
{
	if(!st->last_collected.tv_sec) {
		gettimeofday(&st->last_collected, NULL);
		unsigned long long ut = st->last_collected.tv_sec * 1000000ULL + st->last_collected.tv_usec - st->update_every * 1000000ULL;
		st->last_collected.tv_sec = ut / 1000000ULL;
		st->last_collected.tv_usec = ut % 1000000ULL;
	}

	st->usec_since_last_update = usecdiff(now, &st->last_collected);
	st->last_collected.tv_sec = now->tv_sec;
	st->last_collected.tv_usec = now->tv_usec;

	rrd_stats_next_internal(st);
}

void rrd_stats_next_usec(RRD_STATS *st, unsigned long long microseconds)
{
	if(!st->last_collected.tv_sec) {
		gettimeofday(&st->last_collected, NULL);
		unsigned long long ut = st->last_collected.tv_sec * 1000000ULL + st->last_collected.tv_usec;
		st->last_collected.tv_sec = ut / 1000000ULL;
		st->last_collected.tv_usec = ut % 1000000ULL;
	}
	else {
		unsigned long long ut = st->last_collected.tv_sec * 1000000ULL + st->last_collected.tv_usec + microseconds;
		st->last_collected.tv_sec = ut / 1000000ULL;
		st->last_collected.tv_usec = ut % 1000000ULL;
	}
	st->usec_since_last_update = microseconds;

	rrd_stats_next_internal(st);
}

void rrd_stats_next(RRD_STATS *st)
{
	if(st->last_collected.tv_sec) {
		struct timeval now;
		gettimeofday(&now, NULL);

		rrd_stats_next_timeval(st, &now);
	}
	else
		rrd_stats_next_usec(st, st->update_every * 1000000ULL);
}

void rrd_stats_next_plugins(RRD_STATS *st)
{
	rrd_stats_next_usec(st, st->update_every * 1000000ULL);
}

unsigned long long rrd_stats_done(RRD_STATS *st)
{
	RRD_DIMENSION *rd, *last;

	// a read lock is OK here
	pthread_rwlock_rdlock(&st->rwlock);

	if(!st->last_updated.tv_sec) {
		unsigned long long ut = st->last_collected.tv_sec * 1000000ULL + st->last_collected.tv_usec - st->usec_since_last_update;
		st->last_updated.tv_sec = ut / 1000000ULL;
		st->last_updated.tv_usec = ut % 1000000ULL;
	}

	unsigned long long last_ut = st->last_updated.tv_sec * 1000000ULL + st->last_updated.tv_usec;
	unsigned long long now_ut = st->last_collected.tv_sec * 1000000ULL + st->last_collected.tv_usec + st->usec_since_last_update;
	unsigned long long next_ut = (st->last_updated.tv_sec + st->update_every) * 1000000ULL;

	st->counter_done++;
	if(st->counter_done == 1 || now_ut < next_ut) {
		if(st->debug) debug(D_RRD_STATS, "%s: Skipping collected values (usec since last update = %llu, counter_done = %lu)", st->name, st->usec_since_last_update, st->counter_done);
		// we don't have any usable data yet
		pthread_rwlock_unlock(&st->rwlock);
		return(st->usec_since_last_update);
	}

	if(st->debug) debug(D_RRD_STATS, "microseconds since last update: %llu", st->usec_since_last_update);

	// calculate totals and count the dimensions
	int dimensions;
	st->last_absolute_total  = st->absolute_total;
	st->absolute_total = 0;
	for( rd = st->dimensions, dimensions = 0 ; rd ; rd = rd->next, dimensions++ )
		st->absolute_total += rd->collected_value;

	// process all dimensions to calculate its values
	for( rd = st->dimensions ; rd ; rd = rd->next ) {
		switch(rd->algorithm) {
			case RRD_DIMENSION_PCENT_OVER_DIFF_TOTAL:
				// the percentage of the current increment
				// over the increment of all dimensions together
				if(st->absolute_total == st->last_absolute_total) rd->calculated_value = 0;
				else rd->calculated_value =
					  (calculated_number)100
					* (calculated_number)(rd->collected_value - rd->last_collected_value)
					/ (calculated_number)(st->absolute_total  - st->last_absolute_total);

				if(st->debug)
					debug(D_RRD_STATS, "%s/%s: CALC "
						CALCULATED_NUMBER_FORMAT " = 100"
						" * (" COLLECTED_NUMBER_FORMAT " - " COLLECTED_NUMBER_FORMAT ")"
						" / (" COLLECTED_NUMBER_FORMAT " - " COLLECTED_NUMBER_FORMAT ")"
						, st->id, rd->name
						, rd->calculated_value
						, rd->collected_value, rd->last_collected_value
						, st->absolute_total, st->last_absolute_total
						);
				break;

			case RRD_DIMENSION_PCENT_OVER_ROW_TOTAL:
				if(!st->absolute_total) rd->calculated_value = 0;
				else
				// the percentage of the current value
				// over the total of all dimensions
				rd->calculated_value =
					  (calculated_number)100
					* (calculated_number)rd->collected_value
					/ (calculated_number)st->absolute_total;

				if(st->debug)
					debug(D_RRD_STATS, "%s/%s: CALC "
						CALCULATED_NUMBER_FORMAT " = 100"
						" * " COLLECTED_NUMBER_FORMAT
						" / " COLLECTED_NUMBER_FORMAT
						, st->id, rd->name
						, rd->calculated_value
						, rd->collected_value
						, st->absolute_total
						);
				break;

			case RRD_DIMENSION_INCREMENTAL:
				// we need the incremental calculation to produce per second results
				// so, we multiply with 1.000.000 and divide by the microseconds passed since
				// the last entry

				// if the new is smaller than the old (an overflow, or reset), set the old equal to the new
				// to reset the calculation (it will give zero as the calculation for this second)
				if(rd->last_collected_value > rd->collected_value) rd->last_collected_value = rd->collected_value;

				rd->calculated_value =
					  (calculated_number)1000000
					* (calculated_number)(rd->collected_value - rd->last_collected_value)
					/ (calculated_number)st->usec_since_last_update;

				if(st->debug)
					debug(D_RRD_STATS, "%s/%s: CALC "
						CALCULATED_NUMBER_FORMAT " = 1000000"
						" * (" COLLECTED_NUMBER_FORMAT " - " COLLECTED_NUMBER_FORMAT ")"
						" / %llu"
						, st->id, rd->name
						, rd->calculated_value
						, rd->collected_value, rd->last_collected_value
						, st->usec_since_last_update
						);
				break;

			case RRD_DIMENSION_ABSOLUTE:
				rd->calculated_value = (calculated_number)rd->collected_value;

				if(st->debug)
					debug(D_RRD_STATS, "%s/%s: CALC "
						CALCULATED_NUMBER_FORMAT " = "
						COLLECTED_NUMBER_FORMAT
						, st->id, rd->name
						, rd->calculated_value
						, rd->collected_value
						);
				break;

			default:
				// make the default zero, to make sure
				// it gets noticed when we add new types
				rd->calculated_value = 0;

				if(st->debug)
					debug(D_RRD_STATS, "%s/%s: CALC "
						CALCULATED_NUMBER_FORMAT " = 0"
						, st->id, rd->name
						, rd->calculated_value
						);
				break;
		}
	}

	for( ; next_ut < now_ut ; next_ut += st->update_every * 1000000ULL ) {
		unsigned long long np = next_ut - last_ut;

		st->last_updated.tv_sec = next_ut / 1000000ULL;
		st->last_updated.tv_usec = 0;

		for( rd = st->dimensions ; rd ; rd = rd->next ) {
			rd->calculated_value = (calculated_number)
				(	(	  (rd->calculated_value - rd->last_calculated_value)
						* (calculated_number)np
						/ (calculated_number)(now_ut - last_ut)
					)
					+  rd->last_calculated_value
				);

			rd->values[st->current_entry] = (storage_number)
				(	  rd->calculated_value
					* (calculated_number)10
					* (calculated_number)rd->multiplier
					/ (calculated_number)rd->divisor
				);

			if(st->debug)
				debug(D_RRD_STATS, "%s/%s: STORE[%ld] "
					STORAGE_NUMBER_FORMAT " = ((("
					"(" CALCULATED_NUMBER_FORMAT " - " CALCULATED_NUMBER_FORMAT ")"
					" * %llu"
					" / %llu) + " CALCULATED_NUMBER_FORMAT
					" * 10 "
					" * %ld"
					" / %ld"
					, st->id, rd->name
					, st->current_entry
					, rd->values[st->current_entry]
					, rd->calculated_value, rd->last_calculated_value
					, np
					, (now_ut - last_ut), rd->last_calculated_value
					, rd->multiplier
					, rd->divisor
					);

			rd->last_calculated_value = rd->calculated_value;
		}

		if(st->first_entry_t && st->counter >= (unsigned long long)st->entries) {
			// the db is overwriting values
			// add the value we will overwrite
			st->first_entry_t += st->update_every * 1000000ULL;
		}
		
		st->counter++;
		st->current_entry = ((st->current_entry + 1) >= st->entries) ? 0 : st->current_entry + 1;
		if(!st->first_entry_t) st->first_entry_t = next_ut;
		last_ut = next_ut;
	}

	// ALL DONE ABOUT THE DATA UPDATE
	// --------------------------------------------------------------------


	// find if there are any obsolete dimensions (not updated recently)
	for( rd = st->dimensions; rd ; rd = rd->next )
		if((rd->last_collected.tv_sec + (10 * st->update_every)) < st->last_collected.tv_sec)
			break;

	if(rd) {
		// there is dimension to free
		// upgrade our read lock to a write lock
		pthread_rwlock_unlock(&st->rwlock);
		pthread_rwlock_wrlock(&st->rwlock);

		for( rd = st->dimensions, last = NULL ; rd ; ) {
			if((rd->last_collected.tv_sec + (10 * st->update_every)) < st->last_collected.tv_sec) { // remove it only it is not updated in 10 seconds
				debug(D_RRD_STATS, "Removing obsolete dimension '%s' (%s) of '%s' (%s).", rd->name, rd->id, st->name, st->id);

				if(!last) {
					st->dimensions = rd->next;
					rd->next = NULL;
					rrd_stats_dimension_free(rd);
					rd = st->dimensions;
					continue;
				}
				else {
					last->next = rd->next;
					rd->next = NULL;
					rrd_stats_dimension_free(rd);
					rd = last->next;
					continue;
				}
			}

			last = rd;
			rd = rd->next;
		}

		if(!st->dimensions) st->enabled = 0;
	}

	pthread_rwlock_unlock(&st->rwlock);
	return(st->usec_since_last_update);
}



// ----------------------------------------------------------------------------
// web buffer

struct web_buffer {
	long size;	// allocation size of buffer
	long bytes;	// current data length in buffer
	long sent;	// current data length sent to output
	char *buffer;	// the buffer
	int contenttype;
	long rbytes; 	// if non-zero, the excepted size of ifd
	time_t date;	// the date this content has been generated
};

#define web_buffer_printf(wb, args...) wb->bytes += snprintf(&wb->buffer[wb->bytes], (wb->size - wb->bytes), ##args)
#define web_buffer_reset(wb) wb->buffer[wb->bytes = 0] = '\0'

void web_buffer_strcpy(struct web_buffer *wb, const char *txt)
{
	char *buffer = wb->buffer;
	long bytes = wb->bytes, size = wb->size, i = 0;

	while(txt[i] && bytes < size)
		buffer[bytes++] = txt[i++];

	wb->bytes = bytes;
}

void web_buffer_rrd_value(struct web_buffer *wb, storage_number value)
{
	if(wb->size - wb->bytes < 11) return;

	char *str = &wb->buffer[wb->bytes];
	char *wstr = str;

	// make sure it is unsigned
	ustorage_number uvalue = (value < 0) ? -value : value;

	// print each digit
	do *wstr++ = (char)(48 + (uvalue % 10)); while(uvalue /= 10);

	// if it is just one byte, add a zero
	if((wstr - str) == 1) *wstr++ = '0';

	// put the sign back
	if (value < 0) *wstr++ = '-';

	// reverse it
	strreverse(str, wstr-1);

	// move the last digit
	wstr--;
	wstr[1] = wstr[0];

	// put the dot
	wstr[0] = '.';

	// terminate it
	wstr += 2;
	*wstr='\0';

	// update the buffer length
	wb->bytes += (wstr - str);
}

// generate a javascript date, the fastest possible way...
void web_buffer_jsdate(struct web_buffer *wb, int year, int month, int day, int hours, int minutes, int seconds)
{
	//         10        20        30      = 35
	// 01234567890123456789012345678901234
	// Date(2014, 04, 01, 03, 28, 20, 065)

	if(wb->size - wb->bytes < 36) return;

	char *b = &wb->buffer[wb->bytes];

	b[0]='D';
	b[1]='a';
	b[2]='t';
	b[3]='e';
	b[4]='(';
	b[5]= 48 + year / 1000; year -= (year / 1000) * 1000;
	b[6]= 48 + year / 100; year -= (year / 100) * 100;
	b[7]= 48 + year / 10;
	b[8]= 48 + year % 10;
	b[9]=',';
	b[10]=' ';
	b[11]= 48 + month / 10;
	b[12]= 48 + month % 10;
	b[13]=',';
	b[14]=' ';
	b[15]= 48 + day / 10;
	b[16]= 48 + day % 10;
	b[17]=',';
	b[18]=' ';
	b[19]= 48 + hours / 10;
	b[20]= 48 + hours % 10;
	b[21]=',';
	b[22]=' ';
	b[23]= 48 + minutes / 10;
	b[24]= 48 + minutes % 10;
	b[25]=',';
	b[26]=' ';
	b[27]= 48 + seconds / 10;
	b[28]= 48 + seconds % 10;
	b[29]=')';
	b[30]='\0';

	wb->bytes += 30;
}

struct web_buffer *web_buffer_create(long size)
{
	struct web_buffer *b;

	debug(D_WEB_BUFFER, "Creating new web buffer of size %d.", size);

	b = calloc(1, sizeof(struct web_buffer));
	if(!b) {
		error("Cannot allocate a web_buffer.");
		return NULL;
	}

	b->buffer = malloc(size);
	if(!b->buffer) {
		error("Cannot allocate a buffer of size %u.", size);
		free(b);
		return NULL;
	}
	b->buffer[0] = '\0';
	b->size = size;
	b->contenttype = CT_TEXT_PLAIN;
	return(b);
}

void web_buffer_free(struct web_buffer *b)
{
	debug(D_WEB_BUFFER, "Freeing web buffer of size %d.", b->size);

	if(b->buffer) free(b->buffer);
	free(b);
}

void web_buffer_increase(struct web_buffer *b, long free_size_required)
{
	long left = b->size - b->bytes;

	if(left >= free_size_required) return;
	long increase = free_size_required - left;
	if(increase < WEB_DATA_LENGTH_INCREASE_STEP) increase = WEB_DATA_LENGTH_INCREASE_STEP;

	debug(D_WEB_BUFFER, "Increasing data buffer from size %d to %d.", b->size, b->size + increase);

	b->buffer = realloc(b->buffer, b->size + increase);
	if(!b->buffer) fatal("Failed to increase data buffer from size %d to %d.", b->size, b->size + increase);
	
	b->size += increase;
}

#define WEB_CLIENT_MODE_NORMAL		0
#define WEB_CLIENT_MODE_FILECOPY	1

#define URL_MAX 8192

struct web_client {
	unsigned long long id;
	char client_ip[101];
	char last_url[URL_MAX+1];

	struct timeval tv_in, tv_ready;

	int mode;
	int keepalive;

	struct sockaddr_in clientaddr;

	pthread_t thread;				// the thread servicing this client
	int obsolete;					// if set to 1, the listener will remove this client

	int ifd;
	int ofd;

	struct web_buffer *data;

	int zoutput;					// if set to 1, web_client_send() will send compressed data
	z_stream zstream;				// zlib stream for sending compressed output to client
	Bytef zbuffer[ZLIB_CHUNK];		// temporary buffer for storing compressed output
	long zsent;					// the compressed bytes we have sent to the client
	long zhave;					// the compressed bytes that we have to send
	int zinitialized;

	int wait_receive;
	int wait_send;

	char response_header[MAX_HTTP_HEADER_SIZE+1];

	struct web_client *prev;
	struct web_client *next;
} *web_clients = NULL;

unsigned long long web_clients_count = 0;

struct web_client *web_client_create(int listener)
{
	struct web_client *w;
	socklen_t addrlen;
	
	w = calloc(1, sizeof(struct web_client));
	if(!w) {
		error("Cannot allocate new web_client memory.");
		return NULL;
	}

	w->id = ++web_clients_count;
	w->mode = WEB_CLIENT_MODE_NORMAL;

	addrlen = sizeof(w->clientaddr);
	w->ifd = accept(listener, (struct sockaddr *)&w->clientaddr, &addrlen);
	if (w->ifd == -1) {
		error("%llu: Cannot accept new incoming connection.", w->id);
		free(w);
		return NULL;
	}
	w->ofd = w->ifd;

	strncpy(w->client_ip, inet_ntoa(w->clientaddr.sin_addr), 100);
	w->client_ip[100] = '\0';

	debug(D_WEB_CLIENT_ACCESS, "%llu: New web client from %s on socket %d.", w->id, w->client_ip, w->ifd);

	{
		int flag = 1; 
		if(setsockopt(w->ifd, SOL_SOCKET, SO_KEEPALIVE, (char *) &flag, sizeof(int)) != 0) error("%llu: Cannot set SO_KEEPALIVE on socket.", w->id);
	}

	w->data = web_buffer_create(INITIAL_WEB_DATA_LENGTH);
	if(!w->data) {
		close(w->ifd);
		free(w);
		return NULL;
	}

	w->wait_receive = 1;

	if(web_clients) web_clients->prev = w;
	w->next = web_clients;
	web_clients = w;

	global_statistics.connected_clients++;

	return(w);
}

struct web_client *web_client_free(struct web_client *w)
{
	struct web_client *n = w->next;

	debug(D_WEB_CLIENT_ACCESS, "%llu: Closing web client from %s.", w->id, inet_ntoa(w->clientaddr.sin_addr));

	if(w->prev)	w->prev->next = w->next;
	if(w->next) w->next->prev = w->prev;

	if(w == web_clients) web_clients = w->next;

	if(w->data) web_buffer_free(w->data);
	close(w->ifd);
	if(w->ofd != w->ifd) close(w->ofd);
	free(w);

	global_statistics.connected_clients--;

	return(n);
}

#define GROUP_AVERAGE	0
#define GROUP_MAX 		1

// find the oldest entry in the data, skipping all empty slots
time_t rrd_stats_first_entry_t(RRD_STATS *st)
{
	if(!st->first_entry_t) return st->last_updated.tv_sec;
	
	return st->first_entry_t / 1000000;
}

unsigned long rrd_stats_one_json(RRD_STATS *st, char *options, struct web_buffer *wb)
{
	web_buffer_increase(wb, 16384);

	pthread_rwlock_rdlock(&st->rwlock);

	web_buffer_printf(wb,
		"\t\t{\n"
		"\t\t\t\"id\": \"%s\",\n"
		"\t\t\t\"name\": \"%s\",\n"
		"\t\t\t\"type\": \"%s\",\n"
		"\t\t\t\"family\": \"%s\",\n"
		"\t\t\t\"title\": \"%s\",\n"
		"\t\t\t\"priority\": %ld,\n"
		"\t\t\t\"enabled\": %d,\n"
		"\t\t\t\"units\": \"%s\",\n"
		"\t\t\t\"url\": \"/data/%s/%s\",\n"
		"\t\t\t\"chart_type\": \"%s\",\n"
		"\t\t\t\"counter\": %ld,\n"
		"\t\t\t\"entries\": %ld,\n"
		"\t\t\t\"first_entry_t\": %lu,\n"
		"\t\t\t\"last_entry\": %ld,\n"
		"\t\t\t\"last_entry_t\": %lu,\n"
		"\t\t\t\"last_entry_secs_ago\": %lu,\n"
		"\t\t\t\"update_every\": %d,\n"
		"\t\t\t\"isdetail\": %d,\n"
		"\t\t\t\"usec_since_last_update\": %llu,\n"
		"\t\t\t\"absolute_total\": " TOTAL_NUMBER_FORMAT ",\n"
		"\t\t\t\"last_absolute_total\": " TOTAL_NUMBER_FORMAT ",\n"
		"\t\t\t\"dimensions\": [\n"
		, st->id
		, st->name
		, st->type
		, st->family
		, st->title
		, st->priority
		, st->enabled
		, st->units
		, st->name, options?options:""
		, chart_type_name(st->chart_type)
		, st->counter
		, st->entries
		, rrd_stats_first_entry_t(st)
		, st->current_entry
		, st->last_updated.tv_sec
		, time(NULL) - st->last_updated.tv_sec
		, st->update_every
		, st->isdetail
		, st->usec_since_last_update
		, st->absolute_total
		, st->last_absolute_total
		);

	unsigned long memory = st->memsize;

	RRD_DIMENSION *rd;
	for(rd = st->dimensions; rd ; rd = rd->next) {
		memory += rd->memsize;

		web_buffer_printf(wb,
			"\t\t\t\t{\n"
			"\t\t\t\t\t\"id\": \"%s\",\n"
			"\t\t\t\t\t\"name\": \"%s\",\n"
			"\t\t\t\t\t\"entries\": %ld,\n"
			"\t\t\t\t\t\"isHidden\": %d,\n"
			"\t\t\t\t\t\"algorithm\": \"%s\",\n"
			"\t\t\t\t\t\"multiplier\": %ld,\n"
			"\t\t\t\t\t\"divisor\": %ld,\n"
			"\t\t\t\t\t\"last_entry_t\": %lu,\n"
			"\t\t\t\t\t\"collected_value\": " COLLECTED_NUMBER_FORMAT ",\n"
			"\t\t\t\t\t\"calculated_value\": " CALCULATED_NUMBER_FORMAT ",\n"
			"\t\t\t\t\t\"last_collected_value\": " COLLECTED_NUMBER_FORMAT ",\n"
			"\t\t\t\t\t\"last_calculated_value\": " CALCULATED_NUMBER_FORMAT ",\n"
			"\t\t\t\t\t\"memory\": %lu\n"
			"\t\t\t\t}%s\n"
			, rd->id
			, rd->name
			, rd->entries
			, rd->hidden
			, algorithm_name(rd->algorithm)
			, rd->multiplier
			, rd->divisor
			, rd->last_collected.tv_sec
			, rd->collected_value
			, rd->calculated_value
			, rd->last_collected_value
			, rd->last_calculated_value
			, rd->memsize
			, rd->next?",":""
			);
	}

	web_buffer_printf(wb,
		"\t\t\t],\n"
		"\t\t\t\"memory\" : %lu\n"
		"\t\t}"
		, memory
		);

	pthread_rwlock_unlock(&st->rwlock);
	return memory;
}

#define RRD_GRAPH_JSON_HEADER "{\n\t\"charts\": [\n"
#define RRD_GRAPH_JSON_FOOTER "\n\t]\n}\n"

void rrd_stats_graph_json(RRD_STATS *st, char *options, struct web_buffer *wb)
{
	web_buffer_increase(wb, 16384);

	web_buffer_printf(wb, RRD_GRAPH_JSON_HEADER);
	rrd_stats_one_json(st, options, wb);
	web_buffer_printf(wb, RRD_GRAPH_JSON_FOOTER);
}

void rrd_stats_all_json(struct web_buffer *wb)
{
	web_buffer_increase(wb, 1024);

	unsigned long memory = 0;
	long c;
	RRD_STATS *st;

	web_buffer_printf(wb, RRD_GRAPH_JSON_HEADER);

	pthread_rwlock_rdlock(&root_rwlock);
	for(st = root, c = 0; st ; st = st->next) {
		if(st->enabled) {
			if(c) web_buffer_printf(wb, "%s", ",\n");
			memory += rrd_stats_one_json(st, NULL, wb);
			c++;
		}
	}
	pthread_rwlock_unlock(&root_rwlock);
	
	web_buffer_printf(wb, "\n\t],\n"
		"\t\"hostname\": \"%s\",\n"
		"\t\"update_every\": %d,\n"
		"\t\"history\": %d,\n"
		"\t\"memory\": %lu\n"
		"}\n"
		, hostname
		, update_every
		, save_history
		, memory
		);
}

unsigned long rrd_stats_json(int type, RRD_STATS *st, struct web_buffer *wb, int entries_to_show, int group, int group_method, time_t after, time_t before, int only_non_zero)
{
	int c;
	pthread_rwlock_rdlock(&st->rwlock);


	// -------------------------------------------------------------------------
	// switch from JSON to google JSON
	
	char kq[2] = "\"";
	char sq[2] = "\"";
	switch(type) {
		case DATASOURCE_GOOGLE_JSON:
		case DATASOURCE_GOOGLE_JSONP:
			kq[0] = '\0';
			sq[0] = '\'';
			break;

		case DATASOURCE_JSON:
		default:
			break;
	}


	// -------------------------------------------------------------------------
	// validate the parameters
	
	if(entries_to_show < 1) entries_to_show = 1;
	if(group < 1) group = 1;
	
	// make sure current_entry is within limits
	long current_entry = (long)st->current_entry - (long)1;
	if(current_entry < 0) current_entry = 0;
	else if(current_entry >= st->entries) current_entry = st->entries - 1;
	
	// find the oldest entry of the round-robin
	long max_entries_init = (st->counter < (unsigned long)st->entries) ? st->counter : (unsigned long)st->entries;
	
	if(before == 0) before = st->last_updated.tv_sec;
	if(after  == 0) after = rrd_stats_first_entry_t(st);

	time_t time_init = st->last_updated.tv_sec;

	// ---

	// our return value (the last timestamp printed)
	// this is required to detect re-transmit in google JSONP
	time_t last_timestamp = 0;			


	// -------------------------------------------------------------------------
	// find how many dimensions we have
	
	int dimensions = 0;
	RRD_DIMENSION *rd;
	for( rd = st->dimensions ; rd ; rd = rd->next) dimensions++;
	if(!dimensions) {
		pthread_rwlock_unlock(&st->rwlock);
		web_buffer_printf(wb, "No dimensions yet.");
		return 0;
	}

	
	// -------------------------------------------------------------------------
	// prepare various strings, to speed up the loop
	
	char overflow_annotation[201]; snprintf(overflow_annotation, 200, ",{%sv%s:%sRESET OR OVERFLOW%s},{%sv%s:%sThe counters have been wrapped.%s}", kq, kq, sq, sq, kq, kq, sq, sq);
	char normal_annotation[201];   snprintf(normal_annotation,   200, ",{%sv%s:null},{%sv%s:null}", kq, kq, kq, kq);
	char pre_date[51];             snprintf(pre_date,             50, "		{%sc%s:[{%sv%s:%s", kq, kq, kq, kq, sq);
	char post_date[21];            snprintf(post_date,            20, "%s}", sq);
	char pre_value[21];            snprintf(pre_value,            20, ",{%sv%s:", kq, kq);
	char post_value[21];           snprintf(post_value,           20, "}");


	// -------------------------------------------------------------------------
	// checks for debuging
	
	if(st->debug) {
		debug(D_RRD_STATS, "%s first_entry_t = %lu, last_entry_t = %lu, duration = %lu, after = %lu, before = %lu, duration = %lu, entries_to_show = %lu, group = %lu, max_entries = %ld"
			, st->id
			, rrd_stats_first_entry_t(st)
			, st->last_updated.tv_sec
			, st->last_updated.tv_sec - rrd_stats_first_entry_t(st)
			, after
			, before
			, before - after
			, entries_to_show
			, group
			, max_entries_init
			);

		if(before < after)
			debug(D_RRD_STATS, "WARNING: %s The newest value in the database (%lu) is earlier than the oldest (%lu)", st->name, before, after);

		if((before - after) > st->entries * st->update_every)
			debug(D_RRD_STATS, "WARNING: %s The time difference between the oldest and the newest entries (%lu) is higher than the capacity of the database (%lu)", st->name, before - after, st->entries * st->update_every);
	}


	// -------------------------------------------------------------------------
	// temp arrays for keeping values per dimension
	
	calculated_number group_values[dimensions]; // keep sums when grouping
	storage_number    print_values[dimensions]; // keep the final value to be printed
	int               print_hidden[dimensions]; // keep hidden flags
	int               found_non_zero[dimensions];

	// initialize them
	for( rd = st->dimensions, c = 0 ; rd && c < dimensions ; rd = rd->next, c++) {
		group_values[c] = print_values[c] = 0;
		print_hidden[c] = rd->hidden;
		found_non_zero[c] = 0;
	}

	// -------------------------------------------------------------------------
	// remove dimensions that contain only zeros

	int max_loop = 1;
	if(only_non_zero) max_loop = 2;

	for(; max_loop ; max_loop--) {

		// -------------------------------------------------------------------------
		// print the JSON header
		
		web_buffer_printf(wb, "{\n	%scols%s:\n	[\n", kq, kq);
		web_buffer_printf(wb, "		{%sid%s:%s%s,%slabel%s:%stime%s,%spattern%s:%s%s,%stype%s:%sdatetime%s},\n", kq, kq, sq, sq, kq, kq, sq, sq, kq, kq, sq, sq, kq, kq, sq, sq);
		web_buffer_printf(wb, "		{%sid%s:%s%s,%slabel%s:%s%s,%spattern%s:%s%s,%stype%s:%sstring%s,%sp%s:{%srole%s:%sannotation%s}},\n", kq, kq, sq, sq, kq, kq, sq, sq, kq, kq, sq, sq, kq, kq, sq, sq, kq, kq, kq, kq, sq, sq);
		web_buffer_printf(wb, "		{%sid%s:%s%s,%slabel%s:%s%s,%spattern%s:%s%s,%stype%s:%sstring%s,%sp%s:{%srole%s:%sannotationText%s}}", kq, kq, sq, sq, kq, kq, sq, sq, kq, kq, sq, sq, kq, kq, sq, sq, kq, kq, kq, kq, sq, sq);

		// print the header for each dimension
		// and update the print_hidden array for the dimensions that should be hidden
		for( rd = st->dimensions, c = 0 ; rd && c < dimensions ; rd = rd->next, c++) {
			if(!print_hidden[c])
				web_buffer_printf(wb, ",\n		{%sid%s:%s%s,%slabel%s:%s%s%s,%spattern%s:%s%s,%stype%s:%snumber%s}", kq, kq, sq, sq, kq, kq, sq, rd->name, sq, kq, kq, sq, sq, kq, kq, sq, sq);
		}

		// print the begin of row data
		web_buffer_printf(wb, "\n	],\n	%srows%s:\n	[\n", kq, kq);


		// -------------------------------------------------------------------------
		// the main loop

		int annotate_reset = 0;
		int annotation_count = 0;
		
		// to allow grouping on the same values, we need a pad
		long pad = before % group;

		// the minimum line length we expect
		int line_size = 4096 + (dimensions * 200);

		time_t now = time_init;
		long max_entries = max_entries_init;

		long t;

		long count = 0, printed = 0, group_count = 0;
		last_timestamp = 0;
		for(t = current_entry; max_entries ; now--, t--, max_entries--) {
			if(t < 0) t = st->entries - 1;

			int print_this = 0;

			if(st->debug) {
				debug(D_RRD_STATS, "%s t = %ld, count = %ld, group_count = %ld, printed = %ld, now = %lu, %s %s"
					, st->id
					, t
					, count + 1
					, group_count + 1
					, printed
					, now
					, (((count + 1 - pad) % group) == 0)?"PRINT":"  -  "
					, (now >= after && now <= before)?"RANGE":"  -  "
					);
			}

			// make sure we return data in the proper time range
			if(now < after || now > before) continue;

			count++;
			group_count++;

			if(((count - pad) % group) == 0) {
				if(printed >= entries_to_show) {
					// debug(D_RRD_STATS, "Already printed all rows. Stopping.");
					break;
				}
				
				if(group_count != group) {
					// this is an incomplete group, skip it.
					for( rd = st->dimensions, c = 0 ; rd && c < dimensions ; rd = rd->next, c++)
						group_values[c] = 0;
						
					group_count = 0;
					continue;
				}

				// check if we may exceed the buffer provided
				web_buffer_increase(wb, line_size);

				// generate the local date time
				struct tm *tm = localtime(&now);
				if(!tm) { error("localtime() failed."); continue; }
				if(now > last_timestamp) last_timestamp = now;

				if(printed) web_buffer_strcpy(wb, "]},\n");
				web_buffer_strcpy(wb, pre_date);
				web_buffer_jsdate(wb, tm->tm_year + 1900, tm->tm_mon, tm->tm_mday, tm->tm_hour, tm->tm_min, tm->tm_sec);
				web_buffer_strcpy(wb, post_date);

				print_this = 1;
			}

			for(rd = st->dimensions, c = 0 ; rd && c < dimensions ; rd = rd->next, c++) {
				long value = rd->values[t];
				
				switch(group_method) {
					case GROUP_MAX:
						if(abs(value) > abs(group_values[c])) group_values[c] = value;
						break;

					default:
					case GROUP_AVERAGE:
						group_values[c] += value;
						if(print_this) group_values[c] /= group_count;
						break;
				}

				if(print_this) {
					print_values[c] = group_values[c];
					group_values[c] = 0;
				}
			}

			if(print_this) {
				group_count = 0;
				
				if(annotate_reset) {
					annotation_count++;
					web_buffer_strcpy(wb, overflow_annotation);
					annotate_reset = 0;
				}
				else
					web_buffer_strcpy(wb, normal_annotation);

				for(c = 0 ; c < dimensions ; c++) {
					if(!print_hidden[c]) {
						web_buffer_strcpy(wb, pre_value);
						web_buffer_rrd_value(wb, print_values[c]);
						web_buffer_strcpy(wb, post_value);

						if(print_values[c]) found_non_zero[c]++;
					}
				}

				printed++;
			}
		}

		if(printed) web_buffer_printf(wb, "]}");
		web_buffer_printf(wb, "\n	]\n}\n");

		if(only_non_zero && max_loop > 1) {
			int changed = 0;
			for(rd = st->dimensions, c = 0 ; rd && c < dimensions ; rd = rd->next, c++) {
				group_values[c] = 0;

				if(!print_hidden[c] && !found_non_zero[c]) {
					changed = 1;
					print_hidden[c] = 1;
				}
			}

			if(changed) web_buffer_reset(wb);
			else break;
		}
		else break;
		
	} // max_loop

	debug(D_RRD_STATS, "RRD_STATS_JSON: %s total %ld bytes", st->name, wb->bytes);

	pthread_rwlock_unlock(&st->rwlock);
	return last_timestamp;
}

void generate_config(struct web_buffer *wb, int only_changed)
{
	int i, pri;
	struct config *co;
	struct config_value *cv;

	for(i = 0; i < 3 ;i++) {
		web_buffer_increase(wb, 500);
		switch(i) {
			case 0:
				web_buffer_printf(wb, 
					"# NetData Configuration\n"
					"# You can uncomment and change any of the options bellow.\n"
					"# The value shown in the commented settings, is the default value.\n"
					"\n# global netdata configuration\n");
				break;

			case 1:
				web_buffer_printf(wb, "\n\n# per plugin configuration\n");
				break;

			case 2:
				web_buffer_printf(wb, "\n\n# per chart configuration\n");
				break;
		}

		for(co = config_root; co ; co = co->next) {
			if(strcmp(co->name, "global") == 0 || strcmp(co->name, "plugins") == 0) pri = 0;
			else if(strncmp(co->name, "plugin:", 7) == 0) pri = 1;
			else pri = 2;

			if(i == pri) {
				int used = 0;
				int changed = 0;
				int count = 0;
				for(cv = co->values; cv ; cv = cv->next) {
					used += cv->used;
					changed += cv->changed;
					count++;
				}

				if(!count) continue;
				if(only_changed && !changed) continue;

				if(!used) {
					web_buffer_increase(wb, 500);
					web_buffer_printf(wb, "\n# node '%s' is not used.", co->name);
				}

				web_buffer_increase(wb, CONFIG_MAX_NAME + 4);
				web_buffer_printf(wb, "\n[%s]\n", co->name);

				for(cv = co->values; cv ; cv = cv->next) {

					if(used && !cv->used) {
						web_buffer_increase(wb, CONFIG_MAX_NAME + 200);
						web_buffer_printf(wb, "\n\t# option '%s' is not used.\n", cv->name);
					}
					web_buffer_increase(wb, CONFIG_MAX_NAME + CONFIG_MAX_VALUE + 5);
					web_buffer_printf(wb, "\t%s%s = %s\n", (!cv->changed && cv->used)?"# ":"", cv->name, cv->value);
				}
			}
		}
	}
}

int mysendfile(struct web_client *w, char *filename)
{
	static char *web_dir = NULL;
	if(!web_dir) web_dir = config_get("global", "web files directory", "web");

	debug(D_WEB_CLIENT, "%llu: Looking for file '%s'...", w->id, filename);

	// skip leading slashes
	while (*filename == '/') filename++;

	// if the filename contain known paths, skip them
		 if(strncmp(filename, WEB_PATH_DATA       "/", strlen(WEB_PATH_DATA)       + 1) == 0) filename = &filename[strlen(WEB_PATH_DATA)       + 1];
	else if(strncmp(filename, WEB_PATH_DATASOURCE "/", strlen(WEB_PATH_DATASOURCE) + 1) == 0) filename = &filename[strlen(WEB_PATH_DATASOURCE) + 1];
	else if(strncmp(filename, WEB_PATH_GRAPH      "/", strlen(WEB_PATH_GRAPH)      + 1) == 0) filename = &filename[strlen(WEB_PATH_GRAPH)      + 1];
	else if(strncmp(filename, WEB_PATH_FILE       "/", strlen(WEB_PATH_FILE)       + 1) == 0) filename = &filename[strlen(WEB_PATH_FILE)       + 1];

	// if the filename contains a / or a .., refuse to serve it
	if(strchr(filename, '/') != 0 || strstr(filename, "..") != 0) {
		debug(D_WEB_CLIENT_ACCESS, "%llu: File '%s' is not acceptable.", w->id, filename);
		web_buffer_printf(w->data, "File '%s' cannot be served. Filenames cannot contain / or ..", filename);
		return 400;
	}

	// access the file
	char webfilename[FILENAME_MAX + 1];
	snprintf(webfilename, FILENAME_MAX, "%s/%s", web_dir, filename);

	// check if the file exists
	struct stat stat;
	if(lstat(webfilename, &stat) != 0) {
		error("%llu: File '%s' is not found.", w->id, webfilename);
		web_buffer_printf(w->data, "File '%s' does not exist, or is not accessible.", filename);
		return 404;
	}

	// check if the file is owned by us
	if(stat.st_uid != getuid() && stat.st_uid != geteuid()) {
		error("%llu: File '%s' is owned by user %d (I run as user %d). Access Denied.", w->id, webfilename, stat.st_uid, getuid());
		web_buffer_printf(w->data, "Access to file '%s' is not permitted.", filename);
		return 403;
	}

	// open the file
	w->ifd = open(webfilename, O_NONBLOCK, O_RDONLY);
	if(w->ifd == -1) {
		w->ifd = w->ofd;

		if(errno == EBUSY || errno == EAGAIN) {
			error("%llu: File '%s' is busy, sending 307 Moved Temporarily to force retry.", w->id, webfilename);
			snprintf(w->response_header, MAX_HTTP_HEADER_SIZE, "Location: /" WEB_PATH_FILE "/%s\r\n", filename);
			web_buffer_printf(w->data, "The file '%s' is currently busy. Please try again later.", filename);
			return 307;
		}
		else {
			error("%llu: Cannot open file '%s'.", w->id, webfilename);
			web_buffer_printf(w->data, "Cannot open file '%s'.", filename);
			return 404;
		}
	}
	
	// pick a Content-Type for the file
		 if(strstr(filename, ".html") != NULL)	w->data->contenttype = CT_TEXT_HTML;
	else if(strstr(filename, ".js")   != NULL)	w->data->contenttype = CT_APPLICATION_X_JAVASCRIPT;
	else if(strstr(filename, ".css")  != NULL)	w->data->contenttype = CT_TEXT_CSS;
	else if(strstr(filename, ".xml")  != NULL)	w->data->contenttype = CT_TEXT_XML;
	else if(strstr(filename, ".xsl")  != NULL)	w->data->contenttype = CT_TEXT_XSL;
	else if(strstr(filename, ".txt")  != NULL)  w->data->contenttype = CT_TEXT_PLAIN;
	else if(strstr(filename, ".svg")  != NULL)  w->data->contenttype = CT_IMAGE_SVG_XML;
	else if(strstr(filename, ".ttf")  != NULL)  w->data->contenttype = CT_APPLICATION_X_FONT_TRUETYPE;
	else if(strstr(filename, ".otf")  != NULL)  w->data->contenttype = CT_APPLICATION_X_FONT_OPENTYPE;
	else if(strstr(filename, ".woff") != NULL)  w->data->contenttype = CT_APPLICATION_FONT_WOFF;
	else if(strstr(filename, ".eot")  != NULL)  w->data->contenttype = CT_APPLICATION_VND_MS_FONTOBJ;
	else w->data->contenttype = CT_APPLICATION_OCTET_STREAM;

	debug(D_WEB_CLIENT_ACCESS, "%llu: Sending file '%s' (%ld bytes, ifd %d, ofd %d).", w->id, webfilename, stat.st_size, w->ifd, w->ofd);

	w->mode = WEB_CLIENT_MODE_FILECOPY;
	w->wait_receive = 1;
	w->wait_send = 0;
	w->data->bytes = 0;
	w->data->buffer[0] = '\0';
	w->data->rbytes = stat.st_size;
	w->data->date = stat.st_mtim.tv_sec;

	return 200;
}

char *mystrsep(char **ptr, char *s)
{
	char *p = "";
	while ( p && !p[0] && *ptr ) p = strsep(ptr, s);
	return(p);
}

void web_client_reset(struct web_client *w)
{
	struct timeval tv;
	gettimeofday(&tv, NULL);

	long sent = w->zoutput?(long)w->zstream.total_out:((w->mode == WEB_CLIENT_MODE_FILECOPY)?w->data->rbytes:w->data->bytes);
	long size = (w->mode == WEB_CLIENT_MODE_FILECOPY)?w->data->rbytes:w->data->bytes;
	
	if(w->last_url[0]) log_access("%llu: (sent/all = %ld/%ld bytes %0.0f%%, prep/sent/total = %0.2f/%0.2f/%0.2f ms) %s: '%s'",
		w->id,
		sent, size, -((size>0)?((float)(size-sent)/(float)size * 100.0):0.0),
		(float)usecdiff(&w->tv_ready, &w->tv_in) / 1000.0,
		(float)usecdiff(&tv, &w->tv_ready) / 1000.0,
		(float)usecdiff(&tv, &w->tv_in) / 1000.0,
		(w->mode == WEB_CLIENT_MODE_FILECOPY)?"filecopy":"data",
		w->last_url
		);

	debug(D_WEB_CLIENT, "%llu: Reseting client.", w->id);

	if(w->mode == WEB_CLIENT_MODE_FILECOPY) {
		debug(D_WEB_CLIENT, "%llu: Closing filecopy input file.", w->id);
		close(w->ifd);
		w->ifd = w->ofd;
	}

	w->last_url[0] = '\0';

	w->data->contenttype = CT_TEXT_PLAIN;
	w->mode = WEB_CLIENT_MODE_NORMAL;

	w->data->rbytes = 0;
	w->data->bytes = 0;
	w->data->sent = 0;

	w->response_header[0] = '\0';
	w->data->buffer[0] = '\0';

	w->wait_receive = 1;
	w->wait_send = 0;

	// if we had enabled compression, release it
	if(w->zinitialized) {
		debug(D_DEFLATE, "%llu: Reseting compression.", w->id);
		deflateEnd(&w->zstream);
		w->zoutput = 0;
		w->zsent = 0;
		w->zhave = 0;
		w->zstream.avail_in = 0;
		w->zstream.avail_out = 0;
		w->zstream.total_in = 0;
		w->zstream.total_out = 0;
		w->zinitialized = 0;
	}
}

void web_client_enable_deflate(struct web_client *w) {
	if(w->zinitialized == 1) {
		error("%llu: Compression has already be initialized for this client.", w->id);
		return;
	}

	if(w->data->sent) {
		error("%llu: Cannot enable compression in the middle of a conversation.", w->id);
		return;
	}

	w->zstream.zalloc = Z_NULL;
	w->zstream.zfree = Z_NULL;
	w->zstream.opaque = Z_NULL;

	w->zstream.next_in = (Bytef *)w->data->buffer;
	w->zstream.avail_in = 0;
	w->zstream.total_in = 0;

	w->zstream.next_out = w->zbuffer;
	w->zstream.avail_out = 0;
	w->zstream.total_out = 0;

	w->zstream.zalloc = Z_NULL;
	w->zstream.zfree = Z_NULL;
	w->zstream.opaque = Z_NULL;

//	if(deflateInit(&w->zstream, Z_DEFAULT_COMPRESSION) != Z_OK) {
//		error("%llu: Failed to initialize zlib. Proceeding without compression.", w->id);
//		return;
//	}

	// Select GZIP compression: windowbits = 15 + 16 = 31
	if(deflateInit2(&w->zstream, Z_DEFAULT_COMPRESSION, Z_DEFLATED, 31, 8, Z_DEFAULT_STRATEGY) != Z_OK) {
		error("%llu: Failed to initialize zlib. Proceeding without compression.", w->id);
		return;
	}

	w->zsent = 0;
	w->zoutput = 1;
	w->zinitialized = 1;

	debug(D_DEFLATE, "%llu: Initialized compression.", w->id);
}

int web_client_data_request(struct web_client *w, char *url, int datasource_type)
{
	char *args = strchr(url, '?');
	if(args) {
		*args='\0';
		args = &args[1];
	}

	// get the name of the data to show
	char *tok = mystrsep(&url, "/");
	debug(D_WEB_CLIENT, "%llu: Searching for RRD data with name '%s'.", w->id, tok);

	// do we have such a data set?
	RRD_STATS *st = rrd_stats_find_byname(tok);
	if(!st) st = rrd_stats_find(tok);
	if(!st) {
		// we don't have it
		// try to send a file with that name
		w->data->bytes = 0;
		return(mysendfile(w, tok));
	}

	// we have it
	debug(D_WEB_CLIENT, "%llu: Found RRD data with name '%s'.", w->id, tok);

	// how many entries does the client want?
	long lines = save_history;
	long group_count = 1;
	time_t after = 0, before = 0;
	int group_method = GROUP_AVERAGE;
	int nonzero = 0;

	if(url) {
		// parse the lines required
		tok = mystrsep(&url, "/");
		if(tok) lines = atoi(tok);
		if(lines < 1) lines = 1;
	}
	if(url) {
		// parse the group count required
		tok = mystrsep(&url, "/");
		if(tok) group_count = atoi(tok);
		if(group_count < 1) group_count = 1;
		//if(group_count > save_history / 20) group_count = save_history / 20;
	}
	if(url) {
		// parse the grouping method required
		tok = mystrsep(&url, "/");
		if(strcmp(tok, "max") == 0) group_method = GROUP_MAX;
		else if(strcmp(tok, "average") == 0) group_method = GROUP_AVERAGE;
		else debug(D_WEB_CLIENT, "%llu: Unknown group method '%s'", w->id, tok);
	}
	if(url) {
		// parse after time
		tok = mystrsep(&url, "/");
		if(tok) after = strtoul(tok, NULL, 10);
		if(after < 0) after = 0;
	}
	if(url) {
		// parse before time
		tok = mystrsep(&url, "/");
		if(tok) before = strtoul(tok, NULL, 10);
		if(before < 0) before = 0;
	}
	if(url) {
		// parse nonzero
		tok = mystrsep(&url, "/");
		if(tok && strcmp(tok, "nonzero") == 0) nonzero = 1;
	}

	w->data->contenttype = CT_APPLICATION_JSON;
	w->data->bytes = 0;

	char *google_version = "0.6";
	char *google_reqId = "0";
	char *google_sig = "0";
	char *google_out = "json";
	char *google_responseHandler = "google.visualization.Query.setResponse";
	char *google_outFileName = NULL;
	unsigned long last_timestamp_in_data = 0;
	if(datasource_type == DATASOURCE_GOOGLE_JSON || datasource_type == DATASOURCE_GOOGLE_JSONP) {

		w->data->contenttype = CT_APPLICATION_X_JAVASCRIPT;

		while(args) {
			tok = mystrsep(&args, "&");
			if(tok) {
				char *name = mystrsep(&tok, "=");
				if(name && strcmp(name, "tqx") == 0) {
					char *key = mystrsep(&tok, ":");
					char *value = mystrsep(&tok, ";");
					if(key && value && *key && *value) {
						if(strcmp(key, "version") == 0)
							google_version = value;

						else if(strcmp(key, "reqId") == 0)
							google_reqId = value;

						else if(strcmp(key, "sig") == 0)
							google_sig = value;
						
						else if(strcmp(key, "out") == 0)
							google_out = value;
						
						else if(strcmp(key, "responseHandler") == 0)
							google_responseHandler = value;
						
						else if(strcmp(key, "outFileName") == 0)
							google_outFileName = value;
					}
				}
			}
		}

		debug(D_WEB_CLIENT_ACCESS, "%llu: GOOGLE JSONP: version = '%s', reqId = '%s', sig = '%s', out = '%s', responseHandler = '%s', outFileName = '%s'",
			w->id, google_version, google_reqId, google_sig, google_out, google_responseHandler, google_outFileName
			);

		if(datasource_type == DATASOURCE_GOOGLE_JSONP) {
			last_timestamp_in_data = strtoul(google_sig, NULL, 0);

			// check the client wants json
			if(strcmp(google_out, "json") != 0) {
				w->data->bytes = snprintf(w->data->buffer, w->data->size, 
					"%s({version:'%s',reqId:'%s',status:'error',errors:[{reason:'invalid_query',message:'output format is not supported',detailed_message:'the format %s requested is not supported by netdata.'}]});",
					google_responseHandler, google_version, google_reqId, google_out);
					return 200;
			}
		}
	}

	if(datasource_type == DATASOURCE_GOOGLE_JSONP) {
		w->data->bytes = snprintf(w->data->buffer, w->data->size, 
			"%s({version:'%s',reqId:'%s',status:'ok',sig:'%lu',table:",
			google_responseHandler, google_version, google_reqId, st->last_updated.tv_sec);
	}
	
	debug(D_WEB_CLIENT_ACCESS, "%llu: Sending RRD data '%s' (id %s, %d lines, %d group, %d group_method, %lu after, %lu before).", w->id, st->name, st->id, lines, group_count, group_method, after, before);
	unsigned long timestamp_in_data = rrd_stats_json(datasource_type, st, w->data, lines, group_count, group_method, after, before, nonzero);

	if(datasource_type == DATASOURCE_GOOGLE_JSONP) {
		if(timestamp_in_data > last_timestamp_in_data)
			w->data->bytes += snprintf(&w->data->buffer[w->data->bytes], w->data->size - w->data->bytes, "});");

		else {
			// the client already has the latest data
			w->data->bytes = snprintf(w->data->buffer, w->data->size, 
				"%s({version:'%s',reqId:'%s',status:'error',errors:[{reason:'not_modified',message:'Data not modified'}]});",
				google_responseHandler, google_version, google_reqId);
		}
	}

	return 200;
}

void web_client_process(struct web_client *w)
{
	int code = 500;
	int bytes;

	w->wait_receive = 0;

	// check if we have an empty line (end of HTTP header)
	if(strstr(w->data->buffer, "\r\n\r\n")) {
		global_statistics_lock();
		global_statistics.web_requests++;
		global_statistics_unlock();

		gettimeofday(&w->tv_in, NULL);
		debug(D_WEB_DATA, "%llu: Processing data buffer of %d bytes: '%s'.", w->id, w->data->bytes, w->data->buffer);

		// check if the client requested keep-alive HTTP
		if(strcasestr(w->data->buffer, "Connection: keep-alive")) w->keepalive = 1;
		else w->keepalive = 0;

		// check if the client accepts deflate
		if(strstr(w->data->buffer, "gzip"))
			web_client_enable_deflate(w);

		int datasource_type = DATASOURCE_GOOGLE_JSONP;
		//if(strstr(w->data->buffer, "X-DataSource-Auth"))
		//	datasource_type = DATASOURCE_GOOGLE_JSON;

		char *buf = w->data->buffer;
		char *tok = strsep(&buf, " \r\n");
		char *url = NULL;
		char *pointer_to_free = NULL; // keep url_decode() allocated buffer

		if(buf && strcmp(tok, "GET") == 0) {
			tok = strsep(&buf, " \r\n");
			pointer_to_free = url = url_decode(tok);
			debug(D_WEB_CLIENT, "%llu: Processing HTTP GET on url '%s'.", w->id, url);
		}
		else if (buf && strcmp(tok, "POST") == 0) {
			w->keepalive = 0;
			tok = strsep(&buf, " \r\n");
			pointer_to_free = url = url_decode(tok);

			debug(D_WEB_CLIENT, "%llu: I don't know how to handle POST with form data. Assuming it is a GET on url '%s'.", w->id, url);
		}

		w->last_url[0] = '\0';
		if(url) {
			strncpy(w->last_url, url, URL_MAX);
			w->last_url[URL_MAX] = '\0';

			tok = mystrsep(&url, "/?&");

			debug(D_WEB_CLIENT, "%llu: Processing command '%s'.", w->id, tok);

			if(strcmp(tok, WEB_PATH_DATA) == 0) { // "data"
				// the client is requesting rrd data
				datasource_type = DATASOURCE_JSON;
				code = web_client_data_request(w, url, datasource_type);
			}
			else if(strcmp(tok, WEB_PATH_DATASOURCE) == 0) { // "datasource"
				// the client is requesting google datasource
				code = web_client_data_request(w, url, datasource_type);
			}
			else if(strcmp(tok, WEB_PATH_GRAPH) == 0) { // "graph"
				// the client is requesting an rrd graph

				// get the name of the data to show
				tok = mystrsep(&url, "/?&");
				debug(D_WEB_CLIENT, "%llu: Searching for RRD data with name '%s'.", w->id, tok);

				// do we have such a data set?
				RRD_STATS *st = rrd_stats_find_byname(tok);
				if(!st) {
					// we don't have it
					// try to send a file with that name
					w->data->bytes = 0;
					code = mysendfile(w, tok);
				}
				else {
					code = 200;
					debug(D_WEB_CLIENT_ACCESS, "%llu: Sending %s.json of RRD_STATS...", w->id, st->name);
					w->data->contenttype = CT_APPLICATION_JSON;
					w->data->bytes = 0;
					rrd_stats_graph_json(st, url, w->data);
				}
			}
			else if(strcmp(tok, "debug") == 0) {
				w->data->bytes = 0;

				// get the name of the data to show
				tok = mystrsep(&url, "/?&");
				debug(D_WEB_CLIENT, "%llu: Searching for RRD data with name '%s'.", w->id, tok);

				// do we have such a data set?
				RRD_STATS *st = rrd_stats_find_byname(tok);
				if(!st) {
					code = 404;
					web_buffer_printf(w->data, "Chart %s is not found.\r\n", tok);
					debug(D_WEB_CLIENT_ACCESS, "%llu: %s is not found.", w->id, tok);
				}
				else {
					code = 200;
					debug_flags |= D_RRD_STATS;
					st->debug = st->debug?0:1;
					web_buffer_printf(w->data, "Chart %s has now debug %s.\r\n", tok, st->debug?"enabled":"disabled");
					debug(D_WEB_CLIENT_ACCESS, "%llu: debug for %s is %s.", w->id, tok, st->debug?"enabled":"disabled");
				}
			}
			else if(strcmp(tok, "mirror") == 0) {
				code = 200;

				debug(D_WEB_CLIENT_ACCESS, "%llu: Mirroring...", w->id);

				// replace the zero bytes with spaces
				int i;
				for(i = 0; i < w->data->size; i++)
					if(w->data->buffer[i] == '\0') w->data->buffer[i] = ' ';

				// just leave the buffer as is
				// it will be copied back to the client
			}
			else if(strcmp(tok, "list") == 0) {
				code = 200;

				debug(D_WEB_CLIENT_ACCESS, "%llu: Sending list of RRD_STATS...", w->id);

				w->data->bytes = 0;
				RRD_STATS *st = root;

				for ( ; st ; st = st->next )
					web_buffer_printf(w->data, "%s\n", st->name);
			}
			else if(strcmp(tok, "all.json") == 0) {
				code = 200;
				debug(D_WEB_CLIENT_ACCESS, "%llu: Sending JSON list of all monitors of RRD_STATS...", w->id);

				w->data->contenttype = CT_APPLICATION_JSON;
				w->data->bytes = 0;
				rrd_stats_all_json(w->data);
			}
			else if(strcmp(tok, "netdata.conf") == 0) {
				code = 200;
				debug(D_WEB_CLIENT_ACCESS, "%llu: Sending netdata.conf ...", w->id);

				w->data->contenttype = CT_TEXT_PLAIN;
				w->data->bytes = 0;
				generate_config(w->data, 0);
			}
			else if(strcmp(tok, WEB_PATH_FILE) == 0) { // "file"
				tok = mystrsep(&url, "/?&");
				if(tok && *tok) code = mysendfile(w, tok);
				else {
					code = 400;
					w->data->bytes = 0;
					strcpy(w->data->buffer, "You have to give a filename to get.\r\n");
					w->data->bytes = strlen(w->data->buffer);
				}
			}
			else if(!tok[0]) {
				w->data->bytes = 0;
				code = mysendfile(w, "index.html");
			}
			else {
				w->data->bytes = 0;
				code = mysendfile(w, tok);
			}

		}
		else {
			strcpy(w->last_url, "not a valid response");

			if(buf) debug(D_WEB_CLIENT_ACCESS, "%llu: Cannot understand '%s'.", w->id, buf);

			code = 500;
			w->data->bytes = 0;
			strcpy(w->data->buffer, "I don't understand you...\r\n");
			w->data->bytes = strlen(w->data->buffer);
		}
		
		// free url_decode() buffer
		if(pointer_to_free) free(pointer_to_free);
	}
	else if(w->data->bytes > 8192) {
		strcpy(w->last_url, "too big request");

		debug(D_WEB_CLIENT_ACCESS, "%llu: Received request is too big.", w->id);

		code = 400;
		w->data->bytes = 0;
		strcpy(w->data->buffer, "Received request is too big.\r\n");
		w->data->bytes = strlen(w->data->buffer);
	}
	else {
		// wait for more data
		w->wait_receive = 1;
		return;
	}

	if(w->data->bytes > w->data->size) {
		error("%llu: memory overflow encountered (size is %ld, written %ld).", w->data->size, w->data->bytes);
	}

	gettimeofday(&w->tv_ready, NULL);
	w->data->date = time(NULL);
	w->data->sent = 0;

	// prepare the HTTP response header
	debug(D_WEB_CLIENT, "%llu: Generating HTTP header with response %d.", w->id, code);

	char *content_type_string = "";
	switch(w->data->contenttype) {
		case CT_TEXT_HTML:
			content_type_string = "text/html";
			break;

		case CT_APPLICATION_XML:
			content_type_string = "application/xml";
			break;

		case CT_APPLICATION_JSON:
			content_type_string = "application/json";
			break;

		case CT_APPLICATION_X_JAVASCRIPT:
			content_type_string = "application/x-javascript";
			break;

		case CT_TEXT_CSS:
			content_type_string = "text/css";
			break;

		case CT_TEXT_XML:
			content_type_string = "text/xml";
			break;

		case CT_TEXT_XSL:
			content_type_string = "text/xsl";
			break;

		case CT_APPLICATION_OCTET_STREAM:
			content_type_string = "application/octet-stream";
			break;

		case CT_IMAGE_SVG_XML:
			content_type_string = "image/svg+xml";
			break;

		case CT_APPLICATION_X_FONT_TRUETYPE:
			content_type_string = "application/x-font-truetype";
			break;

		case CT_APPLICATION_X_FONT_OPENTYPE:
			content_type_string = "application/x-font-opentype";
			break;

		case CT_APPLICATION_FONT_WOFF:
			content_type_string = "application/font-woff";
			break;

		case CT_APPLICATION_VND_MS_FONTOBJ:
			content_type_string = "application/vnd.ms-fontobject";
			break;

		default:
		case CT_TEXT_PLAIN:
			content_type_string = "text/plain";
			break;
	}

	char *code_msg = "";
	switch(code) {
		case 200:
			code_msg = "OK";
			break;

		case 307:
			code_msg = "Temporary Redirect";
			break;

		case 400:
			code_msg = "Bad Request";
			break;

		case 403:
			code_msg = "Forbidden";
			break;

		case 404:
			code_msg = "Not Found";
			break;

		default:
			code_msg = "Internal Server Error";
			break;
	}

	char date[100];
	struct tm tm = *gmtime(&w->data->date);
	strftime(date, sizeof(date), "%a, %d %b %Y %H:%M:%S %Z", &tm);

	char custom_header[MAX_HTTP_HEADER_SIZE + 1] = "";
	if(w->response_header[0]) 
		strcpy(custom_header, w->response_header);

	int headerlen = 0;
	headerlen += snprintf(&w->response_header[headerlen], MAX_HTTP_HEADER_SIZE - headerlen,
		"HTTP/1.1 %d %s\r\n"
		"Connection: %s\r\n"
		"Server: NetData Embedded HTTP Server\r\n"
		"Content-Type: %s\r\n"
		"Access-Control-Allow-Origin: *\r\n"
		"Date: %s\r\n"
		, code, code_msg
		, w->keepalive?"keep-alive":"close"
		, content_type_string
		, date
		);

	if(custom_header[0])
		headerlen += snprintf(&w->response_header[headerlen], MAX_HTTP_HEADER_SIZE - headerlen, "%s", custom_header);

	if(w->mode == WEB_CLIENT_MODE_NORMAL) {
		headerlen += snprintf(&w->response_header[headerlen], MAX_HTTP_HEADER_SIZE - headerlen,
			"Expires: %s\r\n"
			"Cache-Control: no-cache\r\n"
			, date
			);
	}
	else {
		headerlen += snprintf(&w->response_header[headerlen], MAX_HTTP_HEADER_SIZE - headerlen,
			"Cache-Control: public\r\n"
			);
	}

	// if we know the content length, put it
	if(!w->zoutput && (w->data->bytes || w->data->rbytes))
		headerlen += snprintf(&w->response_header[headerlen], MAX_HTTP_HEADER_SIZE - headerlen,
			"Content-Length: %ld\r\n"
			, w->data->bytes?w->data->bytes:w->data->rbytes
			);
	else if(!w->zoutput)
		w->keepalive = 0;	// content-length is required for keep-alive

	if(w->zoutput) {
		headerlen += snprintf(&w->response_header[headerlen], MAX_HTTP_HEADER_SIZE - headerlen,
			"Content-Encoding: gzip\r\n"
			"Transfer-Encoding: chunked\r\n"
			);
	}

	headerlen += snprintf(&w->response_header[headerlen], MAX_HTTP_HEADER_SIZE - headerlen, "\r\n");

	// disable TCP_NODELAY, to buffer the header
	int flag = 0;
	if(setsockopt(w->ofd, IPPROTO_TCP, TCP_NODELAY, (char *) &flag, sizeof(int)) != 0) error("%llu: failed to disable TCP_NODELAY on socket.", w->id);

	// sent the HTTP header
	debug(D_WEB_DATA, "%llu: Sending response HTTP header of size %d: '%s'", w->id, headerlen, w->response_header);

	bytes = send(w->ofd, w->response_header, headerlen, 0);
	if(bytes != headerlen)
		error("%llu: HTTP Header failed to be sent (I sent %d bytes but the system sent %d bytes).", w->id, headerlen, bytes);
	else {
		global_statistics_lock();
		global_statistics.bytes_sent += bytes;
		global_statistics_unlock();
	}

	// enable TCP_NODELAY, to send all data immediately at the next send()
	flag = 1;
	if(setsockopt(w->ofd, IPPROTO_TCP, TCP_NODELAY, (char *) &flag, sizeof(int)) != 0) error("%llu: failed to enable TCP_NODELAY on socket.", w->id);

	// enable sending immediately if we have data
	if(w->data->bytes) w->wait_send = 1;
	else w->wait_send = 0;

	// pretty logging
	switch(w->mode) {
		case WEB_CLIENT_MODE_NORMAL:
			debug(D_WEB_CLIENT, "%llu: Done preparing the response. Sending data (%d bytes) to client.", w->id, w->data->bytes);
			break;

		case WEB_CLIENT_MODE_FILECOPY:
			if(w->data->rbytes) {
				debug(D_WEB_CLIENT, "%llu: Done preparing the response. Will be sending data file of %d bytes to client.", w->id, w->data->rbytes);
				w->wait_receive = 1;

				/*
				// utilize the kernel sendfile() for copying the file to the socket.
				// this block of code can be commented, without anything missing.
				// when it is commented, the program will copy the data using async I/O.
				{
					long len = sendfile(w->ofd, w->ifd, NULL, w->data->rbytes);
					if(len != w->data->rbytes) error("%llu: sendfile() should copy %ld bytes, but copied %ld. Falling back to manual copy.", w->id, w->data->rbytes, len);
					else web_client_reset(w);
				}
				*/
			}
			else
				debug(D_WEB_CLIENT, "%llu: Done preparing the response. Will be sending an unknown amount of bytes to client.", w->id);
			break;

		default:
			fatal("%llu: Unknown client mode %d.", w->id, w->mode);
			break;
	}
}

long web_client_send_chunk_header(struct web_client *w, int len)
{
	debug(D_DEFLATE, "%llu: OPEN CHUNK of %d bytes (hex: %x).", w->id, len, len);
	char buf[1024]; 
	sprintf(buf, "%X\r\n", len);
	int bytes = send(w->ofd, buf, strlen(buf), MSG_DONTWAIT);

	if(bytes > 0) debug(D_DEFLATE, "%llu: Sent chunk header %d bytes.", w->id, bytes);
	else if(bytes == 0) debug(D_DEFLATE, "%llu: Did not send chunk header to the client.", w->id);
	else debug(D_DEFLATE, "%llu: Failed to send chunk header to client. Reason: %s", w->id, strerror(errno));

	return bytes;
}

long web_client_send_chunk_close(struct web_client *w)
{
	//debug(D_DEFLATE, "%llu: CLOSE CHUNK.", w->id);

	int bytes = send(w->ofd, "\r\n", 2, MSG_DONTWAIT);

	if(bytes > 0) debug(D_DEFLATE, "%llu: Sent chunk suffix %d bytes.", w->id, bytes);
	else if(bytes == 0) debug(D_DEFLATE, "%llu: Did not send chunk suffix to the client.", w->id);
	else debug(D_DEFLATE, "%llu: Failed to send chunk suffix to client. Reason: %s", w->id, strerror(errno));

	return bytes;
}

long web_client_send_chunk_finalize(struct web_client *w)
{
	//debug(D_DEFLATE, "%llu: FINALIZE CHUNK.", w->id);

	int bytes = send(w->ofd, "\r\n0\r\n\r\n", 7, MSG_DONTWAIT);

	if(bytes > 0) debug(D_DEFLATE, "%llu: Sent chunk suffix %d bytes.", w->id, bytes);
	else if(bytes == 0) debug(D_DEFLATE, "%llu: Did not send chunk suffix to the client.", w->id);
	else debug(D_DEFLATE, "%llu: Failed to send chunk suffix to client. Reason: %s", w->id, strerror(errno));

	return bytes;
}

long web_client_send_deflate(struct web_client *w)
{
	long bytes = 0, t = 0;

	// when using compression,
	// w->data->sent is the amount of bytes passed through compression

	// debug(D_DEFLATE, "%llu: TEST w->data->bytes = %d, w->data->sent = %d, w->zhave = %d, w->zsent = %d, w->zstream.avail_in = %d, w->zstream.avail_out = %d, w->zstream.total_in = %d, w->zstream.total_out = %d.", w->id, w->data->bytes, w->data->sent, w->zhave, w->zsent, w->zstream.avail_in, w->zstream.avail_out, w->zstream.total_in, w->zstream.total_out);

	if(w->data->bytes - w->data->sent == 0 && w->zstream.avail_in == 0 && w->zhave == w->zsent && w->zstream.avail_out != 0) {
		// there is nothing to send

		debug(D_WEB_CLIENT, "%llu: Out of output data.", w->id);

		// finalize the chunk
		if(w->data->sent != 0)
			t += web_client_send_chunk_finalize(w);

		// there can be two cases for this
		// A. we have done everything
		// B. we temporarily have nothing to send, waiting for the buffer to be filled by ifd

		if(w->mode == WEB_CLIENT_MODE_FILECOPY && w->wait_receive && w->ifd != w->ofd && w->data->rbytes && w->data->rbytes > w->data->bytes) {
			// we have to wait, more data will come
			debug(D_WEB_CLIENT, "%llu: Waiting for more data to become available.", w->id);
			w->wait_send = 0;
			return(0);
		}

		if(w->keepalive == 0) {
			debug(D_WEB_CLIENT, "%llu: Closing (keep-alive is not enabled). %ld bytes sent.", w->id, w->data->sent);
			errno = 0;
			return(-1);
		}

		// reset the client
		web_client_reset(w);
		debug(D_WEB_CLIENT, "%llu: Done sending all data on socket. Waiting for next request on the same socket.", w->id);
		return(0);
	}

	if(w->zstream.avail_out == 0 && w->zhave == w->zsent) {
		// compress more input data

		// close the previous open chunk
		if(w->data->sent != 0) t += web_client_send_chunk_close(w);

		debug(D_DEFLATE, "%llu: Compressing %d bytes starting from %d.", w->id, (w->data->bytes - w->data->sent), w->data->sent);

		// give the compressor all the data not passed through the compressor yet
		if(w->data->bytes > w->data->sent) {
			w->zstream.next_in = (Bytef *)&w->data->buffer[w->data->sent];
			w->zstream.avail_in = (w->data->bytes - w->data->sent);
		}

		// reset the compressor output buffer
		w->zstream.next_out = w->zbuffer;
		w->zstream.avail_out = ZLIB_CHUNK;

		// ask for FINISH if we have all the input
		int flush = Z_SYNC_FLUSH;
		if(w->mode == WEB_CLIENT_MODE_NORMAL
			|| (w->mode == WEB_CLIENT_MODE_FILECOPY && w->data->bytes == w->data->rbytes)) {
			flush = Z_FINISH;
			debug(D_DEFLATE, "%llu: Requesting Z_FINISH.", w->id);
		}
		else {
			debug(D_DEFLATE, "%llu: Requesting Z_SYNC_FLUSH.", w->id);
		}

		// compress
		if(deflate(&w->zstream, flush) == Z_STREAM_ERROR) {
			error("%llu: Compression failed. Closing down client.", w->id);
			web_client_reset(w);
			return(-1);
		}

		w->zhave = ZLIB_CHUNK - w->zstream.avail_out;
		w->zsent = 0;

		// keep track of the bytes passed through the compressor
		w->data->sent = w->data->bytes;

		debug(D_DEFLATE, "%llu: Compression produced %d bytes.", w->id, w->zhave);

		// open a new chunk
		t += web_client_send_chunk_header(w, w->zhave);
	}

	bytes = send(w->ofd, &w->zbuffer[w->zsent], w->zhave - w->zsent, MSG_DONTWAIT);
	if(bytes > 0) {
		w->zsent += bytes;
		if(t > 0) bytes += t;
		debug(D_WEB_CLIENT, "%llu: Sent %d bytes.", w->id, bytes);
	}
	else if(bytes == 0) debug(D_WEB_CLIENT, "%llu: Did not send any bytes to the client.", w->id);
	else debug(D_WEB_CLIENT, "%llu: Failed to send data to client. Reason: %s", w->id, strerror(errno));

	return(bytes);
}

long web_client_send(struct web_client *w)
{
	if(w->zoutput) return web_client_send_deflate(w);

	long bytes;

	if(w->data->bytes - w->data->sent == 0) {
		// there is nothing to send

		debug(D_WEB_CLIENT, "%llu: Out of output data.", w->id);

		// there can be two cases for this
		// A. we have done everything
		// B. we temporarily have nothing to send, waiting for the buffer to be filled by ifd

		if(w->mode == WEB_CLIENT_MODE_FILECOPY && w->wait_receive && w->ifd != w->ofd && w->data->rbytes && w->data->rbytes > w->data->bytes) {
			// we have to wait, more data will come
			debug(D_WEB_CLIENT, "%llu: Waiting for more data to become available.", w->id);
			w->wait_send = 0;
			return(0);
		}

		if(w->keepalive == 0) {
			debug(D_WEB_CLIENT, "%llu: Closing (keep-alive is not enabled). %ld bytes sent.", w->id, w->data->sent);
			errno = 0;
			return(-1);
		}

		web_client_reset(w);
		debug(D_WEB_CLIENT, "%llu: Done sending all data on socket. Waiting for next request on the same socket.", w->id);
		return(0);
	}

	bytes = send(w->ofd, &w->data->buffer[w->data->sent], w->data->bytes - w->data->sent, MSG_DONTWAIT);
	if(bytes > 0) {
		w->data->sent += bytes;
		debug(D_WEB_CLIENT, "%llu: Sent %d bytes.", w->id, bytes);
	}
	else if(bytes == 0) debug(D_WEB_CLIENT, "%llu: Did not send any bytes to the client.", w->id);
	else debug(D_WEB_CLIENT, "%llu: Failed to send data to client. Reason: %s", w->id, strerror(errno));


	return(bytes);
}

long web_client_receive(struct web_client *w)
{
	// do we have any space for more data?
	web_buffer_increase(w->data, WEB_DATA_LENGTH_INCREASE_STEP);

	long left = w->data->size - w->data->bytes;
	long bytes;

	if(w->mode == WEB_CLIENT_MODE_FILECOPY)
		bytes = read(w->ifd, &w->data->buffer[w->data->bytes], (left-1));
	else
		bytes = recv(w->ifd, &w->data->buffer[w->data->bytes], left-1, MSG_DONTWAIT);

	if(bytes > 0) {
		int old = w->data->bytes;
		w->data->bytes += bytes;
		w->data->buffer[w->data->bytes] = '\0';

		debug(D_WEB_CLIENT, "%llu: Received %d bytes.", w->id, bytes);
		debug(D_WEB_DATA, "%llu: Received data: '%s'.", w->id, &w->data->buffer[old]);

		if(w->mode == WEB_CLIENT_MODE_FILECOPY) {
			w->wait_send = 1;
			if(w->data->rbytes && w->data->bytes >= w->data->rbytes) w->wait_receive = 0;
		}
	}
	else if(bytes == 0) {
		debug(D_WEB_CLIENT, "%llu: Out of input data.", w->id);

		// if we cannot read, it means we have an error on input.
		// if however, we are copying a file from ifd to ofd, we should not return an error.
		// in this case, the error should be generated when the file has been sent to the client.

		if(w->mode == WEB_CLIENT_MODE_FILECOPY) {
			// we are copying data fron ifd to ofd
			// let it finish copying...
			w->wait_receive = 0;
			debug(D_WEB_CLIENT, "%llu: Disabling input.", w->id);
		}
		else {
			bytes = -1;
			errno = 0;
		}
	}

	return(bytes);
}


// --------------------------------------------------------------------------------------
// the thread of a single client

// 1. waits for input and output, using async I/O
// 2. it processes HTTP requests
// 3. it generates HTTP responses
// 4. it copies data from input to output if mode is FILECOPY

void *new_client(void *ptr)
{
	struct timeval tv;
	struct web_client *w = ptr;
	int retval;
	fd_set ifds, ofds, efds;
	int fdmax = 0;

	for(;;) {
		FD_ZERO (&ifds);
		FD_ZERO (&ofds);
		FD_ZERO (&efds);

		FD_SET(w->ifd, &efds);
		if(w->ifd != w->ofd)	FD_SET(w->ofd, &efds);
		if (w->wait_receive) {
			FD_SET(w->ifd, &ifds);
			if(w->ifd > fdmax) fdmax = w->ifd;
		}
		if (w->wait_send) {
			FD_SET(w->ofd, &ofds);
			if(w->ofd > fdmax) fdmax = w->ofd;
		}

		tv.tv_sec = 30;
		tv.tv_usec = 0;

		debug(D_WEB_CLIENT, "%llu: Waiting socket async I/O for %s %s", w->id, w->wait_receive?"INPUT":"", w->wait_send?"OUTPUT":"");
		retval = select(fdmax+1, &ifds, &ofds, &efds, &tv);

		if(retval == -1) {
			error("%llu: LISTENER: select() failed.", w->id);
			continue;
		}
		else if(!retval) {
			// timeout
			web_client_reset(w);
			w->obsolete = 1;
			return NULL;
		}

		if(FD_ISSET(w->ifd, &efds)) {
			debug(D_WEB_CLIENT_ACCESS, "%llu: Received error on input socket (%s).", w->id, strerror(errno));
			web_client_reset(w);
			w->obsolete = 1;
			return NULL;
		}

		if(FD_ISSET(w->ofd, &efds)) {
			debug(D_WEB_CLIENT_ACCESS, "%llu: Received error on output socket (%s).", w->id, strerror(errno));
			web_client_reset(w);
			w->obsolete = 1;
			return NULL;
		}

		if(w->wait_send && FD_ISSET(w->ofd, &ofds)) {
			long bytes;
			if((bytes = web_client_send(w)) < 0) {
				debug(D_WEB_CLIENT, "%llu: Closing client (input: %s).", w->id, strerror(errno));
				web_client_reset(w);
				w->obsolete = 1;
				errno = 0;
				return NULL;
			}
			else {
				global_statistics_lock();
				global_statistics.bytes_sent += bytes;
				global_statistics_unlock();
			}
		}

		if(w->wait_receive && FD_ISSET(w->ifd, &ifds)) {
			long bytes;
			if((bytes = web_client_receive(w)) < 0) {
				debug(D_WEB_CLIENT, "%llu: Closing client (output: %s).", w->id, strerror(errno));
				web_client_reset(w);
				w->obsolete = 1;
				errno = 0;
				return NULL;
			}
			else {
				if(w->mode != WEB_CLIENT_MODE_FILECOPY) {
					global_statistics_lock();
					global_statistics.bytes_received += bytes;
					global_statistics_unlock();
				}
			}

			if(w->mode == WEB_CLIENT_MODE_NORMAL) web_client_process(w);
		}
	}
	debug(D_WEB_CLIENT, "%llu: done...", w->id);

	return NULL;
}

// --------------------------------------------------------------------------------------
// the main socket listener

// 1. it accepts new incoming requests on our port
// 2. creates a new web_client for each connection received
// 3. spawns a new pthread to serve the client (this is optimal for keep-alive clients)
// 4. cleans up old web_clients that their pthreads have been exited

void log_allocations(void)
{
	static int mem = 0;

	struct mallinfo mi;

	mi = mallinfo();
	if(mi.uordblks > mem) {
		int clients = 0;
		struct web_client *w;
		for(w = web_clients; w ; w = w->next) clients++;

		info("Allocated memory increased from %d to %d (increased by %d bytes). There are %d web clients connected.", mem, mi.uordblks, mi.uordblks - mem, clients);
		mem = mi.uordblks;
	}
}

void *socket_listen_main(void *ptr)
{
	if(ptr) { ; }
	struct web_client *w;
	struct timeval tv;
	int retval;

	// int listener = create_listen_socket(listen_port);
	int listener = listen_fd;
	if(listener == -1) fatal("LISTENER: Cannot create listening socket on port 19999.");

	fd_set ifds, ofds, efds;
	int fdmax = listener;

	FD_ZERO (&ifds);
	FD_ZERO (&ofds);
	FD_ZERO (&efds);

	for(;;) {
		tv.tv_sec = 0;
		tv.tv_usec = 200000;

		FD_SET(listener, &ifds);
		FD_SET(listener, &efds);

		// debug(D_WEB_CLIENT, "LISTENER: Waiting...");
		retval = select(fdmax+1, &ifds, &ofds, &efds, &tv);

		if(retval == -1) {
			error("LISTENER: select() failed.");
			continue;
		}
		else if(retval) {
			// check for new incoming connections
			if(FD_ISSET(listener, &ifds)) {
				w = web_client_create(listener);

				if(pthread_create(&w->thread, NULL, new_client, w) != 0) {
					error("%llu: failed to create new thread for web client.");
					w->obsolete = 1;
				}
				else if(pthread_detach(w->thread) != 0) {
					error("%llu: Cannot request detach of newly created web client thread.", w->id);
					w->obsolete = 1;
				}
				
				log_access("%llu: %s connected", w->id, w->client_ip);
			}
			else debug(D_WEB_CLIENT, "LISTENER: select() didn't do anything.");

		}
		//else debug(D_WEB_CLIENT, "LISTENER: select() timeout.");

		// cleanup unused clients
		for(w = web_clients; w ; w = w?w->next:NULL) {
			if(w->obsolete) {
				log_access("%llu: %s disconnected", w->id, w->client_ip);
				debug(D_WEB_CLIENT, "%llu: Removing client.", w->id);
				// pthread_join(w->thread,  NULL);
				w = web_client_free(w);
				log_allocations();
			}
		}
	}

	error("LISTENER: exit!");

	close(listener);
	exit(2);

	return NULL;
}

// ----------------------------------------------------------------------------
// /proc/net/dev processor

#define MAX_PROC_NET_DEV_LINE 4096
#define MAX_PROC_NET_DEV_IFACE_NAME 1024

int do_proc_net_dev() {
	static int enable_new_interfaces = -1;
	static int do_bandwidth = -1, do_packets = -1, do_errors = -1, do_fifo = -1, do_compressed = -1;

	if(enable_new_interfaces == -1)	enable_new_interfaces = config_get_boolean("plugin:proc:/proc/net/dev", "enable new interfaces detected at runtime", 1);

	if(do_bandwidth == -1)	do_bandwidth = config_get_boolean("plugin:proc:/proc/net/dev", "bandwidth for all interfaces", 1);
	if(do_packets == -1)	do_packets = config_get_boolean("plugin:proc:/proc/net/dev", "packets for all interfaces", 1);
	if(do_errors == -1)		do_errors = config_get_boolean("plugin:proc:/proc/net/dev", "errors for all interfaces", 1);
	if(do_fifo == -1) 		do_fifo = config_get_boolean("plugin:proc:/proc/net/dev", "fifo for all interfaces", 1);
	if(do_compressed == -1)	do_compressed = config_get_boolean("plugin:proc:/proc/net/dev", "compressed packets for all interfaces", 1);

	char buffer[MAX_PROC_NET_DEV_LINE+1] = "";
	char iface[MAX_PROC_NET_DEV_IFACE_NAME + 1] = "";
	unsigned long long rbytes, rpackets, rerrors, rdrops, rfifo, rframe, rcompressed, rmulticast;
	unsigned long long tbytes, tpackets, terrors, tdrops, tfifo, tcollisions, tcarrier, tcompressed;
	
	int r;
	char *p;
	
	FILE *fp = fopen("/proc/net/dev", "r");
	if(!fp) {
		error("Cannot read /proc/net/dev.");
		return 1;
	}
	
	// skip the first two lines
	p = fgets(buffer, MAX_PROC_NET_DEV_LINE, fp);
	p = fgets(buffer, MAX_PROC_NET_DEV_LINE, fp);
	
	// read the rest of the lines
	for(;1;) {
		char *c;
		p = fgets(buffer, MAX_PROC_NET_DEV_LINE, fp);
		if(!p) break;
		
		c = strchr(buffer, ':');
		if(c) *c = '\t';
		
		r = sscanf(buffer, "%s %llu %llu %llu %llu %llu %llu %llu %llu %llu %llu %llu %llu %llu %llu %llu %llu\n",
			iface,
			&rbytes, &rpackets, &rerrors, &rdrops, &rfifo, &rframe, &rcompressed, &rmulticast,
			&tbytes, &tpackets, &terrors, &tdrops, &tfifo, &tcollisions, &tcarrier, &tcompressed);
		if(r == EOF) break;
		if(r != 17) {
			error("Cannot read /proc/net/dev line. Expected 17 params, read %d.", r);
			continue;
		}

		// check if it is enabled
		{
			char var_name[4096 + 1];
			snprintf(var_name, 4096, "interface %s", iface);
			if(!config_get_boolean("plugin:proc:/proc/net/dev", var_name, enable_new_interfaces)) continue;
		}

		RRD_STATS *st;

		// --------------------------------------------------------------------

		if(do_bandwidth) {
			st = rrd_stats_find_bytype(RRD_TYPE_NET, iface);
			if(!st) {
				st = rrd_stats_create(RRD_TYPE_NET, iface, NULL, iface, "Bandwidth", "kilobits/s", 1000, update_every, CHART_TYPE_AREA);

				rrd_stats_dimension_add(st, "received", NULL, 8, 1024, RRD_DIMENSION_INCREMENTAL);
				rrd_stats_dimension_add(st, "sent", NULL, -8, 1024, RRD_DIMENSION_INCREMENTAL);
			}
			else rrd_stats_next(st);

			rrd_stats_dimension_set(st, "received", rbytes);
			rrd_stats_dimension_set(st, "sent", tbytes);
			rrd_stats_done(st);
		}

		// --------------------------------------------------------------------

		if(do_packets) {
			st = rrd_stats_find_bytype("net_packets", iface);
			if(!st) {
				st = rrd_stats_create("net_packets", iface, NULL, iface, "Packets", "packets/s", 1001, update_every, CHART_TYPE_LINE);
				st->isdetail = 1;

				rrd_stats_dimension_add(st, "received", NULL, 1, 1, RRD_DIMENSION_INCREMENTAL);
				rrd_stats_dimension_add(st, "sent", NULL, -1, 1, RRD_DIMENSION_INCREMENTAL);
			}
			else rrd_stats_next(st);

			rrd_stats_dimension_set(st, "received", rpackets);
			rrd_stats_dimension_set(st, "sent", tpackets);
			rrd_stats_done(st);
		}

		// --------------------------------------------------------------------

		if(do_errors) {
			st = rrd_stats_find_bytype("net_errors", iface);
			if(!st) {
				st = rrd_stats_create("net_errors", iface, NULL, iface, "Interface Errors", "errors/s", 1002, update_every, CHART_TYPE_LINE);
				st->isdetail = 1;

				rrd_stats_dimension_add(st, "receive", NULL, 1, 1, RRD_DIMENSION_INCREMENTAL);
				rrd_stats_dimension_add(st, "transmit", NULL, -1, 1, RRD_DIMENSION_INCREMENTAL);
			}
			else rrd_stats_next(st);

			rrd_stats_dimension_set(st, "receive", rerrors);
			rrd_stats_dimension_set(st, "transmit", terrors);
			rrd_stats_done(st);
		}

		// --------------------------------------------------------------------

		if(do_fifo) {
			st = rrd_stats_find_bytype("net_fifo", iface);
			if(!st) {
				st = rrd_stats_create("net_fifo", iface, NULL, iface, "Interface Queue", "packets", 1100, update_every, CHART_TYPE_LINE);
				st->isdetail = 1;

				rrd_stats_dimension_add(st, "receive", NULL, 1, 1, RRD_DIMENSION_ABSOLUTE);
				rrd_stats_dimension_add(st, "transmit", NULL, -1, 1, RRD_DIMENSION_ABSOLUTE);
			}
			else rrd_stats_next(st);

			rrd_stats_dimension_set(st, "receive", rfifo);
			rrd_stats_dimension_set(st, "transmit", tfifo);
			rrd_stats_done(st);
		}

		// --------------------------------------------------------------------

		if(do_compressed) {
			st = rrd_stats_find_bytype("net_compressed", iface);
			if(!st) {
				st = rrd_stats_create("net_compressed", iface, NULL, iface, "Compressed Packets", "packets/s", 1200, update_every, CHART_TYPE_LINE);
				st->isdetail = 1;

				rrd_stats_dimension_add(st, "received", NULL, 1, 1, RRD_DIMENSION_INCREMENTAL);
				rrd_stats_dimension_add(st, "sent", NULL, -1, 1, RRD_DIMENSION_INCREMENTAL);
			}
			else rrd_stats_next(st);

			rrd_stats_dimension_set(st, "received", rcompressed);
			rrd_stats_dimension_set(st, "sent", tcompressed);
			rrd_stats_done(st);
		}
	}
	
	fclose(fp);
	return 0;
}

// ----------------------------------------------------------------------------
// /proc/diskstats processor

#define MAX_PROC_DISKSTATS_LINE 4096
#define MAX_PROC_DISKSTATS_DISK_NAME 1024

int do_proc_diskstats() {
	static int enable_new_disks = -1;
	static int do_io = -1, do_ops = -1, do_merged_ops = -1, do_iotime = -1, do_cur_ops = -1;

	if(enable_new_disks == -1)	enable_new_disks = config_get_boolean("plugin:proc:/proc/diskstats", "enable new disks detected at runtime", 1);

	if(do_io == -1)			do_io = config_get_boolean("plugin:proc:/proc/diskstats", "bandwidth for all disks", 1);
	if(do_ops == -1)		do_ops = config_get_boolean("plugin:proc:/proc/diskstats", "operations for all disks", 1);
	if(do_merged_ops == -1)	do_merged_ops = config_get_boolean("plugin:proc:/proc/diskstats", "merged operations for all disks", 1);
	if(do_iotime == -1)		do_iotime = config_get_boolean("plugin:proc:/proc/diskstats", "i/o time for all disks", 1);
	if(do_cur_ops == -1)	do_cur_ops = config_get_boolean("plugin:proc:/proc/diskstats", "current operations for all disks", 1);

	char buffer[MAX_PROC_DISKSTATS_LINE+1] = "";
	char disk[MAX_PROC_DISKSTATS_DISK_NAME + 1] = "";
	
	int r;
	char *p;
	
	FILE *fp = fopen("/proc/diskstats", "r");
	if(!fp) {
		error("Cannot read /proc/diskstats.");
		return 1;
	}
	
	for(;1;) {
		unsigned long long 	major = 0, minor = 0,
							reads = 0,  reads_merged = 0,  readsectors = 0,  readms = 0,
							writes = 0, writes_merged = 0, writesectors = 0, writems = 0,
							currentios = 0, iosms = 0, wiosms = 0;

		p = fgets(buffer, MAX_PROC_DISKSTATS_LINE, fp);
		if(!p) break;
		
		r = sscanf(buffer, "%llu %llu %s %llu %llu %llu %llu %llu %llu %llu %llu %llu %llu %llu\n",
			&major, &minor, disk,
			&reads, &reads_merged, &readsectors, &readms, &writes, &writes_merged, &writesectors, &writems, &currentios, &iosms, &wiosms
		);
		if(r == EOF) break;
		if(r != 14) {
			error("Cannot read /proc/diskstats line. Expected 14 params, read %d.", r);
			continue;
		}

		int def_enabled = 0;

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

		RRD_STATS *st;

		// --------------------------------------------------------------------

		if(do_io) {
			st = rrd_stats_find_bytype(RRD_TYPE_DISK, disk);
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

				st = rrd_stats_create(RRD_TYPE_DISK, disk, NULL, disk, "Disk I/O", "kilobytes/s", 2000, update_every, CHART_TYPE_AREA);

				rrd_stats_dimension_add(st, "reads", NULL, sector_size, 1024, RRD_DIMENSION_INCREMENTAL);
				rrd_stats_dimension_add(st, "writes", NULL, sector_size * -1, 1024, RRD_DIMENSION_INCREMENTAL);
			}
			else rrd_stats_next(st);

			rrd_stats_dimension_set(st, "reads", readsectors);
			rrd_stats_dimension_set(st, "writes", writesectors);
			rrd_stats_done(st);
		}

		// --------------------------------------------------------------------

		if(do_ops) {
			st = rrd_stats_find_bytype("disk_ops", disk);
			if(!st) {
				st = rrd_stats_create("disk_ops", disk, NULL, disk, "Disk Operations", "operations/s", 2001, update_every, CHART_TYPE_LINE);
				st->isdetail = 1;

				rrd_stats_dimension_add(st, "reads", NULL, 1, 1, RRD_DIMENSION_INCREMENTAL);
				rrd_stats_dimension_add(st, "writes", NULL, -1, 1, RRD_DIMENSION_INCREMENTAL);
			}
			else rrd_stats_next(st);

			rrd_stats_dimension_set(st, "reads", reads);
			rrd_stats_dimension_set(st, "writes", writes);
			rrd_stats_done(st);
		}
		
		// --------------------------------------------------------------------

		if(do_merged_ops) {
			st = rrd_stats_find_bytype("disk_merged_ops", disk);
			if(!st) {
				st = rrd_stats_create("disk_merged_ops", disk, NULL, disk, "Merged Disk Operations", "operations/s", 2010, update_every, CHART_TYPE_LINE);
				st->isdetail = 1;

				rrd_stats_dimension_add(st, "reads", NULL, 1, 1, RRD_DIMENSION_INCREMENTAL);
				rrd_stats_dimension_add(st, "writes", NULL, -1, 1, RRD_DIMENSION_INCREMENTAL);
			}
			else rrd_stats_next(st);

			rrd_stats_dimension_set(st, "reads", reads_merged);
			rrd_stats_dimension_set(st, "writes", writes_merged);
			rrd_stats_done(st);
		}

		// --------------------------------------------------------------------

		if(do_iotime) {
			st = rrd_stats_find_bytype("disk_iotime", disk);
			if(!st) {
				st = rrd_stats_create("disk_iotime", disk, NULL, disk, "Disk I/O Time", "milliseconds/s", 2005, update_every, CHART_TYPE_LINE);
				st->isdetail = 1;

				rrd_stats_dimension_add(st, "reads", NULL, 1, 1, RRD_DIMENSION_INCREMENTAL);
				rrd_stats_dimension_add(st, "writes", NULL, -1, 1, RRD_DIMENSION_INCREMENTAL);
				rrd_stats_dimension_add(st, "latency", NULL, 1, 1, RRD_DIMENSION_INCREMENTAL);
				rrd_stats_dimension_add(st, "weighted", NULL, 1, 1, RRD_DIMENSION_INCREMENTAL);
			}
			else rrd_stats_next(st);

			rrd_stats_dimension_set(st, "reads", readms);
			rrd_stats_dimension_set(st, "writes", writems);
			rrd_stats_dimension_set(st, "latency", iosms);
			rrd_stats_dimension_set(st, "weighted", wiosms);
			rrd_stats_done(st);
		}

		// --------------------------------------------------------------------

		if(do_cur_ops) {
			st = rrd_stats_find_bytype("disk_cur_ops", disk);
			if(!st) {
				st = rrd_stats_create("disk_cur_ops", disk, NULL, disk, "Current Disk I/O operations", "operations", 2004, update_every, CHART_TYPE_LINE);
				st->isdetail = 1;

				rrd_stats_dimension_add(st, "operations", NULL, 1, 1, RRD_DIMENSION_ABSOLUTE);
			}
			else rrd_stats_next(st);

			rrd_stats_dimension_set(st, "operations", currentios);
			rrd_stats_done(st);
		}
	}
	
	fclose(fp);
	return 0;
}

// ----------------------------------------------------------------------------
// /proc/net/snmp processor

int do_proc_net_snmp() {
	static int do_ip_packets = -1, do_ip_fragsout = -1, do_ip_fragsin = -1, do_ip_errors = -1,
		do_tcp_sockets = -1, do_tcp_packets = -1, do_tcp_errors = -1, do_tcp_handshake = -1, 
		do_udp_packets = -1, do_udp_errors = -1;

	if(do_ip_packets == -1)		do_ip_packets = config_get_boolean("plugin:proc:/proc/net/snmp", "ipv4 packets", 1);
	if(do_ip_fragsout == -1)	do_ip_fragsout = config_get_boolean("plugin:proc:/proc/net/snmp", "ipv4 fragrments sent", 1);
	if(do_ip_fragsin == -1)		do_ip_fragsin = config_get_boolean("plugin:proc:/proc/net/snmp", "ipv4 fragments assembly", 1);
	if(do_ip_errors == -1)		do_ip_errors = config_get_boolean("plugin:proc:/proc/net/snmp", "ipv4 errors", 1);
	if(do_tcp_sockets == -1)	do_tcp_sockets = config_get_boolean("plugin:proc:/proc/net/snmp", "ipv4 TCP connections", 1);
	if(do_tcp_packets == -1)	do_tcp_packets = config_get_boolean("plugin:proc:/proc/net/snmp", "ipv4 TCP packets", 1);
	if(do_tcp_errors == -1)		do_tcp_errors = config_get_boolean("plugin:proc:/proc/net/snmp", "ipv4 TCP errors", 1);
	if(do_tcp_handshake == -1)	do_tcp_handshake = config_get_boolean("plugin:proc:/proc/net/snmp", "ipv4 TCP handshake issues", 1);
	if(do_udp_packets == -1)	do_udp_packets = config_get_boolean("plugin:proc:/proc/net/snmp", "ipv4 UDP packets", 1);
	if(do_udp_errors == -1)		do_udp_errors = config_get_boolean("plugin:proc:/proc/net/snmp", "ipv4 UDP errors", 1);

	char buffer[MAX_PROC_NET_SNMP_LINE+1] = "";

	FILE *fp = fopen("/proc/net/snmp", "r");
	if(!fp) {
		error("Cannot read /proc/net/snmp.");
		return 1;
	}

	RRD_STATS *st;

	for(;1;) {
		char *p = fgets(buffer, MAX_PROC_NET_SNMP_LINE, fp);
		if(!p) break;

		if(strncmp(p, "Ip: ", 4) == 0) {
			// skip the header line, read the data
			p = fgets(buffer, MAX_PROC_NET_SNMP_LINE, fp);
			if(!p) break;

			if(strncmp(p, "Ip: ", 4) != 0) {
				error("Cannot read IP line from /proc/net/snmp.");
				break;
			}

			// see also http://net-snmp.sourceforge.net/docs/mibs/ip.html
			unsigned long long Forwarding, DefaultTTL, InReceives, InHdrErrors, InAddrErrors, ForwDatagrams, InUnknownProtos, InDiscards, InDelivers,
				OutRequests, OutDiscards, OutNoRoutes, ReasmTimeout, ReasmReqds, ReasmOKs, ReasmFails, FragOKs, FragFails, FragCreates;

			int r = sscanf(&buffer[4], "%llu %llu %llu %llu %llu %llu %llu %llu %llu %llu %llu %llu %llu %llu %llu %llu %llu %llu %llu\n",
				&Forwarding, &DefaultTTL, &InReceives, &InHdrErrors, &InAddrErrors, &ForwDatagrams, &InUnknownProtos, &InDiscards, &InDelivers,
				&OutRequests, &OutDiscards, &OutNoRoutes, &ReasmTimeout, &ReasmReqds, &ReasmOKs, &ReasmFails, &FragOKs, &FragFails, &FragCreates);

			if(r == EOF) break;
			if(r != 19) error("Cannot read /proc/net/snmp IP line. Expected 19 params, read %d.", r);

			// --------------------------------------------------------------------

			if(do_ip_packets) {
				st = rrd_stats_find(RRD_TYPE_NET_SNMP ".packets");
				if(!st) {
					st = rrd_stats_create(RRD_TYPE_NET_SNMP, "packets", NULL, RRD_TYPE_NET_SNMP, "IPv4 Packets", "packets/s", 3000, update_every, CHART_TYPE_LINE);

					rrd_stats_dimension_add(st, "received", NULL, 1, 1, RRD_DIMENSION_INCREMENTAL);
					rrd_stats_dimension_add(st, "sent", NULL, -1, 1, RRD_DIMENSION_INCREMENTAL);
					rrd_stats_dimension_add(st, "forwarded", NULL, 1, 1, RRD_DIMENSION_INCREMENTAL);
				}
				else rrd_stats_next(st);

				rrd_stats_dimension_set(st, "sent", OutRequests);
				rrd_stats_dimension_set(st, "received", InReceives);
				rrd_stats_dimension_set(st, "forwarded", ForwDatagrams);
				rrd_stats_done(st);
			}

			// --------------------------------------------------------------------

			if(do_ip_fragsout) {
				st = rrd_stats_find(RRD_TYPE_NET_SNMP ".fragsout");
				if(!st) {
					st = rrd_stats_create(RRD_TYPE_NET_SNMP, "fragsout", NULL, RRD_TYPE_NET_SNMP, "IPv4 Fragments Sent", "packets/s", 3010, update_every, CHART_TYPE_LINE);
					st->isdetail = 1;

					rrd_stats_dimension_add(st, "ok", NULL, 1, 1, RRD_DIMENSION_INCREMENTAL);
					rrd_stats_dimension_add(st, "failed", NULL, -1, 1, RRD_DIMENSION_INCREMENTAL);
					rrd_stats_dimension_add(st, "all", NULL, 1, 1, RRD_DIMENSION_INCREMENTAL);
				}
				else rrd_stats_next(st);

				rrd_stats_dimension_set(st, "ok", FragOKs);
				rrd_stats_dimension_set(st, "failed", FragFails);
				rrd_stats_dimension_set(st, "all", FragCreates);
				rrd_stats_done(st);
			}

			// --------------------------------------------------------------------

			if(do_ip_fragsin) {
				st = rrd_stats_find(RRD_TYPE_NET_SNMP ".fragsin");
				if(!st) {
					st = rrd_stats_create(RRD_TYPE_NET_SNMP, "fragsin", NULL, RRD_TYPE_NET_SNMP, "IPv4 Fragments Reassembly", "packets/s", 3011, update_every, CHART_TYPE_LINE);
					st->isdetail = 1;

					rrd_stats_dimension_add(st, "ok", NULL, 1, 1, RRD_DIMENSION_INCREMENTAL);
					rrd_stats_dimension_add(st, "failed", NULL, -1, 1, RRD_DIMENSION_INCREMENTAL);
					rrd_stats_dimension_add(st, "all", NULL, 1, 1, RRD_DIMENSION_INCREMENTAL);
				}
				else rrd_stats_next(st);

				rrd_stats_dimension_set(st, "ok", ReasmOKs);
				rrd_stats_dimension_set(st, "failed", ReasmFails);
				rrd_stats_dimension_set(st, "all", ReasmReqds);
				rrd_stats_done(st);
			}

			// --------------------------------------------------------------------

			if(do_ip_errors) {
				st = rrd_stats_find(RRD_TYPE_NET_SNMP ".errors");
				if(!st) {
					st = rrd_stats_create(RRD_TYPE_NET_SNMP, "errors", NULL, RRD_TYPE_NET_SNMP, "IPv4 Errors", "packets/s", 3002, update_every, CHART_TYPE_LINE);
					st->isdetail = 1;

					rrd_stats_dimension_add(st, "InDiscards", NULL, 1, 1, RRD_DIMENSION_INCREMENTAL);
					rrd_stats_dimension_add(st, "OutDiscards", NULL, -1, 1, RRD_DIMENSION_INCREMENTAL);

					rrd_stats_dimension_add(st, "InHdrErrors", NULL, 1, 1, RRD_DIMENSION_INCREMENTAL);
					rrd_stats_dimension_add(st, "InAddrErrors", NULL, 1, 1, RRD_DIMENSION_INCREMENTAL);
					rrd_stats_dimension_add(st, "InUnknownProtos", NULL, 1, 1, RRD_DIMENSION_INCREMENTAL);

					rrd_stats_dimension_add(st, "OutNoRoutes", NULL, -1, 1, RRD_DIMENSION_INCREMENTAL);
				}
				else rrd_stats_next(st);

				rrd_stats_dimension_set(st, "InDiscards", InDiscards);
				rrd_stats_dimension_set(st, "OutDiscards", OutDiscards);
				rrd_stats_dimension_set(st, "InHdrErrors", InHdrErrors);
				rrd_stats_dimension_set(st, "InAddrErrors", InAddrErrors);
				rrd_stats_dimension_set(st, "InUnknownProtos", InUnknownProtos);
				rrd_stats_dimension_set(st, "OutNoRoutes", OutNoRoutes);
				rrd_stats_done(st);
			}
		}
		else if(strncmp(p, "Tcp: ", 5) == 0) {
			// skip the header line, read the data
			p = fgets(buffer, MAX_PROC_NET_SNMP_LINE, fp);
			if(!p) break;

			if(strncmp(p, "Tcp: ", 5) != 0) {
				error("Cannot read TCP line from /proc/net/snmp.");
				break;
			}

			unsigned long long RtoAlgorithm, RtoMin, RtoMax, MaxConn, ActiveOpens, PassiveOpens, AttemptFails, EstabResets,
				CurrEstab, InSegs, OutSegs, RetransSegs, InErrs, OutRsts;

			int r = sscanf(&buffer[5], "%llu %llu %llu %llu %llu %llu %llu %llu %llu %llu %llu %llu %llu %llu\n",
				&RtoAlgorithm, &RtoMin, &RtoMax, &MaxConn, &ActiveOpens, &PassiveOpens, &AttemptFails, &EstabResets, &CurrEstab, &InSegs, &OutSegs, &RetransSegs, &InErrs, &OutRsts);

			if(r == EOF) break;
			if(r != 14) error("Cannot read /proc/net/snmp TCP line. Expected 14 params, read %d.", r);

			// --------------------------------------------------------------------
			
			// see http://net-snmp.sourceforge.net/docs/mibs/tcp.html
			if(do_tcp_sockets) {
				st = rrd_stats_find(RRD_TYPE_NET_SNMP ".tcpsock");
				if(!st) {
					st = rrd_stats_create(RRD_TYPE_NET_SNMP, "tcpsock", NULL, "tcp", "IPv4 TCP Connections", "active connections", 2500, update_every, CHART_TYPE_LINE);

					rrd_stats_dimension_add(st, "connections", NULL, 1, 1, RRD_DIMENSION_ABSOLUTE);
				}
				else rrd_stats_next(st);

				rrd_stats_dimension_set(st, "connections", CurrEstab);
				rrd_stats_done(st);
			}

			// --------------------------------------------------------------------
			
			if(do_tcp_packets) {
				st = rrd_stats_find(RRD_TYPE_NET_SNMP ".tcppackets");
				if(!st) {
					st = rrd_stats_create(RRD_TYPE_NET_SNMP, "tcppackets", NULL, "tcp", "IPv4 TCP Packets", "packets/s", 2600, update_every, CHART_TYPE_LINE);

					rrd_stats_dimension_add(st, "received", NULL, 1, 1, RRD_DIMENSION_INCREMENTAL);
					rrd_stats_dimension_add(st, "sent", NULL, -1, 1, RRD_DIMENSION_INCREMENTAL);
				}
				else rrd_stats_next(st);

				rrd_stats_dimension_set(st, "received", InSegs);
				rrd_stats_dimension_set(st, "sent", OutSegs);
				rrd_stats_done(st);
			}

			// --------------------------------------------------------------------
			
			if(do_tcp_errors) {
				st = rrd_stats_find(RRD_TYPE_NET_SNMP ".tcperrors");
				if(!st) {
					st = rrd_stats_create(RRD_TYPE_NET_SNMP, "tcperrors", NULL, "tcp", "IPv4 TCP Errors", "packets/s", 2700, update_every, CHART_TYPE_LINE);
					st->isdetail = 1;

					rrd_stats_dimension_add(st, "InErrs", NULL, 1, 1, RRD_DIMENSION_INCREMENTAL);
					rrd_stats_dimension_add(st, "RetransSegs", NULL, -1, 1, RRD_DIMENSION_INCREMENTAL);
				}
				else rrd_stats_next(st);

				rrd_stats_dimension_set(st, "InErrs", InErrs);
				rrd_stats_dimension_set(st, "RetransSegs", RetransSegs);
				rrd_stats_done(st);
			}

			// --------------------------------------------------------------------
			
			if(do_tcp_handshake) {
				st = rrd_stats_find(RRD_TYPE_NET_SNMP ".tcphandshake");
				if(!st) {
					st = rrd_stats_create(RRD_TYPE_NET_SNMP, "tcphandshake", NULL, "tcp", "IPv4 TCP Handshake Issues", "events/s", 2900, update_every, CHART_TYPE_LINE);
					st->isdetail = 1;

					rrd_stats_dimension_add(st, "EstabResets", NULL, 1, 1, RRD_DIMENSION_INCREMENTAL);
					rrd_stats_dimension_add(st, "OutRsts", NULL, -1, 1, RRD_DIMENSION_INCREMENTAL);
					rrd_stats_dimension_add(st, "ActiveOpens", NULL, 1, 1, RRD_DIMENSION_INCREMENTAL);
					rrd_stats_dimension_add(st, "PassiveOpens", NULL, 1, 1, RRD_DIMENSION_INCREMENTAL);
					rrd_stats_dimension_add(st, "AttemptFails", NULL, 1, 1, RRD_DIMENSION_INCREMENTAL);
				}
				else rrd_stats_next(st);

				rrd_stats_dimension_set(st, "EstabResets", EstabResets);
				rrd_stats_dimension_set(st, "OutRsts", OutRsts);
				rrd_stats_dimension_set(st, "ActiveOpens", ActiveOpens);
				rrd_stats_dimension_set(st, "PassiveOpens", PassiveOpens);
				rrd_stats_dimension_set(st, "AttemptFails", AttemptFails);
				rrd_stats_done(st);
			}
		}
		else if(strncmp(p, "Udp: ", 5) == 0) {
			// skip the header line, read the data
			p = fgets(buffer, MAX_PROC_NET_SNMP_LINE, fp);
			if(!p) break;

			if(strncmp(p, "Udp: ", 5) != 0) {
				error("Cannot read UDP line from /proc/net/snmp.");
				break;
			}

			unsigned long long InDatagrams, NoPorts, InErrors, OutDatagrams, RcvbufErrors, SndbufErrors;

			int r = sscanf(&buffer[5], "%llu %llu %llu %llu %llu %llu\n",
				&InDatagrams, &NoPorts, &InErrors, &OutDatagrams, &RcvbufErrors, &SndbufErrors);

			if(r == EOF) break;
			if(r != 6) error("Cannot read /proc/net/snmp UDP line. Expected 6 params, read %d.", r);

			// --------------------------------------------------------------------
			
			// see http://net-snmp.sourceforge.net/docs/mibs/udp.html
			if(do_udp_packets) {
				st = rrd_stats_find(RRD_TYPE_NET_SNMP ".udppackets");
				if(!st) {
					st = rrd_stats_create(RRD_TYPE_NET_SNMP, "udppackets", NULL, "udp", "IPv4 UDP Packets", "packets/s", 2601, update_every, CHART_TYPE_LINE);

					rrd_stats_dimension_add(st, "received", NULL, 1, 1, RRD_DIMENSION_INCREMENTAL);
					rrd_stats_dimension_add(st, "sent", NULL, -1, 1, RRD_DIMENSION_INCREMENTAL);
				}
				else rrd_stats_next(st);

				rrd_stats_dimension_set(st, "received", InDatagrams);
				rrd_stats_dimension_set(st, "sent", OutDatagrams);
				rrd_stats_done(st);
			}

			// --------------------------------------------------------------------
			
			if(do_udp_errors) {
				st = rrd_stats_find(RRD_TYPE_NET_SNMP ".udperrors");
				if(!st) {
					st = rrd_stats_create(RRD_TYPE_NET_SNMP, "udperrors", NULL, "udp", "IPv4 UDP Errors", "events/s", 2701, update_every, CHART_TYPE_LINE);
					st->isdetail = 1;

					rrd_stats_dimension_add(st, "RcvbufErrors", NULL, 1, 1, RRD_DIMENSION_INCREMENTAL);
					rrd_stats_dimension_add(st, "SndbufErrors", NULL, -1, 1, RRD_DIMENSION_INCREMENTAL);
					rrd_stats_dimension_add(st, "InErrors", NULL, 1, 1, RRD_DIMENSION_INCREMENTAL);
					rrd_stats_dimension_add(st, "NoPorts", NULL, 1, 1, RRD_DIMENSION_INCREMENTAL);
				}
				else rrd_stats_next(st);

				rrd_stats_dimension_set(st, "InErrors", InErrors);
				rrd_stats_dimension_set(st, "NoPorts", NoPorts);
				rrd_stats_dimension_set(st, "RcvbufErrors", RcvbufErrors);
				rrd_stats_dimension_set(st, "SndbufErrors", SndbufErrors);
				rrd_stats_done(st);
			}
		}
	}
	
	fclose(fp);
	return 0;
}

// ----------------------------------------------------------------------------
// /proc/net/netstat processor

int do_proc_net_netstat() {
	static int do_bandwidth = -1, do_inerrors = -1, do_mcast = -1, do_bcast = -1, do_mcast_p = -1, do_bcast_p = -1;

	if(do_bandwidth == -1)	do_bandwidth = config_get_boolean("plugin:proc:/proc/net/netstat", "bandwidth", 1);
	if(do_inerrors == -1)	do_inerrors = config_get_boolean("plugin:proc:/proc/net/netstat", "input errors", 1);
	if(do_mcast == -1)		do_mcast = config_get_boolean("plugin:proc:/proc/net/netstat", "multicast bandwidth", 1);
	if(do_bcast == -1)		do_bcast = config_get_boolean("plugin:proc:/proc/net/netstat", "broadcast bandwidth", 1);
	if(do_mcast_p == -1)	do_mcast_p = config_get_boolean("plugin:proc:/proc/net/netstat", "multicast packets", 1);
	if(do_bcast_p == -1)	do_bcast_p = config_get_boolean("plugin:proc:/proc/net/netstat", "broadcast packets", 1);

	char buffer[MAX_PROC_NET_SNMP_LINE+1] = "";

	FILE *fp = fopen("/proc/net/netstat", "r");
	if(!fp) {
		error("Cannot read /proc/net/netstat.");
		return 1;
	}

	for(;1;) {
		char *p = fgets(buffer, MAX_PROC_NET_SNMP_LINE, fp);
		if(!p) break;

		if(strncmp(p, "IpExt: ", 7) == 0) {
			// skip the header line, read the data
			p = fgets(buffer, MAX_PROC_NET_SNMP_LINE, fp);
			if(!p) break;

			if(strncmp(p, "IpExt: ", 7) != 0) {
				error("Cannot read IpExt line from /proc/net/netstat.");
				break;
			}

			unsigned long long
				InNoRoutes = 0, InTruncatedPkts = 0,
				InOctets = 0,  InMcastPkts = 0,  InBcastPkts = 0,  InMcastOctets = 0,  InBcastOctets = 0,
				OutOctets = 0, OutMcastPkts = 0, OutBcastPkts = 0, OutMcastOctets = 0, OutBcastOctets = 0;
	
			int r = sscanf(&buffer[7], "%llu %llu %llu %llu %llu %llu %llu %llu %llu %llu %llu %llu\n",
				&InNoRoutes, &InTruncatedPkts, &InMcastPkts, &OutMcastPkts, &InBcastPkts, &OutBcastPkts,
				&InOctets, &OutOctets, &InMcastOctets, &OutMcastOctets, &InBcastOctets, &OutBcastOctets);

			if(r == EOF) break;
			if(r != 12) {
				error("Cannot read /proc/net/netstat IpExt line. Expected 12 params, read %d.", r);
				continue;
			}

			RRD_STATS *st;

			// --------------------------------------------------------------------

			if(do_bandwidth) {
				st = rrd_stats_find("system.ipv4");
				if(!st) {
					st = rrd_stats_create("system", "ipv4", NULL, "ipv4", "IPv4 Bandwidth", "kilobits/s", 2000, update_every, CHART_TYPE_AREA);

					rrd_stats_dimension_add(st, "received", NULL, 8, 1024, RRD_DIMENSION_INCREMENTAL);
					rrd_stats_dimension_add(st, "sent", NULL, -8, 1024, RRD_DIMENSION_INCREMENTAL);
				}
				else rrd_stats_next(st);

				rrd_stats_dimension_set(st, "sent", OutOctets);
				rrd_stats_dimension_set(st, "received", InOctets);
				rrd_stats_done(st);
			}

			// --------------------------------------------------------------------

			if(do_inerrors) {
				st = rrd_stats_find("ipv4.inerrors");
				if(!st) {
					st = rrd_stats_create("ipv4", "inerrors", NULL, "ipv4", "IPv4 Input Errors", "packets/s", 4000, update_every, CHART_TYPE_LINE);
					st->isdetail = 1;

					rrd_stats_dimension_add(st, "noroutes", NULL, 1, 1, RRD_DIMENSION_INCREMENTAL);
					rrd_stats_dimension_add(st, "trunkated", NULL, 1, 1, RRD_DIMENSION_INCREMENTAL);
				}
				else rrd_stats_next(st);

				rrd_stats_dimension_set(st, "noroutes", InNoRoutes);
				rrd_stats_dimension_set(st, "trunkated", InTruncatedPkts);
				rrd_stats_done(st);
			}

			// --------------------------------------------------------------------

			if(do_mcast) {
				st = rrd_stats_find("ipv4.mcast");
				if(!st) {
					st = rrd_stats_create("ipv4", "mcast", NULL, "ipv4", "IPv4 Multicast Bandwidth", "kilobits/s", 9000, update_every, CHART_TYPE_AREA);
					st->isdetail = 1;

					rrd_stats_dimension_add(st, "received", NULL, 8, 1024, RRD_DIMENSION_INCREMENTAL);
					rrd_stats_dimension_add(st, "sent", NULL, -8, 1024, RRD_DIMENSION_INCREMENTAL);
				}
				else rrd_stats_next(st);

				rrd_stats_dimension_set(st, "sent", OutMcastOctets);
				rrd_stats_dimension_set(st, "received", InMcastOctets);
				rrd_stats_done(st);
			}

			// --------------------------------------------------------------------

			if(do_bcast) {
				st = rrd_stats_find("ipv4.bcast");
				if(!st) {
					st = rrd_stats_create("ipv4", "bcast", NULL, "ipv4", "IPv4 Broadcast Bandwidth", "kilobits/s", 8000, update_every, CHART_TYPE_AREA);
					st->isdetail = 1;

					rrd_stats_dimension_add(st, "received", NULL, 8, 1024, RRD_DIMENSION_INCREMENTAL);
					rrd_stats_dimension_add(st, "sent", NULL, -8, 1024, RRD_DIMENSION_INCREMENTAL);
				}
				else rrd_stats_next(st);

				rrd_stats_dimension_set(st, "sent", OutBcastOctets);
				rrd_stats_dimension_set(st, "received", InBcastOctets);
				rrd_stats_done(st);
			}

			// --------------------------------------------------------------------

			if(do_mcast_p) {
				st = rrd_stats_find("ipv4.mcastpkts");
				if(!st) {
					st = rrd_stats_create("ipv4", "mcastpkts", NULL, "ipv4", "IPv4 Multicast Packets", "packets/s", 9500, update_every, CHART_TYPE_LINE);
					st->isdetail = 1;

					rrd_stats_dimension_add(st, "received", NULL, 1, 1, RRD_DIMENSION_INCREMENTAL);
					rrd_stats_dimension_add(st, "sent", NULL, -1, 1, RRD_DIMENSION_INCREMENTAL);
				}
				else rrd_stats_next(st);

				rrd_stats_dimension_set(st, "sent", OutMcastPkts);
				rrd_stats_dimension_set(st, "received", InMcastPkts);
				rrd_stats_done(st);
			}

			// --------------------------------------------------------------------

			if(do_bcast_p) {
				st = rrd_stats_find("ipv4.bcastpkts");
				if(!st) {
					st = rrd_stats_create("ipv4", "bcastpkts", NULL, "ipv4", "IPv4 Broadcast Packets", "packets/s", 8500, update_every, CHART_TYPE_LINE);
					st->isdetail = 1;

					rrd_stats_dimension_add(st, "received", NULL, 1, 1, RRD_DIMENSION_INCREMENTAL);
					rrd_stats_dimension_add(st, "sent", NULL, -1, 1, RRD_DIMENSION_INCREMENTAL);
				}
				else rrd_stats_next(st);

				rrd_stats_dimension_set(st, "sent", OutBcastPkts);
				rrd_stats_dimension_set(st, "received", InBcastPkts);
				rrd_stats_done(st);
			}
		}
	}
	
	fclose(fp);
	return 0;
}

// ----------------------------------------------------------------------------
// /proc/net/stat/nf_conntrack processor

int do_proc_net_stat_conntrack() {
	static int do_sockets = -1, do_new = -1, do_changes = -1, do_expect = -1, do_search = -1, do_errors = -1;

	if(do_sockets == -1)	do_sockets = config_get_boolean("plugin:proc:/proc/net/stat/nf_conntrack", "netfilter connections", 1);
	if(do_new == -1)		do_new = config_get_boolean("plugin:proc:/proc/net/stat/nf_conntrack", "netfilter new connections", 1);
	if(do_changes == -1)	do_changes = config_get_boolean("plugin:proc:/proc/net/stat/nf_conntrack", "netfilter connection changes", 1);
	if(do_expect == -1)		do_expect = config_get_boolean("plugin:proc:/proc/net/stat/nf_conntrack", "netfilter connection expectations", 1);
	if(do_search == -1)		do_search = config_get_boolean("plugin:proc:/proc/net/stat/nf_conntrack", "netfilter connection searches", 1);
	if(do_errors == -1)		do_errors = config_get_boolean("plugin:proc:/proc/net/stat/nf_conntrack", "netfilter errors", 1);


	char buffer[MAX_PROC_NET_STAT_CONNTRACK_LINE+1] = "";

	FILE *fp = fopen("/proc/net/stat/nf_conntrack", "r");
	if(!fp) {
		error("Cannot read /proc/net/stat/nf_conntrack.");
		return 1;
	}

	// read and discard the header
	char *p = fgets(buffer, MAX_PROC_NET_STAT_CONNTRACK_LINE, fp);

	unsigned long long aentries = 0, asearched = 0, afound = 0, anew = 0, ainvalid = 0, aignore = 0, adelete = 0, adelete_list = 0,
		ainsert = 0, ainsert_failed = 0, adrop = 0, aearly_drop = 0, aicmp_error = 0, aexpect_new = 0, aexpect_create = 0, aexpect_delete = 0, asearch_restart = 0;

	for(;1;) {
		p = fgets(buffer, MAX_PROC_NET_STAT_CONNTRACK_LINE, fp);
		if(!p) break;

		unsigned long long tentries = 0, tsearched = 0, tfound = 0, tnew = 0, tinvalid = 0, tignore = 0, tdelete = 0, tdelete_list = 0, tinsert = 0, tinsert_failed = 0, tdrop = 0, tearly_drop = 0, ticmp_error = 0, texpect_new = 0, texpect_create = 0, texpect_delete = 0, tsearch_restart = 0;

		int r = sscanf(buffer, "%llx %llx %llx %llx %llx %llx %llx %llx %llx %llx %llx %llx %llx %llx %llx %llx %llx\n",
			&tentries, &tsearched, &tfound, &tnew, &tinvalid, &tignore, &tdelete, &tdelete_list, &tinsert, &tinsert_failed, &tdrop, &tearly_drop, &ticmp_error, &texpect_new, &texpect_create, &texpect_delete, &tsearch_restart);

		if(r == EOF) break;
		if(r < 16) error("Cannot read /proc/net/stat/nf_conntrack. Expected 17 params, read %d.", r);

		if(!aentries) aentries =  tentries;

		// sum all the cpus together
		asearched 			+= tsearched;			// conntrack.search
		afound 				+= tfound;				// conntrack.search
		anew 				+= tnew;				// conntrack.new
		ainvalid 			+= tinvalid;			// conntrack.new
		aignore 			+= tignore;				// conntrack.new
		adelete 			+= tdelete;				// conntrack.changes
		adelete_list 		+= tdelete_list;		// conntrack.changes
		ainsert 			+= tinsert;				// conntrack.changes
		ainsert_failed 		+= tinsert_failed;		// conntrack.errors
		adrop 				+= tdrop;				// conntrack.errors
		aearly_drop 		+= tearly_drop;			// conntrack.errors
		aicmp_error 		+= ticmp_error;			// conntrack.errors
		aexpect_new 		+= texpect_new;			// conntrack.expect
		aexpect_create 		+= texpect_create;		// conntrack.expect
		aexpect_delete 		+= texpect_delete;		// conntrack.expect
		asearch_restart 	+= tsearch_restart;		// conntrack.search
	}
	fclose(fp);

	RRD_STATS *st;

	// --------------------------------------------------------------------
	
	if(do_sockets) {
		st = rrd_stats_find(RRD_TYPE_NET_STAT_CONNTRACK ".sockets");
		if(!st) {
			st = rrd_stats_create(RRD_TYPE_NET_STAT_CONNTRACK, "sockets", NULL, RRD_TYPE_NET_STAT_CONNTRACK, "Netfilter Connections", "active connections", 1000, update_every, CHART_TYPE_LINE);

			rrd_stats_dimension_add(st, "connections", NULL, 1, 1, RRD_DIMENSION_ABSOLUTE);
		}
		else rrd_stats_next(st);

		rrd_stats_dimension_set(st, "connections", aentries);
		rrd_stats_done(st);
	}

	// --------------------------------------------------------------------

	if(do_new) {
		st = rrd_stats_find(RRD_TYPE_NET_STAT_CONNTRACK ".new");
		if(!st) {
			st = rrd_stats_create(RRD_TYPE_NET_STAT_CONNTRACK, "new", NULL, RRD_TYPE_NET_STAT_CONNTRACK, "Netfilter New Connections", "connections/s", 1001, update_every, CHART_TYPE_LINE);

			rrd_stats_dimension_add(st, "new", NULL, 1, 1, RRD_DIMENSION_INCREMENTAL);
			rrd_stats_dimension_add(st, "ignore", NULL, -1, 1, RRD_DIMENSION_INCREMENTAL);
			rrd_stats_dimension_add(st, "invalid", NULL, -1, 1, RRD_DIMENSION_INCREMENTAL);
		}
		else rrd_stats_next(st);

		rrd_stats_dimension_set(st, "new", anew);
		rrd_stats_dimension_set(st, "ignore", aignore);
		rrd_stats_dimension_set(st, "invalid", ainvalid);
		rrd_stats_done(st);
	}

	// --------------------------------------------------------------------

	if(do_changes) {
		st = rrd_stats_find(RRD_TYPE_NET_STAT_CONNTRACK ".changes");
		if(!st) {
			st = rrd_stats_create(RRD_TYPE_NET_STAT_CONNTRACK, "changes", NULL, RRD_TYPE_NET_STAT_CONNTRACK, "Netfilter Connection Changes", "changes/s", 1002, update_every, CHART_TYPE_LINE);
			st->isdetail = 1;

			rrd_stats_dimension_add(st, "inserted", NULL, 1, 1, RRD_DIMENSION_INCREMENTAL);
			rrd_stats_dimension_add(st, "deleted", NULL, -1, 1, RRD_DIMENSION_INCREMENTAL);
			rrd_stats_dimension_add(st, "delete_list", NULL, -1, 1, RRD_DIMENSION_INCREMENTAL);
		}
		else rrd_stats_next(st);

		rrd_stats_dimension_set(st, "inserted", ainsert);
		rrd_stats_dimension_set(st, "deleted", adelete);
		rrd_stats_dimension_set(st, "delete_list", adelete_list);
		rrd_stats_done(st);
	}

	// --------------------------------------------------------------------

	if(do_expect) {
		st = rrd_stats_find(RRD_TYPE_NET_STAT_CONNTRACK ".expect");
		if(!st) {
			st = rrd_stats_create(RRD_TYPE_NET_STAT_CONNTRACK, "expect", NULL, RRD_TYPE_NET_STAT_CONNTRACK, "Netfilter Connection Expectations", "expectations/s", 1003, update_every, CHART_TYPE_LINE);
			st->isdetail = 1;

			rrd_stats_dimension_add(st, "created", NULL, 1, 1, RRD_DIMENSION_INCREMENTAL);
			rrd_stats_dimension_add(st, "deleted", NULL, -1, 1, RRD_DIMENSION_INCREMENTAL);
			rrd_stats_dimension_add(st, "new", NULL, 1, 1, RRD_DIMENSION_INCREMENTAL);
		}
		else rrd_stats_next(st);

		rrd_stats_dimension_set(st, "created", aexpect_create);
		rrd_stats_dimension_set(st, "deleted", aexpect_delete);
		rrd_stats_dimension_set(st, "new", aexpect_new);
		rrd_stats_done(st);
	}

	// --------------------------------------------------------------------

	if(do_search) {
		st = rrd_stats_find(RRD_TYPE_NET_STAT_CONNTRACK ".search");
		if(!st) {
			st = rrd_stats_create(RRD_TYPE_NET_STAT_CONNTRACK, "search", NULL, RRD_TYPE_NET_STAT_CONNTRACK, "Netfilter Connection Searches", "searches/s", 1010, update_every, CHART_TYPE_LINE);
			st->isdetail = 1;

			rrd_stats_dimension_add(st, "searched", NULL, 1, 1, RRD_DIMENSION_INCREMENTAL);
			rrd_stats_dimension_add(st, "restarted", NULL, -1, 1, RRD_DIMENSION_INCREMENTAL);
			rrd_stats_dimension_add(st, "found", NULL, 1, 1, RRD_DIMENSION_INCREMENTAL);
		}
		else rrd_stats_next(st);

		rrd_stats_dimension_set(st, "searched", asearched);
		rrd_stats_dimension_set(st, "restarted", asearch_restart);
		rrd_stats_dimension_set(st, "found", afound);
		rrd_stats_done(st);
	}

	// --------------------------------------------------------------------

	if(do_errors) {
		st = rrd_stats_find(RRD_TYPE_NET_STAT_CONNTRACK ".errors");
		if(!st) {
			st = rrd_stats_create(RRD_TYPE_NET_STAT_CONNTRACK, "errors", NULL, RRD_TYPE_NET_STAT_CONNTRACK, "Netfilter Errors", "events/s", 1005, update_every, CHART_TYPE_LINE);
			st->isdetail = 1;

			rrd_stats_dimension_add(st, "icmp_error", NULL, 1, 1, RRD_DIMENSION_INCREMENTAL);
			rrd_stats_dimension_add(st, "insert_failed", NULL, -1, 1, RRD_DIMENSION_INCREMENTAL);
			rrd_stats_dimension_add(st, "drop", NULL, -1, 1, RRD_DIMENSION_INCREMENTAL);
			rrd_stats_dimension_add(st, "early_drop", NULL, -1, 1, RRD_DIMENSION_INCREMENTAL);
		}
		else rrd_stats_next(st);

		rrd_stats_dimension_set(st, "icmp_error", aicmp_error);
		rrd_stats_dimension_set(st, "insert_failed", ainsert_failed);
		rrd_stats_dimension_set(st, "drop", adrop);
		rrd_stats_dimension_set(st, "early_drop", aearly_drop);
		rrd_stats_done(st);
	}

	return 0;
}

// ----------------------------------------------------------------------------
// /proc/net/ip_vs_stats processor

int do_proc_net_ip_vs_stats() {
	static int do_bandwidth = -1, do_sockets = -1, do_packets = -1;

	if(do_bandwidth == -1)	do_bandwidth = config_get_boolean("plugin:proc:/proc/net/ip_vs_stats", "IPVS bandwidth", 1);
	if(do_sockets == -1)	do_sockets = config_get_boolean("plugin:proc:/proc/net/ip_vs_stats", "IPVS connections", 1);
	if(do_packets == -1)	do_packets = config_get_boolean("plugin:proc:/proc/net/ip_vs_stats", "IPVS packets", 1);

	char buffer[MAX_PROC_NET_IPVS_LINE+1] = "";

	FILE *fp = fopen("/proc/net/ip_vs_stats", "r");
	if(!fp) {
		error("Cannot read /proc/net/ip_vs_stats.");
		return 1;
	}

	// read the discard the 2 header lines
	char *p = fgets(buffer, MAX_PROC_NET_IPVS_LINE, fp);
	if(!p) {
		error("Cannot read /proc/net/ip_vs_stats.");
		return 1;
	}

	p = fgets(buffer, MAX_PROC_NET_IPVS_LINE, fp);
	if(!p) {
		error("Cannot read /proc/net/ip_vs_stats.");
		return 1;
	}

	p = fgets(buffer, MAX_PROC_NET_IPVS_LINE, fp);
	if(!p) {
		error("Cannot read /proc/net/ip_vs_stats.");
		return 1;
	}

	unsigned long long entries, InPackets, OutPackets, InBytes, OutBytes;

	int r = sscanf(buffer, "%llx %llx %llx %llx %llx\n", &entries, &InPackets, &OutPackets, &InBytes, &OutBytes);

	if(r == EOF) {
		error("Cannot read /proc/net/ip_vs_stats.");
		return 1;
	}
	if(r != 5) error("Cannot read /proc/net/ip_vs_stats. Expected 5 params, read %d.", r);

	fclose(fp);

	RRD_STATS *st;

	// --------------------------------------------------------------------

	if(do_sockets) {
		st = rrd_stats_find(RRD_TYPE_NET_IPVS ".sockets");
		if(!st) {
			st = rrd_stats_create(RRD_TYPE_NET_IPVS, "sockets", NULL, RRD_TYPE_NET_IPVS, "IPVS New Connections", "connections/s", 1001, update_every, CHART_TYPE_LINE);

			rrd_stats_dimension_add(st, "connections", NULL, 1, 1, RRD_DIMENSION_INCREMENTAL);
		}
		else rrd_stats_next(st);

		rrd_stats_dimension_set(st, "connections", entries);
		rrd_stats_done(st);
	}

	// --------------------------------------------------------------------
	
	if(do_packets) {
		st = rrd_stats_find(RRD_TYPE_NET_IPVS ".packets");
		if(!st) {
			st = rrd_stats_create(RRD_TYPE_NET_IPVS, "packets", NULL, RRD_TYPE_NET_IPVS, "IPVS Packets", "packets/s", 1002, update_every, CHART_TYPE_LINE);

			rrd_stats_dimension_add(st, "received", NULL, 1, 1, RRD_DIMENSION_INCREMENTAL);
			rrd_stats_dimension_add(st, "sent", NULL, -1, 1, RRD_DIMENSION_INCREMENTAL);
		}
		else rrd_stats_next(st);

		rrd_stats_dimension_set(st, "received", InPackets);
		rrd_stats_dimension_set(st, "sent", OutPackets);
		rrd_stats_done(st);
	}

	// --------------------------------------------------------------------
	
	if(do_bandwidth) {
		st = rrd_stats_find(RRD_TYPE_NET_IPVS ".net");
		if(!st) {
			st = rrd_stats_create(RRD_TYPE_NET_IPVS, "net", NULL, RRD_TYPE_NET_IPVS, "IPVS Bandwidth", "kilobits/s", 1000, update_every, CHART_TYPE_AREA);

			rrd_stats_dimension_add(st, "received", NULL, 8, 1024, RRD_DIMENSION_INCREMENTAL);
			rrd_stats_dimension_add(st, "sent", NULL, -8, 1024, RRD_DIMENSION_INCREMENTAL);
		}
		else rrd_stats_next(st);

		rrd_stats_dimension_set(st, "received", InBytes);
		rrd_stats_dimension_set(st, "sent", OutBytes);
		rrd_stats_done(st);
	}

	return 0;
}

int do_proc_stat() {
	static int do_cpu = -1, do_cpu_cores = -1, do_interrupts = -1, do_context = -1, do_forks = -1, do_processes = -1;

	if(do_cpu == -1)		do_cpu = config_get_boolean("plugin:proc:/proc/stat", "cpu utilization", 1);
	if(do_cpu_cores == -1)	do_cpu_cores = config_get_boolean("plugin:proc:/proc/stat", "per cpu core utilization", 1);
	if(do_interrupts == -1)	do_interrupts = config_get_boolean("plugin:proc:/proc/stat", "cpu interrupts", 1);
	if(do_context == -1)	do_context = config_get_boolean("plugin:proc:/proc/stat", "context switches", 1);
	if(do_forks == -1)		do_forks = config_get_boolean("plugin:proc:/proc/stat", "processes started", 1);
	if(do_processes == -1)	do_processes = config_get_boolean("plugin:proc:/proc/stat", "processes running", 1);

	char buffer[MAX_PROC_STAT_LINE+1] = "";

	FILE *fp = fopen("/proc/stat", "r");
	if(!fp) {
		error("Cannot read /proc/stat.");
		return 1;
	}

	unsigned long long processes = 0, running = 0 , blocked = 0;
	RRD_STATS *st;

	for(;1;) {
		char *p = fgets(buffer, MAX_PROC_STAT_LINE, fp);
		if(!p) break;

		if(strncmp(p, "cpu", 3) == 0) {
			char id[MAX_PROC_STAT_NAME + 1] = "";
			unsigned long long user = 0, nice = 0, system = 0, idle = 0, iowait = 0, irq = 0, softirq = 0, steal = 0, guest = 0, guest_nice = 0;

			int r = sscanf(buffer, "%s %llu %llu %llu %llu %llu %llu %llu %llu %llu %llu\n",
				id, &user, &nice, &system, &idle, &iowait, &irq, &softirq, &steal, &guest, &guest_nice);

			if(r == EOF) break;
			if(r < 10) error("Cannot read /proc/stat cpu line. Expected 11 params, read %d.", r);

			char *title = "Core utilization";
			char *type = RRD_TYPE_STAT;
			long priority = 1000;
			int isthistotal = 0;
			if(strcmp(id, "cpu") == 0) {
				isthistotal = 1;
				title = "Total CPU utilization";
				type = "system";
				priority = 100;
			}

			if((isthistotal && do_cpu) || (!isthistotal && do_cpu_cores)) {
				st = rrd_stats_find_bytype(type, id);
				if(!st) {
					st = rrd_stats_create(type, id, NULL, "cpu", title, "percentage", priority, update_every, CHART_TYPE_STACKED);

					long multiplier = 1;
					long divisor = 1; // sysconf(_SC_CLK_TCK);

					rrd_stats_dimension_add(st, "guest_nice", NULL, multiplier, divisor, RRD_DIMENSION_PCENT_OVER_DIFF_TOTAL);
					rrd_stats_dimension_add(st, "guest", NULL, multiplier, divisor, RRD_DIMENSION_PCENT_OVER_DIFF_TOTAL);
					rrd_stats_dimension_add(st, "steal", NULL, multiplier, divisor, RRD_DIMENSION_PCENT_OVER_DIFF_TOTAL);
					rrd_stats_dimension_add(st, "softirq", NULL, multiplier, divisor, RRD_DIMENSION_PCENT_OVER_DIFF_TOTAL);
					rrd_stats_dimension_add(st, "irq", NULL, multiplier, divisor, RRD_DIMENSION_PCENT_OVER_DIFF_TOTAL);
					rrd_stats_dimension_add(st, "user", NULL, multiplier, divisor, RRD_DIMENSION_PCENT_OVER_DIFF_TOTAL);
					rrd_stats_dimension_add(st, "system", NULL, multiplier, divisor, RRD_DIMENSION_PCENT_OVER_DIFF_TOTAL);
					rrd_stats_dimension_add(st, "nice", NULL, multiplier, divisor, RRD_DIMENSION_PCENT_OVER_DIFF_TOTAL);
					rrd_stats_dimension_add(st, "iowait", NULL, multiplier, divisor, RRD_DIMENSION_PCENT_OVER_DIFF_TOTAL);

					rrd_stats_dimension_add(st, "idle", NULL, multiplier, divisor, RRD_DIMENSION_PCENT_OVER_DIFF_TOTAL);
					rrd_stats_dimension_hide(st, "idle");
				}
				else rrd_stats_next(st);

				rrd_stats_dimension_set(st, "user", user);
				rrd_stats_dimension_set(st, "nice", nice);
				rrd_stats_dimension_set(st, "system", system);
				rrd_stats_dimension_set(st, "idle", idle);
				rrd_stats_dimension_set(st, "iowait", iowait);
				rrd_stats_dimension_set(st, "irq", irq);
				rrd_stats_dimension_set(st, "softirq", softirq);
				rrd_stats_dimension_set(st, "steal", steal);
				rrd_stats_dimension_set(st, "guest", guest);
				rrd_stats_dimension_set(st, "guest_nice", guest_nice);
				rrd_stats_done(st);
			}
		}
		else if(strncmp(p, "intr ", 5) == 0) {
			char id[MAX_PROC_STAT_NAME + 1];

			unsigned long long value;

			int r = sscanf(buffer, "%s %llu ", id, &value);
			if(r == EOF) break;
			if(r != 2) error("Cannot read /proc/stat intr line. Expected 2 params, read %d.", r);

			// --------------------------------------------------------------------
	
			if(do_interrupts) {
				st = rrd_stats_find_bytype("system", id);
				if(!st) {
					st = rrd_stats_create("system", id, NULL, "cpu", "CPU Interrupts", "interrupts/s", 900, update_every, CHART_TYPE_LINE);
					st->isdetail = 1;

					rrd_stats_dimension_add(st, "interrupts", NULL, 1, 1, RRD_DIMENSION_INCREMENTAL);
				}
				else rrd_stats_next(st);

				rrd_stats_dimension_set(st, "interrupts", value);
				rrd_stats_done(st);
			}
		}
		else if(strncmp(p, "ctxt ", 5) == 0) {
			char id[MAX_PROC_STAT_NAME + 1] = "";

			unsigned long long value;

			int r = sscanf(buffer, "%s %llu ", id, &value);
			if(r == EOF) break;
			if(r != 2) error("Cannot read /proc/stat ctxt line. Expected 2 params, read %d.", r);

			// --------------------------------------------------------------------
	
			if(do_context) {
				st = rrd_stats_find_bytype("system", id);
				if(!st) {
					st = rrd_stats_create("system", id, NULL, "cpu", "CPU Context Switches", "context switches/s", 800, update_every, CHART_TYPE_LINE);

					rrd_stats_dimension_add(st, "switches", NULL, 1, 1, RRD_DIMENSION_INCREMENTAL);
				}
				else rrd_stats_next(st);

				rrd_stats_dimension_set(st, "switches", value);
				rrd_stats_done(st);
			}
		}
		else if(strncmp(p, "processes ", 10) == 0) {
			char id[MAX_PROC_STAT_NAME + 1] = "";

			unsigned long long value;

			int r = sscanf(buffer, "%s %llu ", id, &value);
			if(r == EOF) break;
			if(r != 2) error("Cannot read /proc/stat processes line. Expected 2 params, read %d.", r);

			processes = value;
		}
		else if(strncmp(p, "procs_running ", 14) == 0) {
			char id[MAX_PROC_STAT_NAME + 1] = "";

			unsigned long long value;

			int r = sscanf(buffer, "%s %llu ", id, &value);
			if(r == EOF) break;
			if(r != 2) error("Cannot read /proc/stat procs_running line. Expected 2 params, read %d.", r);

			running = value;
		}
		else if(strncmp(p, "procs_blocked ", 14) == 0) {
			char id[MAX_PROC_STAT_NAME + 1] = "procs_running";

			unsigned long long value;

			int r = sscanf(buffer, "%s %llu ", id, &value);
			if(r == EOF) break;
			if(r != 2) error("Cannot read /proc/stat procs_blocked line. Expected 2 params, read %d.", r);

			blocked = value;
		}
	}
	fclose(fp);

	// --------------------------------------------------------------------

	if(do_forks) {
		st = rrd_stats_find_bytype("system", "forks");
		if(!st) {
			st = rrd_stats_create("system", "forks", NULL, "cpu", "New Processes", "processes/s", 700, update_every, CHART_TYPE_LINE);
			st->isdetail = 1;

			rrd_stats_dimension_add(st, "started", NULL, 1, 1, RRD_DIMENSION_INCREMENTAL);
		}
		else rrd_stats_next(st);

		rrd_stats_dimension_set(st, "started", processes);
		rrd_stats_done(st);
	}

	// --------------------------------------------------------------------

	if(do_processes) {
		st = rrd_stats_find_bytype("system", "processes");
		if(!st) {
			st = rrd_stats_create("system", "processes", NULL, "cpu", "Processes", "processes", 600, update_every, CHART_TYPE_LINE);

			rrd_stats_dimension_add(st, "running", NULL, 1, 1, RRD_DIMENSION_ABSOLUTE);
			rrd_stats_dimension_add(st, "blocked", NULL, -1, 1, RRD_DIMENSION_ABSOLUTE);
		}
		else rrd_stats_next(st);

		rrd_stats_dimension_set(st, "running", running);
		rrd_stats_dimension_set(st, "blocked", blocked);
		rrd_stats_done(st);
	}

	return 0;
}

// ----------------------------------------------------------------------------
// /proc/meminfo processor

#define MAX_PROC_MEMINFO_LINE 4096
#define MAX_PROC_MEMINFO_NAME 1024

int do_proc_meminfo() {
	static int do_ram = -1, do_swap = -1, do_hwcorrupt = -1, do_committed = -1, do_writeback = -1, do_kernel = -1, do_slab = -1;

	if(do_ram == -1)		do_ram = config_get_boolean("plugin:proc:/proc/meminfo", "system ram", 1);
	if(do_swap == -1)		do_swap = config_get_boolean("plugin:proc:/proc/meminfo", "system swap", 1);
	if(do_hwcorrupt == -1)	do_hwcorrupt = config_get_boolean("plugin:proc:/proc/meminfo", "hardware corrupted ECC", 1);
	if(do_committed == -1)	do_committed = config_get_boolean("plugin:proc:/proc/meminfo", "committed memory", 1);
	if(do_writeback == -1)	do_writeback = config_get_boolean("plugin:proc:/proc/meminfo", "writeback memory", 1);
	if(do_kernel == -1)		do_kernel = config_get_boolean("plugin:proc:/proc/meminfo", "kernel memory", 1);
	if(do_slab == -1)		do_slab = config_get_boolean("plugin:proc:/proc/meminfo", "slab memory", 1);

	char buffer[MAX_PROC_MEMINFO_LINE+1] = "";

	FILE *fp = fopen("/proc/meminfo", "r");
	if(!fp) {
		error("Cannot read /proc/meminfo.");
		return 1;
	}

	int hwcorrupted = 0;

	unsigned long long MemTotal = 0, MemFree = 0, Buffers = 0, Cached = 0, SwapCached = 0,
		Active = 0, Inactive = 0, ActiveAnon = 0, InactiveAnon = 0, ActiveFile = 0, InactiveFile = 0,
		Unevictable = 0, Mlocked = 0, SwapTotal = 0, SwapFree = 0, Dirty = 0, Writeback = 0, AnonPages = 0,
		Mapped = 0, Shmem = 0, Slab = 0, SReclaimable = 0, SUnreclaim = 0, KernelStack = 0, PageTables = 0,
		NFS_Unstable = 0, Bounce = 0, WritebackTmp = 0, CommitLimit = 0, Committed_AS = 0,
		VmallocTotal = 0, VmallocUsed = 0, VmallocChunk = 0,
		AnonHugePages = 0, HugePages_Total = 0, HugePages_Free = 0, HugePages_Rsvd = 0, HugePages_Surp = 0, Hugepagesize = 0,
		DirectMap4k = 0, DirectMap2M = 0, HardwareCorrupted = 0;

	for(;1;) {
		char *p = fgets(buffer, MAX_PROC_MEMINFO_LINE, fp);
		if(!p) break;

		// remove ':'
		while((p = strchr(buffer, ':'))) *p = ' ';

		char name[MAX_PROC_MEMINFO_NAME + 1] = "";
		unsigned long long value = 0;

		int r = sscanf(buffer, "%s %llu kB\n", name, &value);

		if(r == EOF) break;
		if(r != 2) error("Cannot read /proc/meminfo line. Expected 2 params, read %d.", r);

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
	fclose(fp);

	RRD_STATS *st;

	// --------------------------------------------------------------------
	
	// http://stackoverflow.com/questions/3019748/how-to-reliably-measure-available-memory-in-linux
	unsigned long long MemUsed = MemTotal - MemFree - Cached - Buffers;

	if(do_ram) {
		st = rrd_stats_find("system.ram");
		if(!st) {
			st = rrd_stats_create("system", "ram", NULL, "mem", "System RAM", "MB", 200, update_every, CHART_TYPE_STACKED);

			rrd_stats_dimension_add(st, "buffers", NULL, 1, 1024, RRD_DIMENSION_ABSOLUTE);
			rrd_stats_dimension_add(st, "used",    NULL, 1, 1024, RRD_DIMENSION_ABSOLUTE);
			rrd_stats_dimension_add(st, "cached",  NULL, 1, 1024, RRD_DIMENSION_ABSOLUTE);
			rrd_stats_dimension_add(st, "free",    NULL, 1, 1024, RRD_DIMENSION_ABSOLUTE);
		}
		else rrd_stats_next(st);

		rrd_stats_dimension_set(st, "used", MemUsed);
		rrd_stats_dimension_set(st, "free", MemFree);
		rrd_stats_dimension_set(st, "cached", Cached);
		rrd_stats_dimension_set(st, "buffers", Buffers);
		rrd_stats_done(st);
	}

	// --------------------------------------------------------------------
	
	unsigned long long SwapUsed = SwapTotal - SwapFree;

	if(do_swap) {
		st = rrd_stats_find("system.swap");
		if(!st) {
			st = rrd_stats_create("system", "swap", NULL, "mem", "System Swap", "MB", 201, update_every, CHART_TYPE_STACKED);
			st->isdetail = 1;

			rrd_stats_dimension_add(st, "free",    NULL, 1, 1024, RRD_DIMENSION_ABSOLUTE);
			rrd_stats_dimension_add(st, "used",    NULL, 1, 1024, RRD_DIMENSION_ABSOLUTE);
		}
		else rrd_stats_next(st);

		rrd_stats_dimension_set(st, "used", SwapUsed);
		rrd_stats_dimension_set(st, "free", SwapFree);
		rrd_stats_done(st);
	}

	// --------------------------------------------------------------------
	
	if(hwcorrupted && do_hwcorrupt) {
		st = rrd_stats_find("mem.hwcorrupt");
		if(!st) {
			st = rrd_stats_create("mem", "hwcorrupt", NULL, "mem", "Hardware Corrupted ECC", "MB", 9000, update_every, CHART_TYPE_LINE);
			st->isdetail = 1;

			rrd_stats_dimension_add(st, "HardwareCorrupted", NULL, 1, 1024, RRD_DIMENSION_ABSOLUTE);
		}
		else rrd_stats_next(st);

		rrd_stats_dimension_set(st, "HardwareCorrupted", HardwareCorrupted);
		rrd_stats_done(st);
	}

	// --------------------------------------------------------------------
	
	if(do_committed) {
		st = rrd_stats_find("mem.committed");
		if(!st) {
			st = rrd_stats_create("mem", "committed", NULL, "mem", "Committed (Allocated) Memory", "MB", 5000, update_every, CHART_TYPE_AREA);
			st->isdetail = 1;

			rrd_stats_dimension_add(st, "Committed_AS", NULL, 1, 1024, RRD_DIMENSION_ABSOLUTE);
		}
		else rrd_stats_next(st);

		rrd_stats_dimension_set(st, "Committed_AS", Committed_AS);
		rrd_stats_done(st);
	}

	// --------------------------------------------------------------------
	
	if(do_writeback) {
		st = rrd_stats_find("mem.writeback");
		if(!st) {
			st = rrd_stats_create("mem", "writeback", NULL, "mem", "Writeback Memory", "MB", 4000, update_every, CHART_TYPE_LINE);
			st->isdetail = 1;

			rrd_stats_dimension_add(st, "Dirty", NULL, 1, 1024, RRD_DIMENSION_ABSOLUTE);
			rrd_stats_dimension_add(st, "Writeback", NULL, 1, 1024, RRD_DIMENSION_ABSOLUTE);
			rrd_stats_dimension_add(st, "FuseWriteback", NULL, 1, 1024, RRD_DIMENSION_ABSOLUTE);
			rrd_stats_dimension_add(st, "NfsWriteback", NULL, 1, 1024, RRD_DIMENSION_ABSOLUTE);
			rrd_stats_dimension_add(st, "Bounce", NULL, 1, 1024, RRD_DIMENSION_ABSOLUTE);
		}
		else rrd_stats_next(st);

		rrd_stats_dimension_set(st, "Dirty", Dirty);
		rrd_stats_dimension_set(st, "Writeback", Writeback);
		rrd_stats_dimension_set(st, "FuseWriteback", WritebackTmp);
		rrd_stats_dimension_set(st, "NfsWriteback", NFS_Unstable);
		rrd_stats_dimension_set(st, "Bounce", Bounce);
		rrd_stats_done(st);
	}

	// --------------------------------------------------------------------
	
	if(do_kernel) {
		st = rrd_stats_find("mem.kernel");
		if(!st) {
			st = rrd_stats_create("mem", "kernel", NULL, "mem", "Memory Used by Kernel", "MB", 6000, update_every, CHART_TYPE_STACKED);
			st->isdetail = 1;

			rrd_stats_dimension_add(st, "Slab", NULL, 1, 1024, RRD_DIMENSION_ABSOLUTE);
			rrd_stats_dimension_add(st, "KernelStack", NULL, 1, 1024, RRD_DIMENSION_ABSOLUTE);
			rrd_stats_dimension_add(st, "PageTables", NULL, 1, 1024, RRD_DIMENSION_ABSOLUTE);
			rrd_stats_dimension_add(st, "VmallocUsed", NULL, 1, 1024, RRD_DIMENSION_ABSOLUTE);
		}
		else rrd_stats_next(st);

		rrd_stats_dimension_set(st, "KernelStack", KernelStack);
		rrd_stats_dimension_set(st, "Slab", Slab);
		rrd_stats_dimension_set(st, "PageTables", PageTables);
		rrd_stats_dimension_set(st, "VmallocUsed", VmallocUsed);
		rrd_stats_done(st);
	}

	// --------------------------------------------------------------------
	
	if(do_slab) {
		st = rrd_stats_find("mem.slab");
		if(!st) {
			st = rrd_stats_create("mem", "slab", NULL, "mem", "Reclaimable Kernel Memory", "MB", 6500, update_every, CHART_TYPE_STACKED);
			st->isdetail = 1;

			rrd_stats_dimension_add(st, "reclaimable", NULL, 1, 1024, RRD_DIMENSION_ABSOLUTE);
			rrd_stats_dimension_add(st, "unreclaimable", NULL, 1, 1024, RRD_DIMENSION_ABSOLUTE);
		}
		else rrd_stats_next(st);

		rrd_stats_dimension_set(st, "reclaimable", SReclaimable);
		rrd_stats_dimension_set(st, "unreclaimable", SUnreclaim);
		rrd_stats_done(st);
	}

	return 0;
}

// ----------------------------------------------------------------------------
// /proc/vmstat processor

#define MAX_PROC_VMSTAT_LINE 4096
#define MAX_PROC_VMSTAT_NAME 1024

int do_proc_vmstat() {
	static int do_swapio = -1, do_io = -1, do_pgfaults = -1;

	if(do_swapio == -1)		do_swapio = config_get_boolean("plugin:proc:/proc/vmstat", "swap i/o", 1);
	if(do_io == -1)			do_io = config_get_boolean("plugin:proc:/proc/vmstat", "disk i/o", 1);
	if(do_pgfaults == -1)	do_pgfaults = config_get_boolean("plugin:proc:/proc/vmstat", "memory page faults", 1);

	char buffer[MAX_PROC_VMSTAT_LINE+1] = "";

	FILE *fp = fopen("/proc/vmstat", "r");
	if(!fp) {
		error("Cannot read /proc/vmstat.");
		return 1;
	}

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

	for(;1;) {
		char *p = fgets(buffer, MAX_PROC_VMSTAT_NAME, fp);
		if(!p) break;

		// remove ':'
		while((p = strchr(buffer, ':'))) *p = ' ';

		char name[MAX_PROC_MEMINFO_NAME + 1] = "";
		unsigned long long value = 0;

		int r = sscanf(buffer, "%s %llu\n", name, &value);

		if(r == EOF) break;
		if(r != 2) error("Cannot read /proc/meminfo line. Expected 2 params, read %d.", r);

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
	fclose(fp);

	RRD_STATS *st;

	// --------------------------------------------------------------------
	
	if(do_swapio) {
		st = rrd_stats_find("system.swapio");
		if(!st) {
			st = rrd_stats_create("system", "swapio", NULL, "mem", "Swap I/O", "kilobytes/s", 250, update_every, CHART_TYPE_AREA);

			rrd_stats_dimension_add(st, "in",  NULL, sysconf(_SC_PAGESIZE), 1024, RRD_DIMENSION_INCREMENTAL);
			rrd_stats_dimension_add(st, "out", NULL, -sysconf(_SC_PAGESIZE), 1024, RRD_DIMENSION_INCREMENTAL);
		}
		else rrd_stats_next(st);

		rrd_stats_dimension_set(st, "in", pswpin);
		rrd_stats_dimension_set(st, "out", pswpout);
		rrd_stats_done(st);
	}

	// --------------------------------------------------------------------
	
	if(do_io) {
		st = rrd_stats_find("system.io");
		if(!st) {
			st = rrd_stats_create("system", "io", NULL, "disk", "Disk I/O", "kilobytes/s", 150, update_every, CHART_TYPE_AREA);

			rrd_stats_dimension_add(st, "in",  NULL,  1, 1, RRD_DIMENSION_INCREMENTAL);
			rrd_stats_dimension_add(st, "out", NULL, -1, 1, RRD_DIMENSION_INCREMENTAL);
		}
		else rrd_stats_next(st);

		rrd_stats_dimension_set(st, "in", pgpgin);
		rrd_stats_dimension_set(st, "out", pgpgout);
		rrd_stats_done(st);
	}

	// --------------------------------------------------------------------
	
	if(do_pgfaults) {
		st = rrd_stats_find("system.pgfaults");
		if(!st) {
			st = rrd_stats_create("system", "pgfaults", NULL, "mem", "Memory Page Faults", "page faults/s", 500, update_every, CHART_TYPE_LINE);
			st->isdetail = 1;

			rrd_stats_dimension_add(st, "minor",  NULL,  1, 1, RRD_DIMENSION_INCREMENTAL);
			rrd_stats_dimension_add(st, "major", NULL, -1, 1, RRD_DIMENSION_INCREMENTAL);
		}
		else rrd_stats_next(st);

		rrd_stats_dimension_set(st, "minor", pgfault);
		rrd_stats_dimension_set(st, "major", pgmajfault);
		rrd_stats_done(st);
	}

	return 0;
}

// ----------------------------------------------------------------------------
// /proc processor

void *proc_main(void *ptr)
{
	if(ptr) { ; }
	struct rusage me, me_last;
	struct timeval last, now;

	gettimeofday(&last, NULL);
	last.tv_sec -= update_every;
	
	// disable (by default) various interface that are not needed
	config_get_boolean("plugin:proc:/proc/net/dev", "interface lo", 0);
	config_get_boolean("plugin:proc:/proc/net/dev", "interface fireqos_monitor", 0);

	// when ZERO, attempt to do it
	int	vdo_proc_net_dev = !config_get_boolean("plugin:proc", "/proc/net/dev", 1);
	int vdo_proc_diskstats = !config_get_boolean("plugin:proc", "/proc/diskstats", 1);
	int vdo_proc_net_snmp = !config_get_boolean("plugin:proc", "/proc/net/snmp", 1);
	int vdo_proc_net_netstat = !config_get_boolean("plugin:proc", "/proc/net/netstat", 1);
	int vdo_proc_net_stat_conntrack = !config_get_boolean("plugin:proc", "/proc/net/stat/conntrack", 1);
	int vdo_proc_net_ip_vs_stats = !config_get_boolean("plugin:proc", "/proc/net/ip_vs/stats", 1);
	int vdo_proc_stat = !config_get_boolean("plugin:proc", "/proc/stat", 1);
	int vdo_proc_meminfo = !config_get_boolean("plugin:proc", "/proc/meminfo", 1);
	int vdo_proc_vmstat = !config_get_boolean("plugin:proc", "/proc/vmstat", 1);
	int vdo_cpu_netdata = !config_get_boolean("plugin:proc", "netdata server resources", 1);

	RRD_STATS *stcpu = NULL, *stclients = NULL, *streqs = NULL, *stbytes = NULL;

	gettimeofday(&last, NULL);
	getrusage(RUSAGE_SELF, &me_last);

	unsigned long long usec = 0, susec = 0;
	for(;1;) {
		
		// BEGIN -- the job to be done
		if(!vdo_proc_net_dev)				vdo_proc_net_dev			= do_proc_net_dev(usec);
		if(!vdo_proc_diskstats)				vdo_proc_diskstats			= do_proc_diskstats(usec);
		if(!vdo_proc_net_snmp)				vdo_proc_net_snmp			= do_proc_net_snmp(usec);
		if(!vdo_proc_net_netstat)			vdo_proc_net_netstat		= do_proc_net_netstat(usec);
		if(!vdo_proc_net_stat_conntrack)	vdo_proc_net_stat_conntrack	= do_proc_net_stat_conntrack(usec);
		if(!vdo_proc_net_ip_vs_stats)		vdo_proc_net_ip_vs_stats	= do_proc_net_ip_vs_stats(usec);
		if(!vdo_proc_stat)					vdo_proc_stat 				= do_proc_stat(usec);
		if(!vdo_proc_meminfo)				vdo_proc_meminfo			= do_proc_meminfo(usec);
		if(!vdo_proc_vmstat)				vdo_proc_vmstat				= do_proc_vmstat(usec);
		// END -- the job is done
		
		// find the time to sleep in order to wait exactly update_every seconds
		gettimeofday(&now, NULL);
		usec = usecdiff(&now, &last) - susec;
		debug(D_PROCNETDEV_LOOP, "PROCNETDEV: last loop took %llu usec (worked for %llu, sleeped for %llu).", usec + susec, usec, susec);
		
		if(usec < (update_every * 1000000ULL / 2ULL)) susec = (update_every * 1000000ULL) - usec;
		else susec = update_every * 1000000ULL / 2ULL;
		
		// --------------------------------------------------------------------

		if(!vdo_cpu_netdata && getrusage(RUSAGE_SELF, &me) == 0) {
		
			unsigned long long cpuuser = me.ru_utime.tv_sec * 1000000ULL + me.ru_utime.tv_usec;
			unsigned long long cpusyst = me.ru_stime.tv_sec * 1000000ULL + me.ru_stime.tv_usec;

			if(!stcpu) stcpu = rrd_stats_find("netdata.server_cpu");
			if(!stcpu) {
				stcpu = rrd_stats_create("netdata", "server_cpu", NULL, "netdata", "NetData CPU usage", "milliseconds/s", 9999, update_every, CHART_TYPE_STACKED);

				rrd_stats_dimension_add(stcpu, "user",  NULL,  1, 1000, RRD_DIMENSION_INCREMENTAL);
				rrd_stats_dimension_add(stcpu, "system", NULL, 1, 1000, RRD_DIMENSION_INCREMENTAL);
			}
			else rrd_stats_next(stcpu);

			rrd_stats_dimension_set(stcpu, "user", cpuuser);
			rrd_stats_dimension_set(stcpu, "system", cpusyst);
			rrd_stats_done(stcpu);
			
			bcopy(&me, &me_last, sizeof(struct rusage));

			// ----------------------------------------------------------------

			if(!stclients) stclients = rrd_stats_find("netdata.clients");
			if(!stclients) {
				stclients = rrd_stats_create("netdata", "clients", NULL, "netdata", "NetData Web Clients", "connected clients", 11000, update_every, CHART_TYPE_LINE);

				rrd_stats_dimension_add(stclients, "clients",  NULL,  1, 1, RRD_DIMENSION_ABSOLUTE);
			}
			else rrd_stats_next(stclients);

			rrd_stats_dimension_set(stclients, "clients", global_statistics.connected_clients);
			rrd_stats_done(stclients);

			// ----------------------------------------------------------------

			if(!streqs) streqs = rrd_stats_find("netdata.requests");
			if(!streqs) {
				streqs = rrd_stats_create("netdata", "requests", NULL, "netdata", "NetData Web Requests", "requests/s", 12000, update_every, CHART_TYPE_LINE);

				rrd_stats_dimension_add(streqs, "requests",  NULL,  1, 1, RRD_DIMENSION_INCREMENTAL);
			}
			else rrd_stats_next(streqs);

			rrd_stats_dimension_set(streqs, "requests", global_statistics.web_requests);
			rrd_stats_done(streqs);

			// ----------------------------------------------------------------

			if(!stbytes) stbytes = rrd_stats_find("netdata.net");
			if(!stbytes) {
				stbytes = rrd_stats_create("netdata", "net", NULL, "netdata", "NetData Network Traffic", "kilobits/s", 13000, update_every, CHART_TYPE_AREA);

				rrd_stats_dimension_add(stbytes, "in",  NULL,  8, 1024, RRD_DIMENSION_INCREMENTAL);
				rrd_stats_dimension_add(stbytes, "out",  NULL,  -8, 1024, RRD_DIMENSION_INCREMENTAL);
			}
			else rrd_stats_next(stbytes);

			rrd_stats_dimension_set(stbytes, "in", global_statistics.bytes_received);
			rrd_stats_dimension_set(stbytes, "out", global_statistics.bytes_sent);
			rrd_stats_done(stbytes);
		}

		usleep(susec);
		
		// copy current to last
		bcopy(&now, &last, sizeof(struct timeval));
	}

	return NULL;
}

// ----------------------------------------------------------------------------
// /sbin/tc processor
// this requires the script plugins.d/tc-qos-helper.sh

#define TC_LINE_MAX 1024

struct tc_class {
	char id[RRD_STATS_NAME_MAX + 1];
	char name[RRD_STATS_NAME_MAX + 1];

	char leafid[RRD_STATS_NAME_MAX + 1];
	char parentid[RRD_STATS_NAME_MAX + 1];

	int hasparent;
	int isleaf;
	unsigned long long bytes;

	struct tc_class *next;
};

struct tc_device {
	char id[RRD_STATS_NAME_MAX + 1];
	char name[RRD_STATS_NAME_MAX + 1];
	char family[RRD_STATS_NAME_MAX + 1];

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
		RRD_STATS *st = rrd_stats_find_bytype(RRD_TYPE_TC, d->id);
		if(!st) {
			debug(D_TC_LOOP, "TC: Committing new TC device '%s'", d->name);

			st = rrd_stats_create(RRD_TYPE_TC, d->id, d->name, d->family, "Class Usage", "kilobits/s", 1000, update_every, CHART_TYPE_STACKED);

			for ( c = d->classes ; c ; c = c->next) {
				if(c->isleaf && c->hasparent)
					rrd_stats_dimension_add(st, c->id, c->name, 8, 1024, RRD_DIMENSION_INCREMENTAL);
			}
		}
		else {
			rrd_stats_next_plugins(st);

			if(strcmp(d->id, d->name) != 0) rrd_stats_set_name(st, d->name);
		}

		for ( c = d->classes ; c ; c = c->next) {
			if(c->isleaf && c->hasparent) {
				if(rrd_stats_dimension_set(st, c->id, c->bytes) != 0) {
					
					// new class, we have to add it
					rrd_stats_dimension_add(st, c->id, c->name, 8, 1024, RRD_DIMENSION_INCREMENTAL);
					rrd_stats_dimension_set(st, c->id, c->bytes);
				}

				// if it has a name, different to the id
				if(strcmp(c->id, c->name) != 0) {
					// update the rrd dimension with the new name
					RRD_DIMENSION *rd;
					for(rd = st->dimensions ; rd ; rd = rd->next) {
						if(strcmp(rd->id, c->id) == 0) { rrd_stats_dimension_set_name(st, rd, c->name); break; }
					}
				}
			}
		}
		rrd_stats_done(st);
	}
}

void tc_device_set_class_name(struct tc_device *d, char *id, char *name)
{
	struct tc_class *c;
	for ( c = d->classes ; c ; c = c->next) {
		if(strcmp(c->id, id) == 0) {
			strncpy(c->name, name, RRD_STATS_NAME_MAX);
			// no need for null termination - it is already null
			break;
		}
	}
}

void tc_device_set_device_name(struct tc_device *d, char *name)
{
	strncpy(d->name, name, RRD_STATS_NAME_MAX);
	// no need for null termination - it is already null
}

void tc_device_set_device_family(struct tc_device *d, char *name)
{
	strncpy(d->family, name, RRD_STATS_NAME_MAX);
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

	strncpy(d->id, name, RRD_STATS_NAME_MAX);
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

	strncpy(c->id, id, RRD_STATS_NAME_MAX);
	strcpy(c->name, c->id);
	if(parentid) strncpy(c->parentid, parentid, RRD_STATS_NAME_MAX);
	if(leafid) strncpy(c->leafid, leafid, RRD_STATS_NAME_MAX);

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
	char buffer[TC_LINE_MAX+1] = "";

	for(;1;) {
		FILE *fp;
		struct tc_device *device = NULL;
		struct tc_class *class = NULL;

		snprintf(buffer, TC_LINE_MAX, "exec %s %d", config_get("plugin:tc", "script to run to get tc values", PLUGINS_DIR "/tc-qos-helper.sh"), update_every);
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
					tc_device_commit(device);
					tc_device_free(device);
					device = NULL;
					class = NULL;
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

		sleep(update_every);
	}

	return NULL;
}

// ----------------------------------------------------------------------------
// cpu jitter calculation

#define CPU_IDLEJITTER_SLEEP_TIME_MS 20

void *cpuidlejitter_main(void *ptr)
{
	if(ptr) { ; }
	int sleep_ms = config_get_number("plugin:idlejitter", "loop time in ms", CPU_IDLEJITTER_SLEEP_TIME_MS);
	if(sleep_ms <= 0) {
		config_set_number("plugin:idlejitter", "loop time in ms", CPU_IDLEJITTER_SLEEP_TIME_MS);
		sleep_ms = CPU_IDLEJITTER_SLEEP_TIME_MS;
	}

	struct timeval before, after;

	while(1) {
		unsigned long long usec = 0, susec = 0;

		while(susec < (update_every * 1000000ULL)) {

			gettimeofday(&before, NULL);
			usleep(sleep_ms * 1000);
			gettimeofday(&after, NULL);

			// calculate the time it took for a full loop
			usec = usecdiff(&after, &before);
			susec += usec;
		}
		usec -= (sleep_ms * 1000);

		RRD_STATS *st = rrd_stats_find("system.idlejitter");
		if(!st) {
			st = rrd_stats_create("system", "idlejitter", NULL, "cpu", "CPU Idle Jitter", "microseconds lost/s", 9999, update_every, CHART_TYPE_LINE);

			rrd_stats_dimension_add(st, "jitter", NULL, 1, 1, RRD_DIMENSION_ABSOLUTE);
		}
		else rrd_stats_next_usec(st, susec);

		rrd_stats_dimension_set(st, "jitter", usec);
		rrd_stats_done(st);
	}

	return NULL;
}

// ----------------------------------------------------------------------------
// netdata checks

void *checks_main(void *ptr)
{
	if(ptr) { ; }
	unsigned long long usec = 0, susec = update_every * 1000000ULL, loop_usec = 0, total_susec = 0;
	struct timeval now, last, loop;

	RRD_STATS *check1, *check2, *check3, *apps_cpu = NULL;

	check1 = rrd_stats_create("netdata", "check1", NULL, "netdata", "Caller gives microseconds", "a million !", 99999, update_every, CHART_TYPE_LINE);
	rrd_stats_dimension_add(check1, "absolute", NULL, -1, 1, RRD_DIMENSION_ABSOLUTE);
	rrd_stats_dimension_add(check1, "incremental", NULL, 1, 1, RRD_DIMENSION_INCREMENTAL);

	check2 = rrd_stats_create("netdata", "check2", NULL, "netdata", "Netdata calcs microseconds", "a million !", 99999, update_every, CHART_TYPE_LINE);
	rrd_stats_dimension_add(check2, "absolute", NULL, -1, 1, RRD_DIMENSION_ABSOLUTE);
	rrd_stats_dimension_add(check2, "incremental", NULL, 1, 1, RRD_DIMENSION_INCREMENTAL);

	check3 = rrd_stats_create("netdata", "checkdt", NULL, "netdata", "Clock difference", "microseconds diff", 99999, update_every, CHART_TYPE_LINE);
	rrd_stats_dimension_add(check3, "caller", NULL, 1, 1, RRD_DIMENSION_ABSOLUTE);
	rrd_stats_dimension_add(check3, "netdata", NULL, 1, 1, RRD_DIMENSION_ABSOLUTE);
	rrd_stats_dimension_add(check3, "apps.plugin", NULL, 1, 1, RRD_DIMENSION_ABSOLUTE);

	gettimeofday(&last, NULL);
	while(1) {
		usleep(susec);

		// find the time to sleep in order to wait exactly update_every seconds
		gettimeofday(&now, NULL);
		loop_usec = usecdiff(&now, &last);
		usec = loop_usec - susec;
		debug(D_PROCNETDEV_LOOP, "CHECK: last loop took %llu usec (worked for %llu, sleeped for %llu).", loop_usec, usec, susec);
		
		if(usec < (update_every * 1000000ULL / 2ULL)) susec = (update_every * 1000000ULL) - usec;
		else susec = update_every * 1000000ULL / 2ULL;

		// --------------------------------------------------------------------
		// Calculate loop time

		last.tv_sec = now.tv_sec;
		last.tv_usec = now.tv_usec;
		total_susec += loop_usec;

		// --------------------------------------------------------------------
		// check chart 1

		if(check1->counter_done) rrd_stats_next_usec(check1, loop_usec);
		rrd_stats_dimension_set(check1, "absolute", 1000000);
		rrd_stats_dimension_set(check1, "incremental", total_susec);
		rrd_stats_done(check1);

		// --------------------------------------------------------------------
		// check chart 2

		if(check2->counter_done) rrd_stats_next(check2);
		rrd_stats_dimension_set(check2, "absolute", 1000000);
		rrd_stats_dimension_set(check2, "incremental", total_susec);
		rrd_stats_done(check2);

		// --------------------------------------------------------------------
		// check chart 3

		if(!apps_cpu) apps_cpu = rrd_stats_find("apps.cpu");
		if(check3->counter_done) rrd_stats_next_usec(check3, loop_usec);
		gettimeofday(&loop, NULL);
		rrd_stats_dimension_set(check3, "caller", (long long)usecdiff(&loop, &check1->last_collected));
		rrd_stats_dimension_set(check3, "netdata", (long long)usecdiff(&loop, &check2->last_collected));
		if(apps_cpu) rrd_stats_dimension_set(check3, "apps.plugin", (long long)usecdiff(&loop, &apps_cpu->last_collected));
		rrd_stats_done(check3);
	}

	return NULL;
}

// ----------------------------------------------------------------------------
// plugins.d

#define PLUGINSD_FILE_SUFFIX ".plugin"
#define PLUGINSD_FILE_SUFFIX_LEN strlen(PLUGINSD_FILE_SUFFIX)
#define PLUGINSD_CMD_MAX (FILENAME_MAX*2)
#define PLUGINSD_LINE_MAX 1024

struct plugind {
	char id[CONFIG_MAX_NAME+1];			// config node id

	char filename[FILENAME_MAX+1];		// just the filename
	char fullfilename[FILENAME_MAX+1];	// with path
	char cmd[PLUGINSD_CMD_MAX+1];		// the command that is executes

	pid_t pid;
	pthread_t thread;

	int update_every;
	int obsolete;
	int enabled;

	time_t started_t;

	struct plugind *next;
} *pluginsd_root = NULL;

// like strsep() but:
// it trims spaces before and after each value
// it accepts quoted values in single or double quotes
char *qstrsep(char **ptr)
{
	if(!*ptr || !**ptr) return NULL;
	
	char *s, *p = *ptr;

	// skip leading spaces
	while(isspace(*p)) p++;

	// if the first char is a quote, assume quoted
	if(*p == '"' || *p == '\'') {
		char q = *p;
		s = ++p;
		while(*p && *p != q) p++;

		if(*p == q) {
			*p = '\0';
			p++;
		}

		*ptr = p;
		return s;
	}

	s = p;
	while(*p && !isspace(*p)) p++;
	if(!*p) *ptr = NULL;
	else {
		*p = '\0';
		*ptr = ++p;
	}

	return s;
}

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

		RRD_STATS *st = NULL;

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
				rrd_stats_dimension_set(st, dimension, atoll(value));

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

				st = rrd_stats_find(id);
				if(!st) {
					error("PLUGINSD: '%s' is requesting a BEGIN on chart '%s', which does not exist. Disabling it.", cd->fullfilename, id);
					cd->enabled = 0;
					kill(cd->pid, SIGTERM);
					break;
				}

				if(st->counter_done) {
					unsigned long long microseconds = 0;
					if(microseconds_txt && *microseconds_txt) microseconds = strtoull(microseconds_txt, NULL, 10);
					if(microseconds) rrd_stats_next_usec(st, microseconds);
					else rrd_stats_next_plugins(st);
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

				rrd_stats_done(st);
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

				int chart_type = CHART_TYPE_LINE;
				if(chart) chart_type = chart_type_id(chart);

				if(!name || !*name) name = NULL;
				if(!family || !*family) family = id;
				if(!category || !*category) category = type;

				st = rrd_stats_find_bytype(type, id);
				if(!st) {
					debug(D_PLUGINSD, "PLUGINSD: Creating chart type='%s', id='%s', name='%s', family='%s', category='%s', chart='%s', priority=%d, update_every=%d"
						, type, id
						, name?name:""
						, family?family:""
						, category?category:""
						, chart_type_name(chart_type)
						, priority
						, update_every
						);

					st = rrd_stats_create(type, id, name, family, title, units, priority, update_every, chart_type);
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
					, algorithm_name(algorithm_id(algorithm))
					, multiplier
					, divisor
					, hidden?hidden:""
					);

				RRD_DIMENSION *rd = rrd_stats_dimension_find(st, id);
				if(!rd) {
					rd = rrd_stats_dimension_add(st, id, name, multiplier, divisor, algorithm_id(algorithm));
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
					susec = cd->update_every * 1000000ULL;
				}
				else {
					// second+ run
					usec = usecdiff(&now, &last) - susec;
					error("PLUGINSD: %s last loop took %llu usec (worked for %llu, sleeped for %llu).\n", cd->fullfilename, usec + susec, usec, susec);
					if(usec < (update_every * 1000000ULL / 2ULL)) susec = (update_every * 1000000ULL) - usec;
					else susec = update_every * 1000000ULL / 2ULL;
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
			if(strcmp(PLUGINSD_FILE_SUFFIX, &file->d_name[len - PLUGINSD_FILE_SUFFIX_LEN]) != 0) {
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
				cd->update_every = config_get_number(cd->id, "update every", update_every);
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


// ----------------------------------------------------------------------------
// main and related functions

void kill_childs()
{
	if(tc_child_pid) kill(tc_child_pid, SIGTERM);
	tc_child_pid = 0;

	struct plugind *cd;
	for(cd = pluginsd_root ; cd ; cd = cd->next)
		if(cd->pid && !cd->obsolete) {
			kill(cd->pid, SIGTERM);
			cd->pid = 0;
		}
}

void sig_handler(int signo)
{
	switch(signo) {
		case SIGTERM:
		case SIGQUIT:
		case SIGINT:
		case SIGHUP:
		case SIGFPE:
		case SIGSEGV:
			error("Signaled exit (signal %d). Errno: %d (%s)", signo, errno, strerror(errno));
			kill_childs();
			rrd_stats_free_all();
			exit(1);
			break;

		case SIGPIPE:
			error("Ignoring signal %d. Errno: %d (%s)", signo, errno, strerror(errno));
			break;


		case SIGCHLD:
			error("Received SIGCHLD (signal %d).", signo);
			siginfo_t info;
			while(waitid(P_ALL, 0, &info, WEXITED|WNOHANG) == 0) {
				if(!info.si_pid) break;
				switch(info.si_code) {
					case CLD_EXITED:
						error("pid %d exited with code %d.", info.si_pid, info.si_status);
						break;

					case CLD_KILLED:
						error("pid %d killed by signal %d.", info.si_pid, info.si_status);
						break;

					case CLD_DUMPED: 
						error("pid %d core dumped by signal %d.", info.si_pid, info.si_status);
						break;

					case CLD_STOPPED:
						error("pid %d stopped by signal %d.", info.si_pid, info.si_status);
						break;

					case CLD_TRAPPED:
						error("pid %d trapped by signal %d.", info.si_pid, info.si_status);
						break;

					case CLD_CONTINUED:
						error("pid %d continued by signal %d.", info.si_pid, info.si_status);
						break;

					default:
						error("pid %d gave us a SIGCHLD with code %d and status %d.", info.si_pid, info.si_code, info.si_status);
						break;
				}
			}
			break;

		default:
			error("Signal %d received. Falling back to default action for it.", signo);
			signal(signo, SIG_DFL);
			break;
	}
}

int become_user(const char *username)
{
	struct passwd *pw = getpwnam(username);
	if(!pw) {
		fprintf(stderr, "User %s is not present. Error: %s\n", username, strerror(errno));
		return -1;
	}
	if(setgid(pw->pw_gid) != 0) {
		fprintf(stderr, "Cannot switch to user's %s group (gid: %d). Error: %s\n", username, pw->pw_gid, strerror(errno));
		return -1;
	}
	if(setegid(pw->pw_gid) != 0) {
		fprintf(stderr, "Cannot effectively switch to user's %s group (gid: %d). Error: %s\n", username, pw->pw_gid, strerror(errno));
		return -1;
	}
	if(setuid(pw->pw_uid) != 0) {
		fprintf(stderr, "Cannot switch to user %s (uid: %d). Error: %s\n", username, pw->pw_uid, strerror(errno));
		return -1;
	}
	if(seteuid(pw->pw_uid) != 0) {
		fprintf(stderr, "Cannot effectively switch to user %s (uid: %d). Error: %s\n", username, pw->pw_uid, strerror(errno));
		return -1;
	}

	return(0);
}

int fd_is_valid(int fd)
{
    return fcntl(fd, F_GETFD) != -1 || errno != EBADF;
}

int become_daemon(int close_all_files, const char *input, const char *output, const char *error, const char *access, int *access_fd, FILE **access_fp)
{
	fflush(NULL);

	// open the files before forking
	int input_fd = -1, output_fd = -1, error_fd = -1, dev_null = -1;

	if(input && *input) {
		if((input_fd = open(input, O_RDONLY, 0666)) == -1) {
			fprintf(stderr, "Cannot open input file '%s' (%s).", input, strerror(errno));
			return -1;
		}
	}

	if(output && *output) {
		if((output_fd = open(output, O_RDWR | O_APPEND | O_CREAT, 0666)) == -1) {
			fprintf(stderr, "Cannot open output log file '%s' (%s).", output, strerror(errno));
			if(input_fd != -1) close(input_fd);
			return -1;
		}
	}

	if(error && *error) {
		if((error_fd = open(error, O_RDWR | O_APPEND | O_CREAT, 0666)) == -1) {
			fprintf(stderr, "Cannot open error log file '%s' (%s).", error, strerror(errno));
			if(input_fd != -1) close(input_fd);
			if(output_fd != -1) close(output_fd);
			return -1;
		}
	}

	if(access && *access && access_fd) {
		if((*access_fd = open(access, O_RDWR | O_APPEND | O_CREAT, 0666)) == -1) {
			fprintf(stderr, "Cannot open access log file '%s' (%s).", access, strerror(errno));
			if(input_fd != -1) close(input_fd);
			if(output_fd != -1) close(output_fd);
			if(error_fd != -1) close(error_fd);
			return -1;
		}

		if(access_fp) {
			*access_fp = fdopen(*access_fd, "w");
			if(!*access_fp) {
				fprintf(stderr, "Cannot migrate file's '%s' fd %d (%s).\n", access, *access_fd, strerror(errno));
				if(input_fd != -1) close(input_fd);
				if(output_fd != -1) close(output_fd);
				if(error_fd != -1) close(error_fd);
				close(*access_fd);
				*access_fd = -1;
				return -1;
			}
		}
	}
	
	if((dev_null = open("/dev/null", O_RDWR, 0666)) == -1) {
		perror("Cannot open /dev/null");
		if(input_fd != -1) close(input_fd);
		if(output_fd != -1) close(output_fd);
		if(error_fd != -1) close(error_fd);
		if(access && access_fd && *access_fd != -1) {
			close(*access_fd);
			*access_fd = -1;
			if(access_fp) {
				fclose(*access_fp);
				*access_fp = NULL;
			}
		}
		return -1;
	}

	// all files opened
	// lets do it

	int i = fork();
	if(i == -1) {
		perror("cannot fork");
		exit(1);
	}
	if(i != 0) {
		exit(0); // the parent
	}

	// become session leader
	if (setsid() < 0)
		exit(2);

	signal(SIGCHLD, SIG_IGN);
	signal(SIGHUP, SIG_IGN);
	signal(SIGWINCH, SIG_IGN);

	// fork() again
	i = fork();
	if(i == -1) {
		perror("cannot fork");
		exit(1);
	}
	if(i != 0) {
		exit(0); // the parent
	}

	// Set new file permissions
	umask(0);

	// close all files
	if(close_all_files) {
		for(i = sysconf(_SC_OPEN_MAX); i > 0; i--)
			if(   
				((access_fd && i != *access_fd) || !access_fd)
				&& i != dev_null
				&& i != input_fd
				&& i != output_fd
				&& i != error_fd
				&& fd_is_valid(i)
				) close(i);
	}
	else {
		close(STDIN_FILENO);
		close(STDOUT_FILENO);
		close(STDERR_FILENO);
	}

	// put the opened files
	// to our standard file descriptors
	if(input_fd != -1) {
		if(input_fd != STDIN_FILENO) {
			dup2(input_fd, STDIN_FILENO);
			close(input_fd);
		}
		input_fd = -1;
	}
	else dup2(dev_null, STDIN_FILENO);
	
	if(output_fd != -1) {
		if(output_fd != STDOUT_FILENO) {
			dup2(output_fd, STDOUT_FILENO);
			close(output_fd);
		}
		output_fd = -1;
	}
	else dup2(dev_null, STDOUT_FILENO);

	if(error_fd != -1) {
		if(error_fd != STDERR_FILENO) {
			dup2(error_fd, STDERR_FILENO);
			close(error_fd);
		}
		error_fd = -1;
	}
	else dup2(dev_null, STDERR_FILENO);

	// close /dev/null
	if(dev_null != STDIN_FILENO && dev_null != STDOUT_FILENO && dev_null != STDERR_FILENO)
		close(dev_null);

	return(0);
}

int main(int argc, char **argv)
{
	int i;
	int config_loaded = 0;


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
		else {
			fprintf(stderr, "Cannot understand option '%s'.\n", argv[i]);
			fprintf(stderr, "\nUSAGE: %s [-d] [-l LINES_TO_SAVE] [-u UPDATE_TIMER] [-p LISTEN_PORT] [-dl debug log file] [-df debug flags].\n\n", argv[0]);
			fprintf(stderr, "  -c CONFIG FILE the configuration file to load. Default: %s.\n", CONFIG_DIR "/" CONFIG_FILENAME);
			fprintf(stderr, "  -l LINES_TO_SAVE can be from 5 to %d lines in JSON data. Default: %d.\n", HISTORY_MAX, HISTORY);
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

		memory_mode = memory_mode_id(config_get("global", "memory mode", memory_mode_name(memory_mode)));

		// --------------------------------------------------------------------

		if(gethostname(buffer, HOSTNAME_MAX) == -1)
			error("WARNING: Cannot get machine hostname.");
		hostname = config_get("global", "hostname", buffer);
		debug(D_OPTIONS, "hostname set to '%s'", hostname);

		// --------------------------------------------------------------------

		save_history = config_get_number("global", "history", HISTORY);
		if(save_history < 5 || save_history > HISTORY_MAX) {
			fprintf(stderr, "Invalid save lines %d given. Defaulting to %d.\n", save_history, HISTORY);
			save_history = HISTORY;
		}
		else {
			debug(D_OPTIONS, "save lines set to %d.", save_history);
		}

		// --------------------------------------------------------------------

		char *user = config_get("global", "run as user", (getuid() == 0)?"nobody":"");
		if(*user) {
			if(become_user(user) != 0) {
				fprintf(stderr, "Cannot become user %s.\n", user);
				exit(1);
			}
			else debug(D_OPTIONS, "Successfully became user %s.", user);
		}

		// --------------------------------------------------------------------

		update_every = config_get_number("global", "update every", UPDATE_EVERY);
		if(update_every < 1 || update_every > 600) {
			fprintf(stderr, "Invalid update timer %d given. Defaulting to %d.\n", update_every, UPDATE_EVERY_MAX);
			update_every = UPDATE_EVERY;
		}
		else debug(D_OPTIONS, "update timer set to %d.", update_every);

		// --------------------------------------------------------------------

		listen_port = config_get_number("global", "port", LISTEN_PORT);
		if(listen_port < 1 || listen_port > 65535) {
			fprintf(stderr, "Invalid listen port %d given. Defaulting to %d.\n", listen_port, LISTEN_PORT);
			listen_port = LISTEN_PORT;
		}
		else debug(D_OPTIONS, "listen port set to %d.", listen_port);

		listen_fd = create_listen_socket(listen_port);
	}

	// never become a problem
	if(nice(20) == -1) {
		fprintf(stderr, "Cannot lower my CPU priority. Error: %s.\n", strerror(errno));
	}

	if(become_daemon(0, input_log_file, output_log_file, error_log_file, access_log_file, &access_fd, &stdaccess) == -1) {
		fprintf(stderr, "Cannot demonize myself (%s).", strerror(errno));
		exit(1);
	}


	if(output_log_syslog || error_log_syslog || access_log_syslog)
		openlog("netdata", LOG_PID, LOG_DAEMON);

	info("NetData started on pid %d", getpid());


	// catch all signals
	for (i = 1 ; i < 65 ;i++) if(i != SIGSEGV && i != SIGFPE) signal(i,  sig_handler);
	

	pthread_t p_proc, p_tc, p_jitter, p_pluginsd, p_checks;

	// spawn childs to collect data
	if(config_get_boolean("plugins", "tc", 1)) {
		if(pthread_create(&p_tc, NULL, tc_main, NULL))
			error("failed to create new thread for tc.");
		else if(pthread_detach(p_tc))
			error("Cannot request detach of newly created tc thread.");
	}

	if(config_get_boolean("plugins", "idlejitter", 1)) {
		if(pthread_create(&p_jitter, NULL, cpuidlejitter_main, NULL))
			error("failed to create new thread for idlejitter.");
		else if(pthread_detach(p_jitter))
			error("Cannot request detach of newly created idlejitter thread.");
	}

	if(config_get_boolean("plugins", "proc", 1)) {
		if(pthread_create(&p_proc, NULL, proc_main, NULL))
			error("failed to create new thread for proc.");
		else if(pthread_detach(p_proc))
			error("Cannot request detach of newly created proc thread.");
	}

	if(pthread_create(&p_pluginsd, NULL, pluginsd_main, NULL))
		error("failed to create new thread for plugins.d.");
	else if(pthread_detach(p_pluginsd))
		error("Cannot request detach of newly created plugins.d thread.");
	
	if(config_get_boolean("plugins", "checks", 1)) {
		if(pthread_create(&p_checks, NULL, checks_main, NULL))
			error("failed to create new thread for checks.");
		else if(pthread_detach(p_checks))
			error("Cannot request detach of newly created checks thread.");
	}

	// the main process - the web server listener
	// this never ends
	socket_listen_main(NULL);

	exit(0);
}
