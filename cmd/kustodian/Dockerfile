FROM alpine:3.16
RUN apk update --no-cache && apk upgrade --no-cache && apk add --no-cache ca-certificates tzdata
COPY ./kustodian /usr/bin/kustodian
ENTRYPOINT ["/usr/bin/kustodian"]
