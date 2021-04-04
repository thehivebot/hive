FROM golang:1.15-alpine as build

RUN apk add --no-cache git

COPY ./ /go/src/github.com/thehivebot/hive

WORKDIR /go/src/github.com/thehivebot/hive

RUN go build -ldflags "-X main.revision=$(git rev-parse --short HEAD)" ./cmd/hive/

FROM alpine:3.12

RUN apk add --no-cache ca-certificates

RUN mkdir -p /go/src/github.com/thehivebot/hive/
WORKDIR /go/src/github.com/thehivebot/hive/

COPY ./config.json /go/src/github.com/thehivebot/hive/

COPY --from=build /go/src/github.com/thehivebot/hive/hive /usr/local/bin/

CMD [ "/usr/local/bin/hive", "serve" ]
