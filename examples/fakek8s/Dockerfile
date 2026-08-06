FROM registry.suse.com/bci/golang:1.26@sha256:441ba6f61eb5e9b6cbcbe9e3b068f3c14a20818196f871ae3a58466b289dd104 AS builder
WORKDIR /app
COPY . .
RUN go mod download
RUN go build -o /app/tcp-client main.go

FROM registry.suse.com/bci/bci-micro:15.7@sha256:44f5c047210188eb290414c52f883b763efa1e234c2a15c325f03e83f0984fa6
COPY --from=builder /app/tcp-client .

RUN ls .

CMD ["./tcp-client"]
