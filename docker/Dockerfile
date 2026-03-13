# Dockerfile for building wile-goast
#
# Build:
#   make docker-build
# Or manually:
#   docker build -f docker/Dockerfile -t wile-goast .
#
# Cross-platform:
#   docker build --platform linux/amd64 -f docker/Dockerfile -t wile-goast .

FROM golang:1.24-bookworm

ARG TARGETOS=linux
ARG TARGETARCH
ENV TARGETOS=${TARGETOS}
ENV TARGETARCH=${TARGETARCH}

RUN apt-get update && apt-get install -y --no-install-recommends \
    build-essential \
    make \
    git \
    curl

WORKDIR /app

# Copy go module files first for better layer caching
COPY go.mod go.sum ./

# Download dependencies
RUN go mod download

# Copy the rest of the source
COPY . .

# Build the binary
RUN make build

# Default command: run tests
CMD ["make", "test"]
