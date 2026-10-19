FROM registry.suse.com/bci/golang:1.27@sha256:3ec2874a5d147caa56f24b826f1425d81ac35f62cad2c8424acfaa4dba48397a AS builder
WORKDIR /app
COPY . .
RUN go mod download
RUN go build -o /app/tcp-client main.go

FROM registry.suse.com/bci/bci-micro:16.1@sha256:8e774758e9a9b081b777f4290a0779bdfe9d44f662793dd9fd1c5e79c8c5a156
COPY --from=builder /app/tcp-client .

RUN ls .

CMD ["./tcp-client"]
