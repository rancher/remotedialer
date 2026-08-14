FROM registry.suse.com/bci/golang:1.26@sha256:a18b80f20192d90c88353e3fa80f61c7c47b3027c0ccebdc0fa900a77e5e6400 AS builder
WORKDIR /app
COPY . .
RUN go mod download
RUN go build -o /app/tcp-client main.go

FROM registry.suse.com/bci/bci-micro:15.7@sha256:44f5c047210188eb290414c52f883b763efa1e234c2a15c325f03e83f0984fa6
COPY --from=builder /app/tcp-client .

RUN ls .

CMD ["./tcp-client"]
