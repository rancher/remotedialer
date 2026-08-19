FROM registry.suse.com/bci/golang:1.26@sha256:f6f04957fbc16399c76b1e8266cc7d0f3b4a5434b824fb251d7e3ae559d6a910 AS builder
WORKDIR /app
COPY . .
RUN go mod download
RUN go build -o /app/tcp-client main.go

FROM registry.suse.com/bci/bci-micro:15.7@sha256:44f5c047210188eb290414c52f883b763efa1e234c2a15c325f03e83f0984fa6
COPY --from=builder /app/tcp-client .

RUN ls .

CMD ["./tcp-client"]
