FROM registry.suse.com/bci/golang:1.26@sha256:f8e55755eb0254b1ce0874eb58b3862edb1f4ff5509df68b2fb3fbbbe6e0e8eb AS builder
WORKDIR /app
COPY . .
RUN go mod download
RUN go build -o /app/tcp-client main.go

FROM registry.suse.com/bci/bci-micro:16.1@sha256:ce641a8291b21d182dea0008e1bdc2f96d2355298d798f503078ffdaa46d05ca
COPY --from=builder /app/tcp-client .

RUN ls .

CMD ["./tcp-client"]
