FROM centos:7

RUN rpm -ivh http://repo.okay.com.mx/centos/7/x86_64/release/okay-release-1-5.el7.noarch.rpm

RUN yum makecache

RUN yum install -y git

RUN git clone https://github.com/netdata/netdata.git -b packaging-split

RUN ./netdata/packaging/installer/dependencies/centos.sh

RUN ./netdata/packaging/installer/kickstart.sh
