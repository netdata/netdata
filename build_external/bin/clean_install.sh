#!/usr/bin/env bash

# Basic setup, check we have a legal Distro / Version combo
Distro=$1
Version=$2
OutBase=$(cd $(dirname $0) && cd .. && pwd)
InBase=/opt/netdata
ImageNameBefore="$Distro"_"$Version"_preinst
ImageNameAfter="$Distro"_"$Version"_dev
Dockerfile=$OutBase/"$Distro"_"$Version"_preinst.Dockerfile
AbsSrc=$(readlink $OutBase/netdata || echo $OutBase/netdata)

if [ ! -e "$Dockerfile" ]
then
  echo "Can't run installer on $Distro/$Version - $Dockerfile not found"
  return -1
fi

# Create the build-state and store the current config.
rm -rf $AbsSrc/build_external
mkdir $AbsSrc/build_external
cat <<EOF >$AbsSrc/build_external/clean_install.sh
#!/usr/bin/env bash
$InBase/source/netdata-installer.sh --install $InBase/install --dont-wait
ln -sf /dev/stdout $InBase/install/netdata/var/log/netdata/access.log
ln -sf /dev/stdout $InBase/install/netdata/var/log/netdata/debug.log
ln -sf /dev/stderr $InBase/install/netdata/var/log/netdata/error.log
EOF
chmod +x $AbsSrc/build_external/clean_install.sh
echo "$Distro"_"$Version" > $AbsSrc/build_external/current

# Check the tag name is vacant.
docker rm $ImageNameAfter 2>/dev/null

# Run the install and wire up the logs as container output.
docker run -it --mount type=bind,source="$AbsSrc",target=$InBase/source \
           --name $ImageNameAfter \
           -w $InBase/source $ImageNameBefore \
           $InBase/source/build_external/clean_install.sh
docker commit $ImageNameAfter $ImageNameAfter:latest
