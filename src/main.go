package main

import (
    "fmt"
    "os"
    "os/exec"
    "regexp"
    "sort"
    "strconv"
    "strings"
)

type photo struct {
    path string
    num  int
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
        photos = append(photos, photo{path: name, num: n})
    }

    if len(photos) == 0 {
        fmt.Fprintln(os.Stderr, "当前目录未找到匹配的 GoPro3 时延摄影照片 (Gxxxxxx.JPG)")
        os.Exit(1)
    }

    // 按数字编号升序排序（旧 → 新）
    sort.Slice(photos, func(i, j int) bool {
        return photos[i].num < photos[j].num
    })

    fmt.Printf("找到 %d 张照片，按编号排序后生成视频...\n", len(photos))

    // 生成 concat 列表文件
    listFile := "gopro3_concat_list.txt"
    f, err := os.Create(listFile)
    if err != nil {
        fmt.Fprintf(os.Stderr, "创建列表文件失败: %v\n", err)
        os.Exit(1)
    }
    defer os.Remove(listFile) // 结束后清理

    for _, p := range photos {
        // 使用绝对路径更安全，但相对路径也可用；这里用相对
        fmt.Fprintf(f, "file '%s'\n", escapePath(p.path))
        fmt.Fprintf(f, "duration 1\n")
    }
    // 最后一张需要再写一次（concat demuxer 已知问题）
    last := photos[len(photos)-1]
    fmt.Fprintf(f, "file '%s'\n", escapePath(last.path))
    f.Close()

    outName := "gopro3_timelapse.mp4"

    // 构建 ffmpeg 命令
    // scale + pad 保持原始比例 + 黑边填充到 1280x720
    // fps=30 输出 30fps，每张图持续 1 秒（30 帧）
    args := []string{
        "-y", // 覆盖输出
        "-f", "concat",
        "-safe", "0",
        "-i", listFile,
        "-vf", "scale=1280:720:force_original_aspect_ratio=decrease,pad=1280:720:(ow-iw)/2:(oh-ih)/2:black,fps=30,format=yuv420p",
        "-c:v", "libx264",
        "-preset", "medium",
        "-crf", "20", // ≈ 80%+ 画质（数值越小画质越高，18~23 常用）
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

// 简单转义单引号，防止路径中有特殊字符
func escapePath(p string) string {
    return strings.ReplaceAll(p, "'", "'\\''")
}
