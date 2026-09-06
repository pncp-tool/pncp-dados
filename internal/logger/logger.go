// Package logger oferece um logger estruturado (slog) minimalista para a
// biblioteca, sem dependencias externas.
package logger

import (
	"log/slog"
	"os"
	"strings"
	"time"
)

var defaultLogger *slog.Logger

func init() {
	opts := &slog.HandlerOptions{
		Level: slog.LevelInfo,
		ReplaceAttr: func(groups []string, a slog.Attr) slog.Attr {
			if a.Key == slog.SourceKey {
				if source, ok := a.Value.Any().(*slog.Source); ok {
					short := source.File
					if idx := strings.LastIndex(short, "/internal/"); idx >= 0 {
						short = short[idx+1:]
					}
					a.Value = slog.StringValue(short)
				}
			}
			if a.Key == slog.TimeKey {
				if t, ok := a.Value.Any().(time.Time); ok {
					a.Value = slog.StringValue(t.Format(time.RFC3339Nano))
				}
			}
			return a
		},
	}
	handler := slog.NewJSONHandler(os.Stdout, opts)
	defaultLogger = slog.New(handler)
}

// Logger envelopa *slog.Logger com um prefixo (tag).
type Logger struct {
	*slog.Logger
}

// New cria um Logger com a tag informada.
func New(prefix string) *Logger {
	return &Logger{
		Logger: defaultLogger.With("tag", prefix),
	}
}
