#include <stdlib.h>
#include <sys/types.h>
#include <sys/socket.h>
#include <netinet/in.h>
#include <arpa/inet.h>

#include "global_statistics.h"
#include "log.h"

#include "web_client.h"

#define INITIAL_WEB_DATA_LENGTH 65536

struct web_client *web_clients = NULL;
unsigned long long web_clients_count = 0;

struct web_client *web_client_create(int listener)
{
	struct web_client *w;
	
	w = calloc(1, sizeof(struct web_client));
	if(!w) {
		error("Cannot allocate new web_client memory.");
		return NULL;
	}

	w->id = ++web_clients_count;
	w->mode = WEB_CLIENT_MODE_NORMAL;

	{
		struct sockaddr *sadr;
		socklen_t addrlen;

		sadr = (struct sockaddr*) &w->clientaddr;
		addrlen = sizeof(w->clientaddr);

		w->ifd = accept(listener, sadr, &addrlen);
		if (w->ifd == -1) {
			error("%llu: Cannot accept new incoming connection.", w->id);
			free(w);
			return NULL;
		}
		w->ofd = w->ifd;

		if(getnameinfo(sadr, addrlen, w->client_ip, NI_MAXHOST, w->client_port, NI_MAXSERV, NI_NUMERICHOST | NI_NUMERICSERV) != 0) {
			error("Cannot getnameinfo() on received client connection.");
			strncpy(w->client_ip,   "UNKNOWN", NI_MAXHOST);
			strncpy(w->client_port, "UNKNOWN", NI_MAXSERV);
		}
		w->client_ip[NI_MAXHOST]   = '\0';
		w->client_port[NI_MAXSERV] = '\0';

		switch(sadr->sa_family) {
		case AF_INET:
			debug(D_WEB_CLIENT_ACCESS, "%llu: New IPv4 web client from %s port %s on socket %d.", w->id, w->client_ip, w->client_port, w->ifd);
			break;
		case AF_INET6:
			if(strncmp(w->client_ip, "::ffff:", 7) == 0) {
				strcpy(w->client_ip, &w->client_ip[7]);
				debug(D_WEB_CLIENT_ACCESS, "%llu: New IPv4 web client from %s port %s on socket %d.", w->id, w->client_ip, w->client_port, w->ifd);
			}
			debug(D_WEB_CLIENT_ACCESS, "%llu: New IPv6 web client from %s port %s on socket %d.", w->id, w->client_ip, w->client_port, w->ifd);
			break;
		default:
			debug(D_WEB_CLIENT_ACCESS, "%llu: New UNKNOWN web client from %s port %s on socket %d.", w->id, w->client_ip, w->client_port, w->ifd);
			break;
		}

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

	debug(D_WEB_CLIENT_ACCESS, "%llu: Closing web client from %s port %s.", w->id, w->client_ip, w->client_port);

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
