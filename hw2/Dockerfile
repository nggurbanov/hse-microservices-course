FROM golang:1.25.6-alpine AS builder

WORKDIR /app

# Install git for downloading go modules
RUN apk add --no-cache git

COPY go.mod go.sum* ./
RUN go mod download

COPY . .

# Ensure oapi-codegen is downloaded (it should be in go.mod since we go get it)
RUN go run github.com/oapi-codegen/oapi-codegen/v2/cmd/oapi-codegen@latest -generate types,chi-server,spec -package api api/openapi.yaml > api/api.gen.go

RUN CGO_ENABLED=0 GOOS=linux go build -o main ./cmd/app

FROM alpine:latest

WORKDIR /app

COPY --from=builder /app/main .

EXPOSE 8080

CMD ["./main"]
