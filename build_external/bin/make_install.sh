#!/usr/bin/env bash

Distro=$1
Version=$2
OutBase=$(cd $(dirname $0) && cd .. && pwd)
InBase=/opt/netdata
Config="$Distro"_"$Version"
ImageName="$Config"_dev
Dockerfile=$OutBase/"$Distro"_"$Version"_preinst.Dockerfile
AbsSrc=$(readlink $OutBase/netdata || echo $OutBase/netdata)

if [ "$Config" != $(cat $AbsSrc/build_external/current) ]
then
  echo "Can't run installer on $Distro/$Version - $Dockerfile not found"
  return -1
fi

cat <<EOF >$AbsSrc/build/rebuild.sh
#!/usr/bin/env bash
make
make install
EOF
chmod +x $AbsSrc/build/rebuild.sh


docker rm $ImageName 2>/dev/null

docker run -it --mount type=bind,source="$AbsSrc",target=$InBase/source \
           --name $ImageName \
           -w $InBase/source $ImageName \
           $InBase/source/build/rebuild.sh 
