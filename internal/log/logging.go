package logs

import (
	"io"
	"log/slog"
	"os"
)

func New(level slog.Level, logFile string) (*slog.Logger, func(), error) {
	var writers []io.Writer
	writers = append(writers, os.Stderr)

	var cleanup func() = func() {}

	if logFile != "" {
		f, err := os.OpenFile(logFile, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
		if err != nil {
			return nil, nil, err
		}
		writers = append(writers, f)
		cleanup = func() { f.Close() }
	}

	opts := &slog.HandlerOptions{Level: level}

	var handler slog.Handler = slog.NewTextHandler(io.MultiWriter(writers...), opts)
	handler = &ContextHandler{Handler: handler}

	logger := slog.New(handler)
	slog.SetDefault(logger)

	return logger, cleanup, nil
}
