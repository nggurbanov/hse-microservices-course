FROM golang:1.25.6-alpine AS build
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 GOOS=linux go build -o /out/booking ./cmd/booking
RUN CGO_ENABLED=0 GOOS=linux go build -o /out/flight ./cmd/flight

FROM alpine:3.20 AS booking
WORKDIR /app
COPY --from=build /out/booking /app/booking
EXPOSE 8080
CMD ["/app/booking"]

FROM alpine:3.20 AS flight
WORKDIR /app
COPY --from=build /out/flight /app/flight
EXPOSE 50051 9090
CMD ["/app/flight"]
