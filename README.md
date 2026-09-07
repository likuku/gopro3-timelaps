# 项目概述
- 纯 CLI 工具（Go 编写），用于将 GoPro3 延时摄影照片序列转码为视频。
- 核心依赖：运行时需系统安装 `ffmpeg`。

# 核心功能与技术要求
- **输入识别**：遍历当前目录，匹配 `Gxxxxxx.JPG`（不区分大小写），按数字编号升序排序（旧到新）。
- **时间戳与硬字幕**：在每张照片左上角绘制拍摄时间戳；字体行高/字号自适应为原始画面高度的 3.5%。
- **视频参数**：
  - 分辨率：720p (`1280x720`)，保持素材原始比例缩放，不裁切，黑边填充 (`scale + pad`)。
  - 帧率/时长：30 fps，每张照片持续播放 1 秒。
  - 编码：H.264 (`libx264`)，BT.709 色彩标准 (`colorspace`/`color_primaries`/`color_trc` = `bt709`, `color_range` = `tv`)，画质 CRF ≈ 20 (`-crf 20`)。
  - 音频：无音轨 (`-an`)。
  - 输出格式：`.mp4` (`gopro3_timelapse.mp4`)，启用 `+faststart`。

# 编译与交叉编译指南
在 Go 1.27 环境下编译前，如根目录下无 `go.mod`，需先生成 `go.mod`：
```bash
go mod init gopro3_timelapse
```

通过 `go build` 构建，建议设置 `CGO_ENABLED=0` 实现静态编译：
- **Windows 64-bit Intel**:
  ```bash
  CGO_ENABLED=0 GOOS=windows GOARCH=amd64 go build -o gopro3_timelapse_win64.exe src/main.go
  ```
- **macOS 64-bit Intel**:
  ```bash
  CGO_ENABLED=0 GOOS=darwin GOARCH=amd64 go build -o gopro3_timelapse_macos_intel src/main.go
  ```
- **Linux 64-bit Intel**:
  ```bash
  CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -o gopro3_timelapse_linux_amd64 src/main.go
  ```
- **Linux ARM 64-bit**:
  ```bash
  CGO_ENABLED=0 GOOS=linux GOARCH=arm64 go build -o gopro3_timelapse_linux_arm64 src/main.go
  ```
- **FreeBSD ARM 64-bit**:
  ```bash
  CGO_ENABLED=0 GOOS=freebsd GOARCH=arm64 go build -o gopro3_timelapse_freebsd_arm64 src/main.go
  ```

# Go 模块文件说明 (`go.mod` 与 `go.sum`)
- **`go.mod`**：定义模块名称（`gopro3_timelapse`）及声明外部依赖（如 `github.com/rwcarlsen/goexif` 和 `golang.org/x/image`）。
- **`go.sum`**：记录所有依赖包及其特定版本的加密校验和（Cryptographic Checksums），用于防止依赖被篡改并确保团队与 CI 构建的一致性。
