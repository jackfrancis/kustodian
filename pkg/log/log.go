// Package log contains logging functions
package log

import (
	"bytes"
	"os"

	"github.com/fatih/color"
	"github.com/sirupsen/logrus"
)

type logger struct {
	out    *logrus.Logger
	file   *logrus.Entry
	writer ClarkezoneWriter

	outputMode string

	buf *bytes.Buffer
}

var log = &logger{
	out: logrus.New(),
}

var (
	// redString = color.New(color.FgHiRed).SprintfFunc()

	greenString = color.New(color.FgGreen).SprintfFunc()

	// yellowString = color.New(color.FgHiYellow).SprintfFunc()

	// blueString = color.New(color.FgHiBlue).SprintfFunc()

	// errorSymbol = " x "
	// coloredErrorSymbol = color.New(color.BgHiRed, color.FgBlack).Sprint(errorSymbol)

	successSymbol        = " ✓ "
	coloredSuccessSymbol = color.New(color.BgGreen, color.FgBlack).Sprint(successSymbol)

	// informationSymbol = " i "
	// coloredInformationSymbol = color.New(color.BgHiBlue, color.FgBlack).Sprint(informationSymbol)

	// warningSymbol = " ! "
	// coloredWarningSymbol = color.New(color.BgHiYellow, color.FgBlack).Sprint(warningSymbol)

	// questionSymbol = " ? "
	// coloredQuestionSymbol = color.New(color.BgHiMagenta, color.FgBlack).Sprint(questionSymbol)

	// InfoLevel is the json level for information
	InfoLevel = "info"
	// WarningLevel is the json level for warning
	WarningLevel = "warn"
	// ErrorLevel is the json level for error
	ErrorLevel = "error"
)

// Init configures the logger for the package to use.
func Init(level logrus.Level) {
	log.out.SetOutput(os.Stdout)
	log.out.SetLevel(level)
	log.writer = log.getWriter(TTYFormat)
	log.buf = &bytes.Buffer{}
}

// SetLevel sets the level of the main logger
func SetLevel(level string) {
	l, err := logrus.ParseLevel(level)
	if err == nil {
		log.out.SetLevel(l)
	}
}

// SetOutputFormat sets the output format
func SetOutputFormat(format string) {
	log.writer = log.getWriter(format)
}

// Debugf writes a debug-level log with a format
func Debugf(format string, args ...interface{}) {
	if log.writer != nil {
		log.writer.Debugf(format, args...)
	}
}

// Infof writes a info-level log with a format
func Infof(format string, args ...interface{}) {
	log.writer.Infof(format, args...)
}

// Successf prints a message with the success symbol first, and the text in green
func Successf(format string, args ...interface{}) {
	log.writer.Successf(format, args...)
}

// Errorf writes a error-level log with a format
func Errorf(format string, args ...interface{}) {
	log.writer.Errorf(format, args...)
}
