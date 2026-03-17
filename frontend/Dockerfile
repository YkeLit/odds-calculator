# Build stage
FROM node:22-alpine AS builder

WORKDIR /app

# Copy package files
COPY frontend/package.json frontend/package-lock.json* ./

# Install dependencies
RUN npm ci

# Copy source code
COPY frontend/ .

# Build
RUN npm run build

# Production stage
FROM nginx:alpine

ENV NGINX_LISTEN_PORT=80
ENV NGINX_SERVER_NAME=localhost
ENV API_PROXY_PASS=http://backend:8080

# Copy built assets
COPY --from=builder /app/dist /usr/share/nginx/html

# Copy nginx template for runtime env substitution
COPY frontend/nginx.conf /etc/nginx/templates/default.conf.template
COPY frontend/docker-entrypoint.sh /docker-entrypoint.d/99-runtime-config.sh

RUN chmod +x /docker-entrypoint.d/99-runtime-config.sh

EXPOSE 80

CMD ["nginx", "-g", "daemon off;"]
