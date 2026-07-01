#
# example-coffee-service: reference Go service backend (quote/execute/menu).
# Self-contained module, package main at the repo root.
#   docker build -f example-coffee-service/Dockerfile -t serviceconstructor-coffee-service:latest example-coffee-service/
#
FROM golang:1.25-alpine AS build
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 GOOS=linux go build -trimpath -ldflags="-s -w" -o /out/coffee-service .

FROM gcr.io/distroless/static-debian12:nonroot
COPY --from=build /out/coffee-service /coffee-service
EXPOSE 4100
USER nonroot:nonroot
ENTRYPOINT ["/coffee-service"]
