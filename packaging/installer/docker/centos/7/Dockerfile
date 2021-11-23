FROM centos:7

RUN yum install -y git

RUN git clone https://github.com/netdata/netdata.git -b packaging-split

RUN yum install -y epel-release

RUN yum makecache

RUN ./netdata/packaging/installer/dependencies/centos.sh

RUN ./netdata/packaging/installer/kickstart.sh
