# Build stage: the Go toolchain, pinned by version and digest
FROM golang:1.27.4-alpine@sha256:0a5f7fe796599ff487605987432e7619ae45beeb2fba4c2ce0b907879932aebf AS build
WORKDIR /src

# Dependencies first, so this layer is reused until go.mod or go.sum change
COPY go.mod go.sum ./
RUN go mod download

COPY . .
# The commit the image is built from; shown at / and used as the image tag
ARG VERSION=dev
# A static binary: no libc needed, so the runtime image can be almost empty
RUN CGO_ENABLED=0 go build -trimpath -ldflags "-s -w -X main.version=${VERSION}" -o /out/service .

# Runtime stage: no shell, no package manager, only the binary
FROM gcr.io/distroless/static-debian12:nonroot@sha256:afa5c872c891853ca7fcf1f12c3edb23f7eeef36189728842dd51042ff57f7ab
COPY --from=build /out/service /service
# Numeric user: Kubernetes cannot verify runAsNonRoot for a user given by name.
# 65532 is "nonroot" in distroless and the default runAsUser of the platform chart
USER 65532:65532
EXPOSE 8080
ENTRYPOINT ["/service"]
