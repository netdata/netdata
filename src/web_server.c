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
#include <poll.h>

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
int web_server_mode = WEB_SERVER_MODE_MULTI_THREADED;

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
		// info("Listening on any IPs (IPv4).");
	}
	else {
		int ret = inet_pton(AF_INET, ip, (void *)&name.sin_addr.s_addr);
		if(ret != 1) {
			error("Failed to convert IP '%s' to a valid IPv4 address.", ip);
			close(sock);
			return -1;
		}
		// info("Listening on IP '%s' (IPv4).", ip);
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
		// info("Listening on all IPs (IPv6 and IPv4)");
	}
	else {
		int ret = inet_pton(AF_INET6, ip, (void *)&name.sin6_addr.s6_addr);
		if(ret != 1) {
			error("Failed to convert IP '%s' to a valid IPv6 address. Disabling IPv6.", ip);
			close(sock);
			return -1;
		}
		// info("Listening on IP '%s' (IPv6)", ip);
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


int create_listen_socket(void) {
	listen_backlog = (int) config_get_number("global", "http port listen backlog", LISTEN_BACKLOG);

	listen_port = (int) config_get_number("global", "port", LISTEN_PORT);
	if(listen_port < 1 || listen_port > 65535) {
		error("Invalid listen port %d given. Defaulting to %d.", listen_port, LISTEN_PORT);
		listen_port = LISTEN_PORT;
	}
	else debug(D_OPTIONS, "Listen port set to %d.", listen_port);

	int ip = 0;
	char *ipv = config_get("global", "ip version", "any");
	if(!strcmp(ipv, "any") || !strcmp(ipv, "both") || !strcmp(ipv, "all")) ip = 0;
	else if(!strcmp(ipv, "ipv4") || !strcmp(ipv, "IPV4") || !strcmp(ipv, "IPv4") || !strcmp(ipv, "4")) ip = 4;
	else if(!strcmp(ipv, "ipv6") || !strcmp(ipv, "IPV6") || !strcmp(ipv, "IPv6") || !strcmp(ipv, "6")) ip = 6;
	else error("Cannot understand ip version '%s'. Assuming 'any'.", ipv);

	if(ip == 0 || ip == 6) listen_fd = create_listen_socket6(config_get("global", "bind socket to IP", "*"), listen_port, listen_backlog);
	if(listen_fd < 0) {
		listen_fd = create_listen_socket4(config_get("global", "bind socket to IP", "*"), listen_port, listen_backlog);
		// if(listen_fd >= 0 && ip != 4) info("Managed to open an IPv4 socket on port %d.", listen_port);
	}

	return listen_fd;
}

// --------------------------------------------------------------------------------------
// the main socket listener

// 1. it accepts new incoming requests on our port
// 2. creates a new web_client for each connection received
// 3. spawns a new pthread to serve the client (this is optimal for keep-alive clients)
// 4. cleans up old web_clients that their pthreads have been exited

#define CLEANUP_EVERY_EVENTS 100

void *socket_listen_main_multi_threaded(void *ptr) {
	(void)ptr;

	web_server_mode = WEB_SERVER_MODE_MULTI_THREADED;
	info("Multi-threaded WEB SERVER thread created with task id %d", gettid());

	struct web_client *w;
	int retval, failures = 0, counter = 0;

	if(pthread_setcanceltype(PTHREAD_CANCEL_DEFERRED, NULL) != 0)
		error("Cannot set pthread cancel type to DEFERRED.");

	if(pthread_setcancelstate(PTHREAD_CANCEL_ENABLE, NULL) != 0)
		error("Cannot set pthread cancel state to ENABLE.");

	if(listen_fd < 0)
		fatal("LISTENER: Listen socket %d is not ready, or invalid.", listen_fd);

	for(;;) {
		struct pollfd fd = { .fd = listen_fd, .events = POLLIN, .revents = 0 };
		int timeout = 10 * 1000;

		// debug(D_WEB_CLIENT, "LISTENER: Waiting...");
		retval = poll(&fd, 1, timeout);

		if(unlikely(retval == -1)) {
			debug(D_WEB_CLIENT, "LISTENER: poll() failed.");
			failures++;

			if(failures > 10) {
				error("LISTENER: our listen port %d seems dead. Re-opening it.", listen_fd);
				close(listen_fd);
				listen_fd = -1;
				sleep(5);
				create_listen_socket();
				if(listen_fd < 0)
					fatal("Cannot listen for web clients (connected clients %llu).", global_statistics.connected_clients);

				failures = 0;
			}

			continue;
		}
		else if(likely(retval)) {
			// check for new incoming connections
			if(fd.revents & POLLIN || fd.revents & POLLPRI) {
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
			else {
				failures++;
				debug(D_WEB_CLIENT, "LISTENER: select() didn't do anything.");
				continue;
			}
		}
		else {
			debug(D_WEB_CLIENT, "LISTENER: select() timeout.");
			counter = CLEANUP_EVERY_EVENTS;
		}

		// cleanup unused clients
		counter++;
		if(counter > CLEANUP_EVERY_EVENTS) {
			counter = 0;
			for (w = web_clients; w;) {
				if (w->obsolete) {
					debug(D_WEB_CLIENT, "%llu: Removing client.", w->id);
					// pthread_cancel(w->thread);
					// pthread_join(w->thread, NULL);
					w = web_client_free(w);
#ifdef NETDATA_INTERNAL_CHECKS
					log_allocations();
#endif
				}
				else w = w->next;
			}
		}

		failures = 0;
	}

	debug(D_WEB_CLIENT, "LISTENER: exit!");
	close(listen_fd);
	listen_fd = -1;
	return NULL;
}

struct web_client *single_threaded_clients[FD_SETSIZE];

static inline int single_threaded_link_client(struct web_client *w, fd_set *ifds, fd_set *ofds, fd_set *efds, int *max) {
	if(unlikely(w->obsolete || w->dead || (!w->wait_receive && !w->wait_send)))
		return 1;

	if(unlikely(w->ifd < 0 || w->ifd >= FD_SETSIZE || w->ofd < 0 || w->ofd >= FD_SETSIZE)) {
		error("%llu: invalid file descriptor, ifd = %d, ofd = %d (required 0 <= fd < FD_SETSIZE (%d)", w->id, w->ifd, w->ofd, FD_SETSIZE);
		return 1;
	}

	FD_SET(w->ifd, efds);
	if(unlikely(*max < w->ifd)) *max = w->ifd;

	if(unlikely(w->ifd != w->ofd)) {
		if(*max < w->ofd) *max = w->ofd;
		FD_SET(w->ofd, efds);
	}

	if(w->wait_receive) FD_SET(w->ifd, ifds);
	if(w->wait_send)    FD_SET(w->ofd, ofds);

	single_threaded_clients[w->ifd] = w;
	single_threaded_clients[w->ofd] = w;

	return 0;
}

static inline int single_threaded_unlink_client(struct web_client *w, fd_set *ifds, fd_set *ofds, fd_set *efds) {
	FD_CLR(w->ifd, efds);
	if(unlikely(w->ifd != w->ofd)) FD_CLR(w->ofd, efds);

	if(w->wait_receive) FD_CLR(w->ifd, ifds);
	if(w->wait_send)    FD_CLR(w->ofd, ofds);

	single_threaded_clients[w->ifd] = NULL;
	single_threaded_clients[w->ofd] = NULL;

	if(unlikely(w->obsolete || w->dead || (!w->wait_receive && !w->wait_send)))
		return 1;

	return 0;
}

void *socket_listen_main_single_threaded(void *ptr) {
	(void)ptr;

	web_server_mode = WEB_SERVER_MODE_SINGLE_THREADED;

	info("Single threaded WEB SERVER thread created with task id %d", gettid());

	struct web_client *w;
	int retval, failures = 0;

	if(ptr) { ; }

	if(pthread_setcanceltype(PTHREAD_CANCEL_DEFERRED, NULL) != 0)
		error("Cannot set pthread cancel type to DEFERRED.");

	if(pthread_setcancelstate(PTHREAD_CANCEL_ENABLE, NULL) != 0)
		error("Cannot set pthread cancel state to ENABLE.");

	if(listen_fd < 0 || listen_fd >= FD_SETSIZE)
		fatal("LISTENER: Listen socket %d is not ready, or invalid.", listen_fd);

	int i;
	for(i = 0; i < FD_SETSIZE ; i++)
		single_threaded_clients[i] = NULL;

	fd_set ifds, ofds, efds, rifds, rofds, refds;
	FD_ZERO (&ifds);
	FD_ZERO (&ofds);
	FD_ZERO (&efds);
	FD_SET(listen_fd, &ifds);
	FD_SET(listen_fd, &efds);
	int fdmax = listen_fd;

	for(;;) {
		debug(D_WEB_CLIENT_ACCESS, "LISTENER: single threaded web server waiting (listen fd = %d, fdmax = %d)...", listen_fd, fdmax);

		struct timeval tv = { .tv_sec = 1, .tv_usec = 0 };
		rifds = ifds;
		rofds = ofds;
		refds = efds;
		retval = select(fdmax+1, &rifds, &rofds, &refds, &tv);

		if(unlikely(retval == -1)) {
			debug(D_WEB_CLIENT, "LISTENER: select() failed.");
			failures++;
			if(failures > 10) {
				if(global_statistics.connected_clients) {
					error("REMOVING ALL %lu WEB CLIENTS !", global_statistics.connected_clients);
					while (web_clients) {
						single_threaded_unlink_client(web_clients, &ifds, &ofds, &efds);
						web_client_free(web_clients);
					}
				}

				error("LISTENER: our listen port %d seems dead. Re-opening it.", listen_fd);

				close(listen_fd);
				listen_fd = -1;
				sleep(5);

				create_listen_socket();
				if(listen_fd < 0 || listen_fd >= FD_SETSIZE)
					fatal("Cannot listen for web clients (connected clients %llu).", global_statistics.connected_clients);

				FD_ZERO (&ifds);
				FD_ZERO (&ofds);
				FD_ZERO (&efds);
				FD_SET(listen_fd, &ifds);
				FD_SET(listen_fd, &efds);
				failures = 0;
			}
		}
		else if(likely(retval)) {
			failures = 0;
			debug(D_WEB_CLIENT_ACCESS, "LISTENER: got something.");

			if(FD_ISSET(listen_fd, &rifds)) {
				debug(D_WEB_CLIENT_ACCESS, "LISTENER: new connection.");
				w = web_client_create(listen_fd);
				if(single_threaded_link_client(w, &ifds, &ofds, &ifds, &fdmax) != 0) {
					web_client_free(w);
				}
			}

			for(i = 0 ; i <= fdmax ; i++) {
				if(likely(!FD_ISSET(i, &rifds) && !FD_ISSET(i, &rofds) && !FD_ISSET(i, &refds)))
					continue;

				w = single_threaded_clients[i];
				if(unlikely(!w))
					continue;

				if(unlikely(single_threaded_unlink_client(w, &ifds, &ofds, &efds) != 0)) {
					web_client_free(w);
					continue;
				}

				if (unlikely(FD_ISSET(w->ifd, &refds) || FD_ISSET(w->ofd, &refds))) {
					web_client_free(w);
					continue;
				}

				if (unlikely(w->wait_receive && FD_ISSET(w->ifd, &rifds))) {
					if (unlikely(web_client_receive(w) < 0)) {
						web_client_free(w);
						continue;
					}

					if (w->mode != WEB_CLIENT_MODE_FILECOPY) {
						debug(D_WEB_CLIENT, "%llu: Processing received data.", w->id);
						web_client_process(w);
					}
				}

				if (unlikely(w->wait_send && FD_ISSET(w->ofd, &rofds))) {
					if (unlikely(web_client_send(w) < 0)) {
						debug(D_WEB_CLIENT, "%llu: Cannot send data to client. Closing client.", w->id);
						web_client_free(w);
						continue;
					}
				}

				if(unlikely(single_threaded_link_client(w, &ifds, &ofds, &efds, &fdmax) != 0)) {
					web_client_free(w);
				}
			}
		}
		else {
			debug(D_WEB_CLIENT_ACCESS, "LISTENER: single threaded web server timeout.");
#ifdef NETDATA_INTERNAL_CHECKS
			log_allocations();
#endif
		}
	}

	debug(D_WEB_CLIENT, "LISTENER: exit!");
	close(listen_fd);
	listen_fd = -1;
	return NULL;
}
