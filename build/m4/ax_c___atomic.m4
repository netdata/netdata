# AC_C___ATOMIC
# -------------
# Define HAVE_C___ATOMIC if __atomic works.
AN_IDENTIFIER([__atomic], [AC_C___ATOMIC])
AC_DEFUN([AC_C___ATOMIC],
[AC_CACHE_CHECK([for __atomic], ac_cv_c___atomic,
[AC_LINK_IFELSE(
   [AC_LANG_SOURCE(
      [[int
        main (int argc, char **argv)
        {
          volatile unsigned long ul1 = 1, ul2 = 0, ul3 = 2;
          __atomic_load_n(&ul1, __ATOMIC_SEQ_CST);
          __atomic_compare_exchange(&ul1, &ul2, &ul3, 1, __ATOMIC_SEQ_CST, __ATOMIC_SEQ_CST);
          __atomic_fetch_add(&ul1, 1, __ATOMIC_SEQ_CST);
          __atomic_fetch_sub(&ul3, 1, __ATOMIC_SEQ_CST);
          __atomic_or_fetch(&ul1, ul2, __ATOMIC_SEQ_CST);
          __atomic_and_fetch(&ul1, ul2, __ATOMIC_SEQ_CST);
          volatile unsigned long long ull1 = 1, ull2 = 0, ull3 = 2;
          __atomic_load_n(&ull1, __ATOMIC_SEQ_CST);
          __atomic_compare_exchange(&ull1, &ull2, &ull3, 1, __ATOMIC_SEQ_CST, __ATOMIC_SEQ_CST);
          __atomic_fetch_add(&ull1, 1, __ATOMIC_SEQ_CST);
          __atomic_fetch_sub(&ull3, 1, __ATOMIC_SEQ_CST);
          __atomic_or_fetch(&ull1, ull2, __ATOMIC_SEQ_CST);
          __atomic_and_fetch(&ull1, ull2, __ATOMIC_SEQ_CST);
          return 0;
        }
      ]])],
   [ac_cv_c___atomic=yes],
   [ac_cv_c___atomic=no])])
if test $ac_cv_c___atomic = yes; then
  AC_DEFINE([HAVE_C___ATOMIC], 1,
           [Define to 1 if __atomic operations work.])
fi
])# AC_C___ATOMIC

