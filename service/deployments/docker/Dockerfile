# Dockerfile for Docube Blockchain Service
#
# Build Stage:
# - Use golang:1.21-alpine as builder
# - Copy go.mod, go.sum for dependency caching
# - Build static binary
#
# Runtime Stage:
# - Use alpine:3.18 for minimal image
# - Copy binary and config files
# - Non-root user for security
# - Health check endpoint
#
# Build: docker build -t docube-backend:latest -f deployments/docker/Dockerfile .
# Run:   docker run -p 8080:8080 docube-backend:latest
