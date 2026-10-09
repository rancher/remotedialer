FROM registry.suse.com/bci/golang:1.26@sha256:0f29993cebfcb033416095e564dfcd9ca2014ffb4a8ed00e5d796da5fbf66a66 AS builder
WORKDIR /app
COPY . .
RUN go mod download
RUN go build -o /app/tcp-client main.go

FROM registry.suse.com/bci/bci-micro:16.1@sha256:8e774758e9a9b081b777f4290a0779bdfe9d44f662793dd9fd1c5e79c8c5a156
COPY --from=builder /app/tcp-client .

RUN ls .

CMD ["./tcp-client"]
