FROM golang:1.22-bookworm AS build
WORKDIR /app
COPY . ./
RUN go build -o /bin/heygem-helper

FROM debian:bookworm-slim
WORKDIR /app
RUN apt-get update && apt-get install -y --no-install-recommends ffmpeg ca-certificates curl jq docker.io && rm -rf /var/lib/apt/lists/*
COPY --from=build /bin/heygem-helper /usr/local/bin/heygem-helper
EXPOSE 8090
ENV APP_PORT=8090
CMD ["heygem-helper"]

