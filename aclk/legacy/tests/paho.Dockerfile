FROM archlinux/base:latest

RUN pacman -Syyu --noconfirm
RUN pacman --noconfirm --needed -S python-pip

RUN pip install paho-mqtt

RUN mkdir -p /opt/paho
COPY paho-inspection.py /opt/paho/

WORKDIR /opt/paho
ARG HOST_HOSTNAME
RUN echo $HOST_HOSTNAME >host
CMD ["/bin/bash", "-c", "/usr/sbin/python paho-inspection.py $(cat host)"]
