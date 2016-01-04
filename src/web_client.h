
#ifdef NETDATA_WITH_ZLIB
#include <zlib.h>
#endif

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
#define WEB_CLIENT_MODE_OPTIONS		2

#define URL_MAX 8192
#define ZLIB_CHUNK 	16384
#define HTTP_RESPONSE_HEADER_SIZE 4096

struct response {
	BUFFER *header;					// our response header
	BUFFER *header_output;			// internal use
	BUFFER *data;					// our response data buffer

	int code;						// the HTTP response code

	size_t rlen;					// if non-zero, the excepted size of ifd (input)
	size_t sent;					// current data length sent to output

	int zoutput;					// if set to 1, web_client_send() will send compressed data
#ifdef NETDATA_WITH_ZLIB
	z_stream zstream;				// zlib stream for sending compressed output to client
	Bytef zbuffer[ZLIB_CHUNK];		// temporary buffer for storing compressed output
	long zsent;						// the compressed bytes we have sent to the client
	long zhave;						// the compressed bytes that we have to send
	int zinitialized;
#endif

};

struct web_client {
	unsigned long long id;

	char client_ip[NI_MAXHOST+1];
	char client_port[NI_MAXSERV+1];

	char last_url[URL_MAX+1];

	struct timeval tv_in, tv_ready;

	int mode;
	int keepalive;

	struct sockaddr_storage clientaddr;

	pthread_t thread;				// the thread servicing this client
	int obsolete;					// if set to 1, the listener will remove this client

	int ifd;
	int ofd;

	struct response response;

	int wait_receive;
	int wait_send;

	struct web_client *prev;
	struct web_client *next;
};

extern struct web_client *web_clients;

extern uid_t web_files_uid(void);

extern struct web_client *web_client_create(int listener);
extern struct web_client *web_client_free(struct web_client *w);

extern void *web_client_main(void *ptr);

#endif
