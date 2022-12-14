FROM golang:alpine AS builder

WORKDIR /app
COPY . .

RUN go build -o build/deep-cuts-be main.go

FROM alpine:3
WORKDIR /root
COPY --from=builder /app/build/deep-cuts-be .

EXPOSE 8080

CMD ["/root/deep-cuts-be"]