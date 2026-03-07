package logs

import (
	"io"
	"log/slog"
	"os"
	"time"

	"github.com/lmittmann/tint"
)

func New(level slog.Level, logFile string) (*slog.Logger, func(), error) {
	var writers []io.Writer
	writers = append(writers, os.Stderr)

	cleanup := func() {}

	if logFile != "" {
		f, err := os.OpenFile(logFile, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
		if err != nil {
			return nil, nil, err
		}
		writers = append(writers, f)
		cleanup = func() { f.Close() }
	}

	handler := tint.NewHandler(io.MultiWriter(writers...), &tint.Options{
		Level:      level,
		TimeFormat: time.Kitchen,
	})

	handler = &ContextHandler{Handler: handler}

	logger := slog.New(handler)
	slog.SetDefault(logger)

	return logger, cleanup, nil
}
