FROM golang:1.16 AS build

WORKDIR /go/src/container_state_exporter
COPY . /go/src/container_state_exporter

RUN GOOS=linux GOARCH=amd64 CGO_ENABLED=1 \
    go build -v \
    -o /bin/container_state_exporter

WORKDIR  /bin

CMD ["./container_state_exporter"]

