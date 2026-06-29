# syntax=docker/dockerfile:1

# --- build stage: compile a static binary with the full Go toolchain ---
FROM golang:1.26.4 AS build
WORKDIR /src

# Download modules first so this layer is cached until go.mod/go.sum change.
COPY go.mod go.sum ./
RUN go mod download

COPY . .
# CGO off -> a fully static binary that runs on the tiny distroless image below.
# The web UI is go:embed'd into this binary, so nothing else ships at runtime.
RUN CGO_ENABLED=0 GOOS=linux go build -trimpath -ldflags="-s -w" -o /out/agent-x .

# --- runtime stage: just the binary on a minimal, non-root base ---
FROM gcr.io/distroless/static-debian12:nonroot
WORKDIR /sandbox

# A sample file so the read_file tool has something to read out of the box.
# Compose mounts its own sandbox over this; for a bare `docker run` it's the demo.
COPY --from=build /src/test.txt /sandbox/test.txt
COPY --from=build /out/agent-x /usr/local/bin/agent-x

EXPOSE 8080
ENTRYPOINT ["/usr/local/bin/agent-x"]
# Serve the web UI on all interfaces *inside* the container; compose maps it to
# 127.0.0.1 on the host, so it stays off the network. Sandbox read_file to /sandbox.
CMD ["-serve", "-addr", "0.0.0.0:8080", "-sandbox", "/sandbox"]
