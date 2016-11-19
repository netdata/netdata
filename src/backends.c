#include "common.h"

int connect_to_socket4(const char *ip, int port) {
    int sock;

    debug(D_LISTENER, "IPv4 connecting to ip '%s' port %d", ip, port);

    sock = socket(AF_INET, SOCK_STREAM, 0);
    if(sock < 0) {
        error("IPv4 socket() on ip '%s' port %d failed.", ip, port);
        return -1;
    }

    struct sockaddr_in name;
    memset(&name, 0, sizeof(struct sockaddr_in));
    name.sin_family = AF_INET;
    name.sin_port = htons(port);

    int ret = inet_pton(AF_INET, ip, (void *)&name.sin_addr.s_addr);
    if(ret != 1) {
        error("Failed to convert '%s' to a valid IPv4 address.", ip);
        close(sock);
        return -1;
    }

    if(connect(sock, (struct sockaddr *) &name, sizeof(name)) < 0) {
        close(sock);
        error("IPv4 failed to connect to '%s', port %d", ip, port);
        return -1;
    }

    debug(D_LISTENER, "Connected to IPv4 ip '%s' port %d", ip, port);
    return sock;
}

int connect_to_socket6(const char *ip, int port) {
    int sock = -1;
    int ipv6only = 1;

    debug(D_LISTENER, "IPv6 connecting to ip '%s' port %d", ip, port);

    sock = socket(AF_INET6, SOCK_STREAM, 0);
    if (sock < 0) {
        error("IPv6 socket() on ip '%s' port %d failed.", ip, port);
        return -1;
    }

    /* IPv6 only */
    if(setsockopt(sock, IPPROTO_IPV6, IPV6_V6ONLY, (void*)&ipv6only, sizeof(ipv6only)) != 0)
        error("Cannot set IPV6_V6ONLY on ip '%s' port's %d.", ip, port);

    struct sockaddr_in6 name;
    memset(&name, 0, sizeof(struct sockaddr_in6));
    name.sin6_family = AF_INET6;
    name.sin6_port = htons ((uint16_t) port);

    int ret = inet_pton(AF_INET6, ip, (void *)&name.sin6_addr.s6_addr);
    if(ret != 1) {
        error("Failed to convert IP '%s' to a valid IPv6 address.", ip);
        close(sock);
        return -1;
    }

    name.sin6_scope_id = 0;

    if(connect(sock, (struct sockaddr *)&name, sizeof(name)) < 0) {
        close(sock);
        error("IPv6 failed to connect to '%s', port %d", ip, port);
        return -1;
    }

    debug(D_LISTENER, "Connected to IPv6 ip '%s' port %d", ip, port);
    return sock;
}


static inline int connect_to_one(const char *definition, int default_port) {
    struct addrinfo hints;
    struct addrinfo *result = NULL, *rp = NULL;

    char buffer[strlen(definition) + 1];
    strcpy(buffer, definition);

    char buffer2[10 + 1];
    snprintfz(buffer2, 10, "%d", default_port);

    char *ip = buffer, *port = buffer2;

    char *e = ip;
    if(*e == '[') {
        e = ++ip;
        while(*e && *e != ']') e++;
        if(*e == ']') {
            *e = '\0';
            e++;
        }
    }
    else {
        while(*e && *e != ':') e++;
    }

    if(*e == ':') {
        port = e + 1;
        *e = '\0';
    }

    if(!*ip)
        return -1;

    if(!*port)
        port = buffer2;

    memset(&hints, 0, sizeof(struct addrinfo));
    hints.ai_family = AF_UNSPEC;    /* Allow IPv4 or IPv6 */
    hints.ai_socktype = SOCK_DGRAM; /* Datagram socket */
    hints.ai_flags = AI_PASSIVE;    /* For wildcard IP address */
    hints.ai_protocol = 0;          /* Any protocol */
    hints.ai_canonname = NULL;
    hints.ai_addr = NULL;
    hints.ai_next = NULL;

    int r = getaddrinfo(ip, port, &hints, &result);
    if (r != 0) {
        error("Cannot resolve host '%s', port '%s': %s\n", ip, port, gai_strerror(r));
        return -1;
    }

    int fd = -1;
    for (rp = result; rp != NULL && fd == -1; rp = rp->ai_next) {
        char rip[INET_ADDRSTRLEN + INET6_ADDRSTRLEN] = "INVALID";
        int rport;

        switch (rp->ai_addr->sa_family) {
            case AF_INET: {
                struct sockaddr_in *sin = (struct sockaddr_in *) rp->ai_addr;
                inet_ntop(AF_INET, &sin->sin_addr, rip, INET_ADDRSTRLEN);
                rport = ntohs(sin->sin_port);
                fd = connect_to_socket4(rip, rport);
                break;
            }

            case AF_INET6: {
                struct sockaddr_in6 *sin6 = (struct sockaddr_in6 *) rp->ai_addr;
                inet_ntop(AF_INET6, &sin6->sin6_addr, rip, INET6_ADDRSTRLEN);
                rport = ntohs(sin6->sin6_port);
                fd = connect_to_socket6(rip, rport);
                break;
            }
        }
    }

    freeaddrinfo(result);

    return fd;
}

static inline void format_dimension_collected_graphite_plaintext(BUFFER *b, const char *prefix, RRDHOST *host, RRDSET *st, RRDDIM *rd, time_t after, time_t before) {
    time_t last = rd->last_collected_time.tv_sec;
    if(likely(last >= after && last < before))
        buffer_sprintf(b, "%s.%s.%s.%s " COLLECTED_NUMBER_FORMAT " %u\n", prefix, host->hostname, st->id, rd->id, rd->last_collected_value, (uint32_t)last);
}

static inline void format_dimension_stored_graphite_plaintext(BUFFER *b, const char *prefix, RRDHOST *host, RRDSET *st, RRDDIM *rd, time_t after, time_t before) {
    time_t last = rd->last_collected_time.tv_sec;
    if(likely(last >= after && last < before))
        buffer_sprintf(b, "%s.%s.%s.%s " CALCULATED_NUMBER_FORMAT " %u\n", prefix, host->hostname, st->id, rd->id, rd->last_stored_value, (uint32_t)last);
}

static inline void format_dimension_collected_opentsdb_telnet(BUFFER *b, const char *prefix, RRDHOST *host, RRDSET *st, RRDDIM *rd, time_t after, time_t before) {
    time_t last = rd->last_collected_time.tv_sec;
    if(likely(last >= after && last < before))
        buffer_sprintf(b, "put %s.%s.%s %u " COLLECTED_NUMBER_FORMAT " host=%s\n", prefix, st->id, rd->id, (uint32_t)last, rd->last_collected_value, host->hostname);
}

static inline void format_dimension_stored_opentsdb_telnet(BUFFER *b, const char *prefix, RRDHOST *host, RRDSET *st, RRDDIM *rd, time_t after, time_t before) {
    time_t last = rd->last_collected_time.tv_sec;
    if(likely(last >= after && last < before))
        buffer_sprintf(b, "put %s.%s.%s %u " CALCULATED_NUMBER_FORMAT " host=%s\n", prefix, st->id, rd->id, (uint32_t)last, rd->last_stored_value, host->hostname);
}

void *backends_main(void *ptr) {
    (void)ptr;
    BUFFER *b = buffer_create(1);
    void (*formatter)(BUFFER *b, const char *prefix, RRDHOST *host, RRDSET *st, RRDDIM *rd, time_t after, time_t before);

    info("BACKENDs thread created with task id %d", gettid());

    if(pthread_setcanceltype(PTHREAD_CANCEL_DEFERRED, NULL) != 0)
        error("Cannot set pthread cancel type to DEFERRED.");

    if(pthread_setcancelstate(PTHREAD_CANCEL_ENABLE, NULL) != 0)
        error("Cannot set pthread cancel state to ENABLE.");

    int default_port = 0;
    int enabled = config_get_boolean("backends", "enable", 0);
    const char *source = config_get("backends", "data source", "as stored");
    const char *type = config_get("backends", "type", "graphite");
    const char *destination = config_get("backends", "destination", "localhost");
    const char *prefix = config_get("backends", "prefix", "netdata");
    int frequency = (int)config_get_number("backends", "update every", 10);

    if(!enabled)
        goto cleanup;

    if(!strcmp(type, "graphite") || !strcmp(type, "graphite:plaintext")) {
        default_port = 2003;
        if(!strcmp(source, "as collected"))
            formatter = format_dimension_collected_graphite_plaintext;
        else
            formatter = format_dimension_stored_graphite_plaintext;
    }
    else if(!strcmp(type, "opentsdb") || !strcmp(type, "opentsdb:telnet")) {
        default_port = 2003;
        if(!strcmp(source, "as collected"))
            formatter = format_dimension_collected_opentsdb_telnet;
        else
            formatter = format_dimension_stored_opentsdb_telnet;
    }
    else {
        error("Unknown backend type '%s'", type);
        goto cleanup;
    }

    time_t after, before = (time_t)(time_usec() / 10000000ULL);

    unsigned long long step = frequency * 1000000ULL;
    for(;;) {
        unsigned long long now = time_usec();
        unsigned long long next = now - (now % step) + step + (step / 2);

        while(now < next) {
            sleep_usec(next - now);
            now = time_usec();
        }

        after = before;
        before = (time_t)(now / 10000000ULL);
        RRDSET *st;
        int pthreadoldcancelstate;
        buffer_flush(b);

        if(unlikely(pthread_setcancelstate(PTHREAD_CANCEL_DISABLE, &pthreadoldcancelstate) != 0))
            error("Cannot set pthread cancel state to DISABLE.");

        rrdhost_rdlock(&localhost);
        for(st = localhost.rrdset_root; st ;st = st->next) {
            pthread_rwlock_rdlock(&st->rwlock);
            RRDDIM *rd;
            for(rd = st->dimensions; rd ;rd = rd->next) {
                formatter(b, prefix, &localhost, st, rd, after, before);
            }
            pthread_rwlock_unlock(&st->rwlock);
        }
        rrdhost_unlock(&localhost);

        if(unlikely(pthread_setcancelstate(pthreadoldcancelstate, NULL) != 0))
            error("Cannot set pthread cancel state to RESTORE (%d).", pthreadoldcancelstate);

        if(unlikely(netdata_exit)) break;

        break;
    }

cleanup:
    info("BACKENDs thread exiting");

    pthread_exit(NULL);
    return NULL;
}
