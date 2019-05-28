#!/bin/bash
# shellcheck disable=SC2181

# Doc link checker
# Validates and tries to fix all links that will cause issues either in the repo, or in the html site

GENERATOR_DIR="docs/generator"

dbg () {
	if [ "$VERBOSE" -eq 1 ] ; then printf "%s\\n" "${1}" ; fi
}

printhelp () {
	echo "Usage: docs/generator/checklinks.sh [-r OR -f <fname>] [OPTIONS]
	-r Recursively check all mds in all child directories, except docs/generator and node_modules (which is generatord by netlify)
	-f Just check the passed md file
	General Options:
	 -x Execute commands. By default the script runs in test mode with no files changed by the script (results and fixes are just shown). Use -x to have it apply the changes.
	 -u trys to follow URLs using curl
	 -v Outputs debugging messages
	By default, nothing is actually checked. The following options tell it what to check:
	 -a Check all link types
	 -w Check wiki links (and just warn if you see one)
	 -b Check absolute links to the Netdata repo (and change them to relative). Only checks links to https://github.com/netdata/netdata/????/master*
	 -l Check relative links to the Netdata repo (and replace them with links that the html static site can live with, under docs/generator/src only)
	 -e Check external links, outside the wiki or the repo (useless without adding the -u option, to verify that they're not broken)
	"
}

fix () {
	if [ "$EXECUTE" -eq 0 ] ; then
		echo "-- SHOULD EXECUTE: $1"
	else
		dbg "-- EXECUTING: $1"
		eval "$1"
	fi
}

testURL () {
	if [ "$TESTURLS" -eq 0 ] ; then return 0 ; fi
	dbg "-- Testing URL $1"
	curl -sS "$1" > /dev/null
	if [ $? -gt 0 ] ; then
		return 1
	fi
	return 0
}

testinternal () {
	# Check if the header referred to by the internal link exists in the same file
	ff=${1}
	ifile=${2}
	ilnk=${3}
	header=${ilnk//-/}
	dbg "-- Searching for \"$header\" in $ifile"
	tr -d '[],_.:? `'< "$ifile" | sed 's/-//g' | grep -i "^\\#*$header\$" >/dev/null
	if [ $? -eq 0 ] ; then
		dbg "-- $ilnk found in $ifile"
		return 0
	else
		echo "-- ERROR: $ff - $ilnk header not found in file $ifile"
		EXITCODE=1
		return 1
	fi
}

testf () {
	sf=$1
	tf=$2
	
	if [ -f "$tf" ] ; then 
		dbg "-- $tf exists"
		return 0
	else
		echo "-- ERROR: $sf - $tf does not exist"
		EXITCODE=1
		return 1
	fi	
}

ck_netdata_relative () {
	f=${1}
	rlnk=${2}
	dbg "-- Checking relative link $rlnk"
	fpath="."
	fname="$f"
	# First ensure that the link works in the repo, then try to fix it in htmldocs
	if [[ $f =~ ^(.*)/([^/]*)$ ]] ; then
		fpath="${BASH_REMATCH[1]}"
		fname="${BASH_REMATCH[2]}"
		dbg "-- Current file is at $fpath"
	else
		dbg "-- Current file is at root directory"
	fi
	# Cases to handle:
	# (#somelink)
	# (path/)
	# (path/#somelink)
	# (path/filename.md) -> htmldoc (path/filename/)
	# (path/filename.md#somelink) -> htmldoc (path/filename/#somelink)
	# (path#somelink) -> htmldoc (path/#somelink)
	# (path/someotherfile) -> htmldoc (absolutelink) 
	# (path) -> htmldoc (path/)

	TRGT=""
	s=""

	case "$rlnk" in
		\#* ) 
			dbg "-- # (#somelink)"
			testinternal "$f" "$f" "$rlnk"
			;;
		*/ ) 
			dbg "-- # (path/)"
			TRGT="$fpath/${rlnk}README.md"
			testf "$f" "$TRGT"
			if [ $? -eq 0 ] ; then
				if [ "$fname" != "README.md" ] ; then s="../$rlnk"; fi
			fi
			;;
		*/\#* )
			dbg "-- # (path/#somelink)"
			if [[ $rlnk =~ ^(.*)/#(.*)$ ]] ; then
				TRGT="$fpath/${BASH_REMATCH[1]}/README.md"
				LNK="#${BASH_REMATCH[2]}"
				dbg "-- Look for $LNK in $TRGT"
				testf "$f" "$TRGT"
				if [ $? -eq 0 ] ; then
					testinternal "$f" "$TRGT" "$LNK"
					if [ $? -eq 0 ] ; then
						if [ "$fname" != "README.md" ] ; then s="../$rlnk"; fi
					fi
				fi
			fi
			;;
		*.md )
			dbg "-- # (path/filename.md) -> htmldoc (path/filename/)"
			testf "$f" "$fpath/$rlnk"
			if [ $? -eq 0 ] ; then
				if [[ $rlnk =~ ^(.*)/(.*).md$ ]] ; then
					if [ "${BASH_REMATCH[2]}" = "README" ] ; then
						s="../${BASH_REMATCH[1]}/"
					else
						s="../${BASH_REMATCH[1]}/${BASH_REMATCH[2]}/"
					fi
					if [ "$fname" != "README.md" ] ; then s="../$s"; fi
				fi
			fi
			;;
		*.md\#* )
			dbg "-- # (path/filename.md#somelink) -> htmldoc (path/filename/#somelink)"
			if [[ $rlnk =~ ^(.*)#(.*)$ ]] ; then
				TRGT="$fpath/${BASH_REMATCH[1]}"
				LNK="#${BASH_REMATCH[2]}"
				testf "$f" "$TRGT"
				if [ $? -eq 0 ] ; then
					testinternal "$f" "$TRGT" "$LNK"
					if [ $? -eq 0 ] ; then
						if [[ $lnk =~ ^(.*)/(.*).md#(.*)$ ]] ; then
							if [ "${BASH_REMATCH[2]}" = "README" ] ; then
								s="../${BASH_REMATCH[1]}/#${BASH_REMATCH[3]}"
							else
								s="../${BASH_REMATCH[1]}/${BASH_REMATCH[2]}/#${BASH_REMATCH[3]}"
							fi
							if [ "$fname" != "README.md" ] ; then s="../$s"; fi
						fi			
					fi
				fi
			fi
			;;
		*\#* )
			dbg "-- # (path#somelink) -> (path/#somelink)"
			if [[ $rlnk =~ ^(.*)#(.*)$ ]] ; then
				TRGT="$fpath/${BASH_REMATCH[1]}/README.md"
				LNK="#${BASH_REMATCH[2]}"
				testf "$f" "$TRGT"
				if [ $? -eq 0 ] ; then
					testinternal "$f" "$TRGT" "$LNK"
					if [ $? -eq 0 ] ; then
						if [[ $rlnk =~ ^(.*)#(.*)$ ]] ; then
							s="${BASH_REMATCH[1]}/#${BASH_REMATCH[2]}"
							if [ "$fname" != "README.md" ] ; then s="../$s"; fi
						fi			
					fi
				fi
			fi
			;;
		* )
			if [ -f "$fpath/$rlnk" ] ; then
				dbg "-- # (path/someotherfile) $rlnk"
				if [ "$fpath" = "." ] ; then
					s="https://github.com/netdata/netdata/tree/master/$rlnk"
				else
					s="https://github.com/netdata/netdata/tree/master/$fpath/$rlnk"
				fi
			else
				if [ -d "$fpath/$rlnk" ] ; then
					dbg "-- # (path) -> htmldoc (path/)"
					testf "$f" "$fpath/$rlnk/README.md"
					if [ $? -eq 0 ] ; then
						s="$rlnk/"
						if [ "$fname" != "README.md" ] ; then s="../$s"; fi
					fi
				else
					echo "-- ERROR: $f - $rlnk is neither a file or a directory. Giving up!"
					EXITCODE=1
				fi
			fi
			;;
		esac
		
		if [[ ! -z $s ]] ; then
			srch=$(echo "$rlnk" | sed 's/\//\\\//g')
			rplc=$(echo "$s" | sed 's/\//\\\//g')
			fix "sed -i 's/($srch)/($rplc)/g' $GENERATOR_DIR/doc/$f"
		fi
}


checklinks () {
	f=$1
	dbg "Checking $f"
	while read -r l ; do
		for word in $l ; do
			if [[ $word =~ .*\]\(([^\(\) ]*)\).* ]] ; then
				lnk="${BASH_REMATCH[1]}"
				if [ -z "$lnk" ] ; then continue ; fi
				dbg "-$lnk"
				case "$lnk" in
					mailto:* ) dbg "-- Mailto link, ignoring" ;;
					https://github.com/netdata/netdata/wiki* )
						dbg "-- Wiki Link $lnk"
						if [ "$CHKWIKI" -eq 1 ] ; then echo "-- WARNING: $f - $lnk points to the wiki. Please replace it manually" ; fi
					;;
					https://github.com/netdata/netdata/????/master* )
						echo "-- ERROR: $f - $lnk is an absolute link to a Netdata file. Please convert to relative."
						EXITCODE=1
					;;
					http* ) 
						dbg "-- External link $lnk"
						if [ "$CHKEXTERNAL" -eq 1 ] ; then 
							testURL "$lnk"
							if [ $? -eq 1 ] ; then
								echo "-- ERROR: $f - $lnk is a broken link"
								EXITCODE=1
							fi
						fi
					;;
					* ) 
						dbg "-- Relative link $lnk"
						if [ "$CHKRELATIVE" -eq 1 ] ; then ck_netdata_relative "$f" "$lnk" ; fi 
					;;
				esac
			fi
		done
	done < "$f"
}

TESTURLS=0
VERBOSE=0
RECURSIVE=0
EXECUTE=0
CHKWIKI=0
CHKABSOLUTE=0
CHKEXTERNAL=0
CHKRELATIVE=0
while getopts :f:rxuvwbela option
do
    case "$option" in
    f)
         file=$OPTARG
         ;;
	r)
		RECURSIVE=1
		;;
	x)
		EXECUTE=1
		;;
	u) 
		TESTURLS=1
		;;
	v)
		VERBOSE=1
		;;
	w)
		CHKWIKI=1
		;;
	b)
		CHKABSOLUTE=1
		;;
	e)
		CHKEXTERNAL=1
		;;
	l)
		CHKRELATIVE=1
		;;	
	a)
		CHKWIKI=1
		CHKABSOLUTE=1
		CHKEXTERNAL=1
		CHKRELATIVE=1
		;;
	*)
		printhelp
		exit 1
		;;
	esac
done

EXITCODE=0

if [ -z "${file}" ] ; then 
	if [ $RECURSIVE -eq 0 ] ; then 
		printhelp
		exit 1
	fi
	for f in $(find . -type d \( -path ./${GENERATOR_DIR} -o -path ./node_modules \) -prune -o -name "*.md" -print); do
		checklinks "$f"
	done
else
	if [ $RECURSIVE -eq 1 ] ; then 
		printhelp
		exit 1
	fi	
	checklinks "$file"
fi

exit $EXITCODE
