FROM golang:alpine AS builder

WORKDIR /app

COPY . .

ENV CGO_ENABLED=0

RUN go build -o /cache cmd/main.go

FROM alpine

COPY --from=builder /cache /bin/cache

EXPOSE 5000

CMD /bin/cache serve
