FROM node:22-alpine AS builder

WORKDIR /app

COPY frontend/package.json frontend/pnpm-lock.yaml* ./
RUN corepack enable && pnpm install

COPY frontend/ ./
RUN pnpm build

FROM nginx:1.27-alpine

COPY deploy/nginx/nginx.conf /etc/nginx/conf.d/default.conf
COPY --from=builder /app/dist /usr/share/nginx/html

EXPOSE 80