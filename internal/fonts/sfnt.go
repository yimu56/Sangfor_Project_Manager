package fonts

import (
	"bytes"
	"encoding/binary"
	"errors"
	"fmt"
)

const ttcTag = "ttcf"

// Split 把字体文件拆成若干张独立的 TrueType 字面。
// 普通 .ttf 原样返回一项；.ttc 则逐个重建独立的 sfnt 文件（表偏移改为相对新文件）。
func Split(data []byte) ([][]byte, error) {
	if len(data) < 12 {
		return nil, errors.New("字体文件过小")
	}
	if string(data[0:4]) != ttcTag {
		return [][]byte{data}, nil
	}
	numFonts := int(be32(data[8:12]))
	if numFonts <= 0 || numFonts > 1024 || len(data) < 12+4*numFonts {
		return nil, errors.New("TTC 头部异常")
	}
	faces := make([][]byte, 0, numFonts)
	var firstErr error
	for i := 0; i < numFonts; i++ {
		off := int(be32(data[12+4*i : 16+4*i]))
		face, err := extractFace(data, off)
		if err != nil {
			if firstErr == nil {
				firstErr = err
			}
			continue
		}
		faces = append(faces, face)
	}
	if len(faces) == 0 {
		return nil, fmt.Errorf("TTC 内没有可用的 TrueType 字体: %w", firstErr)
	}
	return faces, nil
}

type sfntTable struct {
	tag      string
	checksum uint32
	offset   int
	length   int
}

// extractFace 从 TTC 的 base 偏移处读出一张字面，并重建成独立字体文件。
func extractFace(data []byte, base int) ([]byte, error) {
	if base < 0 || base+12 > len(data) {
		return nil, errors.New("字体偏移越界")
	}
	if v := be32(data[base : base+4]); v != 0x00010000 {
		return nil, fmt.Errorf("不支持的字体轮廓（0x%08X）", v)
	}
	n := int(be16(data[base+4 : base+6]))
	if n <= 0 || n > 4096 || base+12+16*n > len(data) {
		return nil, errors.New("字体表目录异常")
	}
	tables := make([]sfntTable, 0, n)
	for i := 0; i < n; i++ {
		p := base + 12 + 16*i
		off := int(be32(data[p+8 : p+12]))
		ln := int(be32(data[p+12 : p+16]))
		if off < 0 || ln < 0 || off > len(data) || ln > len(data)-off {
			return nil, fmt.Errorf("字体表 %q 越界", string(data[p:p+4]))
		}
		tables = append(tables, sfntTable{
			tag:      string(data[p : p+4]),
			checksum: be32(data[p+4 : p+8]),
			offset:   off,
			length:   ln,
		})
	}

	out := bytes.NewBuffer(make([]byte, 0, len(data)))
	out.Write(data[base : base+12]) // sfnt 版本 + 表数量 + 搜索参数
	for _, t := range tables {
		var rec [16]byte
		copy(rec[0:4], t.tag)
		binary.BigEndian.PutUint32(rec[4:8], t.checksum)
		binary.BigEndian.PutUint32(rec[12:16], uint32(t.length))
		out.Write(rec[:])
	}
	buf := out.Bytes()
	for i, t := range tables {
		for len(buf)%4 != 0 {
			buf = append(buf, 0)
		}
		off := len(buf)
		buf = append(buf, data[t.offset:t.offset+t.length]...)
		binary.BigEndian.PutUint32(buf[12+i*16+8:12+i*16+12], uint32(off))
	}
	return buf, nil
}

// tableMap 解析 sfnt 表目录，返回 tag → 原始表数据。
func tableMap(data []byte) (map[string][]byte, error) {
	if len(data) < 12 {
		return nil, errors.New("字体数据不完整")
	}
	if v := be32(data[0:4]); v != 0x00010000 {
		return nil, fmt.Errorf("不是 TrueType 字体（0x%08X）", v)
	}
	n := int(be16(data[4:6]))
	if n <= 0 || n > 4096 || 12+16*n > len(data) {
		return nil, errors.New("字体表目录异常")
	}
	m := make(map[string][]byte, n)
	for i := 0; i < n; i++ {
		p := 12 + 16*i
		tag := string(data[p : p+4])
		off := int(be32(data[p+8 : p+12]))
		ln := int(be32(data[p+12 : p+16]))
		if off < 0 || ln < 0 || off > len(data) || ln > len(data)-off {
			return nil, fmt.Errorf("字体表 %q 越界", tag)
		}
		m[tag] = data[off : off+ln]
	}
	return m, nil
}

// checkTrueType 确认字体是 gopdf 能解析的 glyf 轮廓字体。
func checkTrueType(data []byte) error {
	t, err := tableMap(data)
	if err != nil {
		return err
	}
	for _, need := range []string{"head", "hhea", "maxp", "hmtx", "loca", "glyf", "cmap"} {
		if _, ok := t[need]; !ok {
			return fmt.Errorf("缺少 %s 表（可能是 CFF/OTF 轮廓，gopdf 不支持）", need)
		}
	}
	return nil
}

// hasCJK 检查 cmap 是否真的覆盖常用汉字，避免选中 DejaVu 之类无中文字形的字体。
func hasCJK(data []byte) bool {
	t, err := tableMap(data)
	if err != nil {
		return false
	}
	cm, ok := t["cmap"]
	if !ok {
		return false
	}
	for _, r := range []rune{'中', '文'} {
		if !cmapHas(cm, uint32(r)) {
			return false
		}
	}
	return true
}

func cmapHas(cm []byte, cp uint32) bool {
	if len(cm) < 4 {
		return false
	}
	n := int(be16(cm[2:4]))
	if n <= 0 || 4+8*n > len(cm) {
		return false
	}
	for i := 0; i < n; i++ {
		off := int(be32(cm[4+8*i+4 : 4+8*i+8]))
		if off < 0 || off+2 > len(cm) {
			continue
		}
		switch be16(cm[off : off+2]) {
		case 4:
			if cp <= 0xFFFF && cmap4Has(cm[off:], uint16(cp)) {
				return true
			}
		case 12:
			if cmap12Has(cm[off:], cp) {
				return true
			}
		}
	}
	return false
}

func cmap4Has(t []byte, cp uint16) bool {
	if len(t) < 14 {
		return false
	}
	seg := int(be16(t[6:8])) / 2
	if seg <= 0 {
		return false
	}
	endBase := 14
	startBase := endBase + 2*seg + 2 // 跳过 reservedPad
	deltaBase := startBase + 2*seg
	rangeBase := deltaBase + 2*seg
	if rangeBase+2*seg > len(t) {
		return false
	}
	for i := 0; i < seg; i++ {
		end := be16(t[endBase+2*i : endBase+2*i+2])
		if cp > end {
			continue
		}
		start := be16(t[startBase+2*i : startBase+2*i+2])
		if cp < start {
			continue
		}
		delta := int32(int16(be16(t[deltaBase+2*i : deltaBase+2*i+2])))
		ro := int(be16(t[rangeBase+2*i : rangeBase+2*i+2]))
		if ro == 0 {
			return uint16(int32(cp)+delta) != 0
		}
		addr := rangeBase + 2*i + ro + 2*int(cp-start)
		if addr < 0 || addr+2 > len(t) {
			return false
		}
		g := be16(t[addr : addr+2])
		if g == 0 {
			return false
		}
		return uint16(int32(g)+delta) != 0
	}
	return false
}

func cmap12Has(t []byte, cp uint32) bool {
	if len(t) < 16 {
		return false
	}
	n := int(be32(t[12:16]))
	if n <= 0 || 16+12*n > len(t) {
		return false
	}
	for i := 0; i < n; i++ {
		p := 16 + 12*i
		if cp >= be32(t[p:p+4]) && cp <= be32(t[p+4:p+8]) {
			return true
		}
	}
	return false
}

func be16(b []byte) uint16 {
	if len(b) < 2 {
		return 0
	}
	return binary.BigEndian.Uint16(b)
}

func be32(b []byte) uint32 {
	if len(b) < 4 {
		return 0
	}
	return binary.BigEndian.Uint32(b)
}
