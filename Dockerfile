# ビルドステージ: Goアプリケーションをビルド
FROM golang:1.24-bookworm AS builder
WORKDIR /app

# Goモジュールファイルをコピー
COPY go.mod go.sum ./
# 依存関係をダウンロード
RUN go mod download

# ソースコードをコピー
COPY . .

# アプリケーションをビルド
# CGO_ENABLED=0 は静的リンクを強制し、軽量な実行可能ファイルを生成します
RUN CGO_ENABLED=0 go build -o /main

# 実行ステージ: 軽量なベースイメージを使用
FROM debian:bookworm-slim

# 💡 証明書ストアを更新するコマンドを追加
RUN apt-get update && apt-get install -y ca-certificates && rm -rf /var/lib/apt/lists/*

# タイムゾーン設定 (JSTで実行したい場合は調整が必要)
ENV TZ=Asia/Tokyo

# ビルドステージから実行可能ファイルをコピー
COPY --from=builder /main /main

# Cloud Runが環境変数 $PORT で指定するポートを公開
ENV PORT=8080

# アプリケーションを実行
CMD ["/main"]