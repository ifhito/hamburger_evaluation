FROM golang:1.27

WORKDIR /app

COPY go.mod go.sum ./
RUN go mod download

COPY . .

# 依存ツリー全体を image のビルド時にコンパイルする（ランタイムのメモリ
# cgroup の制限を受けない）。これにより /root/.cache/go-build が温まった状態
# になる。Docker は新規の named volume である go_build_cache をそこから
# 初期化するので、ランタイムの `go build` はインクリメンタルになる
# （main パッケージのみ）。
RUN go build -o /tmp/api ./cmd/api

EXPOSE 8080

# ビルドしてから exec することで、コンパイルされたバイナリが PID 1 になり
# SIGTERM を受け取る（go run はそれを API プロセスに転送しない）。
CMD ["sh", "-c", "go build -o /tmp/api ./cmd/api && exec /tmp/api"]
