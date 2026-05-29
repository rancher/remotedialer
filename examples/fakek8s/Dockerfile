FROM registry.suse.com/bci/golang:1.26@sha256:9b829b1026dac4281baa0715c4dd975735665d5ff843b0e1cd20e3fbbde95302 AS builder
WORKDIR /app
COPY . .
RUN go mod download
RUN go build -o /app/tcp-client main.go

FROM registry.suse.com/bci/bci-micro:16.1@sha256:86c3c55a0e8b19e951076fc117ee0604d744a5e9c05e171bcca8a43c937fcc84
COPY --from=builder /app/tcp-client .

RUN ls .

CMD ["./tcp-client"]
