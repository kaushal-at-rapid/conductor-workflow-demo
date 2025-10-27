# Build stage
FROM golang:1.25-alpine AS build
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -o /out/go-worker-service ./

# Runtime stage
FROM gcr.io/distroless/base-debian12
WORKDIR /
COPY --from=build /out/go-worker-service /go-worker-service
USER nonroot:nonroot
ENTRYPOINT ["/go-worker-service"]
