FROM golang:1.22

WORKDIR /app

COPY go.mod go.sum ./
RUN go mod download

COPY . .

EXPOSE 8080

# Build then exec so the compiled binary is PID 1 and receives SIGTERM
# (go run would not forward it to the API process).
CMD ["sh", "-c", "go build -o /tmp/api ./cmd/api && exec /tmp/api"]
