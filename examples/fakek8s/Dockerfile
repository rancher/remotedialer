FROM registry.suse.com/bci/golang:1.26@sha256:a18b80f20192d90c88353e3fa80f61c7c47b3027c0ccebdc0fa900a77e5e6400 AS builder
WORKDIR /app
COPY . .
RUN go mod download
RUN go build -o /app/tcp-client main.go

FROM registry.suse.com/bci/bci-micro:16.1@sha256:9f226d25882507c45444d3f742f04ca663877f6374979d10ea83ecfcdf947799
COPY --from=builder /app/tcp-client .

RUN ls .

CMD ["./tcp-client"]
