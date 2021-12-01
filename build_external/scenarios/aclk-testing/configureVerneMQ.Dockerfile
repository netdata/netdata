FROM vernemq:latest
EXPOSE 9002
COPY vernemq.conf /vernemq/etc/vernemq.conf
WORKDIR /vernemq
#RUN openssl req -newkey rsa:2048 -nodes -keyout server.key -x509 -days 365 -out server.crt -subj '/CN=vernemq'
RUN openssl req -newkey rsa:4096 -x509 -sha256 -days 3650 -nodes -out server.crt -keyout server.key -subj "/C=SK/ST=XX/L=XX/O=NetdataIsAwesome/OU=NotSupremeLeader/CN=netdata.cloud"
RUN chown vernemq:vernemq /vernemq/server.key /vernemq/server.crt
RUN cat /vernemq/etc/vernemq.conf
