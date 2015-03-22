// enable strcasestr()
#define _GNU_SOURCE

#include <stdlib.h>
#include <sys/types.h>
#include <sys/socket.h>
#include <netinet/in.h>
#include <arpa/inet.h>
#include <errno.h>
#include <pthread.h>
#include <sys/stat.h>
#include <fcntl.h>
#include <netinet/tcp.h>
#include <malloc.h>

#include "common.h"
#include "log.h"
#include "config.h"
#include "url.h"
#include "web_buffer.h"
#include "web_client.h"
#include "web_server.h"
#include "global_statistics.h"
#include "rrd.h"
#include "rrd2json.h"

int listen_fd = -1;
int listen_port = LISTEN_PORT;

static void log_allocations(void)
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
	RRDSET *st = rrdset_find_byname(tok);
	if(!st) st = rrdset_find(tok);
	if(!st) {
		// we don't have it
		// try to send a file with that name
		w->data->bytes = 0;
		return(mysendfile(w, tok));
	}

	// we have it
	debug(D_WEB_CLIENT, "%llu: Found RRD data with name '%s'.", w->id, tok);

	// how many entries does the client want?
	long lines = rrd_default_history_entries;
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
		else if(strcmp(tok, "sum") == 0) group_method = GROUP_SUM;
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
				RRDSET *st = rrdset_find_byname(tok);
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
				RRDSET *st = rrdset_find_byname(tok);
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
				RRDSET *st = rrdset_root;

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
	if(pthread_setcanceltype(PTHREAD_CANCEL_DEFERRED, NULL) != 0)
		error("Cannot set pthread cancel type to DEFERRED.");

	if(pthread_setcancelstate(PTHREAD_CANCEL_ENABLE, NULL) != 0)
		error("Cannot set pthread cancel state to ENABLE.");

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

void *socket_listen_main(void *ptr)
{
	if(ptr) { ; }
	struct web_client *w;
	struct timeval tv;
	int retval;

	if(ptr) { ; }

	if(pthread_setcanceltype(PTHREAD_CANCEL_DEFERRED, NULL) != 0)
		error("Cannot set pthread cancel type to DEFERRED.");

	if(pthread_setcancelstate(PTHREAD_CANCEL_ENABLE, NULL) != 0)
		error("Cannot set pthread cancel state to ENABLE.");

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
		//else {
		//	debug(D_WEB_CLIENT, "LISTENER: select() timeout.");
		//}

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

