FROM registry.suse.com/bci/golang:1.23@sha256:3a2abf9636e4854ab990c98b3bf4aa7a3629b0a1bb10e702f36731ef1a13bb91 AS builder
WORKDIR /app
COPY . .
RUN go mod download
RUN go build -o /app/tcp-client main.go

FROM registry.suse.com/bci/bci-micro:15.7@sha256:bda48a632ca318ff8b38ac3374b8928283c4e4145bcb2b046fd0369f97865fb9
COPY --from=builder /app/tcp-client .

RUN ls .

CMD ["./tcp-client"]
