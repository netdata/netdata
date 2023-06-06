#ifndef NETDATA_SECURITY_H
# define NETDATA_SECURITY_H

typedef enum __attribute__((packed)) {
    NETDATA_SSL_HANDSHAKE_COMPLETE  = (0 << 0), // All the steps were successful
    NETDATA_SSL_START               = (1 << 0), // Starting handshake, conn variable is NULL
    NETDATA_SSL_WANT_READ           = (1 << 2), // The connection wanna read from socket
    NETDATA_SSL_WANT_WRITE          = (1 << 3), // The connection wanna write on socket
    NETDATA_SSL_NO_HANDSHAKE        = (1 << 4), // Continue without encrypt connection.
    NETDATA_SSL_OPTIONAL            = (1 << 5), // Flag to define the HTTP request
    NETDATA_SSL_FORCE               = (1 << 6), // We only accept HTTPS request
    NETDATA_SSL_INVALID_CERTIFICATE = (1 << 7), // Accept invalid certificate
    NETDATA_SSL_VALID_CERTIFICATE   = (1 << 8), // Accept only valid certificate
    NETDATA_SSL_PROXY_HTTPS         = (1 << 9), // Proxy is using HTTPS
} NETDATA_SSL_HANDSHAKE;

#define NETDATA_SSL_WEB_SERVER_CTX 0
#define NETDATA_SSL_STREAMING_SENDER_CTX 1
#define NETDATA_SSL_CONTEXT_EXPORTING 2

# ifdef ENABLE_HTTPS

#define OPENSSL_VERSION_095 0x00905100L
#define OPENSSL_VERSION_097 0x0907000L
#define OPENSSL_VERSION_110 0x10100000L
#define OPENSSL_VERSION_111 0x10101000L
#define OPENSSL_VERSION_300 0x30000000L

#  include <openssl/ssl.h>
#  include <openssl/err.h>
#  include <openssl/evp.h>
#  include <openssl/pem.h>
#  if (SSLEAY_VERSION_NUMBER >= OPENSSL_VERSION_097) && (OPENSSL_VERSION_NUMBER < OPENSSL_VERSION_110)
#   include <openssl/conf.h>
#  endif

#if OPENSSL_VERSION_NUMBER >= OPENSSL_VERSION_300
#include <openssl/core_names.h>
#include <openssl/decoder.h>
#endif

struct netdata_ssl {
    SSL *conn;                   // SSL connection
    NETDATA_SSL_HANDSHAKE flags; // The flags for SSL connection
};

#define NETDATA_SSL_UNSET_CONNECTION (struct netdata_ssl){ .conn = NULL, .flags = NETDATA_SSL_START }

#define SSL_handshake_complete(ssl) ((ssl)->conn && (ssl)->flags == NETDATA_SSL_HANDSHAKE_COMPLETE)

extern SSL_CTX *netdata_ssl_exporting_ctx;
extern SSL_CTX *netdata_ssl_streaming_sender_ctx;
extern SSL_CTX *netdata_ssl_web_server_ctx;
extern const char *netdata_ssl_security_key;
extern const char *netdata_ssl_security_cert;
extern const char *tls_version;
extern const char *tls_ciphers;
extern int netdata_ssl_validate_server;
int ssl_security_location_for_context(SSL_CTX *ctx,char *file,char *path);

void security_openssl_library();
void security_clean_openssl();
void security_start_ssl(int selector);
NETDATA_SSL_HANDSHAKE security_process_accept(SSL *ssl,int msg);
int security_test_certificate(SSL *ssl);
SSL_CTX * security_initialize_openssl_client();

void security_log_ssl_error_queue(const char *call);

# endif //ENABLE_HTTPS
#endif //NETDATA_SECURITY_H
