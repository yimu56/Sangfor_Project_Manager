// Package fonts 负责在运行环境中找到一份可用于 PDF 导出的中文字体。
//
// 背景：gopdf 只支持 TrueType（glyf 轮廓）字体，而 Linux 上常见的中文字体
// 要么是 TrueType Collection（.ttc，如 wqy-zenhei、arphic uming），要么是
// CFF/OTF 轮廓（如 Noto Sans CJK 的 .ttc/.otf）。直接调用
// gopdf.AddTTFFont 加载 .ttc 会失败（解析器要求文件头是 0x00010000），
// 退回 DejaVuSans.ttf 又能加载但没有中文字形，导出结果会丢字。
//
// 因此这里做三件事：
//  1. 把 .ttc 里的单个字体拆成独立的 TrueType 字节流，交给 gopdf 使用；
//  2. 解析 cmap 校验字体是否真的覆盖中文，避免选中无中文字形的兜底字体；
//  3. 按“环境变量 → 各系统常见路径 → fc-list → 字体目录扫描”的顺序查找。
package fonts

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"log"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"sort"
	"strings"
	"sync"
	"time"
)

// Face 指字体文件中的某一张字面（.ttc 内可能有多张）。
type Face struct {
	Path  string
	Index int
	Name  string
}

func (f *Face) String() string { return f.Path }

var (
	cacheMu sync.Mutex
	cached  *Face
)

// ErrNoFont 表示当前系统里没有找到可用的中文字体。
var ErrNoFont = errors.New("未找到含中文字形的 TrueType 字体（可安装 fonts-wqy-zenhei / " +
	"fonts-arphic-uming / fonts-droid-fallback，或用环境变量 PM_PDF_FONT 指定 .ttf/.ttc 文件）")

// Find 返回第一个可用的中文字体，结果会缓存（只缓存成功结果，
// 这样运行期安装了字体后无需重启也能生效）。
func Find() (*Face, error) {
	cacheMu.Lock()
	defer cacheMu.Unlock()
	if cached != nil {
		return cached, nil
	}
	seen := make(map[string]bool)
	for _, p := range candidatePaths() {
		if p == "" || seen[p] {
			continue
		}
		seen[p] = true
		if f := inspect(p); f != nil {
			cached = f
			log.Printf("PDF 字体: %s", f.Path)
			return f, nil
		}
	}
	return nil, ErrNoFont
}

// Bytes 读取并返回该字面独立的 TrueType 字节流（.ttc 会被拆出其中一张）。
func (f *Face) Bytes() ([]byte, error) {
	if f == nil {
		return nil, errors.New("字体为空")
	}
	raw, err := os.ReadFile(f.Path)
	if err != nil {
		return nil, err
	}
	faces, err := Split(raw)
	if err != nil {
		return nil, err
	}
	if f.Index < 0 || f.Index >= len(faces) {
		return nil, fmt.Errorf("字体索引 %d 超出范围（可能字体文件已被替换）", f.Index)
	}
	face := faces[f.Index]
	if err := checkTrueType(face); err != nil {
		return nil, err
	}
	return face, nil
}

// inspect 检查一个字体文件是否可直接用于中文 PDF。
func inspect(path string) *Face {
	info, err := os.Stat(path)
	if err != nil || !info.Mode().IsRegular() || info.Size() < 1024 || info.Size() > 256<<20 {
		return nil
	}
	raw, err := os.ReadFile(path)
	if err != nil {
		return nil
	}
	faces, err := Split(raw)
	if err != nil {
		return nil
	}
	for i, face := range faces {
		if checkTrueType(face) != nil {
			continue
		}
		if !hasCJK(face) {
			continue
		}
		return &Face{Path: path, Index: i, Name: filepath.Base(path)}
	}
	return nil
}

// candidatePaths 按优先级返回待检查的字体路径。
func candidatePaths() []string {
	var out []string
	if v := strings.TrimSpace(os.Getenv("PM_PDF_FONT")); v != "" {
		for _, p := range strings.Split(v, string(os.PathListSeparator)) {
			if p = strings.TrimSpace(p); p != "" {
				out = append(out, p)
			}
		}
	}
	out = append(out, platformPaths()...)
	out = append(out, fcListPaths()...)
	out = append(out, scanPaths()...)
	return out
}

// platformPaths 列出各系统上最常见的中文字体路径。
func platformPaths() []string {
	switch runtime.GOOS {
	case "windows":
		dir := os.Getenv("WINDIR")
		if dir == "" {
			dir = `C:\Windows`
		}
		fonts := filepath.Join(dir, "Fonts")
		return []string{
			filepath.Join(fonts, "simhei.ttf"),
			filepath.Join(fonts, "Deng.ttf"),
			filepath.Join(fonts, "simkai.ttf"),
			filepath.Join(fonts, "simsun.ttc"),
			filepath.Join(fonts, "msyh.ttc"),
			filepath.Join(fonts, "simfang.ttf"),
		}
	case "darwin":
		return []string{
			"/System/Library/Fonts/PingFang.ttc",
			"/System/Library/Fonts/STHeiti Medium.ttc",
			"/System/Library/Fonts/Hiragino Sans GB.ttc",
			"/System/Library/Fonts/Supplemental/Songti.ttc",
			"/Library/Fonts/Arial Unicode.ttf",
		}
	default:
		return []string{
			// 真 TrueType（glyf）字体，命中率最高
			"/usr/share/fonts/truetype/droid/DroidSansFallbackFull.ttf",
			"/usr/share/fonts/truetype/droid/DroidSansFallback.ttf",
			"/usr/share/fonts/truetype/wqy/wqy-microhei.ttc",
			"/usr/share/fonts/truetype/wqy/wqy-zenhei.ttc",
			"/usr/share/fonts/truetype/arphic/uming.ttc",
			"/usr/share/fonts/truetype/arphic/ukai.ttc",
			"/usr/share/fonts/wqy-zenhei/wqy-zenhei.ttc",
			"/usr/share/fonts/wqy-microhei/wqy-microhei.ttc",
			"/usr/share/fonts/truetype/wqy/wqy-zenhei.ttf",
			"/usr/share/fonts/truetype/dejavu/DejaVuSans.ttf", // 无中文字形，会被 cmap 校验挡掉
		}
	}
}

// fcListPaths 用 fontconfig 查出所有标称支持中文的字体（最快、最准）。
func fcListPaths() []string {
	if runtime.GOOS == "windows" {
		return nil
	}
	if _, err := exec.LookPath("fc-list"); err != nil {
		return nil
	}
	ctx, cancel := context.WithTimeout(context.Background(), 1500*time.Millisecond)
	defer cancel()
	out, err := exec.CommandContext(ctx, "fc-list", ":lang=zh", "-f", "%{file}\n").Output()
	if err != nil {
		return nil
	}
	var paths []string
	for _, line := range strings.Split(string(out), "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		// 某些 fontconfig 版本会输出 "file: /path"
		if i := strings.Index(line, ": "); i > 0 && strings.HasPrefix(line, "file:") {
			line = strings.TrimSpace(line[i+2:])
		}
		paths = append(paths, line)
	}
	return rank(paths)
}

// scanPaths 扫描系统字体目录，按文件名挑出可能是中文字体的文件。
func scanPaths() []string {
	var roots []string
	switch runtime.GOOS {
	case "windows":
		if dir := os.Getenv("WINDIR"); dir != "" {
			roots = append(roots, filepath.Join(dir, "Fonts"))
		}
	case "darwin":
		roots = append(roots,
			"/System/Library/Fonts",
			"/Library/Fonts",
		)
		if home, err := os.UserHomeDir(); err == nil {
			roots = append(roots, filepath.Join(home, "Library/Fonts"))
		}
	default:
		roots = append(roots, "/usr/share/fonts", "/usr/local/share/fonts")
		if home, err := os.UserHomeDir(); err == nil {
			roots = append(roots, filepath.Join(home, ".local/share/fonts"), filepath.Join(home, ".fonts"))
		}
	}
	var found []string
	for _, root := range roots {
		_ = filepath.WalkDir(root, func(path string, e fs.DirEntry, err error) error {
			if err != nil {
				return nil
			}
			if e.IsDir() {
				rel, rerr := filepath.Rel(root, path)
				if rerr == nil && strings.Count(rel, string(os.PathSeparator)) >= 4 {
					return fs.SkipDir
				}
				return nil
			}
			if len(found) >= 64 {
				return fs.SkipAll
			}
			name := strings.ToLower(e.Name())
			if !strings.HasSuffix(name, ".ttf") && !strings.HasSuffix(name, ".ttc") {
				return nil
			}
			if !looksCJK(name) {
				return nil
			}
			found = append(found, path)
			return nil
		})
	}
	return rank(found)
}

// looksCJK 依据文件名判断是否值得解析。
func looksCJK(name string) bool {
	for _, k := range []string{
		"cjk", "wqy", "droid", "hei", "song", "kai", "ming", "yahei",
		"pingfang", "hiragino", "uming", "ukai", "arphic", "simsun",
		"simhei", "sourcehan", "sarasa", "notosanssc", "notoserifsc",
	} {
		if strings.Contains(name, k) {
			return true
		}
	}
	return false
}

// rank 让更可能成功的字体排在前面（Droid/WQY > 黑体/宋体 > 其他）。
func rank(paths []string) []string {
	score := func(p string) int {
		n := strings.ToLower(filepath.Base(p))
		s := 0
		switch {
		case strings.Contains(n, "droid"):
			s += 6
		case strings.Contains(n, "wqy"):
			s += 5
		case strings.Contains(n, "uming"), strings.Contains(n, "ukai"):
			s += 4
		case strings.Contains(n, "hei"), strings.Contains(n, "song"),
			strings.Contains(n, "kai"), strings.Contains(n, "ming"):
			s += 3
		}
		if strings.HasSuffix(n, ".ttf") {
			s++ // .ttf 无需拆包
		}
		return s
	}
	out := append([]string(nil), paths...)
	sort.SliceStable(out, func(i, j int) bool { return score(out[i]) > score(out[j]) })
	return out
}
