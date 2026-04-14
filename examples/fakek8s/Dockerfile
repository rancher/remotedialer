FROM registry.suse.com/bci/golang:1.25@sha256:c1a6327cfa677209e87db4737d350a441786e0afe4e78fcf9c2bdc7c1fc67e16 AS builder
WORKDIR /app
COPY . .
RUN go mod download
RUN go build -o /app/tcp-client main.go

FROM registry.suse.com/bci/bci-micro:15.7@sha256:5ca1a44ca5be8afd3e4abc721abf1efe6b0fe69b83cf01a0c204c16160913edc
COPY --from=builder /app/tcp-client .

RUN ls .

CMD ["./tcp-client"]
