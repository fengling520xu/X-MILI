package job
import (
	"os"
	"github.com/mhsanaei/3x-ui/v2/logger"
	"github.com/mhsanaei/3x-ui/v2/xray"
)
type ClearLogsJob struct{}
func NewClearLogsJob() *ClearLogsJob { return new(ClearLogsJob) }
func (j *ClearLogsJob) Run() {
	logFiles := []string{
		xray.GetIPLimitLogPath(),
		xray.GetIPLimitBannedLogPath(),
		xray.GetAccessPersistentLogPath(),
		xray.GetIPLimitBannedPrevLogPath(),
		xray.GetAccessPersistentPrevLogPath(),
	}
	for _, path := range logFiles {
		if err := os.Remove(path); err == nil {
			logger.Debug("[LITE ClearLogs] removed:", path)
		} else {
			_ = os.Truncate(path, 0)
		}
	}
	if accessPath, err := xray.GetAccessLogPath(); err == nil && accessPath != "" {
		_ = os.Truncate(accessPath, 0)
	}
}
