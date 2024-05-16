#ifndef NETDATA_SSL_H
#define NETDATA_SSL_H

// External SSL libraries used with netdata

#ifdef ENABLE_HTTPS

#define OPENSSL_VERSION_095 0x00905100L
#define OPENSSL_VERSION_097 0x0907000L
#define OPENSSL_VERSION_110 0x10100000L
#define OPENSSL_VERSION_111 0x10101000L
#define OPENSSL_VERSION_300 0x30000000L

#ifdef ENABLE_OPENSSL

#  include <openssl/ssl.h>
#  include <openssl/err.h>
#  include <openssl/sha.h>
#  include <openssl/evp.h>
#  include <openssl/pem.h>
#  if (SSLEAY_VERSION_NUMBER >= OPENSSL_VERSION_097) && (OPENSSL_VERSION_NUMBER < OPENSSL_VERSION_110)
#   include <openssl/conf.h>
#  endif

#if OPENSSL_VERSION_NUMBER >= OPENSSL_VERSION_300
#include <openssl/core_names.h>
#include <openssl/decoder.h>
#endif
#elif defined(ENABLE_WOLFSSL)
#include <wolfssl/options.h>
#include <wolfssl/version.h>
#include <wolfssl/ssl.h>
#include <wolfssl/error-ssl.h>

#include <wolfssl/openssl/ssl.h>
#include <wolfssl/openssl/err.h>
#include <wolfssl/openssl/sha.h>
#include <wolfssl/openssl/evp.h>
#endif // ENABLE_OPENSSL

#endif // ENABLE_HTTPS

#endif
