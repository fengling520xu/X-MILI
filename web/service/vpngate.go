package service
import (
	"sort"
	"sync"
	"time"
	"github.com/mhsanaei/3x-ui/v2/logger"
)
type VPNGateService struct{}
const (
	vpnGateAPIURL     = "https://www.vpngate.net/api/iphone/"
	vpnGateCacheTTL   = 10 * time.Minute
	vpnGateMaxServers = 100
)
type VPNGateServer struct {
	HostName string `json:"hostName"`; IP string `json:"ip"`; CountryLong string `json:"countryLong"`; CountryShort string `json:"countryShort"`; CountryShortLower string `json:"countryShortLower"`; NumSessions int64 `json:"numSessions"`; ISP string `json:"isp"`; ASN string `json:"asn"`; IPType string `json:"ipType"`; LocalPing int64 `json:"localPing"`; Proto string `json:"proto"`; Port string `json:"port"`; OpenVPNConfig string `json:"openVPNConfig"`
}
var vpnGateCache struct{sync.RWMutex;servers []VPNGateServer;expires time.Time}
func (s *VPNGateService) ListServers(refresh bool) ([]VPNGateServer,error){return s.ListServersWithUnavailable(refresh,false)}
func (s *VPNGateService) ListServersWithUnavailable(refresh bool,includeUnavailable bool)([]VPNGateServer,error){
	if !refresh{vpnGateCache.RLock();if time.Now().Before(vpnGateCache.expires)&&len(vpnGateCache.servers)>0{servers:=cloneVPNGateServers(vpnGateCache.servers);vpnGateCache.RUnlock();if includeUnavailable{return servers,nil};return filterVPNGateAvailable(servers),nil};vpnGateCache.RUnlock()}
	servers,err:=loadVPNGateServers();if err!=nil{vpnGateCache.RLock();if len(vpnGateCache.servers)>0{old:=cloneVPNGateServers(vpnGateCache.servers);vpnGateCache.RUnlock();logger.Warningf("[VPNGate LITE] fetch failed, stale cache: %v",err);if includeUnavailable{return old,nil};return filterVPNGateAvailable(old),nil};vpnGateCache.RUnlock();return nil,err}
	vpnGateCache.Lock();vpnGateCache.servers=limitVPNGateServers(servers,vpnGateMaxServers);vpnGateCache.expires=time.Now().Add(vpnGateCacheTTL);sc:=cloneVPNGateServers(vpnGateCache.servers);vpnGateCache.Unlock()
	lastFetchTimeMutex.Lock();lastFetchTime=time.Now();lastFetchTimeMutex.Unlock()
	if includeUnavailable{return sc,nil};return filterVPNGateAvailable(sc),nil}
func loadVPNGateServers()([]VPNGateServer,error){servers,err:=(VPNGateFetcher{}).Fetch();if err!=nil{return nil,err};servers=(VPNGateValidator{}).Validate(servers);sortVPNGateServers(servers);return servers,nil}
func sortVPNGateServers(servers []VPNGateServer){sort.Slice(servers,func(i,j int)bool{pi,pj:=servers[i].LocalPing,servers[j].LocalPing;if pi==-1&&pj==-1{return servers[i].NumSessions>servers[j].NumSessions};if pi==-1{return false};if pj==-1{return true};if pi!=pj{return pi<pj};return servers[i].NumSessions>servers[j].NumSessions})}
func cloneVPNGateServers(servers []VPNGateServer)[]VPNGateServer{clone:=make([]VPNGateServer,len(servers));copy(clone,servers);return clone}
func limitVPNGateServers(servers []VPNGateServer,limit int)[]VPNGateServer{if limit<=0||len(servers)<=limit{return servers};return servers[:limit]}
func filterVPNGateAvailable(servers []VPNGateServer)[]VPNGateServer{active:=make([]VPNGateServer,0,len(servers));for _,s:=range servers{if s.LocalPing>=0{active=append(active,s)}};return active}
var (lastFetchTime time.Time;lastFetchTimeMutex sync.Mutex)
func CheckAndRefreshVPNGate(intervalMinutes int){lastFetchTimeMutex.Lock();defer lastFetchTimeMutex.Unlock();if lastFetchTime.IsZero()||time.Since(lastFetchTime)>=time.Duration(intervalMinutes)*time.Minute{lastFetchTime=time.Now();go func(){logger.Info("[VPNGate LITE] background refresh");_,err:=(&VPNGateService{}).ListServers(true);if err!=nil{logger.Errorf("[VPNGate] refresh failed: %v",err);lastFetchTimeMutex.Lock();lastFetchTime=time.Time{};lastFetchTimeMutex.Unlock()}else{logger.Info("[VPNGate LITE] refresh ok")}}()}}
func (s *VPNGateService) ClearCache(){vpnGateCache.Lock();defer vpnGateCache.Unlock();vpnGateCache.servers=nil;vpnGateCache.expires=time.Time{}}
