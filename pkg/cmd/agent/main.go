package main

import (
	"os"
	"syscall"
	"os/signal"

	"gopkg.in/alecthomas/kingpin.v2"
	"github.com/go-kit/kit/log/level"
	"github.com/prometheus/common/promlog"
	"github.com/prometheus/common/promlog/flag"
	"github.com/prometheus/common/version"

	"xprober/pkg/agent"
)

var (
	grpcServerAddress = kingpin.Flag("grpc.server-address", "server addr").Default(":6001").String()
)

func main() {

	// init logger
	promlogConfig := &promlog.Config{}
	flag.AddFlags(kingpin.CommandLine, promlogConfig)
	kingpin.Version(version.Print("xprober-agent"))
	kingpin.HelpFlag.Short('h')
	kingpin.Parse()
	logger := promlog.New(promlogConfig)

	//init local region and get ip
	if regionSucc := agent.GetLocalRegionByEc2(logger); regionSucc == false {
		level.Error(logger).Log("msg", "failed_to_get_region_exit...")
		return
	}

	if ipSucc := agent.GetLocalIp(logger); ipSucc == false {
		level.Error(logger).Log("msg", "failed_to_get_ip_exit...")
		return
	}
	level.Info(logger).Log("msg", "agent_metadata", "ip", agent.LocalIp, "region", agent.LocalRegion)
	// init rpc pool
	//ctx, cancelAll := context.WithCancel(context.Background())
	isSuccess := agent.InitRpcPool(*grpcServerAddress, logger)
	if isSuccess == false {
		level.Error(logger).Log("msg", "init_rpc_pool_failed_and_exit")
		os.Exit(1)
	}
	level.Info(logger).Log("msg", "init_rpc_pool_success")
	// report ip

	go agent.ReportIp(logger)
	// refresh target
	agent.Init(logger)
	go agent.RefreshTarget(logger)
	go agent.PushWork(logger)

	// term handler
	term := make(chan os.Signal, 1)
	signal.Notify(term, os.Interrupt, syscall.SIGTERM)
	for {
		select {
		case <-term:
			level.Info(logger).Log("msg", "Received SIGTERM, exiting gracefully...")
			agent.GrpcPool.Close()
			return
		}
	}

}
