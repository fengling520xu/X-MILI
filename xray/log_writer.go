package xray
import (
	"regexp"
	"runtime"
	"strings"
	"github.com/mhsanaei/3x-ui/v2/logger"
)
func NewLogWriter() *LogWriter { return &LogWriter{} }
type LogWriter struct{ lastLine string }
func (lw *LogWriter) Write(m []byte) (n int, err error) {
	message := strings.TrimSpace(string(m))
	if message == "" { return len(m), nil }
	msgLower := strings.ToLower(message)
	if runtime.GOOS == "windows" && strings.Contains(msgLower, "exit status 1") { return len(m), nil }
	if len(message) > 1024 { message = message[:1024] }
	if strings.Contains(msgLower, "tls handshake error") || strings.Contains(msgLower, "connection ends") {
		logger.Debug("XRAY: " + message); return len(m), nil
	}
	crashRegex := regexp.MustCompile(`(?i)(panic|exception|stack trace|fatal error)`)
	if crashRegex.MatchString(message) {
		logger.Error("Core crash detected (no file write in LITE): " + message[:200])
		lw.lastLine = message; return len(m), nil
	}
	regex := regexp.MustCompile(`^(\d{4}/\d{2}/\d{2} \d{2}:\d{2}:\d{2}\.\d{6}) \[([^\]]+)\] (.+)$`)
	for msg := range strings.SplitSeq(message, "\n") {
		if msg == "" { continue }
		matches := regex.FindStringSubmatch(msg)
		if len(matches) > 3 {
			level := matches[2]; body := matches[3]
			if len(body) > 512 { body = body[:512] }
			lowerBody := strings.ToLower(body)
			if strings.Contains(lowerBody, "tls handshake error") || strings.Contains(lowerBody, "connection ends") {
				logger.Debug("XRAY: " + body); continue
			}
			if strings.Contains(lowerBody, "failed") {
				logger.Error("XRAY: " + body)
			} else {
				switch level {
				case "Debug": logger.Debug("XRAY: " + body)
				case "Info": logger.Info("XRAY: " + body)
				case "Warning": logger.Warning("XRAY: " + body)
				case "Error": logger.Error("XRAY: " + body)
				default: logger.Debug("XRAY: " + msg)
				}
			}
		} else {
			if strings.Contains(strings.ToLower(msg), "failed") {
				logger.Error("XRAY: " + msg)
			} else {
				logger.Debug("XRAY: " + msg)
			}
		}
	}
	return len(m), nil
}
