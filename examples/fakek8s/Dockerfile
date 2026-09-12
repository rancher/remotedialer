FROM registry.suse.com/bci/golang:1.27@sha256:aa885ba6bdb36bab28adf79c048b7213d8360dec7a24d2dd56fdccfe37f5ce12 AS builder
WORKDIR /app
COPY . .
RUN go mod download
RUN go build -o /app/tcp-client main.go

FROM registry.suse.com/bci/bci-micro:16.1@sha256:b714c0dc5f2650c85a646701e80c0df3d54ee907919b23393cfe6fdc284eba47
COPY --from=builder /app/tcp-client .

RUN ls .

CMD ["./tcp-client"]
