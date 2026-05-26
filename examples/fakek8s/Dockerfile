FROM registry.suse.com/bci/golang:1.26@sha256:59204a1a28ed065b75c8e3e8dabbe120d2b31dab820cddb67f0bee1ae8d6421b AS builder
WORKDIR /app
COPY . .
RUN go mod download
RUN go build -o /app/tcp-client main.go

FROM registry.suse.com/bci/bci-micro:16.1@sha256:6118356af3b03034749a78ffe22b30e3be3d45382afafb7779b6c2cbe3c31ee5
COPY --from=builder /app/tcp-client .

RUN ls .

CMD ["./tcp-client"]
