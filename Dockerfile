FROM alpine

WORKDIR /app

COPY ./freenews .
COPY ./config.toml .

EXPOSE 53
EXPOSE 80
EXPOSE 443
EXPOSE 853

CMD ["./freenews"]
