FROM registry.suse.com/bci/golang:1.26@sha256:f6f04957fbc16399c76b1e8266cc7d0f3b4a5434b824fb251d7e3ae559d6a910 AS builder
WORKDIR /app
COPY . .
RUN go mod download
RUN go build -o /app/tcp-client main.go

FROM registry.suse.com/bci/bci-micro:16.1@sha256:c01e87816c4da88cb3cea6f0b78a35857fade15fd358d310900bfd2d0fc1204f
COPY --from=builder /app/tcp-client .

RUN ls .

CMD ["./tcp-client"]
