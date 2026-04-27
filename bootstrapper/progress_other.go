//go:build !windows

package main

import "fmt"

// showProgress runs fn with a simple text-based progress report on non-Windows.
func showProgress(_ string, description string, fn func(progressReporter) error) error {
	fmt.Println(description)
	last := -1
	return fn(func(done, total int64) {
		if total <= 0 {
			return
		}
		pct := int(done * 100 / total)
		if pct/10 != last/10 {
			fmt.Printf("  %d%%\n", pct)
			last = pct
		}
	})
}
