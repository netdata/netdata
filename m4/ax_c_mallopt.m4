# AC_C_MALLOPT
# -------------
# Define HAVE_C_MALLOPT if mallopt() works.
AN_IDENTIFIER([mallopt], [AC_C_MALLOPT])
AC_DEFUN([AC_C_MALLOPT],
[AC_CACHE_CHECK([for mallopt], ac_cv_c_mallopt,
[AC_LINK_IFELSE(
   [AC_LANG_SOURCE(
      [[#include <malloc.h>
        int main(int argc, char **argv) {
          mallopt(M_ARENA_MAX, 1);
        }
      ]])],
   [ac_cv_c_mallopt=yes],
   [ac_cv_c_mallopt=no])])
if test $ac_cv_c_mallopt = yes; then
  AC_DEFINE([HAVE_C_MALLOPT], 1,
           [Define to 1 if glibc mallopt exists.])
fi
])# AC_C_MALLOPT
