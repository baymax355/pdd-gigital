# 构建阶段
FROM golang:1.21-alpine AS builder

WORKDIR /app

# 复制go mod文件
COPY go.mod go.sum ./

# 下载依赖
RUN go mod download

# 复制源代码
COPY . .

# 构建应用
RUN CGO_ENABLED=0 GOOS=linux go build -a -installsuffix cgo -o heygem .

# 运行阶段
FROM alpine:latest

# 安装必要的工具
RUN apk --no-cache add ca-certificates ffmpeg

WORKDIR /root/

# 从构建阶段复制二进制文件
COPY --from=builder /app/heygem .

# 创建必要的目录
RUN mkdir -p /root/heygem_data/voice/data \
    && mkdir -p /root/heygem_data/face2face \
    && mkdir -p /root/heygem_data/face2face/result \
    && mkdir -p /root/data

# 暴露端口
EXPOSE 8090

# 设置环境变量
ENV APP_PORT=8090
ENV HOST_VOICE_DIR=/root/heygem_data/voice/data
ENV HOST_VIDEO_DIR=/root/heygem_data/face2face
ENV HOST_RESULT_DIR=/root/heygem_data/face2face/result
ENV WIN_COMPANY_DIR=/mnt/c/company
ENV TTS_BASE_URL=http://heygem-tts:8080
ENV VIDEO_BASE_URL=http://heygem-gen-video:8383
ENV GEN_VIDEO_CONTAINER=heygem-gen-video
ENV GEN_VIDEO_CONTAINER_DATA_ROOT=/code/data

# 启动应用
CMD ["./heygem"]