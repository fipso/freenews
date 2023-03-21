FROM alpine

WORKDIR /app

ADD ./freenews .
ADD ./config.toml .

# DNS Ports
EXPOSE 53/tcp
EXPOSE 53/udp
EXPOSE 853/tcp
EXPOSE 853/udp

# HTTP Ports
EXPOSE 80/tcp
EXPOSE 443/tcp

ENTRYPOINT ["./freenews"]
