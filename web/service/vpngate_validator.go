package service
import ("bytes";"context";"net";"os";"os/exec";"regexp";"runtime";"strconv";"time")
type VPNGateValidator struct{}
type vpnGateCheckResult struct{index int;localPing int64;isAlive bool}
var vpnGateMSRegex=regexp.MustCompile(`[=<]\s*(\d+)ms|time[=<]\s*(\d+)\.?(?:\d*)\s*ms`)
func (VPNGateValidator) Validate(servers []VPNGateServer)[]VPNGateServer{
	if len(servers)==0{return servers}
	tasks:=make(chan int,len(servers));results:=make(chan vpnGateCheckResult,len(servers))
	workers:=5;if len(servers)<workers{workers=len(servers)}
	for i:=0;i<workers;i++{go func(){for index:=range tasks{server:=servers[index];ping:=server.LocalPing;alive:=server.OpenVPNConfig!="";if server.Proto=="tcp"&&server.Port!=""{alive=testVPNGateTCP(server.IP,server.Port)};if ping<0&&alive{if index<10{ping=tcpPing(server.IP,server.Port);if ping<0{ping=999999}}else{ping=999999}};results<-vpnGateCheckResult{index:index,localPing:ping,isAlive:alive}} }()}
	for i:=range servers{tasks<-i};close(tasks)
	checked:=make([]vpnGateCheckResult,len(servers));for range servers{res:=<-results;checked[res.index]=res}
	for i,res:=range checked{servers[i].LocalPing=-1;if res.isAlive{servers[i].LocalPing=res.localPing}}
	return servers}
func testVPNGateTCP(ip,port string)bool{conn,err:=net.DialTimeout("tcp",net.JoinHostPort(ip,port),2*time.Second);if err!=nil{return false};conn.Close();return true}
func tcpPing(ip,port string)int64{if port==""{port="443"};start:=time.Now();conn,err:=net.DialTimeout("tcp",net.JoinHostPort(ip,port),2*time.Second);if err!=nil{return -1};conn.Close();return time.Since(start).Milliseconds()}
func TestVPNGateOpenVPN(server VPNGateServer)(bool,int64){
	ovpn,err:=sanitizeVPNGateOpenVPNConfig(server.OpenVPNConfig);if err!=nil||!commandExists("openvpn"){return false,-1}
	tmp,err:=os.CreateTemp("","vpngate-check-*.ovpn");if err!=nil{return false,-1};configPath:=tmp.Name();defer os.Remove(configPath)
	if _,err:=tmp.WriteString(ovpn);err!=nil{tmp.Close();return false,-1};tmp.Close()
	ctx,cancel:=context.WithTimeout(context.Background(),18*time.Second);defer cancel()
	start:=time.Now();writer:=&openVPNLogWriter{}
	cmd:=exec.CommandContext(ctx,"openvpn","--config",configPath,"--route-nopull","--auth-nocache","--verb","2","--connect-retry-max","1","--connect-timeout","10")
	cmd.Stdout=writer;cmd.Stderr=writer;if err:=cmd.Start();err!=nil{return false,-1}
	done:=make(chan error,1);go func(){done<-cmd.Wait()}();ticker:=time.NewTicker(300*time.Millisecond);defer ticker.Stop()
	for{select{case <-ticker.C:if writer.contains("Initialization Sequence Completed"){ms:=time.Since(start).Milliseconds();cancel();<-done;return true,ms};case <-done:if writer.contains("Initialization Sequence Completed"){return true,time.Since(start).Milliseconds()};return false,-1;case <-ctx.Done():<-done;return false,-1}}}
func testVPNGateOpenVPN(s VPNGateServer)(bool,int64){return TestVPNGateOpenVPN(s)}
func pingVPNGateIP(ip string)int64{return tcpPing(ip,"443")}
func pingVPNGateIPOld(ip string)int64{
	args:=[]string{"-c","1","-W","1",ip};if runtime.GOOS=="windows"{args=[]string{"-n","1","-w","1000",ip}}
	cmd:=exec.Command("ping",args...);var out bytes.Buffer;cmd.Stdout=&out;if err:=cmd.Run();err!=nil{return -1}
	matches:=vpnGateMSRegex.FindStringSubmatch(out.String());for _,m:=range matches[1:]{if m==""{continue};n,err:=strconv.ParseInt(m,10,64);if err==nil{return n}};return -1}
