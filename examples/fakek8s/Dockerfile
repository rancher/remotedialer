FROM registry.suse.com/bci/golang:1.27@sha256:a2d38c355ff182ab05f863310c8101f0765b9b206fc2b9b0005cdf0195f5db7b AS builder
WORKDIR /app
COPY . .
RUN go mod download
RUN go build -o /app/tcp-client main.go

FROM registry.suse.com/bci/bci-micro:15.7@sha256:44f5c047210188eb290414c52f883b763efa1e234c2a15c325f03e83f0984fa6
COPY --from=builder /app/tcp-client .

RUN ls .

CMD ["./tcp-client"]
