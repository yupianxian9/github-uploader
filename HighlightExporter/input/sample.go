package main

import (
	"bytes"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"

	"github.com/alecthomas/chroma/v2"
	"github.com/alecthomas/chroma/v2/formatters/html"
	"github.com/alecthomas/chroma/v2/lexers"
	"github.com/alecthomas/chroma/v2/styles"
	"github.com/unidoc/unioffice/common"
	"github.com/unidoc/unioffice/document"
)

// 支持的文件扩展名列表
var supportedExts = map[string]bool{
	".go":     true,
	".py":     true,
	".js":     true,
	".ts":     true,
	".java":   true,
	".c":      true,
	".cpp":    true,
	".h":      true,
	".cs":     true,
	".rb":     true,
	".php":    true,
	".html":   true,
	".htm":    true,
	".css":    true,
	".scss":   true,
	".json":   true,
	".xml":    true,
	".yaml":   true,
	".yml":    true,
	".md":     true,
	".txt":    true,
	".sh":     true,
	".bash":   true,
	".sql":    true,
	".rs":     true,
	".swift":  true,
	".kt":     true,
	".scala":  true,
	".lua":    true,
	".pl":     true,
	".r":      true,
}

// 高亮配置
var (
	// 使用亮色主题 "github"
	style = styles.Get("github")
	// HTML 格式化器，用于获取样式信息（我们实际不用HTML输出，但复用其颜色映射）
	formatter = html.New(html.WithClasses(false))
)

// 解析语言名称
func getLanguage(ext string) string {
	switch ext {
	case ".go":
		return "go"
	case ".py":
		return "python"
	case ".js":
		return "javascript"
	case ".ts":
		return "typescript"
	case ".java":
		return "java"
	case ".c":
		return "c"
	case ".cpp":
		return "cpp"
	case ".h":
		return "c"
	case ".cs":
		return "csharp"
	case ".rb":
		return "ruby"
	case ".php":
		return "php"
	case ".html", ".htm":
		return "html"
	case ".css":
		return "css"
	case ".scss":
		return "scss"
	case ".json":
		return "json"
	case ".xml":
		return "xml"
	case ".yaml", ".yml":
		return "yaml"
	case ".md":
		return "markdown"
	case ".txt":
		return "text"
	case ".sh", ".bash":
		return "bash"
	case ".sql":
		return "sql"
	case ".rs":
		return "rust"
	case ".swift":
		return "swift"
	case ".kt":
		return "kotlin"
	case ".scala":
		return "scala"
	case ".lua":
		return "lua"
	case ".pl":
		return "perl"
	case ".r":
		return "r"
	default:
		return "text"
	}
}

// 将 token 类型映射到 chroma 样式条目
func getStyleForToken(tokenType chroma.TokenType) *chroma.StyleEntry {
	if style == nil {
		return nil
	}
	entry := style.Get(tokenType)
	return &entry
}

// 将文件内容高亮并添加到 docx 段落中
func addHighlightedCodeToParagraph(doc *document.Document, code string, langName string) error {
	// 获取 lexer
	lexer := lexers.Get(langName)
	if lexer == nil {
		lexer = lexers.Fallback
	}
	lexer = chroma.Coalesce(lexer)

	// 词法分析
	iterator, err := lexer.Tokenise(nil, code)
	if err != nil {
		return fmt.Errorf("tokenise failed: %w", err)
	}

	// 创建段落，设置等宽字体
	para := doc.AddParagraph()
	para.Properties().SetStyle("Normal") // 使用默认样式，但我们会覆盖字体
	run := para.AddRun()
	run.Properties().SetFontFamily("Courier New")
	run.Properties().SetSize(10) // 字体大小10pt

	// 辅助函数：将当前缓冲区作为新 run 添加到段落，并重置
	var flushRun = func() {
		if run != nil {
			run = para.AddRun()
			run.Properties().SetFontFamily("Courier New")
			run.Properties().SetSize(10)
		}
	}

	// 遍历 tokens
	for token := iterator(); token != nil; token = iterator() {
		tokenType := token.Type
		value := token.Value

		// 获取样式
		styleEntry := getStyleForToken(tokenType)
		if styleEntry == nil {
			// 无样式，直接添加文本
			run.AddText(value)
			continue
		}

		// 处理文本中的换行，需要拆分为多个 run 并插入换行符
		lines := strings.SplitAfter(value, "\n")
		for i, line := range lines {
			if line == "" && i == len(lines)-1 {
				continue
			}
			// 去掉行尾的换行符以便添加文本，然后单独添加换行符
			hasNewline := strings.HasSuffix(line, "\n")
			textPart := line
			if hasNewline {
				textPart = line[:len(line)-1]
			}

			// 添加文本（如果有）
			if textPart != "" {
				run.AddText(textPart)
			}
			// 如果原行有换行符，添加一个换行
			if hasNewline {
				run.AddBreak()
			}
		}

		// 刷新样式设置（因为样式可能变化，需要新 run）
		// 注意：连续同一样式的 token 可以合并到一个 run，但为了简化，每个 token 独立 run
		// 且样式在同一个 run 内设置即可
		// 重新设置 run 的样式属性
		if styleEntry.Colour.IsSet() {
			// 设置前景色，格式如 "#RRGGBB"
			colorHex := styleEntry.Colour.String()
			run.Properties().SetColor(colorHex)
		}
		if styleEntry.Bold == chroma.Yes {
			run.Properties().SetBold(true)
		} else if styleEntry.Bold == chroma.No {
			run.Properties().SetBold(false)
		}
		if styleEntry.Italic == chroma.Yes {
			run.Properties().SetItalic(true)
		} else if styleEntry.Italic == chroma.No {
			run.Properties().SetItalic(false)
		}
		// 下划线（可选）
		if styleEntry.Underline == chroma.Yes {
			run.Properties().SetUnderline(common.UnderlineStyleSingle)
		} else {
			run.Properties().SetUnderline(common.UnderlineStyleNone)
		}
		// 背景色暂不处理，因为 docx 背景较复杂

		// 为下一个 token 新建 run
		flushRun()
	}
	return nil
}

// 添加文件名标题
func addFileHeading(doc *document.Document, filePath string) {
	heading := doc.AddParagraph()
	run := heading.AddRun()
	run.Properties().SetBold(true)
	run.Properties().SetSize(14)
	run.AddText(filePath)
}

// 处理单个文件
func processFile(doc *document.Document, filePath string) error {
	// 读取文件内容
	content, err := os.ReadFile(filePath)
	if err != nil {
		return fmt.Errorf("read file %s failed: %w", filePath, err)
	}
	if len(content) == 0 {
		fmt.Printf("Skipping empty file: %s\n", filePath)
		return nil
	}

	// 获取扩展名
	ext := strings.ToLower(filepath.Ext(filePath))
	if !supportedExts[ext] {
		fmt.Printf("Skipping unsupported file type: %s\n", filePath)
		return nil
	}

	langName := getLanguage(ext)
	fmt.Printf("Processing: %s (language: %s)\n", filePath, langName)

	// 添加文件标题
	addFileHeading(doc, filePath)

	// 添加高亮代码
	err = addHighlightedCodeToParagraph(doc, string(content), langName)
	if err != nil {
		return fmt.Errorf("highlight code for %s failed: %w", filePath, err)
	}

	// 添加空行分隔不同文件
	emptyPara := doc.AddParagraph()
	emptyPara.AddRun().AddText("")
	return nil
}

// 递归遍历目录并处理文件
func processDirectory(doc *document.Document, rootDir string) error {
	err := filepath.WalkDir(rootDir, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			return nil
		}
		// 处理文件
		return processFile(doc, path)
	})
	return err
}

func main() {
	inputDir := "input"
	outputFile := "output.docx"

	// 检查 input 文件夹是否存在
	if _, err := os.Stat(inputDir); os.IsNotExist(err) {
		fmt.Printf("Error: input directory '%s' does not exist\n", inputDir)
		os.Exit(1)
	}

	// 创建新文档
	doc := document.New()
	defer doc.Close()

	// 添加文档标题
	heading := doc.AddParagraph()
	run := heading.AddRun()
	run.Properties().SetBold(true)
	run.Properties().SetSize(18)
	run.AddText("Code Highlighting Report")

	// 添加生成时间
	timePara := doc.AddParagraph()
	timePara.AddRun().AddText("Generated by code-to-docx converter")

	// 处理目录
	fmt.Println("Starting code file processing...")
	err := processDirectory(doc, inputDir)
	if err != nil {
		fmt.Printf("Error processing files: %v\n", err)
		os.Exit(1)
	}

	// 保存文档
	err = doc.SaveToFile(outputFile)
	if err != nil {
		fmt.Printf("Error saving document: %v\n", err)
		os.Exit(1)
	}

	fmt.Printf("Successfully generated %s\n", outputFile)
}