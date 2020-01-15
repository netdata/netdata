FROM centos:8
RUN sed -ie 's/enabled=0/enabled=1/' /etc/yum.repos.d/CentOS-PowerTools.repo
RUN yum update -y
RUN yum install -y epel-release
RUN yum update -y
RUN rpm -ivh http://repo.okay.com.mx/centos/8/x86_64/release/okay-release-1-3.el8.noarch.rpm
RUN yum update -y
RUN yum install -y autoconf automake curl gcc git libmnl-devel libuuid-devel openssl-devel libuv-devel lz4-devel make nc pkgconfig python36 zlib-devel
RUN curl -L https://downloads.sourceforge.net/project/judy/judy/Judy-1.0.5/Judy-1.0.5.tar.gz -o /opt/judy-1.0.5.tgz 
