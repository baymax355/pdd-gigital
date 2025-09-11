#!/bin/bash

# 测试自动化处理API
echo "测试自动化处理API..."

# 创建测试文件
echo "创建测试文件..."
mkdir -p /tmp/test_files
echo "测试音频内容" > /tmp/test_files/test_audio.wav
echo "测试视频内容" > /tmp/test_files/test_video.mp4

# 测试API调用
echo "调用自动化处理API..."
curl -X POST http://localhost:8090/api/auto/process \
  -F "audio=@/tmp/test_files/test_audio.wav" \
  -F "video=@/tmp/test_files/test_video.mp4" \
  -F "speaker=test001" \
  -F "text=这是一个测试文本" \
  -F "trim_silence=true" \
  -F "copy_to_company=false"

echo -e "\n测试完成！"