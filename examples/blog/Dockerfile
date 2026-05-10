FROM node:20-bookworm-slim
RUN apt-get update && apt-get install -y curl ca-certificates && rm -rf /var/lib/apt/lists/*
RUN npm i -g sqlitedeploy
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
