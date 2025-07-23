# Build stage
FROM registry.access.redhat.com/ubi9/go-toolset:latest AS builder
USER root
WORKDIR /app
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 GOOS=linux go build -a -installsuffix cgo -o app .

# Final stage
FROM registry.access.redhat.com/ubi9/ubi-minimal:latest
RUN microdnf install -y ca-certificates && microdnf clean all
WORKDIR /root/
COPY --from=builder /app/app .
EXPOSE 8080
USER 1001
CMD ["./app"]
