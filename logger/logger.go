package logger
import (
	"fmt"
	"os"
	"runtime"
	"sync"
	"time"
	"github.com/op/go-logging"
)
const (
	maxLogBufferSize = 200
	maxLogEntryLen   = 500
	timeFormat       = "2006/01/02 15:04:05"
)
var (
	logger  *logging.Logger
	logMu   sync.Mutex
	logBuffer []struct {
		time  string
		level logging.Level
		log   string
	}
)
func InitLogger(level logging.Level) {
	newLogger := logging.MustGetLogger("x-ui")
	var backend logging.Backend
	if runtime.GOOS == "windows" {
		backend = logging.NewLogBackend(os.Stderr, "", 0)
	} else {
		if syslogBackend, err := logging.NewSyslogBackend(""); err == nil {
			backend = syslogBackend
		} else {
			backend = logging.NewLogBackend(os.Stderr, "", 0)
		}
	}
	formatter := logging.MustStringFormatter(`%{time:` + timeFormat + `} %{level} - %{message}`)
	backend = logging.NewBackendFormatter(backend, formatter)
	leveled := logging.AddModuleLevel(backend)
	leveled.SetLevel(level, "x-ui")
	logger = newLogger
	logger.SetBackend(leveled)
}
func CloseLogger() {}
func Debug(args ...any) { logger.Debug(args...); addToBuffer("DEBUG", fmt.Sprint(args...)) }
func Debugf(format string, args ...any) { logger.Debugf(format, args...); addToBuffer("DEBUG", fmt.Sprintf(format, args...)) }
func Info(args ...any) { logger.Info(args...); addToBuffer("INFO", fmt.Sprint(args...)) }
func Infof(format string, args ...any) { logger.Infof(format, args...); addToBuffer("INFO", fmt.Sprintf(format, args...)) }
func Notice(args ...any) { logger.Notice(args...); addToBuffer("NOTICE", fmt.Sprint(args...)) }
func Noticef(format string, args ...any) { logger.Noticef(format, args...); addToBuffer("NOTICE", fmt.Sprintf(format, args...)) }
func Warning(args ...any) { logger.Warning(args...); addToBuffer("WARNING", fmt.Sprint(args...)) }
func Warningf(format string, args ...any) { logger.Warningf(format, args...); addToBuffer("WARNING", fmt.Sprintf(format, args...)) }
func Error(args ...any) { logger.Error(args...); addToBuffer("ERROR", fmt.Sprint(args...)) }
func Errorf(format string, args ...any) { logger.Errorf(format, args...); addToBuffer("ERROR", fmt.Sprintf(format, args...)) }
func addToBuffer(level string, newLog string) {
	if len(newLog) > maxLogEntryLen { newLog = newLog[:maxLogEntryLen] + "...[truncated]" }
	logMu.Lock(); defer logMu.Unlock()
	if len(logBuffer) >= maxLogBufferSize { logBuffer = logBuffer[1:] }
	lvl, _ := logging.LogLevel(level)
	logBuffer = append(logBuffer, struct {
		time  string
		level logging.Level
		log   string
	}{time: time.Now().Format(timeFormat), level: lvl, log: newLog})
}
func GetLogs(c int, level string) []string {
	logMu.Lock(); defer logMu.Unlock()
	var output []string
	logLevel, _ := logging.LogLevel(level)
	for i := len(logBuffer) - 1; i >= 0 && len(output) <= c; i-- {
		if logBuffer[i].level <= logLevel {
			output = append(output, fmt.Sprintf("%s %s - %s", logBuffer[i].time, logBuffer[i].level, logBuffer[i].log))
		}
	}
	return output
}
