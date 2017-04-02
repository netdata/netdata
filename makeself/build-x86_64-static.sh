#!/usr/bin/env sh

set -e

if ! sudo docker inspect netdata-package-x86_64-static > /dev/null 2>&1
then
  # To run interactively:
  #   sudo docker run -it netdata-package-x86_64-static /bin/sh
  # (add -v host-dir:guest-dir:rw arguments to mount volumes)
  # To remove images in order to re-create:
  #   sudo docker rm -v $(sudo docker ps -a -q -f status=exited)
  #   sudo docker rmi netdata-package-x86_64-static
  sudo docker run -v `pwd`:/usr/src/netdata.git:rw alpine:3.5 \
              /bin/sh /usr/src/netdata.git/makeself/setup-x86_64-static.sh
  id=`sudo docker ps -l -q`
  sudo docker commit $id netdata-package-x86_64-static
 fi
sudo docker run -v `pwd`:/usr/src/netdata.git:rw netdata-package-x86_64-static \
            /bin/sh /usr/src/netdata.git/makeself/build.sh

if [ "$USER" ]
then
  sudo chown -R "$USER" .
fi
