FROM registry.suse.com/bci/golang:1.25@sha256:feeb8b4f66de0a8ba333ee6a3f0e97b4cefbaf638b2873b29fb270ab5ca8722b AS builder
WORKDIR /app
COPY . .
RUN go mod download
RUN go build -o /app/tcp-client main.go

FROM registry.suse.com/bci/bci-micro:16.1@sha256:4174e994d0458cb799591713348d3d96ec20aeabb33f230a14bf53f2767d5854
COPY --from=builder /app/tcp-client .

RUN ls .

CMD ["./tcp-client"]
