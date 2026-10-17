FROM registry.suse.com/bci/golang:1.26@sha256:3ec2874a5d147caa56f24b826f1425d81ac35f62cad2c8424acfaa4dba48397a AS builder
WORKDIR /app
COPY . .
RUN go mod download
RUN go build -o /app/tcp-client main.go

FROM registry.suse.com/bci/bci-micro:16.1@sha256:630051b14c09f03c92a4497717424ec9e3f2cdc65e38a76a4db3be98978c6bb7
COPY --from=builder /app/tcp-client .

RUN ls .

CMD ["./tcp-client"]
