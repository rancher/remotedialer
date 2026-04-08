FROM registry.suse.com/bci/golang:1.24@sha256:ae722bb8954485d10eb26d90c14d521d90810d8b0c84c3c72b58cbb1fc1ee284 AS builder
WORKDIR /app
COPY . .
RUN go mod download
RUN go build -o /app/tcp-client main.go

FROM registry.suse.com/bci/bci-micro:15.7@sha256:5ca1a44ca5be8afd3e4abc721abf1efe6b0fe69b83cf01a0c204c16160913edc
COPY --from=builder /app/tcp-client .

RUN ls .

CMD ["./tcp-client"]
