#ifndef NETDATA_SECURITY_H
# define NETDATA_SECURITY_H

# include <openssl/ssl.h>
# include <openssl/err.h>
# if (SSLEAY_VERSION_NUMBER >= 0x0907000L) && (OPENSSL_VERSION_NUMBER < 0x10100000L)
#  include <openssl/conf.h>
# endif

#define NETDATA_SSL_HANDSHAKE_COMPLETE 0    //All the steps were successful
#define NETDATA_SSL_START 1                 //Starting handshake, conn variable is NULL
#define NETDATA_SSL_WANT_READ 2             //The connection wanna read from socket
#define NETDATA_SSL_WANT_WRITE 4            //The connection wanna write on socket
#define NETDATA_SSL_NO_HANDSHAKE 8          //Continue without encrypt connection.
struct netdata_ssl{
    SSL *conn; //SSL connection
    int flags;
};

extern SSL_CTX *netdata_cli_ctx;
extern SSL_CTX *netdata_srv_ctx;
extern const char *security_key;
extern const char *security_cert;

void security_openssl_library();
void security_clean_openssl();
void security_start_ssl(int type);
int security_process_accept(SSL *ssl,int sock);

#endif //NETDATA_SECURITY_H
