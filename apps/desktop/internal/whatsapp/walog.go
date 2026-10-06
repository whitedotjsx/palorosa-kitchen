package whatsapp

import (
	"fmt"
	"log"

	waLog "go.mau.fi/whatsmeow/util/log"
)

// panelLogger wraps the whatsmeow stdout logger and also forwards warnings and
// errors to the operator log. The desktop exe has no console, so without this
// whatsmeow's own complaints (encryption, LID lookup, retry receipts) are lost
// and a send that the server accepted but never delivered looks like success.
type panelLogger struct {
	inner  waLog.Logger
	logger *log.Logger
	module string
}

func newPanelLogger(inner waLog.Logger, logger *log.Logger, module string) waLog.Logger {
	return &panelLogger{inner: inner, logger: logger, module: module}
}

func (p *panelLogger) forward(level, msg string, args ...any) {
	p.logger.Printf("whatsmeow %s [%s]: %s", level, p.module, fmt.Sprintf(msg, args...))
}

func (p *panelLogger) Warnf(msg string, args ...any) {
	p.inner.Warnf(msg, args...)
	p.forward("WARN", msg, args...)
}

func (p *panelLogger) Errorf(msg string, args ...any) {
	p.inner.Errorf(msg, args...)
	p.forward("ERROR", msg, args...)
}

func (p *panelLogger) Infof(msg string, args ...any)  { p.inner.Infof(msg, args...) }
func (p *panelLogger) Debugf(msg string, args ...any) { p.inner.Debugf(msg, args...) }

func (p *panelLogger) Sub(module string) waLog.Logger {
	return newPanelLogger(p.inner.Sub(module), p.logger, p.module+"/"+module)
}
