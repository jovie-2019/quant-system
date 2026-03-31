package logging

import (
	"log/slog"
	"os"
)

// Init creates and sets the global default slog.Logger.
// LOG_FORMAT=text selects TextHandler; anything else (including empty) selects JSONHandler.
func Init() *slog.Logger {
	var handler slog.Handler
	if os.Getenv("LOG_FORMAT") == "text" {
		handler = slog.NewTextHandler(os.Stdout, nil)
	} else {
		handler = slog.NewJSONHandler(os.Stdout, nil)
	}
	logger := slog.New(handler)
	slog.SetDefault(logger)
	return logger
}
