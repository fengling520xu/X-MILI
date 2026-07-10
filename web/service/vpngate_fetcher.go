package service
import ("bytes";"context";"encoding/base64";"encoding/csv";"encoding/json";"errors";"fmt";"io";"net/http";"strconv";"strings";"sync";"time";"github.com/mhsanaei/3x-ui/v2/logger";"golang.org/x/sync/singleflight")
var (vpngateSFGroup singleflight.Group;vpngateHTTPClient=&http.Client{Timeout:20*time.Second,Transport:&http.Transport{MaxIdleConns:10,IdleConnTimeout:30*time.Second}})
type VPNGateFetcher struct{}
type vpnGateIPResponse struct{Status string `json:"status"`;ISP string `json:"isp"`;Org string `json:"org"`;AS string `json:"as"`;Hosting bool `json:"hosting"`;Query string `json:"query"`}
type vpnGateIPInfo struct{ISP string;ASN string;IPType string}
func (VPNGateFetcher) Fetch() ([]VPNGateServer,error){ctx,cancel:=context.WithTimeout(context.Background(),45*time.Second);defer cancel();return vpnGateFetchWithFallback(ctx)}
func vpnGateFetchWithFallback(ctx context.Context)([]VPNGateServer,error){
	val,err,_:=vpngateSFGroup.Do("fetch",func()(interface{},error){
		servers,err:=fetchVPNGateFromAPI(ctx)
		if err!=nil{logger.Warningf("[VPNGate] API failed %v fallback",err);servers2,err2:=fetchVPNGateFromWebpage(ctx);if err2!=nil{return nil,fmt.Errorf("api %v webpage %v",err,err2)};servers=servers2}
		if len(servers)==0{return nil,errors.New("no servers")}
		ipInfo:=fetchVPNGateIPDataLite(ctx,servers)
		for i:=range servers{if info,ok:=ipInfo[servers[i].IP];ok{servers[i].ISP=info.ISP;servers[i].ASN=info.ASN;servers[i].IPType=info.IPType}else{servers[i].ISP="Unknown";servers[i].ASN="Unknown";servers[i].IPType="Unknown"}}
		return servers,nil})
	if err!=nil{return nil,err};return val.([]VPNGateServer),nil}
func fetchVPNGateFromAPI(ctx context.Context)([]VPNGateServer,error){
	req,_:=http.NewRequestWithContext(ctx,"GET",vpnGateAPIURL,nil);req.Header.Set("User-Agent","X-MILI-LITE/1.0")
	resp,err:=vpngateHTTPClient.Do(req);if err!=nil{return nil,err};defer resp.Body.Close()
	if resp.StatusCode!=200{return nil,fmt.Errorf("VPNGate status %s",resp.Status)}
	body,err:=io.ReadAll(io.LimitReader(resp.Body,10<<20));if err!=nil{return nil,err}
	return parseVPNGateCSV(string(body))}
func fetchVPNGateFromWebpage(ctx context.Context)([]VPNGateServer,error){return nil,errors.New("webpage fallback needs config")}
func parseVPNGateCSV(body string)([]VPNGateServer,error){
	start:=strings.Index(body,"#HostName");if start<0{return nil,errors.New("header not found")}
	csvData:=body[start:];if end:=strings.LastIndex(csvData,"*");end>=0{csvData=csvData[:end]}
	reader:=csv.NewReader(strings.NewReader(csvData));reader.FieldsPerRecord=-1;reader.LazyQuotes=true
	records,err:=reader.ReadAll();if err!=nil{return nil,err}
	if len(records)<2{return nil,errors.New("no servers")}
	headers:=records[0];headers[0]=strings.TrimPrefix(headers[0],"#")
	col:=map[string]int{};for i,h:=range headers{col[h]=i}
	get:=func(row []string,key string)string{if idx,ok:=col[key];ok&&idx<len(row){return strings.TrimSpace(row[idx])};return ""}
	getInt:=func(row []string,key string)int64{n,_:=strconv.ParseInt(get(row,key),10,64);return n}
	servers:=make([]VPNGateServer,0,len(records)-1)
	for _,row:=range records[1:]{
		if len(row)<10{continue}
		config:=get(row,"OpenVPN_ConfigData_Base64");if config==""||len(config)<100{continue}
		ip:=get(row,"IP");if ip==""{continue}
		countryShort:=get(row,"CountryShort");ping:=getInt(row,"Ping");if ping<=0{ping=-1}
		proto,port:=parseVPNGateProtoPort(config)
		servers=append(servers,VPNGateServer{HostName:get(row,"HostName"),IP:ip,CountryLong:get(row,"CountryLong"),CountryShort:countryShort,CountryShortLower:strings.ToLower(countryShort),NumSessions:getInt(row,"NumVpnSessions"),LocalPing:ping,Proto:proto,Port:port,OpenVPNConfig:config})}
	return servers,nil}
func parseVPNGateProtoPort(base64Config string)(string,string){
	decoded,err:=base64.StdEncoding.DecodeString(base64Config);if err!=nil{return "udp",""}
	proto,port:="udp",""
	for _,line:=range strings.Split(string(decoded),"\n"){
		line=strings.TrimSpace(line);if line==""||strings.HasPrefix(line,"#")||strings.HasPrefix(line,";"){continue}
		fields:=strings.Fields(line)
		if len(fields)>=3&&fields[0]=="remote"{port=fields[2];if len(fields)>=4&&strings.Contains(strings.ToLower(fields[3]),"tcp"){proto="tcp"}}
		if len(fields)>=2&&fields[0]=="proto"{if strings.Contains(strings.ToLower(fields[1]),"tcp"){proto="tcp"}else{proto="udp"}}}
	return proto,port}
func fetchVPNGateIPDataLite(ctx context.Context,servers []VPNGateServer)map[string]vpnGateIPInfo{
	seen:=map[string]struct{}{};ips:=make([]string,0,len(servers))
	for _,s:=range servers{if _,ok:=seen[s.IP];!ok{seen[s.IP]=struct{}{};ips=append(ips,s.IP)}}
	if len(ips)==0{return nil}
	if len(ips)>50{ips=ips[:50]}
	const batchSize=100;const concurrency=3
	sem:=make(chan struct{},concurrency);var wg sync.WaitGroup;var mu sync.Mutex
	result:=make(map[string]vpnGateIPInfo,len(ips))
	client:=&http.Client{Timeout:10*time.Second}
	for i:=0;i<len(ips);i+=batchSize{
		end:=i+batchSize;if end>len(ips){end=len(ips)};batch:=ips[i:end]
		wg.Add(1);sem<-struct{}{}
		go func(b []string){
			defer wg.Done();defer func(){<-sem}()
			payload,_:=json.Marshal(b)
			req,_:=http.NewRequestWithContext(ctx,"POST","https://ip-api.com/batch?fields=status,isp,org,as,hosting,query",bytes.NewReader(payload))
			req.Header.Set("Content-Type","application/json")
			resp,err:=client.Do(req);if err!=nil{return};defer resp.Body.Close()
			var rows []vpnGateIPResponse
			if err:=json.NewDecoder(io.LimitReader(resp.Body,1<<20)).Decode(&rows);err!=nil{return}
			tmp:=map[string]vpnGateIPInfo{}
			for _,row:=range rows{
				if row.Status!="success"{continue}
				isp:=row.ISP;if isp==""{isp=row.Org};if isp==""{isp="Unknown"}
				tmp[row.Query]=vpnGateIPInfo{ISP:isp,ASN:extractVPNGateASN(row.AS),IPType:determineVPNGateIPType(row.Hosting,row.ISP,row.Org)}}
			mu.Lock();for k,v:=range tmp{result[k]=v};mu.Unlock()
		}(batch)}
	wg.Wait();return result}
func fetchVPNGateIPData(ips []string)map[string]vpnGateIPInfo{
	ctx,cancel:=context.WithTimeout(context.Background(),30*time.Second);defer cancel()
	mock:=make([]VPNGateServer,len(ips));for i,ip:=range ips{mock[i]=VPNGateServer{IP:ip}}
	return fetchVPNGateIPDataLite(ctx,mock)}
func extractVPNGateASN(as string)string{if as==""{return "Unknown"};parts:=strings.Fields(as);if len(parts)>0&&strings.HasPrefix(strings.ToUpper(parts[0]),"AS"){return parts[0]};return as}
func determineVPNGateIPType(hosting bool,isp,org string)string{if hosting{return "机房IP"};text:=strings.ToLower(isp+" "+org);for _,kw:=range []string{"datacenter","hosting","cloud","vps","amazon","aws","google","microsoft","azure","oracle","linode","ovh","vultr","hetzner","contabo","tencent","alibaba"}{if strings.Contains(text,kw){return "机房IP"}};return "住宅IP"}
