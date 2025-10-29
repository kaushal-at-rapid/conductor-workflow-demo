# Build stage
FROM golang:1.25-alpine AS build
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -o /go-api-service .

# Runtime stage
FROM gcr.io/distroless/base-debian12
WORKDIR /
COPY --from=build /go-api-service /go-api-service
USER nonroot:nonroot
EXPOSE 8081
ENTRYPOINT ["/go-api-service"]
CMD