FROM oraclelinux:8

#RUN sed -i "s/enabled=.*/enabled=1/g" /etc/yum.repos.d/oracle-linux-ol8.repo

RUN dnf config-manager --set-enabled ol8_codeready_builder 

RUN dnf makecache --refresh

RUN dnf install -y git

RUN git clone https://github.com/netdata/netdata.git -b packaging-split

RUN ./netdata/packaging/installer/dependencies/ol.sh

RUN ./netdata/packaging/installer/kickstart.sh
