FROM registry.suse.com/bci/golang:1.26@sha256:9b829b1026dac4281baa0715c4dd975735665d5ff843b0e1cd20e3fbbde95302 AS builder
WORKDIR /app
COPY . .
RUN go mod download
RUN go build -o /app/tcp-client main.go

FROM registry.suse.com/bci/bci-micro:15.7@sha256:bda48a632ca318ff8b38ac3374b8928283c4e4145bcb2b046fd0369f97865fb9
COPY --from=builder /app/tcp-client .

RUN ls .

CMD ["./tcp-client"]
