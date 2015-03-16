#include <zlib.h>
#include <sys/time.h>
#include <string.h>
#include <sys/socket.h>
#include <netinet/in.h>
#include <arpa/inet.h>

#include "web_buffer.h"

#ifndef NETDATA_WEB_CLIENT_H
#define NETDATA_WEB_CLIENT_H 1

#define WEB_CLIENT_MODE_NORMAL		0
#define WEB_CLIENT_MODE_FILECOPY	1

#define URL_MAX 8192
#define ZLIB_CHUNK 	16384
#define MAX_HTTP_HEADER_SIZE 16384

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
};

extern struct web_client *web_clients;

extern struct web_client *web_client_create(int listener);
extern struct web_client *web_client_free(struct web_client *w);

#endif
