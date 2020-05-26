package agent

import (
	"fmt"
	"time"
	"bytes"
	"strings"
	"strconv"
	"math/rand"
	"os/exec"

	"github.com/go-kit/kit/log/level"
	"github.com/go-kit/kit/log"

	"xprober/pkg/pb"
	"xprober/pkg/common"
)

func execCmd(cmdStr string, logger log.Logger) (success bool, outStr string) {
	cmd := exec.Command("/bin/bash", "-c", cmdStr)
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	err := cmd.Run()
	if err != nil {
		level.Error(logger).Log("execCmdMsg", err, "cmd", cmdStr)
		return false, string(stderr.Bytes())
	}
	outStr = string(stdout.Bytes())
	return true, outStr

}

func randFloats(min, max float64, ) float64 {

	res := min + rand.Float64()*(max-min)

	return res
}

func ProbeICMPMock(lt *LocalTarget) ([]*pb.ProberResultOne) {
	// mock data
	level.Info(lt.logger).Log("msg", "LocalTarget  ProbeICMP start ...", "uid", lt.Uid())
	pingMetrics := []string{
		`ping_latency_millonseconds`,
		`ping_packagedrop_rate`,
	}
	var prs []*pb.ProberResultOne
	for _, pm := range pingMetrics {
		var ro pb.ProberResultOne
		ro.TimeStamp = time.Now().Unix()
		ro.TargetAddr = lt.Addr
		ro.WorkerName = lt.SourceRegion
		ro.MetricName = pm
		ro.SourceRegion = lt.SourceRegion
		ro.TargetRegion = lt.TargetRegion
		ro.ProbeType = lt.ProbeType
		rand.Seed(time.Now().UnixNano())

		ro.Value = float32(randFloats(1.10, 1001.98))
		prs = append(prs, &ro)
	}

	return prs
}

func ProbeICMP(lt *LocalTarget) ([]*pb.ProberResultOne) {

	defer func() {
		if r := recover(); r != nil {
			resultErr, _ := r.(error)
			level.Error(lt.logger).Log("msg", "ProbeICMP panic ...", "resultErr", resultErr)

		}
	}()

	pingCmd := fmt.Sprintf("/usr/bin/timeout --signal=KILL 15s  /usr/bin/ping -q -A -f -s 100 -W 1000 -c 50 %s", lt.Addr)
	level.Info(lt.logger).Log("msg", "LocalTarget  ProbeICMP start ...", "uid", lt.Uid(), "pingcmd", pingCmd)
	success, outPutStr := execCmd(pingCmd, lt.logger)
	prs := make([]*pb.ProberResultOne, 0)
	if success == false {
		level.Error(lt.logger).Log("msg", "ProbeICMP failed ...", "uid", lt.Uid(), "err_str", outPutStr)
		return prs
	}
	var pkgdLine string
	var latenLinke string
	for _, line := range (strings.Split(outPutStr, "\n")) {
		if strings.Contains(line, "packets transmitted") {
			pkgdLine = line
			continue
		}
		if strings.Contains(line, "min/avg/max/mdev") {
			latenLinke = line
			continue
		}

	}
	pkgRate := strings.Split(pkgdLine, " ")[5]
	pkgRate = strings.Replace(pkgRate, "%", "", -1)
	pkgRateNum, _ := strconv.ParseFloat(pkgRate, 64)
	pingEwmas := strings.Split(latenLinke, " ")

	pingEwma := pingEwmas[len(pingEwmas)-2]
	pingEwma = strings.Split(pingEwma, "/")[1]
	pingEwmaNum, _ := strconv.ParseFloat(pingEwma, 64)

	level.Debug(lt.logger).Log("msg", "ProbeICMP_one_res", "pingcmd", pingCmd, "outPutStr", outPutStr, "pkgRateNum", float32(pkgRateNum), "pingEwmaNum", float32(pingEwmaNum))
	prDr := pb.ProberResultOne{
		MetricName:   common.MetricsNamePingPackageDrop,
		WorkerName:   LocalIp,
		TargetAddr:   lt.Addr,
		SourceRegion: LocalRegion,
		TargetRegion: lt.TargetRegion,
		ProbeType:    lt.ProbeType,
		TimeStamp:    time.Now().Unix(),
		Value:        float32(pkgRateNum),
	}

	prLaten := pb.ProberResultOne{
		MetricName:   common.MetricsNamePingLatency,
		WorkerName:   LocalIp,
		TargetAddr:   lt.Addr,
		SourceRegion: LocalRegion,
		TargetRegion: lt.TargetRegion,
		ProbeType:    lt.ProbeType,
		TimeStamp:    time.Now().Unix(),
		Value:        float32(pingEwmaNum),
	}
	prs = append(prs, &prDr)
	prs = append(prs, &prLaten)
	//level.Info(lt.logger).Log("msg", "ping_res_prDr", "ts", prDr.TimeStamp, "value", prDr.Value)
	//level.Info(lt.logger).Log("msg", "ping_res_prLaten", "ts", prLaten.TimeStamp, "value", prLaten.Value)
	return prs
}


