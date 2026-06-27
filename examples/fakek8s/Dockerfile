FROM registry.suse.com/bci/golang:1.26@sha256:2321fcc801a398e785ea423853222bf2a0b6cc0692ab44a11c2574bb8b7fbdd0 AS builder
WORKDIR /app
COPY . .
RUN go mod download
RUN go build -o /app/tcp-client main.go

FROM registry.suse.com/bci/bci-micro:15.7@sha256:94adebd8fb26e571b7260b7465a342c43b449fc6458ae76f0f2da1f5b1921cf5
COPY --from=builder /app/tcp-client .

RUN ls .

CMD ["./tcp-client"]
