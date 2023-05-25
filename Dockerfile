FROM golang:1.20-alpine AS builder

WORKDIR /app

COPY go.mod ./
COPY go.sum ./
RUN go mod download

COPY . .

RUN go build ./cmd/rendezvous

FROM cgr.dev/chainguard/static

COPY --from=builder /app/rendezvous /rendezvous

CMD [ "/rendezvous" ]