FROM registry.suse.com/bci/golang:1.26@sha256:51dc8295e6d0c0423763b457f6235a370c4987f17db2ae0e0a481a7ae45bfdcc AS builder
WORKDIR /app
COPY . .
RUN go mod download
RUN go build -o /app/tcp-client main.go

FROM registry.suse.com/bci/bci-micro:15.7@sha256:94adebd8fb26e571b7260b7465a342c43b449fc6458ae76f0f2da1f5b1921cf5
COPY --from=builder /app/tcp-client .

RUN ls .

CMD ["./tcp-client"]
