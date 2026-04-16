FROM registry.suse.com/bci/golang:1.25@sha256:6ab7d7eb4a23076273da5e885ea71a1f9c72923da07f41fb53fb02e914123c11 AS builder
WORKDIR /app
COPY . .
RUN go mod download
RUN go build -o /app/tcp-client main.go

FROM registry.suse.com/bci/bci-micro:15.7@sha256:5ed5a194daed8dc364d5fafccf7df64511e1d4d8ff18d5cd40bd6b4d2c2e74c9
COPY --from=builder /app/tcp-client .

RUN ls .

CMD ["./tcp-client"]
