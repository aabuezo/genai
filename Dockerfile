FROM golang:1.25.5-alpine AS builder

WORKDIR /app

COPY go.mod go.sum ./
RUN go mod download

COPY . .

RUN go build -o /main cmd/web/main.go

FROM alpine:latest

WORKDIR /

COPY --from=builder /main /main
COPY ui /ui

EXPOSE 4000

CMD ["/main"]
