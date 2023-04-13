# SPDX-License-Identifier: GPL-2.0-or-later
# shellcheck shell=sh
# shellcheck disable=SC2154,SC2039
cat << EOF  > "$archname"
#!/bin/sh
# This script was generated using Makeself $MS_VERSION
# The license covering this archive and its contents, if any, is wholly independent of the Makeself license (GPL)

ORIG_UMASK=\`umask\`
if test "$KEEP_UMASK" = n; then
    umask 077
fi

CRCsum="$CRCsum"
MD5="$MD5sum"
SHA="$SHAsum"
SIGNATURE="$Signature"
TMPROOT=\${TMPDIR:=/tmp}
USER_PWD="\$PWD"
export USER_PWD
ARCHIVE_DIR=\`dirname "\$0"\`
export ARCHIVE_DIR

label="$LABEL"
script="$SCRIPT"
scriptargs="$SCRIPTARGS"
cleanup_script="${CLEANUP_SCRIPT}"
licensetxt="$LICENSE"
helpheader="${HELPHEADER}"
targetdir="$archdirname"
filesizes="$filesizes"
totalsize="$totalsize"
keep="$KEEP"
nooverwrite="$NOOVERWRITE"
quiet="n"
accept="n"
nodiskspace="n"
export_conf="$EXPORT_CONF"
decrypt_cmd="$DECRYPT_CMD"
skip="$SKIP"

print_cmd_arg=""
if type printf > /dev/null; then
    print_cmd="printf"
elif test -x /usr/ucb/echo; then
    print_cmd="/usr/ucb/echo"
else
    print_cmd="echo"
fi

if test -d /usr/xpg4/bin; then
    PATH=/usr/xpg4/bin:\$PATH
    export PATH
fi

if test -d /usr/sfw/bin; then
    PATH=\$PATH:/usr/sfw/bin
    export PATH
fi

unset CDPATH

MS_Printf()
{
    \$print_cmd \$print_cmd_arg "\$1"
}

MS_PrintLicense()
{
  PAGER=\${PAGER:=more}
  if test x"\$licensetxt" != x; then
    PAGER_PATH=\`exec <&- 2>&-; which \$PAGER || command -v \$PAGER || type \$PAGER\`
    if test -x "\$PAGER_PATH"; then
      echo "\$licensetxt" | \$PAGER
    else
      echo "\$licensetxt"
    fi
    if test x"\$accept" != xy; then
      while true
      do
        MS_Printf "Please type y to accept, n otherwise: "
        read yn
        if test x"\$yn" = xn; then
          keep=n
          eval \$finish; exit 1
          break;
        elif test x"\$yn" = xy; then
          break;
        fi
      done
    fi
  fi
}

MS_diskspace()
{
	(
	df -k "\$1" | tail -1 | awk '{ if (\$4 ~ /%/) {print \$3} else {print \$4} }'
	)
}

MS_dd()
{
    blocks=\`expr \$3 / 1024\`
    bytes=\`expr \$3 % 1024\`
    # Test for ibs, obs and conv feature
    if dd if=/dev/zero of=/dev/null count=1 ibs=512 obs=512 conv=sync 2> /dev/null; then
        dd if="\$1" ibs=\$2 skip=1 obs=1024 conv=sync 2> /dev/null | \\
        { test \$blocks -gt 0 && dd ibs=1024 obs=1024 count=\$blocks ; \\
          test \$bytes  -gt 0 && dd ibs=1 obs=1024 count=\$bytes ; } 2> /dev/null
    else
        dd if="\$1" bs=\$2 skip=1 2> /dev/null
    fi
}

MS_dd_Progress()
{
    if test x"\$noprogress" = xy; then
        MS_dd "\$@"
        return \$?
    fi
    file="\$1"
    offset=\$2
    length=\$3
    pos=0
    bsize=4194304
    while test \$bsize -gt \$length; do
        bsize=\`expr \$bsize / 4\`
    done
    blocks=\`expr \$length / \$bsize\`
    bytes=\`expr \$length % \$bsize\`
    (
        dd ibs=\$offset skip=1 count=1 2>/dev/null
        pos=\`expr \$pos \+ \$bsize\`
        MS_Printf "     0%% " 1>&2
        if test \$blocks -gt 0; then
            while test \$pos -le \$length; do
                dd bs=\$bsize count=1 2>/dev/null
                pcent=\`expr \$length / 100\`
                pcent=\`expr \$pos / \$pcent\`
                if test \$pcent -lt 100; then
                    MS_Printf "\b\b\b\b\b\b\b" 1>&2
                    if test \$pcent -lt 10; then
                        MS_Printf "    \$pcent%% " 1>&2
                    else
                        MS_Printf "   \$pcent%% " 1>&2
                    fi
                fi
                pos=\`expr \$pos \+ \$bsize\`
            done
        fi
        if test \$bytes -gt 0; then
            dd bs=\$bytes count=1 2>/dev/null
        fi
        MS_Printf "\b\b\b\b\b\b\b" 1>&2
        MS_Printf " 100%%  " 1>&2
    ) < "\$file"
}

MS_Help()
{
    cat << EOH >&2
Makeself version $MS_VERSION
 1) Getting help or info about \$0 :
  \$0 --help   Print this message
  \$0 --info   Print embedded info : title, default target directory, embedded script ...
  \$0 --lsm    Print embedded lsm entry (or no LSM)
  \$0 --list   Print the list of files in the archive
  \$0 --check  Checks integrity of the archive
  \$0 --verify-sig key Verify signature agains a provided key id

 2) Running \$0 :
  \$0 [options] [--] [additional arguments to embedded script]
  with following options (in that order)
  --confirm             Ask before running embedded script
  --quiet               Do not print anything except error messages
  --accept              Accept the license
  --noexec              Do not run embedded script (implies --noexec-cleanup)
  --noexec-cleanup      Do not run embedded cleanup script
  --keep                Do not erase target directory after running
                        the embedded script
  --noprogress          Do not show the progress during the decompression
  --nox11               Do not spawn an xterm
  --nochown             Do not give the target folder to the current user
  --chown               Give the target folder to the current user recursively
  --nodiskspace         Do not check for available disk space
  --target dir          Extract directly to a target directory (absolute or relative)
                        This directory may undergo recursive chown (see --nochown).
  --tar arg1 [arg2 ...] Access the contents of the archive through the tar command
  --ssl-pass-src src    Use the given src as the source of password to decrypt the data
                        using OpenSSL. See "PASS PHRASE ARGUMENTS" in man openssl.
                        Default is to prompt the user to enter decryption password
                        on the current terminal.
  --cleanup-args args   Arguments to the cleanup script. Wrap in quotes to provide
                        multiple arguments.
  --                    Following arguments will be passed to the embedded script\${helpheader}
EOH
}

MS_Verify_Sig()
{
    GPG_PATH=\`exec <&- 2>&-; which gpg || command -v gpg || type gpg\`
    MKTEMP_PATH=\`exec <&- 2>&-; which mktemp || command -v mktemp || type mktemp\`
    test -x "\$GPG_PATH" || GPG_PATH=\`exec <&- 2>&-; which gpg || command -v gpg || type gpg\`
    test -x "\$MKTEMP_PATH" || MKTEMP_PATH=\`exec <&- 2>&-; which mktemp || command -v mktemp || type mktemp\`
	offset=\`head -n "\$skip" "\$1" | wc -c | sed "s/ //g"\`
    temp_sig=\`mktemp -t XXXXX\`
    echo \$SIGNATURE | base64 --decode > "\$temp_sig"
    gpg_output=\`MS_dd "\$1" \$offset \$totalsize | LC_ALL=C "\$GPG_PATH" --verify "\$temp_sig" - 2>&1\`
    gpg_res=\$?
    rm -f "\$temp_sig"
    if test \$gpg_res -eq 0 && test \`echo \$gpg_output | grep -c Good\` -eq 1; then
        if test \`echo \$gpg_output | grep -c \$sig_key\` -eq 1; then
            test x"\$quiet" = xn && echo "GPG signature is good" >&2
        else
            echo "GPG Signature key does not match" >&2
            exit 2
        fi
    else
        test x"\$quiet" = xn && echo "GPG signature failed to verify" >&2
        exit 2
    fi
}

MS_Check()
{
    OLD_PATH="\$PATH"
    PATH=\${GUESS_MD5_PATH:-"\$OLD_PATH:/bin:/usr/bin:/sbin:/usr/local/ssl/bin:/usr/local/bin:/opt/openssl/bin"}
	MD5_ARG=""
    MD5_PATH=\`exec <&- 2>&-; which md5sum || command -v md5sum || type md5sum\`
    test -x "\$MD5_PATH" || MD5_PATH=\`exec <&- 2>&-; which md5 || command -v md5 || type md5\`
    test -x "\$MD5_PATH" || MD5_PATH=\`exec <&- 2>&-; which digest || command -v digest || type digest\`
    PATH="\$OLD_PATH"

    SHA_PATH=\`exec <&- 2>&-; which shasum || command -v shasum || type shasum\`
    test -x "\$SHA_PATH" || SHA_PATH=\`exec <&- 2>&-; which sha256sum || command -v sha256sum || type sha256sum\`

    if test x"\$quiet" = xn; then
		MS_Printf "Verifying archive integrity..."
    fi
    offset=\`head -n "\$skip" "\$1" | wc -c | sed "s/ //g"\`
    fsize=\`cat "\$1" | wc -c | sed "s/ //g"\`
    if test \$totalsize -ne \`expr \$fsize - \$offset\`; then
        echo " Unexpected archive size." >&2
        exit 2
    fi
    verb=\$2
    i=1
    for s in \$filesizes
    do
		crc=\`echo \$CRCsum | cut -d" " -f\$i\`
		if test -x "\$SHA_PATH"; then
			if test x"\`basename \$SHA_PATH\`" = xshasum; then
				SHA_ARG="-a 256"
			fi
			sha=\`echo \$SHA | cut -d" " -f\$i\`
			if test x"\$sha" = x0000000000000000000000000000000000000000000000000000000000000000; then
				test x"\$verb" = xy && echo " \$1 does not contain an embedded SHA256 checksum." >&2
			else
				shasum=\`MS_dd_Progress "\$1" \$offset \$s | eval "\$SHA_PATH \$SHA_ARG" | cut -b-64\`;
				if test x"\$shasum" != x"\$sha"; then
					echo "Error in SHA256 checksums: \$shasum is different from \$sha" >&2
					exit 2
				elif test x"\$quiet" = xn; then
					MS_Printf " SHA256 checksums are OK." >&2
				fi
				crc="0000000000";
			fi
		fi
		if test -x "\$MD5_PATH"; then
			if test x"\`basename \$MD5_PATH\`" = xdigest; then
				MD5_ARG="-a md5"
			fi
			md5=\`echo \$MD5 | cut -d" " -f\$i\`
			if test x"\$md5" = x00000000000000000000000000000000; then
				test x"\$verb" = xy && echo " \$1 does not contain an embedded MD5 checksum." >&2
			else
				md5sum=\`MS_dd_Progress "\$1" \$offset \$s | eval "\$MD5_PATH \$MD5_ARG" | cut -b-32\`;
				if test x"\$md5sum" != x"\$md5"; then
					echo "Error in MD5 checksums: \$md5sum is different from \$md5" >&2
					exit 2
				elif test x"\$quiet" = xn; then
					MS_Printf " MD5 checksums are OK." >&2
				fi
				crc="0000000000"; verb=n
			fi
		fi
		if test x"\$crc" = x0000000000; then
			test x"\$verb" = xy && echo " \$1 does not contain a CRC checksum." >&2
		else
			sum1=\`MS_dd_Progress "\$1" \$offset \$s | CMD_ENV=xpg4 cksum | awk '{print \$1}'\`
			if test x"\$sum1" != x"\$crc"; then
				echo "Error in checksums: \$sum1 is different from \$crc" >&2
				exit 2
			elif test x"\$quiet" = xn; then
				MS_Printf " CRC checksums are OK." >&2
			fi
		fi
		i=\`expr \$i + 1\`
		offset=\`expr \$offset + \$s\`
    done
    if test x"\$quiet" = xn; then
		echo " All good."
    fi
}

MS_Decompress()
{
    if test x"\$decrypt_cmd" != x""; then
        { eval "\$decrypt_cmd" || echo " ... Decryption failed." >&2; } | eval "$GUNZIP_CMD"
    else
        eval "$GUNZIP_CMD"
    fi
    
    if test \$? -ne 0; then
        echo " ... Decompression failed." >&2
    fi
}

UnTAR()
{
    if test x"\$quiet" = xn; then
		tar \$1vf - $UNTAR_EXTRA 2>&1 || { echo " ... Extraction failed." >&2; kill -15 \$$; }
    else
		tar \$1f - $UNTAR_EXTRA 2>&1 || { echo Extraction failed. >&2; kill -15 \$$; }
    fi
}

MS_exec_cleanup() {
    if test x"\$cleanup" = xy && test x"\$cleanup_script" != x""; then
        cleanup=n
        cd "\$tmpdir"
        eval "\"\$cleanup_script\" \$scriptargs \$cleanupargs"
    fi
}

MS_cleanup()
{
    echo 'Signal caught, cleaning up' >&2
    MS_exec_cleanup
    cd "\$TMPROOT"
    rm -rf "\$tmpdir"
    eval \$finish; exit 15
}

finish=true
xterm_loop=
noprogress=$NOPROGRESS
nox11=$NOX11
copy=$COPY
ownership=$OWNERSHIP
verbose=n
cleanup=y
cleanupargs=
sig_key=

initargs="\$@"

while true
do
    case "\$1" in
    -h | --help)
	MS_Help
	exit 0
	;;
    -q | --quiet)
	quiet=y
	noprogress=y
	shift
	;;
	--accept)
	accept=y
	shift
	;;
    --info)
	echo Identification: "\$label"
	echo Target directory: "\$targetdir"
	echo Uncompressed size: $USIZE KB
	echo Compression: $COMPRESS
	if test x"$ENCRYPT" != x""; then
	    echo Encryption: $ENCRYPT
	fi
	echo Date of packaging: $DATE
	echo Built with Makeself version $MS_VERSION
	echo Build command was: "$MS_COMMAND"
	if test x"\$script" != x; then
	    echo Script run after extraction:
	    echo "    " \$script \$scriptargs
	fi
	if test x"$copy" = xcopy; then
		echo "Archive will copy itself to a temporary location"
	fi
	if test x"$NEED_ROOT" = xy; then
		echo "Root permissions required for extraction"
	fi
	if test x"$KEEP" = xy; then
	    echo "directory \$targetdir is permanent"
	else
	    echo "\$targetdir will be removed after extraction"
	fi
	exit 0
	;;
    --dumpconf)
	echo LABEL=\"\$label\"
	echo SCRIPT=\"\$script\"
	echo SCRIPTARGS=\"\$scriptargs\"
    echo CLEANUPSCRIPT=\"\$cleanup_script\"
	echo archdirname=\"$archdirname\"
	echo KEEP=$KEEP
	echo NOOVERWRITE=$NOOVERWRITE
	echo COMPRESS=$COMPRESS
	echo filesizes=\"\$filesizes\"
    echo totalsize=\"\$totalsize\"
	echo CRCsum=\"\$CRCsum\"
	echo MD5sum=\"\$MD5sum\"
	echo SHAsum=\"\$SHAsum\"
	echo SKIP=\"\$skip\"
	exit 0
	;;
    --lsm)
cat << EOLSM
EOF
eval "$LSM_CMD"
cat << EOF  >> "$archname"
EOLSM
	exit 0
	;;
    --list)
	echo Target directory: \$targetdir
	offset=\`head -n "\$skip" "\$0" | wc -c | sed "s/ //g"\`
	for s in \$filesizes
	do
	    MS_dd "\$0" \$offset \$s | MS_Decompress | UnTAR t
	    offset=\`expr \$offset + \$s\`
	done
	exit 0
	;;
	--tar)
	offset=\`head -n "\$skip" "\$0" | wc -c | sed "s/ //g"\`
	arg1="\$2"
    shift 2 || { MS_Help; exit 1; }
	for s in \$filesizes
	do
	    MS_dd "\$0" \$offset \$s | MS_Decompress | tar "\$arg1" - "\$@"
	    offset=\`expr \$offset + \$s\`
	done
	exit 0
	;;
    --check)
	MS_Check "\$0" y
	exit 0
	;;
    --verify-sig)
    sig_key="\$2"
    shift 2 || { MS_Help; exit 1; }
    MS_Verify_Sig "\$0"
    ;;
    --confirm)
	verbose=y
	shift
	;;
	--noexec)
	script=""
    cleanup_script=""
	shift
	;;
    --noexec-cleanup)
    cleanup_script=""
    shift
    ;;
    --keep)
	keep=y
	shift
	;;
    --target)
	keep=y
	targetdir="\${2:-.}"
    shift 2 || { MS_Help; exit 1; }
	;;
    --noprogress)
	noprogress=y
	shift
	;;
    --nox11)
	nox11=y
	shift
	;;
    --nochown)
	ownership=n
	shift
	;;
    --chown)
        ownership=y
        shift
        ;;
    --nodiskspace)
	nodiskspace=y
	shift
	;;
    --xwin)
	if test "$NOWAIT" = n; then
		finish="echo Press Return to close this window...; read junk"
	fi
	xterm_loop=1
	shift
	;;
    --phase2)
	copy=phase2
	shift
	;;
	--ssl-pass-src)
	if test x"$ENCRYPT" != x"openssl"; then
	    echo "Invalid option --ssl-pass-src: \$0 was not encrypted with OpenSSL!" >&2
	    exit 1
	fi
	decrypt_cmd="\$decrypt_cmd -pass \$2"
    shift 2 || { MS_Help; exit 1; }
	;;
    --cleanup-args)
    cleanupargs="\$2"
    shift 2 || { MS_Help; exit 1; }
    ;;
    --)
	shift
	break ;;
    -*)
	echo Unrecognized flag : "\$1" >&2
	MS_Help
	exit 1
	;;
    *)
	break ;;
    esac
done

if test x"\$quiet" = xy -a x"\$verbose" = xy; then
	echo Cannot be verbose and quiet at the same time. >&2
	exit 1
fi

if test x"$NEED_ROOT" = xy -a \`id -u\` -ne 0; then
	echo "Administrative privileges required for this archive (use su or sudo)" >&2
	exit 1	
fi

if test x"\$copy" \!= xphase2; then
    MS_PrintLicense
fi

case "\$copy" in
copy)
    tmpdir="\$TMPROOT"/makeself.\$RANDOM.\`date +"%y%m%d%H%M%S"\`.\$\$
    mkdir "\$tmpdir" || {
	echo "Could not create temporary directory \$tmpdir" >&2
	exit 1
    }
    SCRIPT_COPY="\$tmpdir/makeself"
    echo "Copying to a temporary location..." >&2
    cp "\$0" "\$SCRIPT_COPY"
    chmod +x "\$SCRIPT_COPY"
    cd "\$TMPROOT"
    export USER_PWD="\$tmpdir"
    exec "\$SCRIPT_COPY" --phase2 -- \$initargs
    ;;
phase2)
    finish="\$finish ; rm -rf \`dirname \$0\`"
    ;;
esac

if test x"\$nox11" = xn; then
    if test -t 1; then  # Do we have a terminal on stdout?
	:
    else
        if test x"\$DISPLAY" != x -a x"\$xterm_loop" = x; then  # No, but do we have X?
            if xset q > /dev/null 2>&1; then # Check for valid DISPLAY variable
                GUESS_XTERMS="xterm gnome-terminal rxvt dtterm eterm Eterm xfce4-terminal lxterminal kvt konsole aterm terminology"
                for a in \$GUESS_XTERMS; do
                    if type \$a >/dev/null 2>&1; then
                        XTERM=\$a
                        break
                    fi
                done
                chmod a+x \$0 || echo Please add execution rights on \$0 >&2
                if test \`echo "\$0" | cut -c1\` = "/"; then # Spawn a terminal!
                    exec \$XTERM -e "\$0 --xwin \$initargs"
                else
                    exec \$XTERM -e "./\$0 --xwin \$initargs"
                fi
            fi
        fi
    fi
fi

if test x"\$targetdir" = x.; then
    tmpdir="."
else
    if test x"\$keep" = xy; then
	if test x"\$nooverwrite" = xy && test -d "\$targetdir"; then
            echo "Target directory \$targetdir already exists, aborting." >&2
            exit 1
	fi
	if test x"\$quiet" = xn; then
	    echo "Creating directory \$targetdir" >&2
	fi
	tmpdir="\$targetdir"
	dashp="-p"
    else
	tmpdir="\$TMPROOT/selfgz\$\$\$RANDOM"
	dashp=""
    fi
    mkdir \$dashp "\$tmpdir" || {
	echo 'Cannot create target directory' \$tmpdir >&2
	echo 'You should try option --target dir' >&2
	eval \$finish
	exit 1
    }
fi

location="\`pwd\`"
if test x"\$SETUP_NOCHECK" != x1; then
    MS_Check "\$0"
fi
offset=\`head -n "\$skip" "\$0" | wc -c | sed "s/ //g"\`

if test x"\$verbose" = xy; then
	MS_Printf "About to extract $USIZE KB in \$tmpdir ... Proceed ? [Y/n] "
	read yn
	if test x"\$yn" = xn; then
		eval \$finish; exit 1
	fi
fi

if test x"\$quiet" = xn; then
    # Decrypting with openssl will ask for password,
    # the prompt needs to start on new line
	if test x"$ENCRYPT" = x"openssl"; then
	    echo "Decrypting and uncompressing \$label..."
	else
        MS_Printf "Uncompressing \$label"
	fi
fi
res=3
if test x"\$keep" = xn; then
    trap MS_cleanup 1 2 3 15
fi

if test x"\$nodiskspace" = xn; then
    leftspace=\`MS_diskspace "\$tmpdir"\`
    if test -n "\$leftspace"; then
        if test "\$leftspace" -lt $USIZE; then
            echo
            echo "Not enough space left in "\`dirname \$tmpdir\`" (\$leftspace KB) to decompress \$0 ($USIZE KB)" >&2
            echo "Use --nodiskspace option to skip this check and proceed anyway" >&2
            if test x"\$keep" = xn; then
                echo "Consider setting TMPDIR to a directory with more free space."
            fi
            eval \$finish; exit 1
        fi
    fi
fi

for s in \$filesizes
do
    if MS_dd_Progress "\$0" \$offset \$s | MS_Decompress | ( cd "\$tmpdir"; umask \$ORIG_UMASK ; UnTAR xp ) 1>/dev/null; then
		if test x"\$ownership" = xy; then
			(cd "\$tmpdir"; chown -R \`id -u\` .;  chgrp -R \`id -g\` .)
		fi
    else
		echo >&2
		echo "Unable to decompress \$0" >&2
		eval \$finish; exit 1
    fi
    offset=\`expr \$offset + \$s\`
done
if test x"\$quiet" = xn; then
	echo
fi

cd "\$tmpdir"
res=0
if test x"\$script" != x; then
    if test x"\$export_conf" = x"y"; then
        MS_BUNDLE="\$0"
        MS_LABEL="\$label"
        MS_SCRIPT="\$script"
        MS_SCRIPTARGS="\$scriptargs"
        MS_ARCHDIRNAME="\$archdirname"
        MS_KEEP="\$KEEP"
        MS_NOOVERWRITE="\$NOOVERWRITE"
        MS_COMPRESS="\$COMPRESS"
        MS_CLEANUP="\$cleanup"
        export MS_BUNDLE MS_LABEL MS_SCRIPT MS_SCRIPTARGS
        export MS_ARCHDIRNAME MS_KEEP MS_NOOVERWRITE MS_COMPRESS
    fi

    if test x"\$verbose" = x"y"; then
		MS_Printf "OK to execute: \$script \$scriptargs \$* ? [Y/n] "
		read yn
		if test x"\$yn" = x -o x"\$yn" = xy -o x"\$yn" = xY; then
			eval "\"\$script\" \$scriptargs \"\\\$@\""; res=\$?;
		fi
    else
		eval "\"\$script\" \$scriptargs \"\\\$@\""; res=\$?
    fi
    if test "\$res" -ne 0; then
		test x"\$verbose" = xy && echo "The program '\$script' returned an error code (\$res)" >&2
    fi
fi

MS_exec_cleanup

if test x"\$keep" = xn; then
    cd "\$TMPROOT"
    rm -rf "\$tmpdir"
fi
eval \$finish; exit \$res
EOF
