/*
 * netipc_win_security.h - Access policy shared by the Windows transports.
 *
 * Every NetIPC pipe, SHM mapping and event grants access only to SYSTEM,
 * Administrators and the account of the process that creates it. Clients
 * accept a server only when the pipe owner is one of those same principals.
 */

#ifndef NETIPC_WIN_SECURITY_H
#define NETIPC_WIN_SECURITY_H

#if defined(_WIN32) || defined(__MSYS__)

#include <stdbool.h>
#include <windows.h>

/* Fill sa with the owner-only descriptor. Returns false with the Win32 error
 * set on failure. Release with nipc_win_security_attrs_free(). */
bool nipc_win_security_attrs_init(SECURITY_ATTRIBUTES *sa);
void nipc_win_security_attrs_free(SECURITY_ATTRIBUTES *sa);

/* True when the object's owner is this process's account, SYSTEM or
 * Administrators. */
bool nipc_win_owner_trusted(HANDLE object);

#endif /* _WIN32 || __MSYS__ */

#endif /* NETIPC_WIN_SECURITY_H */
