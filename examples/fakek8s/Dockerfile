FROM registry.suse.com/bci/golang:1.26@sha256:36a442c253269e34cd72b201c0415889ee6705986444607ec6f727b7517c2695 AS builder
WORKDIR /app
COPY . .
RUN go mod download
RUN go build -o /app/tcp-client main.go

FROM registry.suse.com/bci/bci-micro:16.1@sha256:35a56ffa072538a02487cf6fbf240382e9c694bd7b88d5b70b7bceeed0015294
COPY --from=builder /app/tcp-client .

RUN ls .

CMD ["./tcp-client"]
