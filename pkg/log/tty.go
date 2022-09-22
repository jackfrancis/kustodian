// Package log contains logging functions
package log

import (
	"fmt"
	"io"

	"github.com/sirupsen/logrus"
)

// TTYWriter writes into a tty terminal
type TTYWriter struct {
	out  *logrus.Logger
	file *logrus.Entry
}

// newTTYWriter creates a new ttyWriter
func newTTYWriter(out *logrus.Logger, file *logrus.Entry) *TTYWriter {
	return &TTYWriter{
		out:  out,
		file: file,
	}
}

// Debugf writes a debug-level log with a format
func (*TTYWriter) Debugf(format string, args ...interface{}) {
	log.out.Debugf(format, args...)
	if log.file != nil {
		log.file.Debugf(format, args...)
	}
}

// Infof writes a info-level log with a format
func (*TTYWriter) Infof(format string, args ...interface{}) {
	log.out.Infof(format, args...)
	if log.file != nil {
		log.file.Infof(format, args...)
	}
}

// Successf prints a message with the success symbol first, and the text in green
func (w *TTYWriter) Successf(format string, args ...interface{}) {
	log.out.Infof(format, args...)
	w.Fprintf(w.out.Out, "%s %s\n", coloredSuccessSymbol, greenString(format, args...))
}

// Errorf writes a error-level log with a format
func (*TTYWriter) Errorf(format string, args ...interface{}) {
	log.out.Errorf(format, args...)
	if log.file != nil {
		log.file.Errorf(format, args...)
	}
}

// Fprintf prints a line with format
func (w *TTYWriter) Fprintf(writer io.Writer, format string, a ...interface{}) {
	msg := fmt.Sprintf(format, a...)
	fmt.Fprint(writer, msg)
	// if msg != "" && writer == w.out.Out {
	// 	msg = convertToJSON(InfoLevel, log.stage, msg)
	// 	if msg != "" {
	// 		log.buf.WriteString(msg)
	// 		log.buf.WriteString("\n")
	// 	}
	// }
}
