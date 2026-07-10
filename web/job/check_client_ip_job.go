package job
import (
	"bufio"
	"encoding/json"
	"os"
	"regexp"
	"time"
	"github.com/mhsanaei/3x-ui/v2/database"
	"github.com/mhsanaei/3x-ui/v2/database/model"
	"github.com/mhsanaei/3x-ui/v2/xray"
)
const (
	ipStaleAfterSeconds = int64(30 * 60)
	maxAccessLogLines   = 5000
)
type CheckClientIpJob struct {
	lastClear int64
	disAllowedIps []string
}
type IPWithTimestamp struct {
	IP        string `json:"ip"`
	Timestamp int64  `json:"ts"`
}
var job *CheckClientIpJob
func NewCheckClientIpJob() *CheckClientIpJob { job = new(CheckClientIpJob); return job }
func (j *CheckClientIpJob) Run() {
	if j.lastClear == 0 { j.lastClear = time.Now().Unix() }
	if !j.hasLimitIp() {
		if time.Now().Unix()-j.lastClear > 3600 { j.clearAccessLogLite() }
		return
	}
	shouldClear := j.processLogFileLite()
	if shouldClear || time.Now().Unix()-j.lastClear > 3600 { j.clearAccessLogLite() }
}
func (j *CheckClientIpJob) clearAccessLogLite() {
	accessLogPath, err := xray.GetAccessLogPath()
	if err != nil || accessLogPath == "" { return }
	_ = os.Truncate(accessLogPath, 0)
	j.lastClear = time.Now().Unix()
}
func (j *CheckClientIpJob) hasLimitIp() bool {
	db := database.GetDB()
	var inbounds []*model.Inbound
	if err := db.Model(model.Inbound{}).Find(&inbounds).Error; err != nil { return false }
	for _, inbound := range inbounds {
		if inbound.Settings == "" { continue }
		settings := map[string][]model.Client{}
		_ = json.Unmarshal([]byte(inbound.Settings), &settings)
		for _, client := range settings["clients"] {
			if client.LimitIP > 0 { return true }
		}
	}
	return false
}
func (j *CheckClientIpJob) processLogFileLite() bool {
	accessLogPath, err := xray.GetAccessLogPath()
	if err != nil { return false }
	file, err := os.Open(accessLogPath)
	if err != nil { return false }
	defer file.Close()
	ipRegex := regexp.MustCompile(`from (?:tcp:|udp:)?\[?([0-9a-fA-F\.:]+)\]?:\d+ accepted`)
	emailRegex := regexp.MustCompile(`email: (.+)$`)
	timestampRegex := regexp.MustCompile(`^(\d{4}/\d{2}/\d{2} \d{2}:\d{2}:\d{2})`)
	inboundClientIps := make(map[string]map[string]int64, 100)
	scanner := bufio.NewScanner(file)
	buf := make([]byte, 0, 64*1024)
	scanner.Buffer(buf, 1024*1024)
	lines := 0
	for scanner.Scan() {
		lines++
		if lines > maxAccessLogLines { break }
		line := scanner.Text()
		ipMatches := ipRegex.FindStringSubmatch(line)
		if len(ipMatches) < 2 { continue }
		ip := ipMatches[1]
		if ip == "127.0.0.1" || ip == "::1" { continue }
		emailMatches := emailRegex.FindStringSubmatch(line)
		if len(emailMatches) < 2 { continue }
		email := emailMatches[1]
		var timestamp int64
		if tsMatches := timestampRegex.FindStringSubmatch(line); len(tsMatches) >= 2 {
			if t, err := time.Parse("2006/01/02 15:04:05", tsMatches[1]); err == nil {
				timestamp = t.Unix()
			} else { timestamp = time.Now().Unix() }
		} else { timestamp = time.Now().Unix() }
		if _, ok := inboundClientIps[email]; !ok { inboundClientIps[email] = make(map[string]int64) }
		if existing, ok := inboundClientIps[email][ip]; !ok || timestamp > existing {
			inboundClientIps[email][ip] = timestamp
		}
	}
	return lines >= maxAccessLogLines
}
func (j *CheckClientIpJob) checkAccessLogAvailable(active bool) bool {
	if !active { return false }
	path, err := xray.GetAccessLogPath()
	if err != nil { return false }
	_, err = os.Stat(path)
	return err == nil
}
func (j *CheckClientIpJob) checkFail2BanInstalled() bool { return false }
func (j *CheckClientIpJob) processLogFile() bool { return j.processLogFileLite() }
func (j *CheckClientIpJob) clearAccessLog() { j.clearAccessLogLite() }
func mergeClientIps(old, new []IPWithTimestamp, cutoff int64) []IPWithTimestamp { return new }
func partitionLiveIps(m map[string]int64, observed map[string]bool) ([]IPWithTimestamp, []IPWithTimestamp) { return nil, nil }
func (j *CheckClientIpJob) disconnectClientTemporarily(inbound interface{}, email string, clients []model.Client) {}
func (j *CheckClientIpJob) GetInboundClientIps(email string) (string, error) { return "[]", nil }
func (j *CheckClientIpJob) checkError(err error) {}
