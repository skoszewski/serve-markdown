package main

import (
	"context"
	"errors"
	"fmt"
	"net"
	"net/http"
	"os"
	"time"

	"github.com/chromedp/chromedp"
)

// pdfPaperSizes are the sheets --pdf-page names, as Chrome is told them.
var pdfPaperSizes = map[string]chromedp.PaperSize{
	"a3":      chromedp.PaperA3,
	"a4":      chromedp.PaperA4,
	"a5":      chromedp.PaperA5,
	"letter":  chromedp.PaperLetter,
	"legal":   chromedp.PaperLegal,
	"tabloid": chromedp.PaperTabloid,
}

// pdfMargin is the margin, in inches, on every side of a printed page.
const pdfMargin = 0.4

// pdfTimeout is how long a document is given to be drawn before printing it is given up.
const pdfTimeout = 60 * time.Second

// exportPDF prints the document handler serves at its source route to the file output, on the
// page given.
//
// The handler serves the page on a loopback port of its own for as long as the printing takes,
// and headless Chrome, found where chromedp looks for it, draws the page as a browser would and
// prints it once the page says it is drawn.
func exportPDF(handler *server, output string, page pdfPageSettings) error {
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		return fmt.Errorf("cannot listen for Chrome (%v)", err)
	}
	httpServer := &http.Server{Handler: handler}
	go httpServer.Serve(listener)
	defer httpServer.Close()

	browser, cancel := chromedp.NewContext(context.Background())
	defer cancel()
	if err := chromedp.Do(browser); err != nil {
		return fmt.Errorf("cannot start headless Chrome (%v)", err)
	}

	printing, stop := context.WithTimeout(browser, pdfTimeout)
	defer stop()
	address := "http://" + listener.Addr().String() + handler.sourceRoute()
	if err := chromedp.Do(printing,
		chromedp.Navigate(address),
		chromedp.Poll[chromedp.Void](`document.documentElement.dataset.rendered === "true"`,
			chromedp.WithPollingTimeout(0)),
	); err != nil {
		if errors.Is(err, context.DeadlineExceeded) {
			return fmt.Errorf("the document was not drawn within %s", pdfTimeout)
		}
		return fmt.Errorf("cannot draw the document (%v)", err)
	}

	options := []chromedp.PDFOption{
		chromedp.PDFPaper(pdfPaperSizes[page.Paper]),
		chromedp.PDFMargin(pdfMargin),
		chromedp.PDFPrintBackground(),
		chromedp.PDFOutlineAndTagged(),
	}
	if page.Orientation == pdfLandscape {
		options = append(options, chromedp.PDFLandscape())
	}
	content, err := chromedp.Run(printing, chromedp.PrintToPDF(options...))
	if err != nil {
		return fmt.Errorf("cannot print the document (%v)", err)
	}
	return os.WriteFile(output, content, 0o644)
}
