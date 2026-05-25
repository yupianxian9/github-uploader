package main

import (
	"archive/zip"
	"bytes"
	"fmt"
	"hash/crc32"
	"html"
	"os"
	"path/filepath"
	"strings"

	"github.com/alecthomas/chroma/v2"
	"github.com/alecthomas/chroma/v2/lexers"
	"github.com/alecthomas/chroma/v2/styles"
)

type coloredSegment struct {
	Text  string
	Color string // OOXML RRGGBB (e.g. "FF0000"), empty = default
}

type codeLine struct {
	Segments []coloredSegment
}

const (
	monoFont    = "Consolas"
	fontSizeHP  = "18"
	fontSizeHPC = "18"
)

func main() {
	fmt.Println("========================================")
	fmt.Println("  HighlightExporter - 代码高亮导出工具")
	fmt.Println("========================================")

	if err := run(); err != nil {
		fmt.Fprintf(os.Stderr, "错误: %v\n", err)
	}

	fmt.Print("按 Enter 键退出...")
	fmt.Scanln()
}

func run() error {
	if _, err := os.Stat("input"); os.IsNotExist(err) {
		return fmt.Errorf("找不到 input 目录，请创建 input 文件夹并放入代码文件")
	}

	entries, err := os.ReadDir("input")
	if err != nil {
		return fmt.Errorf("无法读取 input 目录: %w", err)
	}

	var files []os.DirEntry
	for _, e := range entries {
		if !e.IsDir() {
			files = append(files, e)
		}
	}
	if len(files) == 0 {
		return fmt.Errorf("input 目录中没有文件")
	}

	style := styles.Get("github")
	if style == nil {
		fmt.Println("警告: GitHub 主题不可用，使用默认主题")
		style = styles.Fallback
	}

	var lines []codeLine

	for _, f := range files {
		name := f.Name()
		path := filepath.Join("input", name)

		content, err := os.ReadFile(path)
		if err != nil {
			fmt.Printf("跳过 %s: %v\n", name, err)
			continue
		}
		if len(content) == 0 {
			fmt.Printf("跳过 %s: 空文件\n", name)
			continue
		}

		lexer := lexers.Match(name)
		if lexer == nil {
			lexer = lexers.Fallback
		}
		lexer = chroma.Coalesce(lexer)

		it, err := lexer.Tokenise(nil, string(content))
		if err != nil {
			fmt.Printf("跳过 %s: 词法分析失败: %v\n", name, err)
			continue
		}

		fmt.Printf("处理: %s\n", name)

		lines = append(lines, codeLine{Segments: []coloredSegment{{Text: "// ── " + name + " ──", Color: "999999"}}})
		lines = append(lines, codeLine{})

		var cur codeLine
		for {
			tok := it()
			if tok == chroma.EOF {
				break
			}

			entry := style.Get(tok.Type)
			color := ""
			if entry.Colour != 0 {
				color = strings.TrimPrefix(entry.Colour.String(), "#")
			}

			parts := strings.Split(tok.Value, "\n")
			for i, part := range parts {
				if i > 0 {
					if len(cur.Segments) > 0 {
						lines = append(lines, cur)
						cur = codeLine{}
					} else {
						lines = append(lines, codeLine{})
					}
				}
				if part != "" {
					cur.Segments = append(cur.Segments, coloredSegment{
						Text:  part,
						Color: color,
					})
				}
			}
		}
		if len(cur.Segments) > 0 {
			lines = append(lines, cur)
		}
		lines = append(lines, codeLine{})
	}

	return generateDocx(lines)
}

// addZipFile writes a file into the ZIP with OPC-compliant headers
// (CRC+size in local header, no data descriptor bit).
func addZipFile(zw *zip.Writer, name, content string) {
	data := []byte(content)
	crc := crc32.ChecksumIEEE(data)
	fw, err := zw.CreateRaw(&zip.FileHeader{
		Name:               name,
		Method:             zip.Store,
		Flags:              0, // no data descriptor (OPC requirement)
		CRC32:              crc,
		CompressedSize64:   uint64(len(data)),
		UncompressedSize64: uint64(len(data)),
	})
	if err != nil {
		panic(err)
	}
	_, err = fw.Write(data)
	if err != nil {
		panic(err)
	}
}

func generateDocx(lines []codeLine) error {
	buf := new(bytes.Buffer)
	zw := zip.NewWriter(buf)

	addZipFile(zw, "[Content_Types].xml",
		`<?xml version="1.0" encoding="UTF-8" standalone="yes"?>
<Types xmlns="http://schemas.openxmlformats.org/package/2006/content-types">
  <Default Extension="rels" ContentType="application/vnd.openxmlformats-package.relationships+xml"/>
  <Default Extension="xml" ContentType="application/xml"/>
  <Override PartName="/word/document.xml" ContentType="application/vnd.openxmlformats-officedocument.wordprocessingml.document.main+xml"/>
  <Override PartName="/word/styles.xml" ContentType="application/vnd.openxmlformats-officedocument.wordprocessingml.styles+xml"/>
</Types>`)

	addZipFile(zw, "_rels/.rels",
		`<?xml version="1.0" encoding="UTF-8" standalone="yes"?>
<Relationships xmlns="http://schemas.openxmlformats.org/package/2006/relationships">
  <Relationship Id="rId1" Type="http://schemas.openxmlformats.org/officeDocument/2006/relationships/officeDocument" Target="word/document.xml"/>
</Relationships>`)

	addZipFile(zw, "word/_rels/document.xml.rels",
		`<?xml version="1.0" encoding="UTF-8" standalone="yes"?>
<Relationships xmlns="http://schemas.openxmlformats.org/package/2006/relationships">
  <Relationship Id="rId1" Type="http://schemas.openxmlformats.org/officeDocument/2006/relationships/styles" Target="styles.xml"/>
</Relationships>`)

	addZipFile(zw, "docProps/core.xml",
		`<?xml version="1.0" encoding="UTF-8" standalone="yes"?>
<cp:coreProperties xmlns:cp="http://schemas.openxmlformats.org/package/2006/metadata/core-properties" xmlns:dc="http://purl.org/dc/elements/1.1/" xmlns:dcterms="http://purl.org/dc/terms/" xmlns:xsi="http://www.w3.org/2001/XMLSchema-instance">
  <dc:creator>HighlightExporter</dc:creator>
  <dcterms:created xsi:type="dcterms:W3CDTF">2025-01-01T00:00:00Z</dcterms:created>
  <dcterms:modified xsi:type="dcterms:W3CDTF">2025-01-01T00:00:00Z</dcterms:modified>
</cp:coreProperties>`)

	addZipFile(zw, "docProps/app.xml",
		`<?xml version="1.0" encoding="UTF-8" standalone="yes"?>
<Properties xmlns="http://schemas.openxmlformats.org/officeDocument/2006/extended-properties" xmlns:vt="http://schemas.openxmlformats.org/officeDocument/2006/docPropsVTypes">
  <Application>HighlightExporter</Application>
  <DocSecurity>0</DocSecurity>
  <Lines>1</Lines>
  <Paragraphs>1</Paragraphs>
  <AppVersion>16.0000</AppVersion>
</Properties>`)

	addZipFile(zw, "word/styles.xml",
		`<?xml version="1.0" encoding="UTF-8" standalone="yes"?>
<w:styles xmlns:w="http://schemas.openxmlformats.org/wordprocessingml/2006/main">
  <w:docDefaults>
    <w:rPrDefault>
      <w:rPr>
        <w:rFonts w:ascii="Calibri" w:hAnsi="Calibri"/>
        <w:sz w:val="22"/>
        <w:szCs w:val="22"/>
      </w:rPr>
    </w:rPrDefault>
    <w:pPrDefault>
      <w:pPr>
        <w:spacing w:before="0" w:after="0" w:line="240" w:lineRule="auto"/>
      </w:pPr>
    </w:pPrDefault>
  </w:docDefaults>
  <w:style w:type="paragraph" w:styleId="Normal">
    <w:name w:val="Normal"/>
    <w:qFormat/>
  </w:style>
  <w:style w:type="paragraph" w:styleId="Code">
    <w:name w:val="Code"/>
    <w:basedOn w:val="Normal"/>
    <w:qFormat/>
    <w:rPr>
      <w:rFonts w:ascii="` + monoFont + `" w:hAnsi="` + monoFont + `" w:cs="` + monoFont + `"/>
      <w:sz w:val="` + fontSizeHP + `"/>
      <w:szCs w:val="` + fontSizeHPC + `"/>
    </w:rPr>
  </w:style>
</w:styles>`)

	// Build document body
	var body strings.Builder
	body.WriteString(`<?xml version="1.0" encoding="UTF-8" standalone="yes"?>
<w:document xmlns:w="http://schemas.openxmlformats.org/wordprocessingml/2006/main">
<w:body>
`)

	for _, line := range lines {
		body.WriteString("<w:p>\n<w:pPr><w:pStyle w:val=\"Code\"/></w:pPr>\n")

		if len(line.Segments) == 0 {
			body.WriteString("<w:r><w:rPr><w:rFonts w:ascii=\"" + monoFont +
				"\" w:hAnsi=\"" + monoFont +
				"\"/><w:sz w:val=\"" + fontSizeHP +
				"\"/><w:szCs w:val=\"" + fontSizeHPC +
				"\"/></w:rPr><w:t xml:space=\"preserve\"> </w:t></w:r>\n")
		} else {
			for _, seg := range line.Segments {
				text := html.EscapeString(seg.Text)
				if text == "" {
					continue
				}
				body.WriteString("<w:r>\n<w:rPr>\n")
				body.WriteString("<w:rFonts w:ascii=\"" + monoFont +
					"\" w:hAnsi=\"" + monoFont +
					"\" w:cs=\"" + monoFont + "\"/>\n")
				body.WriteString("<w:sz w:val=\"" + fontSizeHP + "\"/>\n")
				body.WriteString("<w:szCs w:val=\"" + fontSizeHPC + "\"/>\n")
				if seg.Color != "" && seg.Color != "000000" {
					body.WriteString("<w:color w:val=\"" + seg.Color + "\"/>\n")
				}
				body.WriteString("</w:rPr>\n")
				body.WriteString("<w:t xml:space=\"preserve\">" + text + "</w:t>\n")
				body.WriteString("</w:r>\n")
			}
		}
		body.WriteString("</w:p>\n")
	}

	body.WriteString("</w:body>\n</w:document>\n")
	addZipFile(zw, "word/document.xml", body.String())

	if err := zw.Close(); err != nil {
		return fmt.Errorf("压缩文档失败: %w", err)
	}
	if err := os.WriteFile("output.docx", buf.Bytes(), 0644); err != nil {
		return fmt.Errorf("写入文件失败: %w", err)
	}
	return nil
}
