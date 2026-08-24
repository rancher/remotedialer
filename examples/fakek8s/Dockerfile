FROM registry.suse.com/bci/golang:1.26@sha256:f6f04957fbc16399c76b1e8266cc7d0f3b4a5434b824fb251d7e3ae559d6a910 AS builder
WORKDIR /app
COPY . .
RUN go mod download
RUN go build -o /app/tcp-client main.go

FROM registry.suse.com/bci/bci-micro:16.1@sha256:9f226d25882507c45444d3f742f04ca663877f6374979d10ea83ecfcdf947799
COPY --from=builder /app/tcp-client .

RUN ls .

CMD ["./tcp-client"]
