FROM vernemq:latest

COPY vernemq.conf /vernemq/etc/vernemq.conf
WORKDIR /vernemq
RUN openssl req -newkey rsa:2048 -nodes -keyout key.pem -x509 -days 365 -out certificate.pem -subj '/CN=vernemq'
RUN chown vernemq:vernemq /vernemq/*.pem
RUN ls -l /vernemq
RUN grep listener /vernemq/etc/vernemq.conf
