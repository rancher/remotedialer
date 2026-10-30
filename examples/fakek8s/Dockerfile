FROM registry.suse.com/bci/golang:1.27@sha256:107c1d9c1be31452eac6d71afff3fc33144001e515f4f7e843ebbdeedc5cee12 AS builder
WORKDIR /app
COPY . .
RUN go mod download
RUN go build -o /app/tcp-client main.go

FROM registry.suse.com/bci/bci-micro:16.1@sha256:55782cf7f96a97b9f9574206393f82dc98a7e103b699081f5da826530630d452
COPY --from=builder /app/tcp-client .

RUN ls .

CMD ["./tcp-client"]
