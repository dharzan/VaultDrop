package pdfutil

import (
	"bytes"
	"fmt"
	"io"
	"strings"

	pdf "github.com/ledongthuc/pdf"
)

// ExtractText reads PDF bytes and returns plain text using ledongthuc/pdf.
func ExtractText(data []byte) (string, error) {
	reader := bytes.NewReader(data)
	doc, err := pdf.NewReader(reader, int64(len(data)))
	if err != nil {
		return "", fmt.Errorf("new pdf reader: %w", err)
	}
	var builder strings.Builder
	total := doc.NumPage()
	for page := 1; page <= total; page++ {
		p := doc.Page(page)
		if p.V.IsNull() {
			continue
		}
		content, err := p.GetPlainText(nil)
		if err != nil {
			return "", fmt.Errorf("page %d: %w", page, err)
		}
		builder.WriteString(content)
		builder.WriteString("\n")
	}
	return builder.String(), nil
}

// ExtractFromReader drains the reader before passing along to ExtractText.
func ExtractFromReader(r io.Reader) (string, error) {
	data, err := io.ReadAll(r)
	if err != nil {
		return "", fmt.Errorf("read pdf: %w", err)
	}
	return ExtractText(data)
}
