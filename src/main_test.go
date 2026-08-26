package main

import (
	"image"
	"image/color"
	"image/jpeg"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"testing"

	"golang.org/x/image/font/gofont/goregular"
	"golang.org/x/image/font/opentype"
)

// 测试照片文件名正则匹配与升序排序
func TestPhotoMatchingAndSorting(t *testing.T) {
	re := regexp.MustCompile(`(?i)^G(\d+)\.(jpe?g)$`)

	filenames := []string{
		"G0033095.JPG",
		"g0013082.jpg",
		"G0023084.JPEG",
		"random.txt",
		"G0003080.JPG",
	}

	type testPhoto struct {
		path string
		num  int
	}

	var photos []testPhoto
	for _, name := range filenames {
		m := re.FindStringSubmatch(name)
		if m == nil {
			continue
		}
		n, err := strconv.Atoi(m[1])
		if err != nil {
			continue
		}
		photos = append(photos, testPhoto{path: name, num: n})
	}

	if len(photos) != 4 {
		t.Errorf("期望匹配 4 张照片，实际匹配 %d 张", len(photos))
	}

	sort.Slice(photos, func(i, j int) bool {
		return photos[i].num < photos[j].num
	})

	expectedOrder := []string{
		"G0003080.JPG",
		"g0013082.jpg",
		"G0023084.JPEG",
		"G0033095.JPG",
	}

	for i, p := range photos {
		if p.path != expectedOrder[i] {
			t.Errorf("索引 %d 排序错误: 期望 %s, 实际 %s", i, expectedOrder[i], p.path)
		}
	}
}

// 测试图片时间戳叠加与 Alpha 混合逻辑
func TestImageTimestampOverlay(t *testing.T) {
	// 创建一个测试用的小图片 (100x100 RGB)
	img := image.NewRGBA(image.Rect(0, 0, 100, 100))
	for y := 0; y < 100; y++ {
		for x := 0; x < 100; x++ {
			img.Set(x, y, color.RGBA{R: 255, G: 255, B: 255, A: 255})
		}
	}

	ttfFont, err := opentype.Parse(goregular.TTF)
	if err != nil {
		t.Fatalf("解析内置字体失败: %v", err)
	}

	fontFace, err := opentype.NewFace(ttfFont, &opentype.FaceOptions{
		Size: 12,
		DPI:  72,
	})
	if err != nil {
		t.Fatalf("创建字体 Face 失败: %v", err)
	}
	defer fontFace.Close()

	// 执行绘制
	drawTimestampOverlay(img, "2023-10-01 12:34:56", fontFace)

	// 验证左上角区域已被修改（背景框或文字改变了像素颜色）
	topLeftColor := img.At(25, 25).(color.RGBA)
	// 原图是纯白 (255, 255, 255)，叠加半透明黑底或白字后颜色应发生变化
	if topLeftColor.R == 255 && topLeftColor.G == 255 && topLeftColor.B == 255 {
		t.Errorf("左上角时间戳 overlay 未能正确渲染到图像上")
	}
}

// 测试时间戳提取降级逻辑（文件不存在时返回默认格式）
func TestExtractTimestampFallback(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "test_exif_*")
	if err != nil {
		t.Fatalf("创建临时目录失败: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	dummyPath := filepath.Join(tmpDir, "G0010001.JPG")
	err = os.WriteFile(dummyPath, []byte("fake jpeg content"), 0644)
	if err != nil {
		t.Fatalf("写入测试文件失败: %v", err)
	}

	ts := extractTimestamp(dummyPath)
	if ts == "" || ts == "0000-00-00 00:00:00" {
		// 如果无 EXIF，应降级为文件的 mod time 格式化字符串
		fi, _ := os.Stat(dummyPath)
		expected := fi.ModTime().Format("2006-01-02 15:04:05")
		if ts != expected {
			t.Errorf("期望降级为文件修改时间 %s, 实际得到 %s", expected, ts)
		}
	}
}

// 测试路径转义逻辑
func TestEscapePath(t *testing.T) {
	tests := []struct {
		input    string
		expected string
	}{
		{"normal/path/photo.jpg", "normal/path/photo.jpg"},
		{"path with 'single' quotes.jpg", "path with '\\''single'\\'' quotes.jpg"},
	}

	for _, tt := range tests {
		actual := escapePath(tt.input)
		if actual != tt.expected {
			t.Errorf("escapePath(%q) = %q, 期望 %q", tt.input, actual, tt.expected)
		}
	}
}

// 测试 Alpha 颜色混合逻辑
func TestBlendRGBA(t *testing.T) {
	dst := color.RGBA{R: 255, G: 255, B: 255, A: 255}
	src := color.RGBA{R: 0, G: 0, B: 0, A: 160}

	result := blendRGBA(dst, src)
	if result.A != 255 {
		t.Errorf("期望合成后 Alpha 为 255, 实际得到 %d", result.A)
	}
	if result.R >= 255 || result.G >= 255 || result.B >= 255 {
		t.Errorf("期望白色背景与半透明黑混合后颜色变暗，实际 R=%d, G=%d, B=%d", result.R, result.G, result.B)
	}

	// 测试透明度为 0 的边界情况
	zeroSrc := color.RGBA{R: 100, G: 100, B: 100, A: 0}
	zeroResult := blendRGBA(dst, zeroSrc)
	if zeroResult != dst {
		t.Errorf("当 src 透明度为 0 时，期望返回 dst 原色 %v, 实际得到 %v", dst, zeroResult)
	}
}

// 测试照片时间戳处理与保存
func TestProcessPhotoWithTimestamp(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "test_process_*")
	if err != nil {
		t.Fatalf("创建临时目录失败: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	srcImgPath := filepath.Join(tmpDir, "G0010001.JPG")
	img := image.NewRGBA(image.Rect(0, 0, 200, 200))
	for y := 0; y < 200; y++ {
		for x := 0; x < 200; x++ {
			img.Set(x, y, color.RGBA{R: 0, G: 0, B: 255, A: 255})
		}
	}
	f, err := os.Create(srcImgPath)
	if err != nil {
		t.Fatalf("创建测试源图失败: %v", err)
	}
	jpeg.Encode(f, img, &jpeg.Options{Quality: 90})
	f.Close()

	ttfFont, err := opentype.Parse(goregular.TTF)
	if err != nil {
		t.Fatalf("解析字体失败: %v", err)
	}
	fontFace, err := opentype.NewFace(ttfFont, &opentype.FaceOptions{Size: 16, DPI: 72})
	if err != nil {
		t.Fatalf("创建字体 Face 失败: %v", err)
	}
	defer fontFace.Close()

	outDir := filepath.Join(tmpDir, "out")
	os.MkdirAll(outDir, 0755)

	outPath, err := processPhotoWithTimestamp(srcImgPath, "2026-06-06 12:00:00", outDir, fontFace, 0)
	if err != nil {
		t.Fatalf("processPhotoWithTimestamp 失败: %v", err)
	}

	if _, err := os.Stat(outPath); os.IsNotExist(err) {
		t.Errorf("期望生成输出文件 %s，但文件不存在", outPath)
	}

	_, err = processPhotoWithTimestamp("non_existent_file.jpg", "2026-06-06 12:00:00", outDir, fontFace, 1)
	if err == nil {
		t.Errorf("期望处理不存在的文件时返回错误，实际为 nil")
	}
}
