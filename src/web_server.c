#ifdef HAVE_CONFIG_H
#include <config.h>
#endif
#include <unistd.h>
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
#include "appconfig.h"
#include "url.h"
#include "web_buffer.h"
#include "web_client.h"
#include "web_server.h"
#include "global_statistics.h"
#include "rrd.h"
#include "rrd2json.h"
#include "../config.h"

int listen_backlog = LISTEN_BACKLOG;

int listen_fd = -1;
int listen_port = LISTEN_PORT;

#ifdef NETDATA_INTERNAL_CHECKS
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
#endif

static int is_ip_anything(const char *ip)
{
	if(!ip || !*ip
			|| !strcmp(ip, "any")
			|| !strcmp(ip, "all")
			|| !strcmp(ip, "*")
			|| !strcmp(ip, "::")
			|| !strcmp(ip, "0.0.0.0")
			) return 1;

	return 0;
}

int create_listen_socket4(const char *ip, int port, int listen_backlog)
{
	int sock;
	int sockopt = 1;

	debug(D_LISTENER, "IPv4 creating new listening socket on port %d", port);

	sock = socket(AF_INET, SOCK_STREAM, 0);
	if(sock < 0) {
		error("IPv4 socket() failed.");
		return -1;
	}

	/* avoid "address already in use" */
	setsockopt(sock, SOL_SOCKET, SO_REUSEADDR, (void*)&sockopt, sizeof(sockopt));

	struct sockaddr_in name;
	memset(&name, 0, sizeof(struct sockaddr_in));
	name.sin_family = AF_INET;
	name.sin_port = htons (port);

	if(is_ip_anything(ip)) {
		name.sin_addr.s_addr = htonl(INADDR_ANY);
		info("Listening on any IPs (IPv4).");
	}
	else {
		int ret = inet_pton(AF_INET, ip, (void *)&name.sin_addr.s_addr);
		if(ret != 1) {
			error("Failed to convert IP '%s' to a valid IPv4 address.", ip);
			close(sock);
			return -1;
		}
		info("Listening on IP '%s' (IPv4).", ip);
	}

	if(bind (sock, (struct sockaddr *) &name, sizeof (name)) < 0) {
		close(sock);
		error("IPv4 bind() failed.");
		return -1;
	}

	if(listen(sock, listen_backlog) < 0) {
		close(sock);
		fatal("IPv4 listen() failed.");
		return -1;
	}

	debug(D_LISTENER, "IPv4 listening port %d created", port);
	return sock;
}

int create_listen_socket6(const char *ip, int port, int listen_backlog)
{
	int sock = -1;
	int sockopt = 1;

	debug(D_LISTENER, "IPv6 creating new listening socket on port %d", port);

	sock = socket(AF_INET6, SOCK_STREAM, 0);
	if (sock < 0) {
		error("IPv6 socket() failed. Disabling IPv6.");
		return -1;
	}

	/* avoid "address already in use" */
	setsockopt(sock, SOL_SOCKET, SO_REUSEADDR, (void*)&sockopt, sizeof(sockopt));

	struct sockaddr_in6 name;
	memset(&name, 0, sizeof(struct sockaddr_in6));
	name.sin6_family = AF_INET6;
	name.sin6_port = htons ((uint16_t) port);

	if(is_ip_anything(ip)) {
		name.sin6_addr = in6addr_any;
		info("Listening on all IPs (IPv6 and IPv4)");
	}
	else {
		int ret = inet_pton(AF_INET6, ip, (void *)&name.sin6_addr.s6_addr);
		if(ret != 1) {
			error("Failed to convert IP '%s' to a valid IPv6 address. Disabling IPv6.", ip);
			close(sock);
			return -1;
		}
		info("Listening on IP '%s' (IPv6)", ip);
	}

	name.sin6_scope_id = 0;

	if (bind (sock, (struct sockaddr *) &name, sizeof (name)) < 0) {
		close(sock);
		error("IPv6 bind() failed. Disabling IPv6.");
		return -1;
	}

	if (listen(sock, listen_backlog) < 0) {
		close(sock);
		error("IPv6 listen() failed. Disabling IPv6.");
		return -1;
	}

	debug(D_LISTENER, "IPv6 listening port %d created", port);
	return sock;
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

	info("WEB SERVER thread created with task id %d", gettid());

	struct web_client *w;
	struct timeval tv;
	int retval;

	if(ptr) { ; }

	if(pthread_setcanceltype(PTHREAD_CANCEL_DEFERRED, NULL) != 0)
		error("Cannot set pthread cancel type to DEFERRED.");

	if(pthread_setcancelstate(PTHREAD_CANCEL_ENABLE, NULL) != 0)
		error("Cannot set pthread cancel state to ENABLE.");

	web_client_timeout = (int) config_get_number("global", "disconnect idle web clients after seconds", DEFAULT_DISCONNECT_IDLE_WEB_CLIENTS_AFTER_SECONDS);
	web_enable_gzip = config_get_boolean("global", "enable web responses gzip compression", web_enable_gzip);

	if(listen_fd < 0) fatal("LISTENER: Listen socket is not ready.");

	fd_set ifds, ofds, efds;
	int fdmax = listen_fd;

	FD_ZERO (&ifds);
	FD_ZERO (&ofds);
	FD_ZERO (&efds);

	for(;;) {
		tv.tv_sec = 0;
		tv.tv_usec = 200000;

		if(listen_fd >= 0) {
			FD_SET(listen_fd, &ifds);
			FD_SET(listen_fd, &efds);
		}

		// debug(D_WEB_CLIENT, "LISTENER: Waiting...");
		retval = select(fdmax+1, &ifds, &ofds, &efds, &tv);

		if(retval == -1) {
			error("LISTENER: select() failed.");
			continue;
		}
		else if(retval) {
			// check for new incoming connections
			if(FD_ISSET(listen_fd, &ifds)) {
				w = web_client_create(listen_fd);
				if(unlikely(!w)) {
					// no need for error log - web_client_create already logged the error
					continue;
				}

				if(pthread_create(&w->thread, NULL, web_client_main, w) != 0) {
					error("%llu: failed to create new thread for web client.");
					w->obsolete = 1;
				}
				else if(pthread_detach(w->thread) != 0) {
					error("%llu: Cannot request detach of newly created web client thread.", w->id);
					w->obsolete = 1;
				}
			}
			else debug(D_WEB_CLIENT, "LISTENER: select() didn't do anything.");

		}
		//else {
		//	debug(D_WEB_CLIENT, "LISTENER: select() timeout.");
		//}

		// cleanup unused clients
		for(w = web_clients; w ; w = w?w->next:NULL) {
			if(w->obsolete) {
				debug(D_WEB_CLIENT, "%llu: Removing client.", w->id);
				// pthread_join(w->thread,  NULL);
				w = web_client_free(w);
#ifdef NETDATA_INTERNAL_CHECKS
				log_allocations();
#endif
			}
		}
	}

	error("LISTENER: exit!");

	if(listen_fd >= 0) close(listen_fd);
	exit(2);

	return NULL;
}

