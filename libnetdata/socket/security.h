#ifndef NETDATA_SECURITY_H
# define NETDATA_SECURITY_H

# define NETDATA_SSL_HANDSHAKE_COMPLETE 0    //All the steps were successful
# define NETDATA_SSL_START 1                 //Starting handshake, conn variable is NULL
# define NETDATA_SSL_WANT_READ 2             //The connection wanna read from socket
# define NETDATA_SSL_WANT_WRITE 4            //The connection wanna write on socket
# define NETDATA_SSL_NO_HANDSHAKE 8          //Continue without encrypt connection.
# define NETDATA_SSL_OPTIONAL 16             //Flag to define the HTTP request
# define NETDATA_SSL_FORCE 32                //We only accepts HTTPS request
# define NETDATA_SSL_INVALID_CERTIFICATE 64  //Accepts invalid certificate
# define NETDATA_SSL_VALID_CERTIFICATE 128  //Accepts invalid certificate

#define NETDATA_SSL_CONTEXT_SERVER 0
#define NETDATA_SSL_CONTEXT_STREAMING 1
#define NETDATA_SSL_CONTEXT_OPENTSDB 2

# ifdef ENABLE_HTTPS

#  include <openssl/ssl.h>
#  include <openssl/err.h>
#  if (SSLEAY_VERSION_NUMBER >= 0x0907000L) && (OPENSSL_VERSION_NUMBER < 0x10100000L)
#   include <openssl/conf.h>
#  endif

struct netdata_ssl{
    SSL *conn; //SSL connection
    int flags; //The flags for SSL connection
};

extern SSL_CTX *netdata_opentsdb_ctx;
extern SSL_CTX *netdata_client_ctx;
extern SSL_CTX *netdata_srv_ctx;
extern const char *security_key;
extern const char *security_cert;
extern int netdata_validate_server;
extern int security_location_for_context(SSL_CTX *ctx,char *file,char *path);

void security_openssl_library();
void security_clean_openssl();
void security_start_ssl(int selector);
int security_process_accept(SSL *ssl,int msg);
int security_test_certificate(SSL *ssl);
SSL_CTX * security_initialize_openssl_client();

# endif //ENABLE_HTTPS
#endif //NETDATA_SECURITY_H
