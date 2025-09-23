# HeyGem 全流程助手（Go+Gin / Vue3+Tailwind）

基于 heygem.txt 的手工流程，封装成后端 API 与前端页面，自动执行：

- 音频上传 -> 去静音(可选) -> 响度归一化(16k/PCM16) -> 拷贝至 `/root/heygem_data/voice/data/ref_norm.wav`
- 视频上传 -> 直通转封装静音 -> 拷贝至 `/root/heygem_data/face2face/silent.mp4`
- 调用 TTS 预处理与合成 -> 保存 `speaker.wav` 至 voice/data 并复制到视频目录（或使用直通转发端点）
- 提交视频合成任务到 `http://127.0.0.1:8383/easy/submit`
- 拉取合成结果（通过 `docker cp` 从 `heygem-gen-video` 导出），落地到共享盘（如 `/mnt/windows-digitalpeople`）并自动清理宿主机/容器内的临时视频

## 目录结构

- `server/` Go + Gin 服务，暴露 `/api/*`
- `client/` Vue3 + Tailwind 前端（Vite）
- `docker-compose.tools.yml` 可选：以容器运行后端（需要挂载 docker.sock）

## 运行（推荐直接在 WSL 主机运行后端）

前置：`ffmpeg`、`docker` 可用，且以下目录（共享盘内）存在：

- `/mnt/windows-digitalpeople/voice/data`
- `/mnt/windows-digitalpeople/face2face`
- `/mnt/windows-digitalpeople/face2face/temp`
- `/mnt/windows-digitalpeople/face2face/result`
- `/mnt/windows-digitalpeople/workdir`
- 已在宿主机挂载 Windows 共享盘，例如 `//192.168.7.10/DIGITALPEOPLE` → `/mnt/windows-digitalpeople`（可使用 `scripts/mount_digitalpeople.sh`，脚本会自动创建上述子目录）

1) 启动后端

```
cd server
go run .
```

### 共享盘挂载（WSL 示例）

```
sudo apt update
sudo apt install -y cifs-utils
./scripts/mount_digitalpeople.sh /mnt/windows-digitalpeople
export DIGITAL_PEOPLE_DIR=/mnt/windows-digitalpeople
```

`scripts/mount_digitalpeople.sh` 会检测 `cifs-utils` 是否已安装，并使用当前用户的 UID/GID 挂载 `//192.168.7.10/DIGITALPEOPLE`（可通过环境变量覆盖）。

如需一步挂载并启动所有容器，可直接运行：

```
./scripts/auto_setup_and_start.sh
```

该脚本默认把共享盘挂载到 `/mnt/windows-digitalpeople`，然后导出所需环境变量并调用 `./start.sh all`。常用选项：

- `--mount /your/path` 自定义挂载点
- `--skip-mount` 仅设置变量与启动，不执行挂载
- `--start-mode web` 只启动 heygem-web 服务
- `--start-arg --skip-build` 传递额外参数给 `start.sh`

默认监听 `:8090`，可通过环境变量覆盖：

```
DIGITAL_PEOPLE_DIR=/mnt/windows-digitalpeople \
APP_PORT=8090 \
APP_WORKDIR=/mnt/windows-digitalpeople/workdir \
HOST_VOICE_DIR=/mnt/windows-digitalpeople/voice/data \
HOST_VIDEO_DIR=/mnt/windows-digitalpeople/face2face \
HOST_RESULT_DIR=/mnt/windows-digitalpeople/face2face/result \
WIN_COMPANY_DIR=/mnt/windows-digitalpeople \
TTS_BASE_URL=http://127.0.0.1:18180 \
VIDEO_BASE_URL=http://127.0.0.1:8383 \
GEN_VIDEO_CONTAINER=heygem-gen-video \
GEN_VIDEO_CONTAINER_DATA_ROOT=/code/data \
go run .
```

2) 启动前端（可选）

```
cd client
npm i
npm run dev
```

开发模式下，前端将通过代理访问 `http://localhost:8090/api`。

若要生产部署，可 `npm run build` 后把 `client/dist` 放在仓库中，后端会静态托管。

3) 以 Docker 运行后端（可选）

```
docker compose -f docker-compose.tools.yml up --build -d
```

注意：容器内通过 `host.docker.internal` 调用宿主上的 TTS 与视频服务；同时挂载了 `/var/run/docker.sock` 以便执行 `docker cp`。
在启动前请确保宿主机已经挂载共享盘，并导出 `DIGITAL_PEOPLE_DIR` 指向该挂载点，例如：`export DIGITAL_PEOPLE_DIR=/mnt/windows-digitalpeople`。

## API 速览

直通封装（可直接用原有调用方式对接本服务）：

- `POST /v1/preprocess_and_tran` → 代理到 `TTS_BASE_URL/v1/preprocess_and_tran`
- `POST /v1/invoke` → 代理到 `TTS_BASE_URL/v1/invoke`（音频流原样返回）
- `POST /easy/submit` → 代理到 `VIDEO_BASE_URL/easy/submit`

- `POST /api/upload/audio` 表单：`file`、`trim_silence=true|false`
  - 输出 `ref_norm.wav` 到 voice/data
- `POST /api/upload/video` 表单：`file`
  - 输出 `silent.mp4` 到 face2face
- `POST /api/tts/preprocess` JSON：`{"format":"wav","reference_audio":"ref_norm.wav","lang":"zh"}`
  - 透传 TTS 预处理结果
- `POST /api/tts/invoke` JSON：与 heygem.txt 中 `invoke` 参数一致（会把响应保存为 `speaker.wav`）并复制到视频目录
- `POST /api/video/submit` JSON：`{"audio_filename":"demo001.wav","video_filename":"silent.mp4","code":"task001"}`
- `GET /api/video/result?code=task001&copy_to_company=1`
  - 优先读取 `HOST_RESULT_DIR`（共享盘）中的结果；若不存在，将从 `heygem-gen-video:/code/data/temp/task001-r.mp4` 抽取到共享盘，并同时清理容器/宿主机临时文件

## 与 heygem.txt 差异说明

- 将“手工拷贝/命令”封装为 API；默认假设容器将视频目录挂载为 `/code/data`。
- 若你的挂载路径或容器名不同，请通过环境变量进行覆盖。

## 登录与用户（新增）

- 前端增加登录页，提交任务前必须登录；登录成功后会在任务列表展示“用户”和“提交时间”。
- 用户名与密码存放在宿主机 JSON 文件中，默认路径：`$APP_WORKDIR/users.json`（Docker 默认为 `/root/data/users.json`）。
- JSON 支持两种格式：
  - 映射：`{"张三":"12345", "李四":"12345"}`
  - 数组：`[{"username":"张三","password":"12345"}]`
- 相关接口：
  - `GET /api/auth/users` 列出可选用户名（不包含密码）
  - `POST /api/auth/login` 登录，JSON：`{"username":"张三","password":"12345"}`
  - `GET /api/auth/me` 获取当前登录用户
  - `POST /api/auth/logout` 退出登录

示例 users.json（初始密码均为 12345）：

```
{
  "陈志芳": "12345",
  "胡国民": "12345",
  "蒲先川": "12345",
  "倪浩宇": "12345",
  "赵婉婷": "12345",
  "陈奕杉": "12345",
  "李悦": "12345",
  "汪冠迪": "12345"
}
```
