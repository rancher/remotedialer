FROM registry.suse.com/bci/golang:1.26@sha256:48310d854fccd578867a510cf9fc8b5bbe80c95a5368679f4f9c830c0e33a868 AS builder
WORKDIR /app
COPY . .
RUN go mod download
RUN go build -o /app/tcp-client main.go

FROM registry.suse.com/bci/bci-micro:15.7@sha256:94adebd8fb26e571b7260b7465a342c43b449fc6458ae76f0f2da1f5b1921cf5
COPY --from=builder /app/tcp-client .

RUN ls .

CMD ["./tcp-client"]
