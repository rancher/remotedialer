FROM registry.suse.com/bci/golang:1.26@sha256:ffe330184fb07e2e2c089b73229eaaec7085ce802f965b80c84780f163a5f062 AS builder
WORKDIR /app
COPY . .
RUN go mod download
RUN go build -o /app/tcp-client main.go

FROM registry.suse.com/bci/bci-micro:15.7@sha256:a0ab4f6803e057fcd3c5b25e57e028ef6b69f2eddbd7273d0f7829fc0ab23630
COPY --from=builder /app/tcp-client .

RUN ls .

CMD ["./tcp-client"]
