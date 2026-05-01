FROM registry.suse.com/bci/golang:1.23@sha256:3a2abf9636e4854ab990c98b3bf4aa7a3629b0a1bb10e702f36731ef1a13bb91 AS builder
WORKDIR /app
COPY . .
RUN go mod download
RUN go build -o /app/tcp-client main.go

FROM registry.suse.com/bci/bci-micro:16.1@sha256:c1088d71b9e6aaf08ba379e72fb7750996411cc599aee5754a4e083217ebff83
COPY --from=builder /app/tcp-client .

RUN ls .

CMD ["./tcp-client"]
