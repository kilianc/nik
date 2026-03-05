package whatsapp

import (
	"fmt"
	"log/slog"

	waLog "go.mau.fi/whatsmeow/util/log"
)

// slogWaLog adapts waLog.Logger to forward through slog.
type slogWaLog struct {
	mod string
}

func newWaLogger(module string) waLog.Logger {
	return &slogWaLog{mod: module}
}

func (s *slogWaLog) Debugf(msg string, args ...interface{}) {
	slog.Debug(fmt.Sprintf(msg, args...), "pkg", s.mod)
}
func (s *slogWaLog) Infof(msg string, args ...interface{}) {
	slog.Debug(fmt.Sprintf(msg, args...), "pkg", s.mod)
}
func (s *slogWaLog) Warnf(msg string, args ...interface{}) {
	slog.Warn(fmt.Sprintf(msg, args...), "pkg", s.mod)
}
func (s *slogWaLog) Errorf(msg string, args ...interface{}) {
	slog.Error(fmt.Sprintf(msg, args...), "pkg", s.mod)
}
func (s *slogWaLog) Sub(module string) waLog.Logger {
	return newWaLogger(s.mod + "/" + module)
}
