#!/bin/bash

# Doc link checker
# Validates and tries to fix all links that will cause issues either in the repo, or in the html site

testlink () {
	if [ $TESTURLS -eq 0 ] ; then return ; fi
	if [ $VERBOSE -eq 1 ] ; then echo " Testing URL $1" ; fi
	curl -sS $1 > /dev/null
	if [ $? -gt 0 ] ; then
		echo " ERROR: $1 is a broken link"
	fi
}

testinternal () {
	# Check if the header referred to by the internal link exists in the same file
	f=${1}
	lnk=${2}
	header=$(echo $lnk | sed 's/#/# /g' | sed 's/-/ /g')
	if [ $VERBOSE -eq 1 ] ; then echo " Searching for \"$header\" in $f"; fi
	grep -i "^\#*$header\$" $f >/dev/null
	if [ $? -eq 0 ] ; then
		if [ $VERBOSE -eq 1 ] ; then echo " $lnk found in $f"; fi
	else
		echo " ERROR: $lnk header not found in $f"
	fi

}

ck_netdata_relative () {
	f=${1}
	lnk=${2}
	if [ $VERBOSE -eq 1 ] ; then echo " Checking relative link $lnk"; fi
	if [ $ISREADME -eq 0 ] ; then
		case "$lnk" in
			\#* ) testinternal $f $lnk ;;
			* ) echo " ERROR: $lnk is a relative link in a Non-README file. Either convert the file to a README under a directory, or replace the relative link with an absolute one" ;;
		esac
	else
		case "$lnk" in
			\#* ) testinternal $f $lnk ;;
			*/*[^/]#* ) 
				echo " ERROR: $lnk - relative directory followed immediately by \# will break html." 
				newlnk=$(echo $lnk | sed 's/#/\/#/g')
				echo " FIX: Replace $lnk with $newlnk"
			;;
		esac
	fi
}

ck_netdata_absolute () {
	f=${1}
	lnk=${2}
	
	testlink $lnk
	case "$lnk" in
		*\/*.md* ) echo "ERROR: $lnk points to an md file, html will break" ;;
		* ) echo " WARNING: $lnk is an absolute link" ;;
	esac
	if [[ $l =~ https://github.com/netdata/netdata/..../master/(.*) ]] ; then
		abspath="${BASH_REMATCH[1]}"
		echo " abspath: $abspath"
	fi
}

checklinks () {
	f=$1
	echo "Processing $f"
	if [[ $f =~ .*README\.md ]] ; then
		ISREADME=1
		if [ $VERBOSE -eq 1 ] ; then echo "README file" ; fi
	else
		ISREADME=0
		if [ $VERBOSE -eq 1 ] ; then echo "WARNING: Not a README file. Links inside this file must remain absolute"; fi
	fi
	while read l ; do
		if [[ $l =~ .*\[[^\[]*\]\(([^\( ]*)\).* ]] ; then
			lnk="${BASH_REMATCH[1]}"
			if [ $VERBOSE -eq 1 ] ; then echo "$lnk"; fi
			case "$lnk" in
				https://github.com/netdata/netdata/wiki* ) 
					testlink $lnk
					echo " WARNING: $lnk points to the wiki" 
				;;
				https://github.com/netdata/netdata/* ) ck_netdata_absolute $f $lnk;;
				http* ) testlink $lnk ;;
				* ) ck_netdata_relative $f $lnk ;;
			esac
		fi
	done < $f
}

REPLACE=0
TESTURLS=0
VERBOSE=0
while getopts :vtf:r: option
do
    case "$option" in
    f)
         file=$OPTARG
         ;;
	r)
		REPLACE=1
		;;
	t) 
		TESTURLS=1
		;;
	v)
		VERBOSE=1
		;;
	*)
		echo "Usage: htmldoc/checklinks.sh [-f <fname>] [-r]
	If no file is passed, recursively checks all files.
	-r option causes the link to be replaced with a proper link, where possible
	-t tests all absolute URLs
"
		;;
	esac
done

if [ -z ${file} ] ; then 
	for f in $(find . -type d \( -path ./htmldoc -o -path ./node_modules \) -prune -o -name "*.md" -print); do
		checklinks $f
	done
else
	checklinks $file
fi

