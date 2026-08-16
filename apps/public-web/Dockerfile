FROM node:22-bookworm-slim AS build
WORKDIR /app
COPY apps/public-web/package.json apps/public-web/package-lock.json ./
RUN npm ci
COPY apps/public-web/ ./
RUN npm run build

FROM nginx:1.27-alpine
ENV NGINX_ENVSUBST_FILTER=ECHOTRAILS_APPLE_APP_ID
COPY apps/public-web/nginx.conf /etc/nginx/templates/default.conf.template
COPY --from=build /app/dist /usr/share/nginx/html
EXPOSE 8080
