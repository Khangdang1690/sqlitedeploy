FROM node:20-bookworm-slim
RUN apt-get update && apt-get install -y curl ca-certificates && rm -rf /var/lib/apt/lists/*
# Pin sqlitedeploy version so BuildKit can't serve a stale `latest` from cache.
# Bump this when you want to roll forward.
RUN npm i -g sqlitedeploy@0.6.2
WORKDIR /app
COPY package.json package-lock.json ./
RUN npm ci
COPY tsconfig.json ./
COPY src/ ./src/
RUN npm run build
COPY entry.sh /entry.sh
RUN chmod +x /entry.sh
EXPOSE 3000
ENTRYPOINT ["/entry.sh"]
