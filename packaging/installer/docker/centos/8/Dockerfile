FROM centos:8

#RUN sed -i "s/enabled=.*/enabled=1/g" /etc/yum.repos.d/CentOS-Linux-Plus.repo

RUN sed -i "s/enabled=.*/enabled=1/g" /etc/yum.repos.d/CentOS-Linux-PowerTools.repo

RUN dnf makecache --refresh

RUN dnf install -y git

RUN git clone https://github.com/netdata/netdata.git -b packaging-split

RUN ./netdata/packaging/installer/dependencies/centos.sh

RUN ./netdata/packaging/installer/kickstart.sh
