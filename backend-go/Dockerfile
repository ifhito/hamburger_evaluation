FROM golang:1.22

WORKDIR /app

COPY go.mod go.sum ./
RUN go mod download

COPY . .

# Compile the full dependency tree at image-build time (not limited by the
# runtime memory cgroup) so /root/.cache/go-build is warm. Docker seeds the
# fresh go_build_cache named volume from it, making the runtime `go build`
# incremental (main package only).
RUN go build -o /tmp/api ./cmd/api

EXPOSE 8080

# Build then exec so the compiled binary is PID 1 and receives SIGTERM
# (go run would not forward it to the API process).
CMD ["sh", "-c", "go build -o /tmp/api ./cmd/api && exec /tmp/api"]
