package fonts

import (
	"bytes"
	"os"
	"path/filepath"
	"runtime"
	"testing"

	"github.com/signintech/gopdf"
)

// systemFont 返回测试用字体路径；不存在则跳过。
func systemFont(t *testing.T, name string) string {
	t.Helper()
	if runtime.GOOS != "windows" {
		t.Skip("Windows 字体仅用于本机验证")
	}
	dir := os.Getenv("WINDIR")
	if dir == "" {
		dir = `C:\Windows`
	}
	p := filepath.Join(dir, "Fonts", name)
	if _, err := os.Stat(p); err != nil {
		t.Skipf("缺少测试字体 %s", name)
	}
	return p
}

func TestSplitPlainTTF(t *testing.T) {
	path := systemFont(t, "simhei.ttf")
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	faces, err := Split(raw)
	if err != nil {
		t.Fatal(err)
	}
	if len(faces) != 1 {
		t.Fatalf("普通 ttf 应只有 1 项，得到 %d", len(faces))
	}
	if err := checkTrueType(faces[0]); err != nil {
		t.Fatalf("simhei.ttf 应可解析: %v", err)
	}
	if !hasCJK(faces[0]) {
		t.Fatal("simhei.ttf 应包含中文字形")
	}
}

func TestSplitTTC(t *testing.T) {
	path := systemFont(t, "msyh.ttc")
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	faces, err := Split(raw)
	if err != nil {
		t.Fatal(err)
	}
	if len(faces) == 0 {
		t.Fatal("msyh.ttc 未拆出任何字面")
	}
	var usable, cjk int
	for _, f := range faces {
		if checkTrueType(f) != nil {
			continue
		}
		usable++
		if hasCJK(f) {
			cjk++
		}
		// 拆出的字面必须是自洽的 sfnt：能被重新解析
		if _, err := tableMap(f); err != nil {
			t.Fatalf("拆出的字面无法解析: %v", err)
		}
	}
	if usable == 0 {
		t.Fatal("msyh.ttc 中没有可用的 TrueType 字面")
	}
	if cjk == 0 {
		t.Fatal("msyh.ttc 中没有覆盖中文的字面")
	}
	t.Logf("msyh.ttc: %d 张字面，%d 张可用，%d 张含中文", len(faces), usable, cjk)
}

// TestFindPicksCJK 确保查找逻辑不会选中没有中文字形的字体（如 DejaVu）。
func TestFindPicksCJK(t *testing.T) {
	face, err := Find()
	if err != nil {
		t.Skipf("本机无可用中文字体: %v", err)
	}
	data, err := face.Bytes()
	if err != nil {
		t.Fatal(err)
	}
	if !hasCJK(data) {
		t.Fatalf("Find 选中的字体不含中文: %s", face.Path)
	}
	t.Logf("使用字体: %s", face.Path)
}

// TestFaceUsableByGoPDF 是端到端验证：把拆出来的字节交给 gopdf 生成含中文的 PDF。
func TestFaceUsableByGoPDF(t *testing.T) {
	face, err := Find()
	if err != nil {
		t.Skipf("本机无可用中文字体: %v", err)
	}
	data, err := face.Bytes()
	if err != nil {
		t.Fatal(err)
	}

	p := &gopdf.GoPdf{}
	p.Start(gopdf.Config{PageSize: *gopdf.PageSizeA4})
	p.AddPage()
	if err := p.AddTTFFontData("cjk", data); err != nil {
		t.Fatalf("gopdf 无法加载字体: %v", err)
	}
	if err := p.SetFont("cjk", "", 12); err != nil {
		t.Fatalf("SetFont 失败: %v", err)
	}
	p.CellWithOption(&gopdf.Rect{W: 400, H: 20}, "中文测试：项目周报 / 验收报告",
		gopdf.CellOption{})

	var buf bytes.Buffer
	if err := p.Write(&buf); err != nil {
		t.Fatalf("生成 PDF 失败: %v", err)
	}
	if buf.Len() < 1000 {
		t.Fatalf("PDF 体积异常: %d 字节", buf.Len())
	}
	if !bytes.Contains(buf.Bytes(), []byte("/FontFile2")) {
		t.Fatal("PDF 未内嵌字体（FontFile2 缺失）")
	}
	t.Logf("PDF %d 字节，字体 %s", buf.Len(), face.Path)
}

// TestRejectNonTrueType 确认 CFF/OTF 会被明确拒绝而不是静默产出空字形。
func TestRejectNonTrueType(t *testing.T) {
	// 构造一个假的 sfnt：版本号是 OTTO（CFF 轮廓）
	fake := make([]byte, 64)
	copy(fake[0:4], []byte("OTTO"))
	if err := checkTrueType(fake); err == nil {
		t.Fatal("OTTO 字体应被拒绝")
	}
}
