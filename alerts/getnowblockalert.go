package alerts

import (
	"encoding/json"
	"fmt"
	"github.com/astaxie/beego/logs"
	"github.com/influxdata/platform/kit/errors"
	"github.com/sasaxie/monitor/common/database/influxdb"
	"github.com/sasaxie/monitor/dingding"
	"github.com/sasaxie/monitor/models"
	"strings"
	"time"
	"github.com/aliyun/alibaba-cloud-sdk-go/sdk"
	"github.com/aliyun/alibaba-cloud-sdk-go/sdk/requests"
	"github.com/sasaxie/monitor/function"
)

// ms: 5min
const Internal5min int64 = 1000 * 60 * 5
const Internal1s float64 = 1
const Internal1s_2 float64 = 2
const InternalEvent float64  = 3
var lastBlockNum =  map[string]int64{}
type GetNowBlockAlert struct {
	Nodes       []*Node
	Result      map[string]*GetNowBlockAlertMsg
	MaxBlockNum map[string]int64
	MinBlockNum map[string]int64
}

type GetNowBlockAlertMsg struct {
	Ip      string
	Port    int
	Type    string
	TagName string
	Num     int64
	MaxNum  int64

	StartTime time.Time
	FreshTime time.Time
	IsFresh   bool
	IsRecover bool
	Msg       string
}

func (g GetNowBlockAlertMsg) String() string {
	return fmt.Sprintf(`ip: %s
port: %d
type: %s
tagName: %s
num: %d
maxNum: %d
duration: %v
msg: %s
`, g.Ip, g.Port, g.Type, g.TagName, g.Num,
		g.MaxNum,
		time.Now().Sub(g.StartTime), g.Msg)
}

func (g *GetNowBlockAlert) Load() {
	if models.NodeList == nil && models.NodeList.Addresses == nil {
		panic("get now block alert load() error")
	}

	if g.Nodes == nil {
		g.Nodes = make([]*Node, 0)
	}

	for _, node := range models.NodeList.Addresses {
		if strings.Contains(node.Monitor, "NowBlock") {
			n := new(Node)
			n.Ip = node.Ip
			n.GrpcPort = node.GrpcPort
			n.HttpPort = node.HttpPort
			n.Type = node.Type
			n.TagName = node.TagName

			g.Nodes = append(g.Nodes, n)
		}
	}

	g.Result = make(map[string]*GetNowBlockAlertMsg)

	logs.Info(
		"get now block alert load() success, node size:",
		len(g.Nodes))
}

func callUpdateAnomaly(number string) {

	client, err := sdk.NewClientWithAccessKey("default", "LTAIbEOdCXFYrP98", "wNVf3zMK6dqwxvwp2oYsq9iTBYPXq1")
	if err != nil {
		panic(err)
	}

	request := requests.NewCommonRequest()
	request.Method = "POST"
	request.Scheme = "https" // https | http
	request.Domain = "dyvmsapi.aliyuncs.com"
	request.Version = "2017-05-25"
	request.ApiName = "SingleCallByTts"
	request.QueryParams["RegionId"] = "default"
	request.QueryParams["CalledShowNumber"] = "01086393840"
	request.QueryParams["CalledNumber"] = number
	request.QueryParams["TtsCode"] = "TTS_163525650"

	request.QueryParams["TtsParam"] = "{\"app\":\"块更新异常\"}"


	_, err = client.ProcessCommonRequest(request)
	if err != nil {
		panic(err)
	}
}

/**
 Rules:
	1. Block number no change;
	2. Block number is zero;
	3. Max block number - Block number > 100;
	4. No data.
*/
func (g *GetNowBlockAlert) Start() {
	g.MaxBlockNum = make(map[string]int64)
	g.MinBlockNum = make(map[string]int64)
	queryTimeS := time.Now().UnixNano() / 1000000

	g.GetMaxBlockNum(queryTimeS)

	for _, n := range g.Nodes {
		maxBlockNum := g.MaxBlockNum[n.TagName]
		num, _ := g.getNodeBlockNum(n.Ip, n.HttpPort, queryTimeS)
		err := g.isOk(n.Ip, n.TagName, n.HttpPort, queryTimeS, maxBlockNum, num)
		k := fmt.Sprintf("%s:%d", n.Ip, n.HttpPort)
		if err != nil {
			if _, ok := g.Result[k]; ok {
				logs.Debug(
					"get now block alert [update result]:",
					k)
				g.Result[k].MaxNum = maxBlockNum
				g.Result[k].Num = num
			} else {
				g.Result[k] = &GetNowBlockAlertMsg{
					Ip:        n.Ip,
					Port:      n.HttpPort,
					Type:      n.Type,
					TagName:   n.TagName,
					Num:       num,
					MaxNum:    maxBlockNum,
					StartTime: time.Now(),
					FreshTime: time.Now(),
					IsFresh:   true,
					IsRecover: false,
					Msg:       "块更新异常",
				}

				// 郭宏
				callUpdateAnomaly("13671062020")
				// 黄文广
				callUpdateAnomaly("15910709326")
				// who
				callWhoUpdateAnomaly();

				g.Result[k].FreshTime = time.Date(
					g.Result[k].FreshTime.Year(),
					g.Result[k].FreshTime.Month(),
					g.Result[k].FreshTime.Day(),
					g.Result[k].FreshTime.Hour(),
					0,
					0,
					0,
					time.Local)
				logs.Debug("get now block alert [new result]:",
					k,
					g.Result[k],
					g.Result[k].FreshTime)
			}
		} else {
			if _, ok := g.Result[k]; ok {
				logs.Debug(
					"get now block alert [recover result]:",
					k)
				g.Result[k].IsRecover = true
				g.Result[k].IsFresh = false
			}
		}
	}

}

func (g *GetNowBlockAlert) Alert() {
	logs.Info("now block alert")

	for k, v := range g.Result {
		msg := ""

		if v.IsRecover {
			msg += k + "块更新恢复正常\n"
			delete(g.Result, k)
		} else if v.IsFresh {
			logs.Debug("get now block alert [alert result]:", k)
			msg += v.String() + "\n"
			g.Result[k].IsFresh = false
		} else {
			if time.Now().Sub(v.FreshTime) > time.Hour {
				logs.Debug("get now block alert [fresh result]:", k)
				g.Result[k].IsFresh = true
				g.Result[k].FreshTime = time.Now()

				g.Result[k].FreshTime = time.Date(
					g.Result[k].FreshTime.Year(),
					g.Result[k].FreshTime.Month(),
					g.Result[k].FreshTime.Day(),
					g.Result[k].FreshTime.Hour(),
					0,
					0,
					0,
					time.Local)
			}
		}

		if len(msg) == 0 || strings.EqualFold(msg, "") {
			continue
		}

		bodyContent := fmt.Sprintf(`
			{
				"msgtype": "text",
				"text": {
					"content": "%s"
				}
			}
			`, msg)

		dingding.DingAlarm.Alarm([]byte(bodyContent), msg)
	}
}

func (g *GetNowBlockAlert) getNodeBlockNum(ip string, port int,
	queryTimeS int64) (int64, error) {
	q := fmt.Sprintf(
		`SELECT max(Number) FROM %s WHERE time <= %s AND time >= %s AND (
"Node" = '%s:%d')`,
		"api_get_now_block",
		fmt.Sprintf("%dms", queryTimeS),
		fmt.Sprintf("%dms", queryTimeS-Internal5min),
		ip,
		port)

	return g.getBlockNum(q)
}

func callWhoUpdateAnomaly() {
	DUTY := []string{"吴彬", "岳瑞鹏", "张思聪", "吴斌", "梁志彦", "张博", "成特学"}
	Phone := []string{"18903830819", "13311527723", "13466613212", "18515212681", "15256073545", "18567720695", "18511911301"}
	who  := (time.Now().Unix() - 86400 * 4) / 86400 / 7 % int64(len(DUTY))
	callUpdateAnomaly(Phone[who])
}

func (g *GetNowBlockAlert) GetMaxBlockNum(
	queryTimeS int64) {

	tagMap := make(map[string]bool)

	for _, n := range g.Nodes {
		if _, ok := tagMap[n.TagName]; !ok {
			tagMap[n.TagName] = true
		}
	}

	for k := range tagMap {
		q := fmt.Sprintf(
			`SELECT max(Number) FROM %s WHERE time <= %s AND time >= %s AND
"TagName" = '%s'`,
			"api_get_now_block", fmt.Sprintf("%dms", queryTimeS),
			fmt.Sprintf("%dms", queryTimeS-Internal5min), k)
		num, _ := g.getBlockNum(q)
		g.MaxBlockNum[k] = num
	}
}

func (g *GetNowBlockAlert) getMinBlockNum(
	queryTimeS int64) {

	tagMap := make(map[string]bool)

	for _, n := range g.Nodes {
		if _, ok := tagMap[n.TagName]; !ok {
			tagMap[n.TagName] = true
		}
	}

	for k := range tagMap {
		q := fmt.Sprintf(
			`SELECT min(Number) FROM %s WHERE time <= %s AND time >= %s AND
"TagName" = '%s'`,
			"api_get_now_block", fmt.Sprintf("%dms", queryTimeS),
			fmt.Sprintf("%dms", queryTimeS-Internal5min), k)

		num, _ := g.getBlockNum(q)
		g.MinBlockNum[k] = num
	}
}

func (g *GetNowBlockAlert) getBlockNum(q string) (int64, error) {
	res, err := influxdb.QueryDB(influxdb.Client.C, q)
	if err != nil {
		logs.Error(err)
		return 0, err
	}

	if res == nil || len(res) == 0 ||
		res[0].Series == nil || len(res[0].Series) == 0 ||
		res[0].Series[0].Values == nil || len(res[0].Series[0].Values) == 0 {
		return 0, errors.New("get block number error: no data")
	}

	val := res[0].Series[0].Values[0][1].(json.Number)

	v, err := val.Int64()
	if err != nil {
		return 0, err
	}

	return v, nil
}

func (g *GetNowBlockAlert) isOk(ip, tag string, port int,
	queryTimeS, maxBlockNum, num int64) error {
	if maxBlockNum == 0 && num == 0 {
		return nil
	}
	if (num == 0 && lastBlockNum[ip] == 0) {
		return nil
	}
	lastBlockNum[ip] = num

	// 存在的问题：每次都会清空结果，不要这样
	if maxBlockNum == 0 {
		logs.Warn(fmt.Sprintf("get now block alert error: [max block num"+
			" error"+
			"]: [ip: %s, "+
			"tag: %s, port: %d, "+
			"maxBlockNum: %d]", ip, tag, port, maxBlockNum))
		return errors.New("max block num is 0")
	}

	if num == 0 {
		logs.Warn(fmt.Sprintf("get now block alert error: [num is 0]: [ip"+
			": %s, tag: %s, port: %d, "+
			"maxBlockNum: %d, num: %d]", ip, tag, port, maxBlockNum, num))
		return errors.New("block num is 0")
	}

	if (maxBlockNum - num) > 100 {
		logs.Warn(fmt.Sprintf("get now block alert error: [slow]: [ip: %s, "+
			"tag: %s, port: %d, "+
			"maxBlockNum: %d, num: %d]", ip, tag, port, maxBlockNum, num))
		return errors.New("block num update slowly")
	}
	return nil
}


func (g *GetNowBlockAlert)ReportDelay(fullUrl []string, solidityUrl []string) {
	for _, n := range g.Nodes {
		var urllist []string
		if (n.HttpPort == 8090) {
			urllist = fullUrl
		} else {
			urllist = solidityUrl
		}

		for _, endpoint := range urllist {
			httpUrl :=  fmt.Sprintf("http://%s:%d/%s", n.Ip, n.HttpPort, endpoint);
			nowElapsed := function.Get(httpUrl)
			httpMap := map[string]string{
				"IP":   fmt.Sprintf("%s:%d", n.Ip, n.HttpPort),
				"EndPoint": endpoint,
			}
			httpFields := map[string]interface{}{
				"IP":       fmt.Sprintf("%s:%d", n.Ip, n.HttpPort),
				"Second":   nowElapsed,
				"URL":      httpUrl,
			}

			influxdb.Client.WriteByTime("api_report_http", httpMap, httpFields, time.Now())
		}
	}
}

//func (g *GetNowBlockAlert)ReportSRDelay() {
//	for _, n := range g.Nodes {
//		if n.Type != "sr_witness_node" {
//			continue
//		}
//		httpUrl :=  fmt.Sprintf("http://%s:%d/wallet/getblockbylatestnum?num=1", n.Ip, n.HttpPort);
//		nowElapsed := function.Get(httpUrl)
//		httpMap := map[string]string{
//			"IP":   fmt.Sprintf("%s:%d", n.Ip, n.HttpPort),
//			"EndPoint": endpoint,
//		}
//		httpFields := map[string]interface{}{
//			"IP":       fmt.Sprintf("%s:%d", n.Ip, n.HttpPort),
//			"Second":   nowElapsed,
//			"URL":      httpUrl,
//		}
//		influxdb.Client.WriteByTime("api_report_http", httpMap, httpFields, time.Now())
//	}
//}

func (g *GetNowBlockAlert)ReportEventQuery(event string, eventQueryUrl []string) {
	for _, endpoint := range eventQueryUrl {
		httpUrl :=  fmt.Sprintf("%s%s", event, endpoint);
		nowElapsed := function.Get(httpUrl)
		httpMap := map[string]string{
			"EndPoint": endpoint,
		}
		httpFields := map[string]interface{}{
			"Second":   nowElapsed,
			"URL":      endpoint,
		}
		influxdb.Client.WriteByTime("api_report_eventQuery", httpMap, httpFields, time.Now())

		if (nowElapsed > InternalEvent) {
			retryGetEvent(httpUrl, endpoint)
		}
	}
}

func retryGetEvent(httpUrl, endpoint string) float64{
	nowElapsed := function.Get(httpUrl)
	if (nowElapsed > InternalEvent) {
		callWhoAPIAnomaly(endpoint)
		dingdingAlertAnomaly(httpUrl, nowElapsed)
	}
	return nowElapsed
}


func retryGet(httpUrl, endpoint string) float64{
	nowElapsed := function.Get(httpUrl)
	if (nowElapsed > Internal1s) {
		callWhoAPIAnomaly(endpoint)
		dingdingAlertAnomaly(httpUrl, nowElapsed)
	}
	return nowElapsed
}

func (g *GetNowBlockAlert)ReportDelayEX(httpex string, fullUrl []string, solidityUrl []string) {
	for _, endpoint := range fullUrl {
		httpUrl :=  fmt.Sprintf("%s%s", httpex, endpoint);
		nowElapsed := function.Get(httpUrl)
		if (nowElapsed > Internal1s) {
			retryGet(httpUrl, endpoint)
		}

		httpMap := map[string]string{
			"EndPoint": endpoint,
		}
		httpFields := map[string]interface{}{
			"Second":   nowElapsed,
			"URL":      endpoint,
		}
		influxdb.Client.WriteByTime("api_report_ex", httpMap, httpFields, time.Now())
	}
	for _, endpoint := range solidityUrl {
		httpUrl :=  fmt.Sprintf("%s%s", httpex, endpoint);
		nowElapsed := function.Get(httpUrl)
		if (nowElapsed > Internal1s) {
			retryGet(httpUrl, endpoint)
		}

		httpMap := map[string]string{
			"EndPoint": endpoint,
		}
		httpFields := map[string]interface{}{
			"Second":   nowElapsed,
			"URL":      endpoint,
		}
		influxdb.Client.WriteByTime("api_report_ex", httpMap, httpFields, time.Now())
	}
}

func dingdingAlertAnomaly(httpUrl string, nowElapsed float64) {
	msg	 := fmt.Sprintf("url: %s 访问超时%v", httpUrl, nowElapsed)
	fmt.Print(msg)
	bodyContent := fmt.Sprintf(`
			{
				"msgtype": "text",
				"text": {
					"content": "%s"
				}
			}
			`, msg)
	dingding.DingAlarm.Alarm([]byte(bodyContent), msg)
}

func callWhoAPIAnomaly(endpoint string) {
	DUTY := []string{"吴彬", "岳瑞鹏", "张思聪", "吴斌", "梁志彦", "张博", "成特学"}
	Phone := []string{"18903830819", "13311527723", "13466613212", "18515212681", "15256073545", "18567720695", "18511911301"}
	who  := (time.Now().Unix() - 86400 * 4) / 86400 / 7 % int64(len(DUTY))
	callAPIAnomaly(endpoint, Phone[who])
}


func callAPIAnomaly(endpoint string, number string) {

	client, err := sdk.NewClientWithAccessKey("default", "LTAIbEOdCXFYrP98", "wNVf3zMK6dqwxvwp2oYsq9iTBYPXq1")
	if err != nil {
		panic(err)
	}

	request := requests.NewCommonRequest()
	request.Method = "POST"
	request.Scheme = "https" // https | http
	request.Domain = "dyvmsapi.aliyuncs.com"
	request.Version = "2017-05-25"
	request.ApiName = "SingleCallByTts"
	request.QueryParams["RegionId"] = "default"
	request.QueryParams["CalledShowNumber"] = "01086393840"
	request.QueryParams["CalledNumber"] = number
	request.QueryParams["TtsCode"] = "TTS_163525650"

	request.QueryParams["TtsParam"] = "{\"app\":\"http api接口"+endpoint+"访问超时\"}"

	_, err = client.ProcessCommonRequest(request)
	if err != nil {
		panic(err)
	}
}
