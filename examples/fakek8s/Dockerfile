FROM registry.suse.com/bci/golang:1.26@sha256:441ba6f61eb5e9b6cbcbe9e3b068f3c14a20818196f871ae3a58466b289dd104 AS builder
WORKDIR /app
COPY . .
RUN go mod download
RUN go build -o /app/tcp-client main.go

FROM registry.suse.com/bci/bci-micro:16.1@sha256:9f226d25882507c45444d3f742f04ca663877f6374979d10ea83ecfcdf947799
COPY --from=builder /app/tcp-client .

RUN ls .

CMD ["./tcp-client"]
