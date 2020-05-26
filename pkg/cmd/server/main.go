package main

import (
	"os"
	"os/signal"
	"syscall"
	"context"
	"net/http"

	"github.com/oklog/run"
	"gopkg.in/alecthomas/kingpin.v2"
	"github.com/go-kit/kit/log/level"
	"github.com/prometheus/common/version"
	"github.com/prometheus/common/promlog"
	"github.com/prometheus/common/promlog/flag"
	"github.com/prometheus/client_golang/prometheus/promhttp"

	rc "xprober/pkg/server"
)

func main() {

	var (
		//app = kingpin.New(filepath.Base(os.Args[0]), "The Xprober")
		configFile = kingpin.Flag("config.file", "xprober configuration file path.").Default("xprober.yml").String()
		//grpcListenAddress = kingpin.Flag("grpc.listen-address", "Address to listen on for the grpc interface, API, and telemetry.").Default(":6001").String()
		//webListenAddr     = kingpin.Flag("web.listen-address", "Address to listen on for the web interface, API, and telemetry.").Default(":6002").String()
	)

	// init logger
	promlogConfig := &promlog.Config{}
	flag.AddFlags(kingpin.CommandLine, promlogConfig)
	kingpin.Version(version.Print("xprober"))
	kingpin.HelpFlag.Short('h')
	kingpin.Parse()
	logger := promlog.New(promlogConfig)

	// new grpc manager
	ctxAll, cancelAll := context.WithCancel(context.Background())

	sConfig, _ := rc.LoadFile(*configFile, logger)
	grpcListenAddress := sConfig.RpcListenAddr

	webListenAddr := sConfig.MetricsListenAddr

	rc.NewManagager(logger, grpcListenAddress)

	// new prome register
	rc.NewMetrics()

	// new target pool manager
	tfm := rc.NewTargetFlushManager(logger, *configFile)

	var g run.Group
	{
		// Termination handler.
		term := make(chan os.Signal, 1)
		signal.Notify(term, os.Interrupt, syscall.SIGTERM)
		cancel := make(chan struct{})
		g.Add(

			func() error {
				select {
				case <-term:
					level.Warn(logger).Log("msg", "Received SIGTERM, exiting gracefully...")
					cancelAll()
					return nil
					//TODO clean work here
				case <-cancel:
					level.Warn(logger).Log("msg", "server finally exit...")
					return nil
				}
			},
			func(err error) {
				close(cancel)
			},
		)
	}

	{
		// grpc server  manager.
		g.Add(func() error {
			err := rc.GRM.Run(ctxAll, logger)
			if err != nil {
				level.Error(logger).Log("msg", "grpc server manager stopped")
			}

			return err
		}, func(err error) {
			rc.GRM.Server.GracefulStop()

		})
	}

	{
		// metrics http handler.
		g.Add(func() error {
			http.Handle("/metrics", promhttp.Handler())
			srv := http.Server{Addr: webListenAddr}
			level.Info(logger).Log("msg", "Listening on address", "address", webListenAddr)
			errchan := make(chan error)

			go func() {
				errchan <- srv.ListenAndServe()
			}()
			select {
			case err := <-errchan:
				level.Error(logger).Log("msg", "Error starting HTTP server", "err", err)
				return err
			case <-ctxAll.Done():
				level.Info(logger).Log("msg", "Web service Exit..")
				return nil

			}

		}, func(err error) {
			cancelAll()
		})
	}

	{
		// data proceess.
		g.Add(func() error {
			err := rc.DataProcess(ctxAll, logger)
			return err
		}, func(err error) {
			cancelAll()
		})
	}

	{
		// target flush manager
		g.Add(func() error {
			err := tfm.Run(ctxAll)
			return err
		}, func(err error) {
			cancelAll()
		})
	}

	g.Run()
}
