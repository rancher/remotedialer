FROM registry.suse.com/bci/golang:1.25@sha256:3190609b31bbdbd4396ecba6c17ef71ccd282ec6973dba7c106b391b3edb1807 AS builder
WORKDIR /app
COPY . .
RUN go mod download
RUN go build -o /app/tcp-client main.go

FROM registry.suse.com/bci/bci-micro:15.7@sha256:bda48a632ca318ff8b38ac3374b8928283c4e4145bcb2b046fd0369f97865fb9
COPY --from=builder /app/tcp-client .

RUN ls .

CMD ["./tcp-client"]
