package api

import (
	"bytes"
	"io"
	"log"
	"strings"
	"sync"
	"testing"
)

// testLogWriter redirects log output to testing.T.Logf
type testLogWriter struct {
	t      *testing.T
	mu     sync.Mutex
	buffer bytes.Buffer
}

// Write implements io.Writer for testLogWriter
func (w *testLogWriter) Write(p []byte) (n int, err error) {
	w.mu.Lock()
	defer w.mu.Unlock()

	// Accumulate in buffer
	n, err = w.buffer.Write(p)

	// If we have a complete line (ending with newline), flush it
	for {
		line, err := w.buffer.ReadString('\n')
		if err != nil {
			// Put back what we couldn't read as a complete line
			if len(line) > 0 {
				w.buffer.WriteString(line)
			}
			break
		}

		// Remove trailing newline and log it
		line = strings.TrimSuffix(line, "\n")
		if line != "" {
			w.t.Logf("%s", line)
		}
	}

	return n, nil
}

// SetupTestLogging redirects the standard log package output to t.Logf.
// It returns a cleanup function that should be called to restore the original logger.
func SetupTestLogging(t *testing.T) func() {
	originalOutput := log.Writer()
	originalFlags := log.Flags()
	originalPrefix := log.Prefix()

	// Create our test log writer
	testWriter := &testLogWriter{t: t}

	// Redirect log output to our test writer
	log.SetOutput(testWriter)
	// Set flags to include more context in test logs
	log.SetFlags(log.Ldate | log.Ltime | log.Lshortfile)

	// Return cleanup function
	return func() {
		// Flush any remaining content in buffer
		testWriter.mu.Lock()
		if testWriter.buffer.Len() > 0 {
			remaining := testWriter.buffer.String()
			if remaining != "" {
				t.Logf("%s", remaining)
			}
		}
		testWriter.mu.Unlock()

		// Restore original logger settings
		log.SetOutput(originalOutput)
		log.SetFlags(originalFlags)
		log.SetPrefix(originalPrefix)
	}
}

// CaptureLogOutput captures log output during the execution of a function.
// This is useful for testing log output without redirecting to t.Logf.
func CaptureLogOutput(fn func()) string {
	originalOutput := log.Writer()
	originalFlags := log.Flags()

	var buf bytes.Buffer
	log.SetOutput(&buf)
	log.SetFlags(0) // No timestamps for cleaner test assertions

	defer func() {
		log.SetOutput(originalOutput)
		log.SetFlags(originalFlags)
	}()

	fn()

	return buf.String()
}

// multiWriter combines multiple writers
type multiWriter struct {
	writers []io.Writer
}

func (mw *multiWriter) Write(p []byte) (n int, err error) {
	for _, w := range mw.writers {
		n, err = w.Write(p)
		if err != nil {
			return
		}
	}
	return
}

// SetupTestLoggingWithCapture redirects logs to both t.Logf and a buffer for capture.
// Returns the buffer and a cleanup function.
func SetupTestLoggingWithCapture(t *testing.T) (*bytes.Buffer, func()) {
	originalOutput := log.Writer()
	originalFlags := log.Flags()
	originalPrefix := log.Prefix()

	// Create our test log writer and capture buffer
	testWriter := &testLogWriter{t: t}
	captureBuffer := &bytes.Buffer{}

	// Create multi-writer to write to both
	mw := &multiWriter{
		writers: []io.Writer{testWriter, captureBuffer},
	}

	// Redirect log output
	log.SetOutput(mw)
	log.SetFlags(log.Ldate | log.Ltime | log.Lshortfile)

	// Return buffer and cleanup function
	return captureBuffer, func() {
		// Flush any remaining content
		testWriter.mu.Lock()
		if testWriter.buffer.Len() > 0 {
			remaining := testWriter.buffer.String()
			if remaining != "" {
				t.Logf("%s", remaining)
			}
		}
		testWriter.mu.Unlock()

		// Restore original logger settings
		log.SetOutput(originalOutput)
		log.SetFlags(originalFlags)
		log.SetPrefix(originalPrefix)
	}
}
