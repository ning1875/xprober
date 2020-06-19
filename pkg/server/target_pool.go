package server

import (
	"sync"
	"time"
	"context"

	"github.com/go-kit/kit/log"
	"github.com/go-kit/kit/log/level"

	"xprober/pkg/pb"
	"fmt"
)

type TargetPool struct {
	ProbeType     string
	regionTargets sync.Map
}

var (
	IcmpRegionProberMap  = sync.Map{}
	OtherRegionProberMap = sync.Map{}
	AgentIpRegionMap     = sync.Map{}
)

type TargetFlushManager struct {
	Logger     log.Logger
	ConfigFile string
}

func rangeIcmpMap() {
	f := func(k, v interface{}) bool {
		region := k.(string)
		data := v.(*pb.Targets)
		fmt.Println("rangeIcmpMap", region, data)
		return true
	}

	IcmpRegionProberMap.Range(f)
}

func (t *TargetFlushManager) flushAgentIpIntoGlobalMap() {
	level.Info(t.Logger).Log("msg", "flushAgentIpIntoGlobalMap run....")
	tmpM := make(map[string][]string)

	f := func(k, v interface{}) bool {
		ip := k.(string)
		region := v.(string)
		tmpM[region] = append(tmpM[region], ip)

		return true
	}
	AgentIpRegionMap.Range(f)
	for region, ips := range tmpM {
		tNew := &pb.Targets{}
		tNew.Region = region
		tNew.ProberType = "icmp"
		tNew.Target = ips

		preData, loaded := IcmpRegionProberMap.LoadOrStore(region, tNew)
		preDataN := preData.(*pb.Targets)
		if loaded {
			thisT := tNew.Target
			originT := preDataN.Target
			thisTM := make(map[string]string)
			for _, tt := range thisT {
				thisTM[tt] = tt
			}
			for _, tt := range originT {
				if _, exists := thisTM[tt]; exists == false {
					thisT = append(thisT, tt)
				}
			}
			a := &pb.Targets{
				Region:     region,
				ProberType: "icmp",
				Target:     thisT,
			}
			IcmpRegionProberMap.Store(region, a)
		}

	}
	//rangeIcmpMap()

}

func NewTargetFlushManager(logger log.Logger, configFile string) *TargetFlushManager {

	return &TargetFlushManager{Logger: logger, ConfigFile: configFile}
}
func (t *TargetFlushManager) Run(ctx context.Context) error {

	ticker := time.NewTicker(TargetFlushManagerInterval)
	level.Info(t.Logger).Log("msg", "TargetFlushManager start....")
	t.refresh()
	defer ticker.Stop()
	for {
		select {
		case <-ticker.C:
			t.refresh()

		case <-ctx.Done():
			level.Info(t.Logger).Log("msg", "TargetFlushManager exit....")
			return nil
		}
	}

	return nil
}

func (t *TargetFlushManager) refreshFromConfigFile() {
	level.Info(t.Logger).Log("msg", "refreshFromConfigFile run....")

	config, _ := LoadFile(t.ConfigFile, t.Logger)
	otmpM := make(map[string][]*pb.Targets)
	icmpM := make(map[string]*pb.Targets)
	if len(config.ProberTargets) <= 0 {
		level.Info(t.Logger).Log("msg", "refreshFromConfigFile empty targets....")
		return
	}
	for _, t := range config.ProberTargets {
		tNew := &pb.Targets{}
		tNew.Region = t.Region
		tNew.ProberType = t.ProberType
		tNew.Target = t.Target
		otmpM[tNew.Region] = append(otmpM[tNew.Region], tNew)
		switch t.ProberType {
		case "icmp":
			icmpM[tNew.Region] = tNew
		default:
			otmpM[tNew.Region] = append(otmpM[tNew.Region], tNew)
		}

	}
	for k, v := range icmpM {
		preData, loaded := IcmpRegionProberMap.LoadOrStore(k, v)
		preDataN := preData.(*pb.Targets)
		if loaded {
			thisT := v.Target
			originT := preDataN.Target
			thisTM := make(map[string]string)
			for _, tt := range thisT {
				thisTM[tt] = tt
			}
			for _, tt := range originT {
				if _, exists := thisTM[tt]; exists == false {
					thisT = append(thisT, tt)
				}
			}
			a := &pb.Targets{
				Region:     k,
				ProberType: "icmp",
				Target:     thisT,
			}
			IcmpRegionProberMap.Store(k, a)
		}

	}

	for k, v := range otmpM {

		OtherRegionProberMap.Store(k, v)
	}
}

func (t *TargetFlushManager) refresh() {
	go t.refreshFromConfigFile()
	go t.flushAgentIpIntoGlobalMap()
}

func GetTargetsByRegion(sourceRegion string) (res []*pb.Targets) {

	f := func(k, v interface{}) bool {
		//key := k.(string)
		va := v.([]*pb.Targets)
		//if key != sourceRegion {
		res = append(res, va...)

		//}
		return true
	}
	fi := func(k, v interface{}) bool {
		key := k.(string)
		va := v.(*pb.Targets)
		if key != sourceRegion {
			res = append(res, va)

		}
		return true
	}

	IcmpRegionProberMap.Range(fi)
	OtherRegionProberMap.Range(f)
	return
}
