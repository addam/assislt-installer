//go:build windows

package main

import (
	"fmt"
	"time"

	"github.com/lxn/walk"
	. "github.com/lxn/walk/declarative"
	"github.com/lxn/win"
)

// showProgress opens a native Windows progress window and calls fn in a
// background goroutine.  fn receives a progressReporter to push byte counts.
// The window closes automatically when fn returns.
func showProgress(title, description string, fn func(progressReporter) error) error {
	var mw *walk.MainWindow
	var bar *walk.ProgressBar
	var infoLbl *walk.Label

	if err := (MainWindow{
		AssignTo: &mw,
		Title:    title,
		MinSize:  Size{Width: 440, Height: 130},
		MaxSize:  Size{Width: 440, Height: 130},
		Layout:   VBox{Margins: Margins{Top: 20, Bottom: 20, Left: 20, Right: 20}, Spacing: 10},
		Children: []Widget{
			Label{Text: description},
			ProgressBar{
				AssignTo: &bar,
				MinValue: 0,
				MaxValue: 1000, // finer than 100 so short files still animate smoothly
			},
			Label{
				AssignTo: &infoLbl,
				Text:     "",
			},
		},
	}).Create(); err != nil {
		return err
	}

	// Center on primary monitor
	sw := int(win.GetSystemMetrics(win.SM_CXSCREEN))
	sh := int(win.GetSystemMetrics(win.SM_CYSCREEN))
	b := mw.Bounds()
	mw.SetBounds(walk.Rectangle{
		X: (sw - b.Width) / 2, Y: (sh - b.Height) / 2,
		Width: b.Width, Height: b.Height,
	})

	// Remove resize handle and maximize button: dialog-like, fixed size
	s := win.GetWindowLong(mw.Handle(), win.GWL_STYLE)
	win.SetWindowLong(mw.Handle(), win.GWL_STYLE, s&^(win.WS_THICKFRAME|win.WS_MAXIMIZEBOX))

	start := time.Now()
	var fnErr error

	go func() {
		fnErr = fn(func(done, total int64) {
			elapsed := time.Since(start).Seconds()
			if elapsed < 0.1 {
				return
			}
			speed := float64(done) / elapsed
			mw.Synchronize(func() {
				if total > 0 {
					bar.SetValue(int(done * 1000 / total))
				}
				infoLbl.SetText(fmtProgressLine(done, total, speed))
			})
		})
		mw.Synchronize(func() { mw.Close() })
	}()

	mw.Run()
	return fnErr
}

func fmtProgressLine(done, total int64, bytesPerSec float64) string {
	if total > 0 {
		return fmt.Sprintf("%s / %s  •  %s/s",
			fmtSize(done), fmtSize(total), fmtSize(int64(bytesPerSec)))
	}
	return fmt.Sprintf("%s downloaded  •  %s/s", fmtSize(done), fmtSize(int64(bytesPerSec)))
}

func fmtSize(b int64) string {
	const u = 1024
	if b < u {
		return fmt.Sprintf("%d B", b)
	}
	div, exp := int64(u), 0
	for n := b / u; n >= u; n /= u {
		div *= u
		exp++
	}
	return fmt.Sprintf("%.1f %cB", float64(b)/float64(div), "KMGTPE"[exp])
}
