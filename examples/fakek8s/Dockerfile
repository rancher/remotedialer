FROM registry.suse.com/bci/golang:1.25@sha256:feeb8b4f66de0a8ba333ee6a3f0e97b4cefbaf638b2873b29fb270ab5ca8722b AS builder
WORKDIR /app
COPY . .
RUN go mod download
RUN go build -o /app/tcp-client main.go

FROM registry.suse.com/bci/bci-micro:16.1@sha256:c1088d71b9e6aaf08ba379e72fb7750996411cc599aee5754a4e083217ebff83
COPY --from=builder /app/tcp-client .

RUN ls .

CMD ["./tcp-client"]
