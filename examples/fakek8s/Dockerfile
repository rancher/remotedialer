FROM registry.suse.com/bci/golang:1.25@sha256:6ab7d7eb4a23076273da5e885ea71a1f9c72923da07f41fb53fb02e914123c11 AS builder
WORKDIR /app
COPY . .
RUN go mod download
RUN go build -o /app/tcp-client main.go

FROM registry.suse.com/bci/bci-micro:15.7@sha256:5ca1a44ca5be8afd3e4abc721abf1efe6b0fe69b83cf01a0c204c16160913edc
COPY --from=builder /app/tcp-client .

RUN ls .

CMD ["./tcp-client"]
