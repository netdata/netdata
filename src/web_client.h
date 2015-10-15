#include <zlib.h>
#include <sys/time.h>
#include <string.h>
#include <sys/socket.h>
#include <netinet/in.h>
#include <arpa/inet.h>
#include <netdb.h>

#include "web_buffer.h"

#define DEFAULT_DISCONNECT_IDLE_WEB_CLIENTS_AFTER_SECONDS 60
extern int web_client_timeout;
extern int web_enable_gzip;

#ifndef NETDATA_WEB_CLIENT_H
#define NETDATA_WEB_CLIENT_H 1

#define WEB_CLIENT_MODE_NORMAL		0
#define WEB_CLIENT_MODE_FILECOPY	1

#define URL_MAX 8192
#define ZLIB_CHUNK 	16384
#define MAX_HTTP_HEADER_SIZE 16384

/*
struct name_value {
	char *name;
	char *value;
	unsigned long hash;
	struct name_value *next;
};

struct web_request {
	char *protocol;
	char *hostname;
	char *path;
	char *query_string;

	struct name_value *headers;
	struct name_value *query_parameters;
	struct name_value *post_parameters;
};
*/

struct web_client {
	unsigned long long id;

	char client_ip[NI_MAXHOST+1];
	char client_port[NI_MAXSERV+1];

	char last_url[URL_MAX+1];

	struct web_request *request;

	struct timeval tv_in, tv_ready;

	int mode;
	int keepalive;

	struct sockaddr_storage clientaddr;

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
};

extern struct web_client *web_clients;

extern struct web_client *web_client_create(int listener);
extern struct web_client *web_client_free(struct web_client *w);

extern void *web_client_main(void *ptr);

#endif
