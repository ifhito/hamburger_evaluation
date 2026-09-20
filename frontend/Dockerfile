FROM node:22-alpine AS base
# pnpm を固定する。pnpm@latest は現在 12.5.1 に解決され、未承認のビルドスクリプト (esbuild) を
# ERR_PNPM_IGNORED_BUILDS のエラーにして pnpm install が失敗するため。
# 10.34.5 で install と build の成功を確認済み。
RUN corepack enable && corepack prepare pnpm@10.34.5 --activate
WORKDIR /app

FROM base AS deps
COPY package.json pnpm-lock.yaml* ./
RUN pnpm install --frozen-lockfile

FROM base AS dev
COPY --from=deps /app/node_modules ./node_modules
COPY . .
EXPOSE 5173
CMD ["pnpm", "dev"]

FROM base AS build
COPY --from=deps /app/node_modules ./node_modules
COPY . .
RUN pnpm build

FROM nginx:alpine AS production
COPY --from=build /app/dist /usr/share/nginx/html
COPY nginx.conf /etc/nginx/conf.d/default.conf
EXPOSE 80
