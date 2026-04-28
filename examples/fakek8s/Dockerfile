FROM registry.suse.com/bci/golang:1.26@sha256:79ab11123495ceeeeee1155bd164772f9bfca3050763338beaeeb645643b661c AS builder
WORKDIR /app
COPY . .
RUN go mod download
RUN go build -o /app/tcp-client main.go

FROM registry.suse.com/bci/bci-micro:15.7@sha256:bda48a632ca318ff8b38ac3374b8928283c4e4145bcb2b046fd0369f97865fb9
COPY --from=builder /app/tcp-client .

RUN ls .

CMD ["./tcp-client"]
