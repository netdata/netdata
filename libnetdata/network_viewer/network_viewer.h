#ifndef NETDATA_NETWORK_VIEWER
# define NETDATA_NETWORK_VIEWER

# ifdef __FreeBSD__

# include <sys/param.h>
# include <sys/socket.h>
# include <sys/socketvar.h>
# include <sys/sysctl.h>
# include <sys/file.h>
# include <sys/user.h>

# include <sys/un.h>
# define _WANT_UNPCB
# include <sys/unpcb.h>

# include <net/route.h>

# include <netinet/in.h>
# include <netinet/in_pcb.h>
# include <netinet/sctp.h>
# include <netinet/tcp.h>
# define TCPSTATES /* load state names */
# include <netinet/tcp_fsm.h>
# include <netinet/tcp_seq.h>
# include <netinet/tcp_var.h>
# include <arpa/inet.h>
# include <jail.h>

# else
# endif

#include <netdb.h>
#include <pwd.h>
#include <stdarg.h>
#include <stdio.h>
#include <stdlib.h>
#include <string.h>
#include <unistd.h>

#endif