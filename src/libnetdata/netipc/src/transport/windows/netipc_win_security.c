/*
 * netipc_win_security.c - Access policy shared by the Windows transports.
 */

#if defined(_WIN32) || defined(__MSYS__)

#include "netipc_win_security.h"
#include "netipc/netipc_named_pipe.h"

#include <aclapi.h>
#include <sddl.h>
#include <stdio.h>
#include <stdlib.h>

static PSID g_test_trusted_owner;

void nipc_np_set_trusted_owner_for_tests(const char *sid) {
  if (g_test_trusted_owner)
    LocalFree(g_test_trusted_owner);
  g_test_trusted_owner = NULL;
  if (sid)
    ConvertStringSidToSidA(sid, &g_test_trusted_owner);
}

/* The process token user, malloc'd. */
static TOKEN_USER *process_token_user(void) {
  HANDLE token;
  if (!OpenProcessToken(GetCurrentProcess(), TOKEN_QUERY, &token))
    return NULL;

  DWORD len = 0;
  TOKEN_USER *user = NULL;
  if (!GetTokenInformation(token, TokenUser, NULL, 0, &len) &&
      GetLastError() == ERROR_INSUFFICIENT_BUFFER) {
    user = malloc(len);
    if (user && !GetTokenInformation(token, TokenUser, user, len, &len)) {
      free(user);
      user = NULL;
    }
  }
  CloseHandle(token);
  return user;
}

bool nipc_win_security_attrs_init(SECURITY_ATTRIBUTES *sa) {
  sa->nLength = sizeof(*sa);
  sa->lpSecurityDescriptor = NULL;
  sa->bInheritHandle = FALSE;

  TOKEN_USER *user = process_token_user();
  if (!user)
    return false;

  char *sid = NULL;
  bool ok = ConvertSidToStringSidA(user->User.Sid, &sid) != 0;
  free(user);
  if (!ok)
    return false;

  /* Protected DACL: nothing is inherited, and no ACE for Everyone or
   * Anonymous, so other accounts cannot even open an instance read-only. */
  char sddl[256];
  int n = snprintf(sddl, sizeof(sddl),
                   "D:P(A;;GA;;;SY)(A;;GA;;;BA)(A;;GA;;;%s)", sid);
  LocalFree(sid);
  if (n < 0 || (size_t)n >= sizeof(sddl)) {
    SetLastError(ERROR_INSUFFICIENT_BUFFER);
    return false;
  }

  return ConvertStringSecurityDescriptorToSecurityDescriptorA(
             sddl, SDDL_REVISION_1, &sa->lpSecurityDescriptor, NULL) != 0;
}

void nipc_win_security_attrs_free(SECURITY_ATTRIBUTES *sa) {
  if (sa->lpSecurityDescriptor)
    LocalFree(sa->lpSecurityDescriptor);
  sa->lpSecurityDescriptor = NULL;
}

bool nipc_win_owner_trusted(HANDLE object) {
  PSID owner = NULL;
  PSECURITY_DESCRIPTOR sd = NULL;
  if (GetSecurityInfo(object, SE_KERNEL_OBJECT, OWNER_SECURITY_INFORMATION,
                      &owner, NULL, NULL, NULL, &sd) != ERROR_SUCCESS)
    return false;

  bool trusted;
  if (g_test_trusted_owner) {
    trusted = owner && EqualSid(owner, g_test_trusted_owner);
    LocalFree(sd);
    return trusted;
  }

  trusted = owner && (IsWellKnownSid(owner, WinLocalSystemSid) ||
                      IsWellKnownSid(owner, WinBuiltinAdministratorsSid));
  if (owner && !trusted) {
    TOKEN_USER *user = process_token_user();
    trusted = user && EqualSid(owner, user->User.Sid);
    free(user);
  }
  LocalFree(sd);
  return trusted;
}

#endif /* _WIN32 || __MSYS__ */
