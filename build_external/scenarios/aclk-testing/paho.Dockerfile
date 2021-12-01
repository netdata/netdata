FROM archlinux/base:latest

RUN pacman -Syyu --noconfirm 
RUN pacman --noconfirm --needed -S python-pip

RUN pip install paho-mqtt

RUN mkdir -p /opt/paho
COPY paho-inspection.py /opt/paho/

WORKDIR /opt/paho
CMD ["/usr/sbin/python", "paho-inspection.py"]