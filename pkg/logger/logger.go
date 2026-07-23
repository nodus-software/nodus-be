package logger

import (
	"context"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"path/filepath"
	"runtime"
	"time"

	"github.com/davecgh/go-spew/spew"
	"gopkg.in/natefinch/lumberjack.v2"
)

// AuditLog matches your data auditing model
type AuditLog struct {
	Date           time.Time `json:"date"`
	Username       string    `json:"Username"`
	RequestHeader  any       `json:"request_header"`
	Request        any       `json:"request"`
	StatusCode     int       `json:"status_code"`
	ResponseHeader any       `json:"response_header"`
	Response       any       `json:"response"`
	ClientID       string    `json:"client_id"`
	Route          string    `json:"route"`
	Duration       float64   `json:"duration_seconds"`
}

type Logger struct {
	slogLogger *slog.Logger
}

// NewLogger initializes a greenfield, high-performance structured logger
func NewLogger() *Logger {
	folder := "./logs"
	if err := os.MkdirAll(folder, 0755); err != nil {
		folder = "../logs"
		_ = os.MkdirAll(folder, 0755)
	}

	rotator := &lumberjack.Logger{
		Filename:   filepath.Join(folder, "app.log"),
		MaxSize:    100, // MBs
		MaxBackups: 30,
		MaxAge:     30, // Days
		Compress:   true,
	}

	handlerOpts := &slog.HandlerOptions{
		Level:     slog.LevelDebug,
		AddSource: false, // Handled manually below for precise line numbers
	}

	return &Logger{
		slogLogger: slog.New(slog.NewJSONHandler(rotator, handlerOpts)),
	}
}

// logInternal intercepts execution frames cleanly to pass records straight to slog
func (l *Logger) logInternal(level slog.Level, msg string, args ...any) {
	if l == nil || l.slogLogger == nil {
		return
	}

	// Capture the true caller's file and line number
	var pcs [1]uintptr
	runtime.Callers(3, pcs[:]) // skips runtime.Callers, logInternal, and the Info/Debug helper

	r := slog.NewRecord(time.Now(), level, msg, pcs[0])
	r.Add(args...) // seamlessly accepts structured pairs like "user_id", 123

	_ = l.slogLogger.Handler().Handle(context.Background(), r)
}

// Modern, idiomatic slog wrappers
func (l *Logger) Debug(msg string, args ...any)   { l.logInternal(slog.LevelDebug, msg, args...) }
func (l *Logger) Info(msg string, args ...any)    { l.logInternal(slog.LevelInfo, msg, args...) }
func (l *Logger) Warning(msg string, args ...any) { l.logInternal(slog.LevelWarn, msg, args...) }
func (l *Logger) Error(msg string, args ...any)   { l.logInternal(slog.LevelError, msg, args...) }

func (l *Logger) Fatal(msg string, args ...any) {
	l.logInternal(slog.LevelError, "[FATAL] "+msg, args...)
	os.Exit(1)
}

// Audit structured execution tracking
func (l *Logger) Audit(record *AuditLog) {
	if l == nil || l.slogLogger == nil {
		return
	}
	l.logInternal(slog.LevelInfo, "API Audit Log", "audit_record", record)
}

// Header2Map transforms HTTP headers safely into unstructured map payloads
func Header2Map(header http.Header) map[string]any {
	head := make(map[string]any)
	for k, v := range header {
		head[k] = v
	}
	return head
}

// SpewResultForDebugging is a helpful developer diagnostics tool
func SpewResultForDebugging(description string, v any) {
	fmt.Println("\n**** Start Result ******")
	fmt.Println(description)
	spew.Dump(v)
	fmt.Println("**** End Result ******")
}
