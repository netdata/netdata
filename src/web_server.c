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

void *socket_listen_main_multi_threaded(void *ptr) {
	if(ptr) { ; }

	info("Multi-threaded WEB SERVER thread created with task id %d", gettid());

	struct web_client *w;
	struct timeval tv;
	int retval, failures = 0;

	if(ptr) { ; }

	if(pthread_setcanceltype(PTHREAD_CANCEL_DEFERRED, NULL) != 0)
		error("Cannot set pthread cancel type to DEFERRED.");

	if(pthread_setcancelstate(PTHREAD_CANCEL_ENABLE, NULL) != 0)
		error("Cannot set pthread cancel state to ENABLE.");

	web_client_timeout = (int) config_get_number("global", "disconnect idle web clients after seconds", DEFAULT_DISCONNECT_IDLE_WEB_CLIENTS_AFTER_SECONDS);
	web_enable_gzip = config_get_boolean("global", "enable web responses gzip compression", web_enable_gzip);

	if(listen_fd < 0) fatal("LISTENER: Listen socket is not ready.");

	fd_set ifds;
	FD_ZERO (&ifds);

	for(;;) {
		tv.tv_sec = 0;
		tv.tv_usec = 200000;

		if(likely(listen_fd >= 0))
			FD_SET(listen_fd, &ifds);

		// debug(D_WEB_CLIENT, "LISTENER: Waiting...");
		retval = select(listen_fd + 1, &ifds, NULL, NULL, &tv);

		if(unlikely(retval == -1)) {
			error("LISTENER: select() failed.");
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
			if(likely(FD_ISSET(listen_fd, &ifds))) {
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

		failures = 0;

		// cleanup unused clients
		for (w = web_clients; w; ) {
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

	error("LISTENER: exit!");

	if(listen_fd >= 0) close(listen_fd);
	exit(2);

	return NULL;
}

void *socket_listen_main_single_threaded(void *ptr) {
	if(ptr) { ; }

	info("Single threaded WEB SERVER thread created with task id %d", gettid());

	struct web_client *w;
	int retval, failures = 0;

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

	for(;;) {
		int has_obsolete = 0;
		FD_ZERO (&ifds);
		FD_ZERO (&ofds);
		FD_ZERO (&efds);

		if(listen_fd >= 0) {
			// debug(D_WEB_CLIENT_ACCESS, "LISTENER: adding listen socket %d to ifds, efds", listen_fd);
			FD_SET(listen_fd, &ifds);
			FD_SET(listen_fd, &efds);
		}

		for(w = web_clients; w ; w = w->next) {
			if(unlikely(w->dead)) {
				error("%llu: client is dead.");
				w->obsolete = 1;
			}
			else if(unlikely(!w->wait_receive && !w->wait_send)) {
				error("%llu: client is not set for neither receiving nor sending data.");
				w->obsolete = 1;
			}

			if(unlikely(w->obsolete)) {
				has_obsolete++;
				continue;
			}

			// debug(D_WEB_CLIENT_ACCESS, "%llu: adding input socket %d to efds", w->id, w->ifd);
			FD_SET(w->ifd, &efds);
			if(w->ifd > fdmax) fdmax = w->ifd;

			if(w->ifd != w->ofd) {
				// debug(D_WEB_CLIENT_ACCESS, "%llu: adding output socket %d to efds", w->id, w->ofd);
				FD_SET(w->ofd, &efds);
				if(w->ofd > fdmax) fdmax = w->ofd;
			}

			if (w->wait_receive) {
				// debug(D_WEB_CLIENT_ACCESS, "%llu: adding input socket %d to ifds", w->id, w->ifd);
				FD_SET(w->ifd, &ifds);
				if(w->ifd > fdmax) fdmax = w->ifd;
			}

			if (w->wait_send) {
				// debug(D_WEB_CLIENT_ACCESS, "%llu: adding output socket %d to ofds", w->id, w->ofd);
				FD_SET(w->ofd, &ofds);
				if(w->ofd > fdmax) fdmax = w->ofd;
			}
		}

		// cleanup unused clients
		if(unlikely(has_obsolete)) {
			for (w = web_clients; w; ) {
				if (w->obsolete) {
					debug(D_WEB_CLIENT, "%llu: Removing client.", w->id);
					w = web_client_free(w);
#ifdef NETDATA_INTERNAL_CHECKS
					log_allocations();
#endif
				}
				else w = w->next;
			}
		}

		debug(D_WEB_CLIENT_ACCESS, "LISTENER: Waiting...");
		struct timeval tv = { .tv_sec = 1, .tv_usec = 0 };
		errno = 0;
		retval = select(fdmax+1, &ifds, &ofds, &efds, &tv);

		if(retval == -1) {
			error("LISTENER: select() failed.");

			if(errno != EAGAIN) {
				// debug(D_WEB_CLIENT_ACCESS, "LISTENER: select() failed.");
				error("REMOVING ALL %lu WEB CLIENTS !", global_statistics.connected_clients);
				while (web_clients) web_client_free(web_clients);
			}

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
		else if(retval) {
			for(w = web_clients; w ; w = w->next) {
				if (unlikely(w->obsolete)) continue;

				if (unlikely(FD_ISSET(w->ifd, &efds))) {
					debug(D_WEB_CLIENT_ACCESS, "%llu: Received error on input socket.", w->id);
					web_client_reset(w);
					w->obsolete = 1;
					continue;
				}

				if (unlikely(FD_ISSET(w->ofd, &efds))) {
					debug(D_WEB_CLIENT_ACCESS, "%llu: Received error on output socket.", w->id);
					web_client_reset(w);
					w->obsolete = 1;
					continue;
				}

				if (unlikely(w->wait_receive && FD_ISSET(w->ifd, &ifds))) {
					long bytes;
					if (unlikely((bytes = web_client_receive(w)) < 0)) {
						debug(D_WEB_CLIENT, "%llu: Cannot receive data from client. Closing client.", w->id);
						errno = 0;
						web_client_reset(w);
						w->obsolete = 1;
						continue;
					}

					if (w->mode == WEB_CLIENT_MODE_NORMAL) {
						debug(D_WEB_CLIENT, "%llu: Processing received data (%ld bytes).", w->id, bytes);
						// info("%llu: Attempting to process received data (%ld bytes).", w->id, bytes);
						web_client_process(w);
					}
					else {
						debug(D_WEB_CLIENT, "%llu: NO Processing for received data (%ld bytes).", w->id, bytes);
					}
				}

				if (unlikely(w->wait_send && FD_ISSET(w->ofd, &ofds))) {
					ssize_t bytes;
					if (unlikely((bytes = web_client_send(w)) < 0)) {
						debug(D_WEB_CLIENT, "%llu: Cannot send data to client. Closing client.", w->id);
						errno = 0;
						web_client_reset(w);
						w->obsolete = 1;
						continue;
					}
				}
			}

			// check for new incoming connections
			if(FD_ISSET(listen_fd, &ifds)) {
				debug(D_WEB_CLIENT_ACCESS, "LISTENER: new connection.");
				web_client_create(listen_fd);
			}
		}
		else {
			debug(D_WEB_CLIENT_ACCESS, "LISTENER: timeout.");
		}

		failures = 0;
	}

	error("LISTENER: exit!");

	if(listen_fd >= 0) close(listen_fd);
	exit(2);

	return NULL;
}
