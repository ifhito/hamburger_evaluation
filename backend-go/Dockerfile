# 本番用。多くの基盤は Dockerfile 名を選べず既定の `Dockerfile` を見るため、本番用をこの名前にする。
# 開発用(コンテナ起動時に go build するもの)は Dockerfile.dev にある。
# 静的バイナリを distroless へ入れるので、イメージは数十 MB、起動は即時になる。
FROM golang:1.27 AS build
WORKDIR /src

COPY go.mod go.sum ./
RUN go mod download

COPY . .
# CGO なしの静的リンク。-trimpath と -ldflags="-s -w" でバイナリを小さくする。
RUN CGO_ENABLED=0 go build -trimpath -ldflags="-s -w" -o /out/api ./cmd/api

# nonroot 版は UID 65532 で動く。書き込み先を持たない(写真は S3 へ、PHOTO_STORAGE=s3)。
FROM gcr.io/distroless/static:nonroot
COPY --from=build /out/api /api

# PORT はホスト(Render / Cloud Run など)が上書きする。
ENV PORT=8080
EXPOSE 8080
USER nonroot:nonroot
ENTRYPOINT ["/api"]
