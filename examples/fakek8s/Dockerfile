FROM registry.suse.com/bci/golang:1.24@sha256:ae722bb8954485d10eb26d90c14d521d90810d8b0c84c3c72b58cbb1fc1ee284 AS builder
WORKDIR /app
COPY . .
RUN go mod download
RUN go build -o /app/tcp-client main.go

FROM registry.suse.com/bci/bci-micro:15.7@sha256:07fce181ac208790fe3986e23ff9f1ded5ec6a25259e12631d835d31c57b7245
COPY --from=builder /app/tcp-client .

RUN ls .

CMD ["./tcp-client"]
