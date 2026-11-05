FROM registry.suse.com/bci/golang:1.27@sha256:107c1d9c1be31452eac6d71afff3fc33144001e515f4f7e843ebbdeedc5cee12 AS builder
WORKDIR /app
COPY . .
RUN go mod download
RUN go build -o /app/tcp-client main.go

FROM registry.suse.com/bci/bci-micro:16.1@sha256:1bad8bd648db037dc98ddeb2d0c67fb45d65de13435d0a1075ba5f68c590d09b
COPY --from=builder /app/tcp-client .

RUN ls .

CMD ["./tcp-client"]
