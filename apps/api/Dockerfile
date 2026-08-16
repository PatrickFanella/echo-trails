FROM golang:1.23-bookworm AS build
WORKDIR /src
COPY apps/api/go.mod apps/api/go.sum ./
RUN go mod download
COPY apps/api/ ./
RUN CGO_ENABLED=0 GOOS=linux go build -trimpath -ldflags="-s -w" -o /out/echotrails-api ./cmd/api
RUN CGO_ENABLED=0 GOOS=linux go build -trimpath -ldflags="-s -w" -o /out/echotrails-bootstrap-admin ./cmd/bootstrap-admin

FROM gcr.io/distroless/static-debian12:nonroot
COPY --from=build /out/echotrails-api /usr/local/bin/echotrails-api
COPY --from=build /out/echotrails-bootstrap-admin /usr/local/bin/echotrails-bootstrap-admin
EXPOSE 8080
ENTRYPOINT ["/usr/local/bin/echotrails-api"]
