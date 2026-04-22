FROM registry.suse.com/bci/golang:1.25@sha256:feeb8b4f66de0a8ba333ee6a3f0e97b4cefbaf638b2873b29fb270ab5ca8722b AS builder
WORKDIR /app
COPY . .
RUN go mod download
RUN go build -o /app/tcp-client main.go

FROM registry.suse.com/bci/bci-micro:15.7@sha256:590f1bead5a54cfd3cee7b7d4459f874f3e1b2789a7d2149a2d80b35a43ac9cf
COPY --from=builder /app/tcp-client .

RUN ls .

CMD ["./tcp-client"]
