FROM golang:1.24-trixie AS build

ADD . /src/friendbot
WORKDIR /src/friendbot
RUN go build -o /bin/friendbot .

FROM ubuntu:24.04

RUN apt-get update && DEBIAN_FRONTEND=noninteractive apt-get install -y --no-install-recommends ca-certificates
COPY --from=build /bin/friendbot /app/
EXPOSE 8004
ENTRYPOINT ["/app/friendbot"]
