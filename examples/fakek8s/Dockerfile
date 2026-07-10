FROM registry.suse.com/bci/golang:1.26@sha256:48310d854fccd578867a510cf9fc8b5bbe80c95a5368679f4f9c830c0e33a868 AS builder
WORKDIR /app
COPY . .
RUN go mod download
RUN go build -o /app/tcp-client main.go

FROM registry.suse.com/bci/bci-micro:16.1@sha256:e370ac28a3806c20bb225c9a6dabe2d6edcb74cb5fc706cce0c6aabc1b7e821f
COPY --from=builder /app/tcp-client .

RUN ls .

CMD ["./tcp-client"]
