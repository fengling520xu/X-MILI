package service
import ("encoding/json";"fmt";"io";"net";"net/http";"net/url";"os";"time";"github.com/mhsanaei/3x-ui/v2/database";"github.com/mhsanaei/3x-ui/v2/database/model";"github.com/mhsanaei/3x-ui/v2/logger";"github.com/mhsanaei/3x-ui/v2/util/json_util";"github.com/mhsanaei/3x-ui/v2/xray";"gorm.io/gorm")
type OutboundService struct{}
var testSemaphore=make(chan struct{},3)
func (s *OutboundService) AddTraffic(traffics []*xray.Traffic,clientTraffics []*xray.ClientTraffic)(error,bool){
	var err error;db:=database.GetDB();tx:=db.Begin();defer func(){if err!=nil{tx.Rollback()}else{tx.Commit()}}()
	err=s.addOutboundTraffic(tx,traffics);return err,false}
func (s *OutboundService) addOutboundTraffic(tx *gorm.DB,traffics []*xray.Traffic)error{
	if len(traffics)==0{return nil}
	for _,traffic:=range traffics{if traffic.IsOutbound{var outbound model.OutboundTraffics;if err:=tx.Model(&model.OutboundTraffics{}).Where("tag = ?",traffic.Tag).FirstOrCreate(&outbound).Error;err!=nil{return err};outbound.Tag=traffic.Tag;outbound.Up+=traffic.Up;outbound.Down+=traffic.Down;outbound.Total=outbound.Up+outbound.Down;if err:=tx.Save(&outbound).Error;err!=nil{return err}}};return nil}
func (s *OutboundService) GetOutboundsTraffic()([]*model.OutboundTraffics,error){db:=database.GetDB();var traffics []*model.OutboundTraffics;if err:=db.Model(model.OutboundTraffics{}).Find(&traffics).Error;err!=nil{logger.Warning("Error retrieving OutboundTraffics: ",err);return nil,err};return traffics,nil}
func (s *OutboundService) ResetOutboundTraffic(tag string)error{db:=database.GetDB();whereText:="tag ";if tag=="-alltags-"{whereText+=" <> ?"}else{whereText+=" = ?"};return db.Model(model.OutboundTraffics{}).Where(whereText,tag).Updates(map[string]any{"up":0,"down":0,"total":0}).Error}
type TestOutboundResult struct{Success bool `json:"success"`;Delay int64 `json:"delay"`;Error string `json:"error,omitempty"`;StatusCode int `json:"statusCode,omitempty"`;Outbound any `json:"outbound,omitempty"`}
func (s *OutboundService) TestOutbound(outboundJSON string,testURL string,allOutboundsJSON string)(*TestOutboundResult,error){
	if testURL==""{testURL=DefaultXrayOutboundTestURL}
	select{case testSemaphore<-struct{}{}:defer func(){<-testSemaphore}():default:return &TestOutboundResult{Success:false,Error:"Another outbound test running (LITE limit 3)"},nil}
	var testOutbound map[string]any;if err:=json.Unmarshal([]byte(outboundJSON),&testOutbound);err!=nil{return &TestOutboundResult{Success:false,Error:fmt.Sprintf("Invalid outbound JSON: %v",err)},nil}
	outboundTag,_:=testOutbound["tag"].(string);if outboundTag==""{return &TestOutboundResult{Success:false,Error:"Outbound has no tag"},nil}
	var allOutbounds []any;if allOutboundsJSON!=""{_=json.Unmarshal([]byte(allOutboundsJSON),&allOutbounds)}
	if len(allOutbounds)==0{allOutbounds=[]any{testOutbound}}
	testPort,err:=findAvailablePort();if err!=nil{return &TestOutboundResult{Success:false,Error:fmt.Sprintf("Failed port: %v",err)},nil}
	testConfig:=s.createTestConfig(outboundTag,allOutbounds,testPort)
	testConfigPath,err:=createTestConfigPath();if err!=nil{return &TestOutboundResult{Success:false,Error:fmt.Sprintf("Failed config path: %v",err)},nil}
	xrayProcess:=xray.NewTestProcess(testConfig,testConfigPath);if err:=xrayProcess.Start();err!=nil{return &TestOutboundResult{Success:false,Error:fmt.Sprintf("Failed start xray: %v",err)},nil};defer xrayProcess.Stop()
	time.Sleep(1*time.Second);if !xrayProcess.IsRunning(){return &TestOutboundResult{Success:false,Error:fmt.Sprintf("Xray failed: %s",xrayProcess.GetResult())},nil}
	client:=&http.Client{Timeout:8*time.Second,Transport:&http.Transport{Proxy:func(req *http.Request)(*url.URL,error){return url.Parse(fmt.Sprintf("socks5://127.0.0.1:%d",testPort))}}}
	start:=time.Now();resp,err:=client.Get(testURL);elapsed:=time.Since(start).Milliseconds()
	if err!=nil{return &TestOutboundResult{Success:false,Error:fmt.Sprintf("Request failed: %v",err)},nil};defer resp.Body.Close();_,_=io.ReadAll(io.LimitReader(resp.Body,1<<20))
	return &TestOutboundResult{Success:resp.StatusCode>=200&&resp.StatusCode<400,Delay:elapsed,StatusCode:resp.StatusCode},nil}
func (s *OutboundService) createTestConfig(outboundTag string,allOutbounds []any,testPort int)*xray.Config{
	inbound:=map[string]any{"tag":"test-in","protocol":"socks","port":testPort,"listen":"127.0.0.1","settings":map[string]any{"auth":"noauth","udp":true}}
	inboundJSON,_:=json.Marshal(inbound);var inboundConfig xray.InboundConfig;_=json.Unmarshal(inboundJSON,&inboundConfig)
	routing:=map[string]any{"rules":[]any{map[string]any{"type":"field","inboundTag":[]string{"test-in"},"outboundTag":outboundTag}}}
	routingJSON,_:=json.Marshal(routing);logJSON:=json_util.RawMessage(`{"loglevel":"none"}`)
	return &xray.Config{LogConfig:logJSON,InboundConfigs:[]xray.InboundConfig{inboundConfig},OutboundConfigs:json_util.RawMessage(func()string{b,_:=json.Marshal(allOutbounds);return string(b)}()),RouterConfig:json_util.RawMessage(string(routingJSON)),}}
func findAvailablePort()(int,error){addr,_:=net.ResolveTCPAddr("tcp","127.0.0.1:0");l,err:=net.ListenTCP("tcp",addr);if err!=nil{return 0,err};defer l.Close();return l.Addr().(*net.TCPAddr).Port,nil}
func createTestConfigPath()(string,error){tmpFile,err:=os.CreateTemp("","xray-test-*.json");if err!=nil{return "",err};path:=tmpFile.Name();tmpFile.Close();return path,nil}
var DefaultXrayOutboundTestURL="https://www.gstatic.com/generate_204"
