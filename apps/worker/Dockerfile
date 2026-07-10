FROM golang:1.23-bookworm AS build
WORKDIR /src
COPY apps/worker/go.mod apps/worker/go.sum ./
RUN go mod download
COPY apps/worker/ ./
RUN CGO_ENABLED=0 GOOS=linux go build -trimpath -ldflags="-s -w" -o /out/echotrails-worker ./cmd/worker

FROM debian:bookworm-slim
RUN apt-get update && DEBIAN_FRONTEND=noninteractive apt-get install -y --no-install-recommends \
    ca-certificates clamav-daemon ffmpeg imagemagick && \
    rm -rf /var/lib/apt/lists/* && \
    printf 'TCPSocket 3310\nTCPAddr clamav\n' > /etc/clamav/clamd.conf
COPY --from=build /out/echotrails-worker /usr/local/bin/echotrails-worker
USER nobody:nogroup
ENTRYPOINT ["/usr/local/bin/echotrails-worker"]
