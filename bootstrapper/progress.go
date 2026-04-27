package main

import "time"

// progressReporter is called periodically with bytes downloaded and total size.
// total is -1 when the server did not send Content-Length.
type progressReporter func(done, total int64)

// progressWriter wraps an io.Writer and fires a progressReporter at most every
// 50 ms so the UI thread is not flooded on fast connections.
type progressWriter struct {
	written  int64
	total    int64
	report   progressReporter
	lastSent time.Time
}

func (pw *progressWriter) Write(p []byte) (int, error) {
	pw.written += int64(len(p))
	if pw.report != nil && time.Since(pw.lastSent) >= 50*time.Millisecond {
		pw.report(pw.written, pw.total)
		pw.lastSent = time.Now()
	}
	return len(p), nil
}
