FROM node:24-alpine AS frontend-build
WORKDIR /app/frontend
COPY frontend/package*.json ./
RUN npm install
COPY frontend/ ./
RUN npm run build

FROM golang:1.25-alpine AS backend-build
WORKDIR /app/backend
ARG GOPROXY=https://proxy.golang.org,direct
ENV GOPROXY=$GOPROXY
COPY backend/go.mod backend/go.sum ./
RUN go mod download
COPY backend/ ./
RUN CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -o /out/app ./cmd/server

FROM alpine:3.22
WORKDIR /app
COPY --from=backend-build /out/app /app/app
COPY --from=frontend-build /app/frontend/dist /app/static
RUN mkdir -p /app/data/config/subscriptions /app/data/db
EXPOSE 8080
CMD ["/app/app"]
