dnl -------------------------------------------------------- -*- autoconf -*-
dnl Licensed to the Apache Software Foundation (ASF) under one or more
dnl contributor license agreements.  See the NOTICE file distributed with
dnl this work for additional information regarding copyright ownership.
dnl The ASF licenses this file to You under the Apache License, Version 2.0
dnl (the "License"); you may not use this file except in compliance with
dnl the License.  You may obtain a copy of the License at
dnl
dnl     http://www.apache.org/licenses/LICENSE-2.0
dnl
dnl Unless required by applicable law or agreed to in writing, software
dnl distributed under the License is distributed on an "AS IS" BASIS,
dnl WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
dnl See the License for the specific language governing permissions and
dnl limitations under the License.

dnl
dnl tcmalloc.m4: Trafficserver's tcmalloc autoconf macros
dnl modified to skip other TS_ helpers
dnl

dnl This is kinda fugly, but need a way to both specify a directory and which
dnl of the many tcmalloc libraries to use ...
AC_DEFUN([TS_CHECK_TCMALLOC], [
AC_ARG_WITH([tcmalloc-lib],
  [AS_HELP_STRING([--with-tcmalloc-lib],[specify the tcmalloc library to use [default=tcmalloc]])],
  [
  with_tcmalloc_lib="$withval"
  ],[
  with_tcmalloc_lib="tcmalloc"
  ]
)

has_tcmalloc=0
AC_ARG_WITH([tcmalloc], [AS_HELP_STRING([--with-tcmalloc=DIR], [use the tcmalloc library])],
[
  if test "$withval" != "no"; then
    if test "x${enable_jemalloc}" = "xyes"; then
      AC_MSG_ERROR([Cannot compile with both tcmalloc and jemalloc])
    fi
    tcmalloc_have_lib=0
    if test "x$withval" != "xyes" && test "x$withval" != "x"; then
      tcmalloc_ldflags="$withval/lib"
      LDFLAGS="${LDFLAGS} -L${tcmalloc_ldflags}"
      LIBTOOL_LINK_FLAGS="${LIBTOOL_LINK_FLAGS} -rpath ${tcmalloc_ldflags}"
    fi
    AC_CHECK_LIB(${with_tcmalloc_lib}, tc_cfree, [tcmalloc_have_lib=1])
    if test "$tcmalloc_have_lib" != "0"; then
      LIBS="${LIBS} -l${with_tcmalloc_lib}"
      has_tcmalloc=1      
      AC_DEFINE(has_tcmalloc, [1], [Link/compile against tcmalloc])
    else
      AC_MSG_ERROR([Couldn't find a tcmalloc installation])
    fi
  fi
])
AC_SUBST(has_tcmalloc)
])
