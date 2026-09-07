package main

import (
	"fmt"
	"image"
	"image/color"
	"image/draw"
	"image/jpeg"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"

	"github.com/rwcarlsen/goexif/exif"
	"golang.org/x/image/font"
	"golang.org/x/image/font/gofont/goregular"
	"golang.org/x/image/font/opentype"
	"golang.org/x/image/math/fixed"
)

type photo struct {
	path      string
	num       int
	timestamp string
}

func main() {
	// 匹配 G0013082.JPG 这类文件名
	re := regexp.MustCompile(`(?i)^G(\d+)\.(jpe?g)$`)

	entries, err := os.ReadDir(".")
	if err != nil {
		fmt.Fprintf(os.Stderr, "读取当前目录失败: %v\n", err)
		os.Exit(1)
	}

	var photos []photo
	for _, e := range entries {
		if e.IsDir() {
			continue
		}
		name := e.Name()
		m := re.FindStringSubmatch(name)
		if m == nil {
			continue
		}
		n, err := strconv.Atoi(m[1])
		if err != nil {
			continue
		}

		// 提取拍摄时间戳
		ts := extractTimestamp(name)
		photos = append(photos, photo{path: name, num: n, timestamp: ts})
	}

	if len(photos) == 0 {
		fmt.Fprintln(os.Stderr, "当前目录未找到匹配的 GoPro3 时延摄影照片 (Gxxxxxx.JPG)")
		os.Exit(1)
	}

	// 按数字编号升序排序（旧 → 新）
	sort.Slice(photos, func(i, j int) bool {
		return photos[i].num < photos[j].num
	})

	fmt.Printf("找到 %d 张照片，开始处理时间戳并生成视频...\n", len(photos))

	// 创建临时目录存放加过时间戳的帧图片
	tmpDir, err := os.MkdirTemp("", "gopro3_frames_*")
	if err != nil {
		fmt.Fprintf(os.Stderr, "创建临时目录失败: %v\n", err)
		os.Exit(1)
	}
	defer os.RemoveAll(tmpDir)

	// 加载字体用于绘制硬字幕
	ttfFont, err := opentype.Parse(goregular.TTF)
	if err != nil {
		fmt.Fprintf(os.Stderr, "解析内置字体失败: %v\n", err)
		os.Exit(1)
	}

	processedPhotos := make([]string, len(photos))
	for i, p := range photos {
		processedPath, err := processPhotoWithTimestamp(p.path, p.timestamp, tmpDir, ttfFont, i)
		if err != nil {
			fmt.Fprintf(os.Stderr, "处理照片 %s 失败: %v\n", p.path, err)
			os.Exit(1)
		}
		processedPhotos[i] = processedPath
	}

	// 生成 concat 列表文件
	listFile := filepath.Join(tmpDir, "gopro3_concat_list.txt")
	f, err := os.Create(listFile)
	if err != nil {
		fmt.Fprintf(os.Stderr, "创建列表文件失败: %v\n", err)
		os.Exit(1)
	}

	for _, pPath := range processedPhotos {
		fmt.Fprintf(f, "file '%s'\n", escapePath(pPath))
		fmt.Fprintf(f, "duration 1\n")
	}
	// 最后一张需要再写一次（concat demuxer 已知问题）
	if len(processedPhotos) > 0 {
		fmt.Fprintf(f, "file '%s'\n", escapePath(processedPhotos[len(processedPhotos)-1]))
	}
	f.Close()

	outName := "gopro3_timelapse.mp4"

	// 构建 ffmpeg 命令
	args := []string{
		"-y", // 覆盖输出
		"-f", "concat",
		"-safe", "0",
		"-i", listFile,
		"-vf", "scale=1280:720:force_original_aspect_ratio=decrease,pad=1280:720:(ow-iw)/2:(oh-ih)/2:black,fps=30,format=yuv420p",
		"-c:v", "libx264",
		"-preset", "medium",
		"-crf", "20",
		"-colorspace", "bt709",
		"-color_primaries", "bt709",
		"-color_trc", "bt709",
		"-color_range", "tv",
		"-an", // 无音轨
		"-movflags", "+faststart",
		outName,
	}

	cmd := exec.Command("ffmpeg", args...)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr

	fmt.Println("正在调用 ffmpeg 生成视频，请稍候...")
	if err := cmd.Run(); err != nil {
		fmt.Fprintf(os.Stderr, "ffmpeg 执行失败: %v\n", err)
		fmt.Fprintln(os.Stderr, "请确认系统已安装 ffmpeg 并在 PATH 中")
		os.Exit(1)
	}

	fmt.Printf("完成！视频已输出: %s\n", outName)
}

// 提取照片 EXIF 拍摄时间，若无则降级为文件修改时间或占位符
func extractTimestamp(path string) string {
	f, err := os.Open(path)
	if err != nil {
		return "0000-00-00 00:00:00"
	}
	defer f.Close()

	x, err := exif.Decode(f)
	if err != nil {
		// 降级使用文件修改时间
		if fi, statErr := os.Stat(path); statErr == nil {
			return fi.ModTime().Format("2006-01-02 15:04:05")
		}
		return "0000-00-00 00:00:00"
	}

	tm, err := x.DateTime()
	if err != nil {
		if fi, statErr := os.Stat(path); statErr == nil {
			return fi.ModTime().Format("2006-01-02 15:04:05")
		}
		return "0000-00-00 00:00:00"
	}

	return tm.Format("2006-01-02 15:04:05")
}

// 在照片左上角绘制时间戳字幕，并保存到临时目录
func processPhotoWithTimestamp(srcPath, timestamp, outDir string, ttfFont *opentype.Font, index int) (string, error) {
	file, err := os.Open(srcPath)
	if err != nil {
		return "", err
	}
	defer file.Close()

	img, _, err := image.Decode(file)
	if err != nil {
		return "", err
	}

	bounds := img.Bounds()
	rgba := image.NewRGBA(bounds)
	draw.Draw(rgba, bounds, img, bounds.Min, draw.Src)

	// 如果时间戳为空，给个默认值
	if timestamp == "" {
		timestamp = "0000-00-00 00:00:00"
	}

	// 字体行高从固定尺寸改为原始画面高度的 3.5%
	fontSize := float64(bounds.Dy()) * 0.035
	if fontSize < 8 {
		fontSize = 8
	}
	fontFace, err := opentype.NewFace(ttfFont, &opentype.FaceOptions{
		Size: fontSize,
		DPI:  72,
	})
	if err != nil {
		return "", err
	}
	defer fontFace.Close()

	// 绘制左上角字幕：半透明背景框 + 白色文字
	drawTimestampOverlay(rgba, timestamp, fontFace)

	outPath := filepath.Join(outDir, fmt.Sprintf("frame_%06d.jpg", index))
	outFile, err := os.Create(outPath)
	if err != nil {
		return "", err
	}
	defer outFile.Close()

	err = jpeg.Encode(outFile, rgba, &jpeg.Options{Quality: 95})
	if err != nil {
		return "", err
	}

	return outPath, nil
}

// 在 RGBA 图像左上角绘制时间戳文字及半透明背景
func drawTimestampOverlay(img *image.RGBA, text string, fontFace font.Face) {
	// 计算文字宽度与高度
	textWidth := font.MeasureString(fontFace, text).Ceil()
	// 简单估算行高 (例如 Size 32 对应 line height 约 38)
	metrics := fontFace.Metrics()
	textHeight := (metrics.Ascent + metrics.Descent).Ceil()
	if textHeight <= 0 {
		textHeight = 36
	}

	paddingX := 16
	paddingY := 12
	boxX := 20
	boxY := 20
	boxWidth := textWidth + paddingX*2
	boxHeight := textHeight + paddingY*2

	// 1. 绘制半透明黑色背景框
	bgColor := color.RGBA{R: 0, G: 0, B: 0, A: 160} // 半透明黑
	for y := boxY; y < boxY+boxHeight; y++ {
		for x := boxX; x < boxX+boxWidth; x++ {
			if x >= img.Bounds().Min.X && x < img.Bounds().Max.X && y >= img.Bounds().Min.Y && y < img.Bounds().Max.Y {
				// Alpha 混合
				origColor := img.At(x, y).(color.RGBA)
				img.Set(x, y, blendRGBA(origColor, bgColor))
			}
		}
	}

	// 2. 绘制白色文字
	d := &font.Drawer{
		Dst:  img,
		Src:  image.NewUniform(color.White),
		Face: fontFace,
		Dot:  fixed.Point26_6{X: fixed.I(boxX + paddingX), Y: fixed.I(boxY + paddingY + metrics.Ascent.Ceil())},
	}
	d.DrawString(text)
}

// Alpha 颜色混合
func blendRGBA(dst, src color.RGBA) color.RGBA {
	srcA := float64(src.A) / 255.0
	dstA := float64(dst.A) / 255.0
	outA := srcA + dstA*(1-srcA)
	if outA == 0 {
		return color.RGBA{0, 0, 0, 0}
	}
	outR := (float64(src.R)*srcA + float64(dst.R)*dstA*(1-srcA)) / outA
	outG := (float64(src.G)*srcA + float64(dst.G)*dstA*(1-srcA)) / outA
	outB := (float64(src.B)*srcA + float64(dst.B)*dstA*(1-srcA)) / outA
	return color.RGBA{
		R: uint8(outR + 0.5),
		G: uint8(outG + 0.5),
		B: uint8(outB + 0.5),
		A: uint8(outA*255 + 0.5),
	}
}

// 简单转义单引号，防止路径中有特殊字符
func escapePath(p string) string {
	return strings.ReplaceAll(p, "'", "'\\''")
}
