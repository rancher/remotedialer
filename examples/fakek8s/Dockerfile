FROM registry.suse.com/bci/golang:1.26@sha256:ffe330184fb07e2e2c089b73229eaaec7085ce802f965b80c84780f163a5f062 AS builder
WORKDIR /app
COPY . .
RUN go mod download
RUN go build -o /app/tcp-client main.go

FROM registry.suse.com/bci/bci-micro:16.1@sha256:86c3c55a0e8b19e951076fc117ee0604d744a5e9c05e171bcca8a43c937fcc84
COPY --from=builder /app/tcp-client .

RUN ls .

CMD ["./tcp-client"]
