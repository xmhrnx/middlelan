package ui

import (
	"context"
	"errors"
	"fmt"
	"io"
	"time"

	"github.com/xmhrnx/giode"
	"github.com/xmhrnx/middlelan/internal/export"
)

// exportData builds the export source from the app's stores.
func (sh *Shell) exportData() export.Data {
	return export.Data{
		Store: sh.svc.Collector.Store(), Devices: sh.svc.Scanner.Inventory(), Speed: sh.svc.Speed,
		Describe: func(ip string) (string, string, string) {
			info, _ := sh.svc.Resolver.Peek(ip)
			return nonEmpty(info.Domain, info.Host), info.Owner(), info.Country
		},
	}
}

// saveExport asks where to save and writes one export there. The picker
// blocks, so it runs on its own goroutine.
func (sh *Shell) saveExport(name string, write func(export.Data, io.Writer) error) {
	data := sh.exportData()
	go func() {
		f, err := sh.svc.App.SaveFile(context.Background(), name)
		if errors.Is(err, giode.ErrCanceled) {
			return
		}
		if err == nil {
			err = write(data, f)
			if cerr := f.Close(); err == nil {
				err = cerr
			}
		}
		if err != nil {
			sh.Toast(giode.ToastError, tr("Export failed: {error}", "error", err.Error()))
			return
		}
		sh.Toast(giode.ToastSuccess, tr("Exported {file}", "file", name))
	}()
}

// exportName is a file name with today's date.
func exportName(what, ext string) string {
	return fmt.Sprintf("middleman-%s-%s.%s", what, time.Now().Format("2006-01-02"), ext)
}
