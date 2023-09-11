#!/bin/sh
#
# SPDX-License-Identifier: GPL-2.0-or-later
#
# shellcheck disable=SC2209,SC2006,SC2016,SC2034,SC2086,SC2003,SC2268,SC1090,SC2002,SC2046
#
# Makeself version 2.5.x
#  by Stephane Peter <megastep@megastep.org>
#
# Utility to create self-extracting tar.gz archives.
# The resulting archive is a file holding the tar.gz archive with
# a small Shell script stub that uncompresses the archive to a temporary
# directory and then executes a given script from withing that directory.
#
# Makeself home page: https://makeself.io/ - Version history available on GitHub
#
# (C) 1998-2023 by Stephane Peter <megastep@megastep.org>
#
# This software is released under the terms of the GNU GPL version 2 and above
# Please read the license at http://www.gnu.org/copyleft/gpl.html
# Self-extracting archives created with this script are explictly NOT released under the term of the GPL
#

MS_VERSION=2.5.0
MS_COMMAND="$0"
unset CDPATH

for f in ${1+"$@"}; do
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
    echo "Usage: $0 [args] archive_dir file_name label startup_script [script_args]"
    echo "args can be one or more of the following :"
    echo "    --version | -v     : Print out Makeself version number and exit"
    echo "    --help | -h        : Print out this help message"
    echo "    --tar-quietly      : Suppress verbose output from the tar command"
    echo "    --quiet | -q       : Do not print any messages other than errors."
    echo "    --gzip             : Compress using gzip (default if detected)"
    echo "    --pigz             : Compress with pigz"
    echo "    --zstd             : Compress with zstd"
    echo "    --bzip2            : Compress using bzip2 instead of gzip"
    echo "    --pbzip2           : Compress using pbzip2 instead of gzip"
    echo "    --bzip3            : Compress using bzip3 instead of gzip"
    echo "    --xz               : Compress using xz instead of gzip"
    echo "    --lzo              : Compress using lzop instead of gzip"
    echo "    --lz4              : Compress using lz4 instead of gzip"
    echo "    --compress         : Compress using the UNIX 'compress' command"
    echo "    --complevel lvl    : Compression level for gzip pigz zstd xz lzo lz4 bzip2 pbzip2 and bzip3 (default 9)"
    echo "    --threads thds     : Number of threads to be used by compressors that support parallelization."
    echo "                         Omit to use compressor's default. Most useful (and required) for opting"
    echo "                         into xz's threading, usually with '--threads=0' for all available cores."
    echo "                         pbzip2 and pigz are parallel by default, and setting this value allows"
    echo "                         limiting the number of threads they use."
    echo "    --base64           : Instead of compressing, encode the data using base64"
    echo "    --gpg-encrypt      : Instead of compressing, encrypt the data using GPG"
    echo "    --gpg-asymmetric-encrypt-sign"
    echo "                       : Instead of compressing, asymmetrically encrypt and sign the data using GPG"
    echo "    --gpg-extra opt    : Append more options to the gpg command line"
    echo "    --ssl-encrypt      : Instead of compressing, encrypt the data using OpenSSL"
    echo "    --ssl-passwd pass  : Use the given password to encrypt the data using OpenSSL"
    echo "    --ssl-pass-src src : Use the given src as the source of password to encrypt the data"
    echo "                         using OpenSSL. See \"PASS PHRASE ARGUMENTS\" in man openssl."
    echo "                         If this option is not supplied, the user will be asked to enter"
    echo "                         encryption password on the current terminal."
    echo "    --ssl-no-md        : Do not use \"-md\" option not supported by older OpenSSL."
    echo "    --nochown          : Do not give the target folder to the current user (default)"
    echo "    --chown            : Give the target folder to the current user recursively"
    echo "    --nocomp           : Do not compress the data"
    echo "    --notemp           : The archive will create archive_dir in the current directory"
    echo "                         and uncompress in ./archive_dir"
    echo "                         Note: persistent archives do not strictly require a startup_script"
    echo "    --needroot         : Check that the root user is extracting the archive before proceeding"
    echo "    --copy             : Upon extraction, the archive will first copy itself to"
    echo "                         a temporary directory"
    echo "    --append           : Append more files to an existing Makeself archive"
    echo "                         The label and startup scripts will then be ignored"
    echo "    --target dir       : Extract directly to a target directory"
    echo "                         directory path can be either absolute or relative"
    echo "    --current          : Files will be extracted to the current directory"
    echo "                         Both --current and --target imply --notemp, and do not require a startup_script"
    echo "    --nooverwrite      : Do not extract the archive if the specified target directory exists"
    echo "    --tar-format opt   : Specify a tar archive format (default is ustar)"
    echo "    --tar-extra opt    : Append more options to the tar command line"
    echo "    --untar-extra opt  : Append more options to the during the extraction of the tar archive"
    echo "    --nomd5            : Don't calculate an MD5 for archive"
    echo "    --nocrc            : Don't calculate a CRC for archive"
    echo "    --sha256           : Compute a SHA256 checksum for the archive"
    echo "    --header file      : Specify location of the header script"
    echo "    --cleanup file     : Specify a cleanup script that executes on interrupt and when finished successfully."
    echo "    --follow           : Follow the symlinks in the archive"
    echo "    --noprogress       : Do not show the progress during the decompression"
    echo "    --nox11            : Disable automatic spawn of a xterm"
    echo "    --nowait           : Do not wait for user input after executing embedded"
    echo "                         program from an xterm"
    echo "    --sign passphrase  : Signature private key to sign the package with"
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
if type gzip >/dev/null 2>&1; then
    COMPRESS=gzip
elif type compress >/dev/null 2>&1; then
    COMPRESS=compress
else
    echo "ERROR: missing commands: gzip, compress" >&2
    MS_Usage
fi
ENCRYPT=n
PASSWD=""
PASSWD_SRC=""
OPENSSL_NO_MD=n
COMPRESS_LEVEL=9
DEFAULT_THREADS=123456 # Sentinel value
THREADS=$DEFAULT_THREADS
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
TAR_ARGS=rvf
TAR_FORMAT=ustar
TAR_EXTRA=""
GPG_EXTRA=""
DU_ARGS=-ks
HEADER=`dirname "$0"`/makeself-header.sh
SIGNATURE=""
TARGETDIR=""
NOOVERWRITE=n
DATE=`LC_ALL=C date`
EXPORT_CONF=n
SHA256=n
OWNERSHIP=n
SIGN=n
GPG_PASSPHRASE=""

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
    --bzip3)
	COMPRESS=bzip3
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
    --zstd)
    	COMPRESS=zstd
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
	COMPRESS=compress
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
    shift 2 || { MS_Usage; exit 1; }
	;;
    --ssl-encrypt)
	ENCRYPT=openssl
 	shift
	;;
    --ssl-passwd)
	PASSWD=$2
    shift 2 || { MS_Usage; exit 1; }
	;;
    --ssl-pass-src)
	PASSWD_SRC=$2
    shift 2 || { MS_Usage; exit 1; }
	;;
    --ssl-no-md)
	OPENSSL_NO_MD=y
	shift
	;;
    --nocomp)
	COMPRESS=none
	shift
	;;
    --complevel)
	COMPRESS_LEVEL="$2"
    shift 2 || { MS_Usage; exit 1; }
	;;
    --threads)
	THREADS="$2"
    shift 2 || { MS_Usage; exit 1; }
	;;
    --nochown)
	OWNERSHIP=n
	shift
	;;
    --chown)
	OWNERSHIP=y
	shift
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
    --tar-format)
	    TAR_FORMAT="$2"
        shift 2 || { MS_Usage; exit 1; }
    ;;
    --tar-extra)
	    TAR_EXTRA="$2"
        shift 2 || { MS_Usage; exit 1; }
    ;;
    --untar-extra)
        UNTAR_EXTRA="$2"
        shift 2 || { MS_Usage; exit 1; }
        ;;
    --target)
	  TARGETDIR="$2"
	  KEEP=y
    shift 2 || { MS_Usage; exit 1; }
 	  ;;
    --sign)
    SIGN=y
    GPG_PASSPHRASE="$2"
    shift 2 || { MS_Usage; exit 1; }
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
    shift 2 || { MS_Usage; exit 1; }
	;;
    --cleanup)
    CLEANUP_SCRIPT="$2"
    shift 2 || { MS_Usage; exit 1; }
    ;;
    --license)
        # We need to escape all characters having a special meaning in double quotes
        LICENSE=$(sed 's/\\/\\\\/g; s/"/\\\"/g; s/`/\\\`/g; s/\$/\\\$/g' "$2")
        shift 2 || { MS_Usage; exit 1; }
	;;
    --follow)
	TAR_ARGS=rvhf
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
    --sha256)
        SHA256=y
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
	LSM_CMD="awk 1 \"$2\" >> \"\$archname\""
    shift 2 || { MS_Usage; exit 1; }
	;;
    --packaging-date)
	DATE="$2"
    shift 2 || { MS_Usage; exit 1; }
        ;;
    --help-header)
	HELPHEADER=`sed -e "s/'/'\\\\\''/g" $2`
    shift 2 || { MS_Usage; exit 1; }
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
    if test "$TAR_ARGS" = "rvf"; then
	    TAR_ARGS="rf"
    elif test "$TAR_ARGS" = "rvhf"; then
	    TAR_ARGS="rhf"
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
	    OLDSKIP=`expr $SKIP + 1`
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
    elif test x"$TARGETDIR" != x; then
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
    if test $THREADS -ne $DEFAULT_THREADS; then # Leave as the default if threads not indicated
        GZIP_CMD="$GZIP_CMD --processes $THREADS"
    fi
    GUNZIP_CMD="gzip -cd"
    ;;
zstd)
    GZIP_CMD="zstd -$COMPRESS_LEVEL"
    if test $THREADS -ne $DEFAULT_THREADS; then # Leave as the default if threads not indicated
        GZIP_CMD="$GZIP_CMD --threads=$THREADS"
    fi
    GUNZIP_CMD="zstd -cd"
    ;;
pbzip2)
    GZIP_CMD="pbzip2 -c$COMPRESS_LEVEL"
    if test $THREADS -ne $DEFAULT_THREADS; then # Leave as the default if threads not indicated
        GZIP_CMD="$GZIP_CMD -p$THREADS"
    fi
    GUNZIP_CMD="bzip2 -d"
    ;;
bzip3)
    # Map the compression level to a block size in MiB as 2^(level-1).
    BZ3_COMPRESS_LEVEL=`echo "2^($COMPRESS_LEVEL-1)" | bc`
    GZIP_CMD="bzip3 -b$BZ3_COMPRESS_LEVEL"
    if test $THREADS -ne $DEFAULT_THREADS; then # Leave as the default if threads not indicated
        GZIP_CMD="$GZIP_CMD -j$THREADS"
    fi
    JOBS=`echo "10-$COMPRESS_LEVEL" | bc`
    GUNZIP_CMD="bzip3 -dj$JOBS"
    ;;
bzip2)
    GZIP_CMD="bzip2 -$COMPRESS_LEVEL"
    GUNZIP_CMD="bzip2 -d"
    ;;
xz)
    GZIP_CMD="xz -c$COMPRESS_LEVEL"
    # Must opt-in by specifying a value since not all versions of xz support threads
    if test $THREADS -ne $DEFAULT_THREADS; then 
        GZIP_CMD="$GZIP_CMD --threads=$THREADS"
    fi
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
    GUNZIP_CMD="base64 --decode -i -"
    ;;
gpg)
    GZIP_CMD="gpg $GPG_EXTRA -ac -z$COMPRESS_LEVEL"
    GUNZIP_CMD="gpg -d"
    ENCRYPT="gpg"
    ;;
gpg-asymmetric)
    GZIP_CMD="gpg $GPG_EXTRA -z$COMPRESS_LEVEL -es"
    GUNZIP_CMD="gpg --yes -d"
    ENCRYPT="gpg"
    ;;
compress)
    GZIP_CMD="compress -fc"
    GUNZIP_CMD="(type compress >/dev/null 2>&1 && compress -fcd || gzip -cd)"
    ;;
none)
    GZIP_CMD="cat"
    GUNZIP_CMD="cat"
    ;;
esac

if test x"$ENCRYPT" = x"openssl"; then
    if test x"$APPEND" = x"y"; then
        echo "Appending to existing archive is not compatible with OpenSSL encryption." >&2
    fi
    
    ENCRYPT_CMD="openssl enc -aes-256-cbc -salt"
    DECRYPT_CMD="openssl enc -aes-256-cbc -d"
    
    if test x"$OPENSSL_NO_MD" != x"y"; then
        ENCRYPT_CMD="$ENCRYPT_CMD -md sha256"
        DECRYPT_CMD="$DECRYPT_CMD -md sha256"
    fi

    if test -n "$PASSWD_SRC"; then
        ENCRYPT_CMD="$ENCRYPT_CMD -pass $PASSWD_SRC"
    elif test -n "$PASSWD"; then 
        ENCRYPT_CMD="$ENCRYPT_CMD -pass pass:$PASSWD"
    fi
fi

tmpfile="${TMPDIR:-/tmp}/mkself$$"

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
	if test "$QUIET" = "n"; then
		echo "Header is $SKIP lines long" >&2
	fi
	archname="$oldarchname"
else
    echo "Unable to open header file: $HEADER" >&2
    exit 1
fi

if test "$QUIET" = "n"; then
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
if test "$QUIET" = "n"; then
   echo "About to compress $USIZE KB of data..."
   echo "Adding files to archive named \"$archname\"..."
fi

# See if we have GNU tar
TAR=`exec <&- 2>&-; which gtar || command -v gtar || type gtar`
test -x "$TAR" || TAR=`exec <&- 2>&-; which bsdtar || command -v bsdtar || type bsdtar`
test -x "$TAR" || TAR=tar

tmparch="${TMPDIR:-/tmp}/mkself$$.tar"
(
    if test "$APPEND" = "y"; then
        tail -n "+$OLDSKIP" "$archname" | eval "$GUNZIP_CMD" > "$tmparch"
    fi
    cd "$archdir"
    # "Determining if a directory is empty"
    # https://www.etalabs.net/sh_tricks.html
    find . \
        \( \
        ! -type d \
        -o \
        \( -links 2 -exec sh -c '
            is_empty () (
                cd "$1"
                set -- .[!.]* ; test -f "$1" && return 1
                set -- ..?* ; test -f "$1" && return 1
                set -- * ; test -f "$1" && return 1
                return 0
            )
            is_empty "$0"' {} \; \
        \) \
        \) -print \
        | LC_ALL=C sort \
        | sed 's/./\\&/g' \
        | xargs $TAR $TAR_EXTRA --format $TAR_FORMAT -$TAR_ARGS "$tmparch"
) || {
    echo "ERROR: failed to create temporary archive: $tmparch"
    rm -f "$tmparch" "$tmpfile"
    exit 1
}

USIZE=`du $DU_ARGS "$tmparch" | awk '{print $1}'`

eval "$GZIP_CMD" <"$tmparch" >"$tmpfile" || {
    echo "ERROR: failed to create temporary file: $tmpfile"
    rm -f "$tmparch" "$tmpfile"
    exit 1
}
rm -f "$tmparch"

if test x"$ENCRYPT" = x"openssl"; then
    echo "About to encrypt archive \"$archname\"..."
    { eval "$ENCRYPT_CMD -in $tmpfile -out ${tmpfile}.enc" && mv -f ${tmpfile}.enc $tmpfile; } || \
        { echo Aborting: could not encrypt temporary file: "$tmpfile".; rm -f "$tmpfile"; exit 1; }
fi

fsize=`cat "$tmpfile" | wc -c | tr -d " "`

# Compute the checksums

shasum=0000000000000000000000000000000000000000000000000000000000000000
md5sum=00000000000000000000000000000000
crcsum=0000000000

if test "$NOCRC" = y; then
	if test "$QUIET" = "n"; then
		echo "skipping crc at user request"
	fi
else
	crcsum=`CMD_ENV=xpg4 cksum < "$tmpfile" | sed -e 's/ /Z/' -e 's/	/Z/' | cut -dZ -f1`
	if test "$QUIET" = "n"; then
		echo "CRC: $crcsum"
	fi
fi

if test "$SHA256" = y; then
	SHA_PATH=`exec <&- 2>&-; which shasum || command -v shasum || type shasum`
	if test -x "$SHA_PATH"; then
		shasum=`eval "$SHA_PATH -a 256" < "$tmpfile" | cut -b-64`
	else
		SHA_PATH=`exec <&- 2>&-; which sha256sum || command -v sha256sum || type sha256sum`
		shasum=`eval "$SHA_PATH" < "$tmpfile" | cut -b-64`
	fi
	if test "$QUIET" = "n"; then
		if test -x "$SHA_PATH"; then
			echo "SHA256: $shasum"
		else
			echo "SHA256: none, SHA command not found"
		fi
	fi
fi
if test "$NOMD5" = y; then
	if test "$QUIET" = "n"; then
		echo "Skipping md5sum at user request"
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
		md5sum=`eval "$MD5_PATH $MD5_ARG" < "$tmpfile" | cut -b-32`
		if test "$QUIET" = "n"; then
			echo "MD5: $md5sum"
		fi
	else
		if test "$QUIET" = "n"; then
			echo "MD5: none, MD5 command not found"
		fi
	fi
fi
if test "$SIGN" = y; then
    GPG_PATH=`exec <&- 2>&-; which gpg || command -v gpg || type gpg`
    if test -x "$GPG_PATH"; then
        SIGNATURE=`$GPG_PATH --pinentry-mode=loopback --batch --yes $GPG_EXTRA --passphrase "$GPG_PASSPHRASE" --output - --detach-sig $tmpfile | base64 | tr -d \\\\n`
        if test "$QUIET" = "n"; then
            echo "Signature: $SIGNATURE"
        fi
    else
        echo "Missing gpg command" >&2
    fi
fi

totalsize=0
for size in $fsize;
do
    totalsize=`expr $totalsize + $size`
done

if test "$APPEND" = y; then
    mv "$archname" "$archname".bak || exit

    # Prepare entry for new archive
    filesizes="$fsize"
    CRCsum="$crcsum"
    MD5sum="$md5sum"
    SHAsum="$shasum"
    Signature="$SIGNATURE"
    # Generate the header
    . "$HEADER"
    # Append the new data
    cat "$tmpfile" >> "$archname"

    chmod +x "$archname"
    rm -f "$archname".bak
    if test "$QUIET" = "n"; then
    	echo "Self-extractable archive \"$archname\" successfully updated."
    fi
else
    filesizes="$fsize"
    CRCsum="$crcsum"
    MD5sum="$md5sum"
    SHAsum="$shasum"
    Signature="$SIGNATURE"

    # Generate the header
    . "$HEADER"

    # Append the compressed tar data after the stub
    if test "$QUIET" = "n"; then
    	echo
    fi
    cat "$tmpfile" >> "$archname"
    chmod +x "$archname"
    if test "$QUIET" = "n"; then
    	echo Self-extractable archive \"$archname\" successfully created.
    fi
fi
rm -f "$tmpfile"
