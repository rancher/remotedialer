FROM registry.suse.com/bci/golang:1.23@sha256:3a2abf9636e4854ab990c98b3bf4aa7a3629b0a1bb10e702f36731ef1a13bb91 AS builder
WORKDIR /app
COPY . .
RUN go mod download
RUN go build -o /app/tcp-client main.go

FROM registry.suse.com/bci/bci-micro:15.7@sha256:07fce181ac208790fe3986e23ff9f1ded5ec6a25259e12631d835d31c57b7245
COPY --from=builder /app/tcp-client .

RUN ls .

CMD ["./tcp-client"]
