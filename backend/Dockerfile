FROM ruby:3.3-slim

# 必要なシステムパッケージをインストール
RUN apt-get update -qq && apt-get install --no-install-recommends -y \
  build-essential \
  libpq-dev \
  libyaml-dev \
  nodejs \
  curl \
  && rm -rf /var/lib/apt/lists/*

WORKDIR /app

# Gemfile を先にコピーして bundle install をキャッシュ活用
COPY Gemfile Gemfile.lock ./
RUN bundle install

# アプリケーションコードをコピー
COPY . .

# entrypoint スクリプトを実行可能にする
COPY entrypoint.sh /usr/bin/entrypoint.sh
RUN chmod +x /usr/bin/entrypoint.sh

EXPOSE 3000

ENTRYPOINT ["entrypoint.sh"]
CMD ["rails", "server", "-b", "0.0.0.0"]
