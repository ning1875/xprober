package agent

import (
	"sync"
	"time"

	"github.com/go-kit/kit/log"
	"github.com/go-kit/kit/log/level"

	"xprober/pkg/pb"
)

var (
	TargetCache        = sync.Map{}
	PbResMap           = sync.Map{}
	Probers            map[string]ProbeFn
	LTM                *LocalTargetManger
	ProberFuncInterval = 15 * time.Second
	TargetUpdateChan   = make(chan *pb.ProberTargetsGetResponse)
)

//type ProbeFn func(ctx context.Context, lt *LocalTarget, logger log.Logger) pb.ProberResultOne
type ProbeFn func(lt *LocalTarget) []*pb.ProberResultOne

type LocalTargetManger struct {
	logger log.Logger
	mux    sync.RWMutex
	Map    map[string]*LocalTarget
}

func (ltm *LocalTargetManger) GetMapKeys() []string {
	//ltm.mux.RLock()
	//defer ltm.mux.RUnlock()
	count := len(ltm.Map)
	keys := make([]string, count)
	i := 0
	for hostname := range ltm.Map {
		keys[i] = hostname
		i++
	}
	return keys
}

func NewLocalTargetManger(logger log.Logger) {
	localM := make(map[string]*LocalTarget)
	LTM = &LocalTargetManger{}
	LTM.logger = logger
	LTM.Map = localM
}

type LocalTarget struct {
	logger       log.Logger
	Addr         string
	SourceRegion string
	TargetRegion string

	ProbeType string
	Prober    ProbeFn
	QuitChan  chan struct{}
}

func PushWork(logger log.Logger) {

	for {
		pushPbResults(logger)

		time.Sleep(PushInterval)
	}
}
func ReportIp(logger log.Logger) {

	for {
		reportAgentIp(logger)

		time.Sleep(ReportInterval)
	}

}

func RefreshTarget(logger log.Logger) {
	go doRefreshWork(logger)
	level.Info(logger).Log("msg", "RefreshTarget start", )
	for {

		getProberTarget(logger)
		time.Sleep(RefreshInterval)
	}

}

func doRefreshWork(logger log.Logger) {
	for {
		select {
		case tgs := <-TargetUpdateChan:
			// refresh local map

			LTM.mux.Lock()
			defer LTM.mux.Unlock()
			remoteTargetIds := make(map[string]bool)

			localIds := LTM.GetMapKeys()
			for _, t := range tgs.Targets {
				pbFunc, funcExists := Probers[t.ProberType]
				if funcExists == false {
					continue
				}

				for _, addr := range t.Target {
					thisId := t.Region + addr + t.ProberType
					remoteTargetIds[thisId] = true
					if _, ok := LTM.Map[thisId]; ok {
						continue
					}

					nt := &LocalTarget{
						logger:       logger,
						Addr:         addr,
						SourceRegion: LocalRegion,
						TargetRegion: t.Region,
						ProbeType:    t.ProberType,
						Prober:       pbFunc,
						QuitChan:     make(chan struct{}),
					}
					LTM.Map[thisId] = nt
					go nt.Start()

				}

			}
			// stop old
			for _, key := range localIds {
				if _, found := remoteTargetIds[key]; !found {
					LTM.Map[key].Stop()
					delete(LTM.Map, key)
				}
			}

		}
	}
}

func Init(logger log.Logger) {
	Probers = map[string]ProbeFn{
		"http": ProbeHTTP,
		"icmp": ProbeICMP,
		//"icmp": ProbeHTTP,
	}
	NewLocalTargetManger(logger)
}

func (lt *LocalTarget) Uid() string {

	return lt.TargetRegion + lt.Addr + lt.ProbeType
}

func (lt *LocalTarget) Start() {
	ticker := time.NewTicker(ProberFuncInterval)
	level.Info(lt.logger).Log("msg", "LocalTarget probe start....", "uid", lt.Uid())
	defer ticker.Stop()
	for {
		select {
		case <-lt.QuitChan:
			level.Info(lt.logger).Log("msg", "receive_quit_signal", "uid", lt.Uid())
			return
		case <-ticker.C:
			res := lt.Prober(lt)
			if len(res) > 0 {
				PbResMap.Store(lt.Uid(), res)
			}

		}

	}

}

func (lt *LocalTarget) Stop() {
	close(lt.QuitChan)
}
