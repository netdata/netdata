#!/bin/sh
# SPDX-License-Identifier: GPL-3.0+
#
# Makeself version 2.3.x
#  by Stephane Peter <megastep@megastep.org>
#
# Utility to create self-extracting tar.gz archives.
# The resulting archive is a file holding the tar.gz archive with
# a small Shell script stub that uncompresses the archive to a temporary
# directory and then executes a given script from withing that directory.
#
# Makeself home page: http://makeself.io/
#
# Version 2.0 is a rewrite of version 1.0 to make the code easier to read and maintain.
#
# Version history :
# - 1.0 : Initial public release
# - 1.1 : The archive can be passed parameters that will be passed on to
#         the embedded script, thanks to John C. Quillan
# - 1.2 : Package distribution, bzip2 compression, more command line options,
#         support for non-temporary archives. Ideas thanks to Francois Petitjean
# - 1.3 : More patches from Bjarni R. Einarsson and Francois Petitjean:
#         Support for no compression (--nocomp), script is no longer mandatory,
#         automatic launch in an xterm, optional verbose output, and -target 
#         archive option to indicate where to extract the files.
# - 1.4 : Improved UNIX compatibility (Francois Petitjean)
#         Automatic integrity checking, support of LSM files (Francois Petitjean)
# - 1.5 : Many bugfixes. Optionally disable xterm spawning.
# - 1.5.1 : More bugfixes, added archive options -list and -check.
# - 1.5.2 : Cosmetic changes to inform the user of what's going on with big 
#           archives (Quake III demo)
# - 1.5.3 : Check for validity of the DISPLAY variable before launching an xterm.
#           More verbosity in xterms and check for embedded command's return value.
#           Bugfix for Debian 2.0 systems that have a different "print" command.
# - 1.5.4 : Many bugfixes. Print out a message if the extraction failed.
# - 1.5.5 : More bugfixes. Added support for SETUP_NOCHECK environment variable to
#           bypass checksum verification of archives.
# - 1.6.0 : Compute MD5 checksums with the md5sum command (patch from Ryan Gordon)
# - 2.0   : Brand new rewrite, cleaner architecture, separated header and UNIX ports.
# - 2.0.1 : Added --copy
# - 2.1.0 : Allow multiple tarballs to be stored in one archive, and incremental updates.
#           Added --nochown for archives
#           Stopped doing redundant checksums when not necesary
# - 2.1.1 : Work around insane behavior from certain Linux distros with no 'uncompress' command
#           Cleaned up the code to handle error codes from compress. Simplified the extraction code.
# - 2.1.2 : Some bug fixes. Use head -n to avoid problems.
# - 2.1.3 : Bug fixes with command line when spawning terminals.
#           Added --tar for archives, allowing to give arbitrary arguments to tar on the contents of the archive.
#           Added --noexec to prevent execution of embedded scripts.
#           Added --nomd5 and --nocrc to avoid creating checksums in archives.
#           Added command used to create the archive in --info output.
#           Run the embedded script through eval.
# - 2.1.4 : Fixed --info output.
#           Generate random directory name when extracting files to . to avoid problems. (Jason Trent)
#           Better handling of errors with wrong permissions for the directory containing the files. (Jason Trent)
#           Avoid some race conditions (Ludwig Nussel)
#           Unset the $CDPATH variable to avoid problems if it is set. (Debian)
#           Better handling of dot files in the archive directory.
# - 2.1.5 : Made the md5sum detection consistent with the header code.
#           Check for the presence of the archive directory
#           Added --encrypt for symmetric encryption through gpg (Eric Windisch)
#           Added support for the digest command on Solaris 10 for MD5 checksums
#           Check for available disk space before extracting to the target directory (Andreas Schweitzer)
#           Allow extraction to run asynchronously (patch by Peter Hatch)
#           Use file descriptors internally to avoid error messages (patch by Kay Tiong Khoo)
# - 2.1.6 : Replaced one dot per file progress with a realtime progress percentage and a spining cursor (Guy Baconniere)
#           Added --noprogress to prevent showing the progress during the decompression (Guy Baconniere)
#           Added --target dir to allow extracting directly to a target directory (Guy Baconniere)
# - 2.2.0 : Many bugfixes, updates and contributions from users. Check out the project page on Github for the details.
# - 2.3.0 : Option to specify packaging date to enable byte-for-byte reproducibility. (Marc Pawlowsky)
#
# (C) 1998-2017 by Stephane Peter <megastep@megastep.org>
#
# This software is released under the terms of the GNU GPL version 2 and above
# Please read the license at http://www.gnu.org/copyleft/gpl.html
#

MS_VERSION=2.3.1
MS_COMMAND="$0"
unset CDPATH

for f in "${1+"$@"}"; do
    MS_COMMAND="$MS_COMMAND \\\\
    \\\"$f\\\""
done

# For Solaris systems
if test -d /usr/xpg4/bin; then
    PATH=/usr/xpg4/bin:$PATH
    export PATH
fi

# Procedures

MS_Usage()
{
    echo "Usage: $0 [params] archive_dir file_name label startup_script [args]"
    echo "params can be one or more of the following :"
    echo "    --version | -v     : Print out Makeself version number and exit"
    echo "    --help | -h        : Print out this help message"
    echo "    --tar-quietly      : Suppress verbose output from the tar command"
    echo "    --quiet | -q       : Do not print any messages other than errors."
    echo "    --gzip             : Compress using gzip (default if detected)"
    echo "    --pigz             : Compress with pigz"
    echo "    --bzip2            : Compress using bzip2 instead of gzip"
    echo "    --pbzip2           : Compress using pbzip2 instead of gzip"
    echo "    --xz               : Compress using xz instead of gzip"
    echo "    --lzo              : Compress using lzop instead of gzip"
    echo "    --lz4              : Compress using lz4 instead of gzip"
    echo "    --compress         : Compress using the UNIX 'compress' command"
    echo "    --complevel lvl    : Compression level for gzip pigz xz lzo lz4 bzip2 and pbzip2 (default 9)"
    echo "    --base64           : Instead of compressing, encode the data using base64"
    echo "    --gpg-encrypt      : Instead of compressing, encrypt the data using GPG"
    echo "    --gpg-asymmetric-encrypt-sign"
    echo "                       : Instead of compressing, asymmetrically encrypt and sign the data using GPG"
    echo "    --gpg-extra opt    : Append more options to the gpg command line"
    echo "    --ssl-encrypt      : Instead of compressing, encrypt the data using OpenSSL"
    echo "    --nocomp           : Do not compress the data"
    echo "    --notemp           : The archive will create archive_dir in the"
    echo "                         current directory and uncompress in ./archive_dir"
    echo "    --needroot         : Check that the root user is extracting the archive before proceeding"
    echo "    --copy             : Upon extraction, the archive will first copy itself to"
    echo "                         a temporary directory"
    echo "    --append           : Append more files to an existing Makeself archive"
    echo "                         The label and startup scripts will then be ignored"
    echo "    --target dir       : Extract directly to a target directory"
    echo "                         directory path can be either absolute or relative"
    echo "    --nooverwrite      : Do not extract the archive if the specified target directory exists"
    echo "    --current          : Files will be extracted to the current directory"
    echo "                         Both --current and --target imply --notemp"
    echo "    --tar-extra opt    : Append more options to the tar command line"
    echo "    --untar-extra opt  : Append more options to the during the extraction of the tar archive"
    echo "    --nomd5            : Don't calculate an MD5 for archive"
    echo "    --nocrc            : Don't calculate a CRC for archive"
    echo "    --header file      : Specify location of the header script"
    echo "    --follow           : Follow the symlinks in the archive"
    echo "    --noprogress       : Do not show the progress during the decompression"
    echo "    --nox11            : Disable automatic spawn of a xterm"
    echo "    --nowait           : Do not wait for user input after executing embedded"
    echo "                         program from an xterm"
    echo "    --lsm file         : LSM file describing the package"
    echo "    --license file     : Append a license file"
    echo "    --help-header file : Add a header to the archive's --help output"
    echo "    --packaging-date date"
    echo "                       : Use provided string as the packaging date"
    echo "                         instead of the current date."
    echo
    echo "    --keep-umask       : Keep the umask set to shell default, rather than overriding when executing self-extracting archive."
    echo "    --export-conf      : Export configuration variables to startup_script"
    echo
    echo "Do not forget to give a fully qualified startup script name"
    echo "(i.e. with a ./ prefix if inside the archive)."
    exit 1
}

# Default settings
if type gzip 2>&1 > /dev/null; then
    COMPRESS=gzip
else
    COMPRESS=Unix
fi
COMPRESS_LEVEL=9
KEEP=n
CURRENT=n
NOX11=n
NOWAIT=n
APPEND=n
TAR_QUIETLY=n
KEEP_UMASK=n
QUIET=n
NOPROGRESS=n
COPY=none
NEED_ROOT=n
TAR_ARGS=cvf
TAR_EXTRA=""
GPG_EXTRA=""
DU_ARGS=-ks
HEADER=`dirname "$0"`/makeself-header.sh
TARGETDIR=""
NOOVERWRITE=n
DATE=`LC_ALL=C date`
EXPORT_CONF=n

# LSM file stuff
LSM_CMD="echo No LSM. >> \"\$archname\""

while true
do
    case "$1" in
    --version | -v)
	echo Makeself version $MS_VERSION
	exit 0
	;;
    --pbzip2)
	COMPRESS=pbzip2
	shift
	;;
    --bzip2)
	COMPRESS=bzip2
	shift
	;;
    --gzip)
	COMPRESS=gzip
	shift
	;;
    --pigz)
    	COMPRESS=pigz
    	shift
    	;;
    --xz)
	COMPRESS=xz
	shift
	;;
    --lzo)
	COMPRESS=lzo
	shift
	;;
    --lz4)
	COMPRESS=lz4
	shift
	;;
    --compress)
	COMPRESS=Unix
	shift
	;;
    --base64)
	COMPRESS=base64
	shift
	;;
    --gpg-encrypt)
	COMPRESS=gpg
	shift
	;;
    --gpg-asymmetric-encrypt-sign)
  COMPRESS=gpg-asymmetric
  shift
  ;;
    --gpg-extra)
  GPG_EXTRA="$2"
  if ! shift 2; then MS_Help; exit 1; fi
  ;;
    --ssl-encrypt)
  COMPRESS=openssl
  shift
  ;;
    --nocomp)
	COMPRESS=none
	shift
	;;
    --complevel)
	COMPRESS_LEVEL="$2"
	if ! shift 2; then MS_Help; exit 1; fi
	;;
    --notemp)
	KEEP=y
	shift
	;;
    --copy)
	COPY=copy
	shift
	;;
    --current)
	CURRENT=y
	KEEP=y
	shift
	;;
    --tar-extra)
	TAR_EXTRA="$2"
        if ! shift 2; then MS_Help; exit 1; fi
        ;;
    --untar-extra)
        UNTAR_EXTRA="$2"
        if ! shift 2; then MS_Help; exit 1; fi
        ;;
    --target)
	TARGETDIR="$2"
	KEEP=y
        if ! shift 2; then MS_Help; exit 1; fi
	;;
    --nooverwrite)
        NOOVERWRITE=y
	shift
        ;;
    --needroot)
	NEED_ROOT=y
	shift
	;;
    --header)
	HEADER="$2"
        if ! shift 2; then MS_Help; exit 1; fi
	;;
    --license)
        LICENSE=`cat $2`
        if ! shift 2; then MS_Help; exit 1; fi
	;;
    --follow)
	TAR_ARGS=cvhf
	DU_ARGS=-ksL
	shift
	;;
    --noprogress)
	NOPROGRESS=y
	shift
	;;
    --nox11)
	NOX11=y
	shift
	;;
    --nowait)
	NOWAIT=y
	shift
	;;
    --nomd5)
	NOMD5=y
	shift
	;;
    --nocrc)
	NOCRC=y
	shift
	;;
    --append)
	APPEND=y
	shift
	;;
    --lsm)
	LSM_CMD="cat \"$2\" >> \"\$archname\""
    if ! shift 2; then MS_Help; exit 1; fi
	;;
    --packaging-date)
	DATE="$2"
	if ! shift 2; then MS_Help; exit 1; fi
        ;;
    --help-header)
	HELPHEADER=`sed -e "s/'/'\\\\\''/g" $2`
    if ! shift 2; then MS_Help; exit 1; fi
	[ -n "$HELPHEADER" ] && HELPHEADER="$HELPHEADER
"
    ;;
    --tar-quietly)
	TAR_QUIETLY=y
	shift
	;;
	--keep-umask)
	KEEP_UMASK=y
	shift
	;;
    --export-conf)
    EXPORT_CONF=y
    shift
    ;;
    -q | --quiet)
	QUIET=y
	shift
	;;
    -h | --help)
	MS_Usage
	;;
    -*)
	echo Unrecognized flag : "$1"
	MS_Usage
	;;
    *)
	break
	;;
    esac
done

if test $# -lt 1; then
	MS_Usage
else
	if test -d "$1"; then
		archdir="$1"
	else
		echo "Directory $1 does not exist." >&2
		exit 1
	fi
fi
archname="$2"

if test "$QUIET" = "y" || test "$TAR_QUIETLY" = "y"; then
    if test "$TAR_ARGS" = "cvf"; then
	TAR_ARGS="cf"
    elif test "$TAR_ARGS" = "cvhf";then
	TAR_ARGS="chf"
    fi
fi

if test "$APPEND" = y; then
    if test $# -lt 2; then
	MS_Usage
    fi

    # Gather the info from the original archive
    OLDENV=`sh "$archname" --dumpconf`
    if test $? -ne 0; then
	echo "Unable to update archive: $archname" >&2
	exit 1
    else
	eval "$OLDENV"
    fi
else
    if test "$KEEP" = n -a $# = 3; then
	echo "ERROR: Making a temporary archive with no embedded command does not make sense!" >&2
	echo >&2
	MS_Usage
    fi
    # We don't want to create an absolute directory unless a target directory is defined
    if test "$CURRENT" = y; then
	archdirname="."
    elif test x$TARGETDIR != x; then
	archdirname="$TARGETDIR"
    else
	archdirname=`basename "$1"`
    fi

    if test $# -lt 3; then
	MS_Usage
    fi

    LABEL="$3"
    SCRIPT="$4"
    test "x$SCRIPT" = x || shift 1
    shift 3
    SCRIPTARGS="$*"
fi

if test "$KEEP" = n -a "$CURRENT" = y; then
    echo "ERROR: It is A VERY DANGEROUS IDEA to try to combine --notemp and --current." >&2
    exit 1
fi

case $COMPRESS in
gzip)
    GZIP_CMD="gzip -c$COMPRESS_LEVEL"
    GUNZIP_CMD="gzip -cd"
    ;;
pigz) 
    GZIP_CMD="pigz -$COMPRESS_LEVEL"
    GUNZIP_CMD="gzip -cd"
    ;;
pbzip2)
    GZIP_CMD="pbzip2 -c$COMPRESS_LEVEL"
    GUNZIP_CMD="bzip2 -d"
    ;;
bzip2)
    GZIP_CMD="bzip2 -$COMPRESS_LEVEL"
    GUNZIP_CMD="bzip2 -d"
    ;;
xz)
    GZIP_CMD="xz -c$COMPRESS_LEVEL"
    GUNZIP_CMD="xz -d"
    ;;
lzo)
    GZIP_CMD="lzop -c$COMPRESS_LEVEL"
    GUNZIP_CMD="lzop -d"
    ;;
lz4)
    GZIP_CMD="lz4 -c$COMPRESS_LEVEL"
    GUNZIP_CMD="lz4 -d"
    ;;
base64)
    GZIP_CMD="base64"
    GUNZIP_CMD="base64 -d -i"
    ;;
gpg)
    GZIP_CMD="gpg $GPG_EXTRA -ac -z$COMPRESS_LEVEL"
    GUNZIP_CMD="gpg -d"
    ;;
gpg-asymmetric)
    GZIP_CMD="gpg $GPG_EXTRA -z$COMPRESS_LEVEL -es"
    GUNZIP_CMD="gpg --yes -d"
    ;;
openssl)
    GZIP_CMD="openssl aes-256-cbc -a -salt -md sha256"
    GUNZIP_CMD="openssl aes-256-cbc -d -a -md sha256"
    ;;
Unix)
    GZIP_CMD="compress -cf"
    GUNZIP_CMD="exec 2>&-; uncompress -c || test \\\$? -eq 2 || gzip -cd"
    ;;
none)
    GZIP_CMD="cat"
    GUNZIP_CMD="cat"
    ;;
esac

tmpfile="${TMPDIR:=/tmp}/mkself$$"

if test -f "$HEADER"; then
	oldarchname="$archname"
	archname="$tmpfile"
	# Generate a fake header to count its lines
	SKIP=0
    . "$HEADER"
    SKIP=`cat "$tmpfile" |wc -l`
	# Get rid of any spaces
	SKIP=`expr $SKIP`
	rm -f "$tmpfile"
    if test "$QUIET" = "n";then
    	echo Header is $SKIP lines long >&2
    fi

	archname="$oldarchname"
else
    echo "Unable to open header file: $HEADER" >&2
    exit 1
fi

if test "$QUIET" = "n";then 
    echo
fi

if test "$APPEND" = n; then
    if test -f "$archname"; then
		echo "WARNING: Overwriting existing file: $archname" >&2
    fi
fi

USIZE=`du $DU_ARGS "$archdir" | awk '{print $1}'`

if test "." = "$archdirname"; then
	if test "$KEEP" = n; then
		archdirname="makeself-$$-`date +%Y%m%d%H%M%S`"
	fi
fi

test -d "$archdir" || { echo "Error: $archdir does not exist."; rm -f "$tmpfile"; exit 1; }
if test "$QUIET" = "n";then
   echo About to compress $USIZE KB of data...
   echo Adding files to archive named \"$archname\"...
fi
exec 3<> "$tmpfile"
( cd "$archdir" && ( tar $TAR_EXTRA -$TAR_ARGS - . | eval "$GZIP_CMD" >&3 ) ) || \
    { echo Aborting: archive directory not found or temporary file: "$tmpfile" could not be created.; exec 3>&-; rm -f "$tmpfile"; exit 1; }
exec 3>&- # try to close the archive

fsize=`cat "$tmpfile" | wc -c | tr -d " "`

# Compute the checksums

md5sum=00000000000000000000000000000000
crcsum=0000000000

if test "$NOCRC" = y; then
	if test "$QUIET" = "n";then
		echo "skipping crc at user request"
	fi
else
	crcsum=`cat "$tmpfile" | CMD_ENV=xpg4 cksum | sed -e 's/ /Z/' -e 's/	/Z/' | cut -dZ -f1`
	if test "$QUIET" = "n";then
		echo "CRC: $crcsum"
	fi
fi

if test "$NOMD5" = y; then
	if test "$QUIET" = "n";then
		echo "skipping md5sum at user request"
	fi
else
	# Try to locate a MD5 binary
	OLD_PATH=$PATH
	PATH=${GUESS_MD5_PATH:-"$OLD_PATH:/bin:/usr/bin:/sbin:/usr/local/ssl/bin:/usr/local/bin:/opt/openssl/bin"}
	MD5_ARG=""
	MD5_PATH=`exec <&- 2>&-; which md5sum || command -v md5sum || type md5sum`
	test -x "$MD5_PATH" || MD5_PATH=`exec <&- 2>&-; which md5 || command -v md5 || type md5`
	test -x "$MD5_PATH" || MD5_PATH=`exec <&- 2>&-; which digest || command -v digest || type digest`
	PATH=$OLD_PATH
	if test -x "$MD5_PATH"; then
		if test `basename ${MD5_PATH}`x = digestx; then
			MD5_ARG="-a md5"
		fi
		md5sum=`cat "$tmpfile" | eval "$MD5_PATH $MD5_ARG" | cut -b-32`;
		if test "$QUIET" = "n";then
			echo "MD5: $md5sum"
		fi
	else
		if test "$QUIET" = "n";then
			echo "MD5: none, MD5 command not found"
		fi
	fi
fi

if test "$APPEND" = y; then
    mv "$archname" "$archname".bak || exit

    # Prepare entry for new archive
    filesizes="$filesizes $fsize"
    CRCsum="$CRCsum $crcsum"
    MD5sum="$MD5sum $md5sum"
    USIZE=`expr $USIZE + $OLDUSIZE`
    # Generate the header
    . "$HEADER"
    # Append the original data
    tail -n +$OLDSKIP "$archname".bak >> "$archname"
    # Append the new data
    cat "$tmpfile" >> "$archname"

    chmod +x "$archname"
    rm -f "$archname".bak
    if test "$QUIET" = "n";then
    	echo Self-extractable archive \"$archname\" successfully updated.
    fi
else
    filesizes="$fsize"
    CRCsum="$crcsum"
    MD5sum="$md5sum"

    # Generate the header
    . "$HEADER"

    # Append the compressed tar data after the stub
    if test "$QUIET" = "n";then
    	echo
    fi
    cat "$tmpfile" >> "$archname"
    chmod +x "$archname"
    if test "$QUIET" = "n";then
    	echo Self-extractable archive \"$archname\" successfully created.
    fi
fi
rm -f "$tmpfile"
