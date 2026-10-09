package main

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/chromedp/chromedp"
)

func TestExportPDFPrintsTheDocument(t *testing.T) {
	// The export needs a browser, which a machine running the tests need not have.
	browser, cancel := chromedp.NewContext(context.Background())
	err := chromedp.Do(browser)
	cancel()
	if err != nil {
		t.Skipf("no headless Chrome to print with: %v", err)
	}

	root := documentRoot(t)
	handler := &server{defaultSource: source{kind: kindLocal}, rootDir: root,
		defaultFile: filepath.Join(root, "README.md"), print: true}
	output := filepath.Join(t.TempDir(), "README.pdf")

	if err := exportPDF(handler, output, defaultPDFPage); err != nil {
		t.Fatal(err)
	}
	content, err := os.ReadFile(output)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.HasPrefix(content, []byte("%PDF-")) {
		t.Errorf("the file written is not a PDF: %q", content[:min(len(content), 16)])
	}
}
