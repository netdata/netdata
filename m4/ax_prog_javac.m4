# ===========================================================================
#      https://www.gnu.org/software/autoconf-archive/ax_prog_javac.html
# ===========================================================================
#
# SYNOPSIS
#
#   AX_PROG_JAVAC
#
# DESCRIPTION
#
#   AX_PROG_JAVAC tests an existing Java compiler. It uses the environment
#   variable JAVAC then tests in sequence various common Java compilers. For
#   political reasons, it starts with the free ones.
#
#   If you want to force a specific compiler:
#
#   - at the configure.in level, set JAVAC=yourcompiler before calling
#   AX_PROG_JAVAC
#
#   - at the configure level, setenv JAVAC
#
#   You can use the JAVAC variable in your Makefile.in, with @JAVAC@.
#
#   *Warning*: its success or failure can depend on a proper setting of the
#   CLASSPATH env. variable.
#
#   TODO: allow to exclude compilers (rationale: most Java programs cannot
#   compile with some compilers like guavac).
#
#   Note: This is part of the set of autoconf M4 macros for Java programs.
#   It is VERY IMPORTANT that you download the whole set, some macros depend
#   on other. Unfortunately, the autoconf archive does not support the
#   concept of set of macros, so I had to break it for submission. The
#   general documentation, as well as the sample configure.in, is included
#   in the AX_PROG_JAVA macro.
#
# LICENSE
#
#   Copyright (c) 2008 Stephane Bortzmeyer <bortzmeyer@pasteur.fr>
#
#   This program is free software; you can redistribute it and/or modify it
#   under the terms of the GNU General Public License as published by the
#   Free Software Foundation; either version 2 of the License, or (at your
#   option) any later version.
#
#   This program is distributed in the hope that it will be useful, but
#   WITHOUT ANY WARRANTY; without even the implied warranty of
#   MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE. See the GNU General
#   Public License for more details.
#
#   You should have received a copy of the GNU General Public License along
#   with this program. If not, see <https://www.gnu.org/licenses/>.
#
#   As a special exception, the respective Autoconf Macro's copyright owner
#   gives unlimited permission to copy, distribute and modify the configure
#   scripts that are the output of Autoconf when processing the Macro. You
#   need not follow the terms of the GNU General Public License when using
#   or distributing such scripts, even though portions of the text of the
#   Macro appear in them. The GNU General Public License (GPL) does govern
#   all other use of the material that constitutes the Autoconf Macro.
#
#   This special exception to the GPL applies to versions of the Autoconf
#   Macro released by the Autoconf Archive. When you make and distribute a
#   modified version of the Autoconf Macro, you may extend this special
#   exception to the GPL to apply to your modified version as well.

#serial 8

AU_ALIAS([AC_PROG_JAVAC], [AX_PROG_JAVAC])
AC_DEFUN([AX_PROG_JAVAC],[
m4_define([m4_ax_prog_javac_list],["gcj -C" guavac jikes javac])dnl
AS_IF([test "x$JAVAPREFIX" = x],
      [test "x$JAVAC" = x && AC_CHECK_PROGS([JAVAC], [m4_ax_prog_javac_list])],
      [test "x$JAVAC" = x && AC_CHECK_PROGS([JAVAC], [m4_ax_prog_javac_list], [], [$JAVAPREFIX/bin])])
m4_undefine([m4_ax_prog_javac_list])dnl
test "x$JAVAC" = x && AC_MSG_ERROR([no acceptable Java compiler found in \$PATH])
AX_PROG_JAVAC_WORKS
AC_PROVIDE([$0])dnl
])
