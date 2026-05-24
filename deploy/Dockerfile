# Multistage build: Vue frontend -> Go backend -> minimal distroless image.

# --- 1. Frontend ---
FROM node:24-alpine AS web-build
WORKDIR /src/web
COPY web/package*.json ./
RUN npm ci
COPY web/. .
RUN npm run build

# --- 2. Backend ---
FROM golang:1.26-alpine AS go-build
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY cmd/ ./cmd/
COPY internal/ ./internal/
COPY pkg/ ./pkg/
# Compiled frontend goes into the embed.FS.
COPY --from=web-build /src/internal/staticui/dist /src/internal/staticui/dist
RUN CGO_ENABLED=0 go build -ldflags='-s -w' -o /modos-cpu ./cmd/modos-cpu
RUN CGO_ENABLED=0 go build -ldflags='-s -w' -o /modos-rag-index ./cmd/modos-rag-index

# --- 3. Final image ---
FROM gcr.io/distroless/static-debian12:nonroot
COPY --from=go-build /modos-cpu /modos-cpu
COPY --from=go-build /modos-rag-index /modos-rag-index
EXPOSE 8080
USER nonroot:nonroot
ENTRYPOINT ["/modos-cpu", "-addr", ":8080"]
