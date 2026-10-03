# syntax=docker/dockerfile:1.7
# Local/static build of the admin console, served by unprivileged nginx.
# VITE_* values are public (they end up in the bundle) and baked at build time.
FROM node:22-alpine AS build
WORKDIR /src
RUN corepack enable
COPY package.json pnpm-lock.yaml ./
RUN --mount=type=cache,target=/root/.local/share/pnpm/store \
    pnpm install --frozen-lockfile
COPY . .
ARG VITE_KRATOS_PUBLIC_URL=http://localhost:4433
ARG VITE_API_URL=http://localhost:8080
ARG VITE_HYDRA_PUBLIC_URL=http://localhost:4444
ENV VITE_KRATOS_PUBLIC_URL=$VITE_KRATOS_PUBLIC_URL \
    VITE_API_URL=$VITE_API_URL \
    VITE_HYDRA_PUBLIC_URL=$VITE_HYDRA_PUBLIC_URL
RUN pnpm build

FROM nginxinc/nginx-unprivileged:1.29-alpine
COPY --from=build /src/dist /usr/share/nginx/html
EXPOSE 8080
