FROM golang:1.22

# HEIC のデコーダは、通常は「あれば OS の libheif を使い、なければ WASM」と切り替わる。OS の libheif は
# 隔離されておらず、環境で動きが変わるので、常に WASM(隔離された実行系)だけを使う。ここで指定した
# build タグは、このイメージの中の go build・go test のすべてに効く。
ENV GOFLAGS=-tags=nodynamic

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
