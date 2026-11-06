FROM golang:1.24-bookworm AS build
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN go build -trimpath -ldflags "-s -w" -o /go-bot ./cmd/go-bot

FROM debian:bookworm-slim
RUN useradd -m -u 10001 gobird 2>/dev/null || useradd -m -u 10001 gobot; mkdir -p /data && chown 10001 /data
COPY --from=build /go-bot /usr/local/bin/go-bot
USER 10001
EXPOSE 7417
ENTRYPOINT ["/usr/local/bin/go-bot"]
