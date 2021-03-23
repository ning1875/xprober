package main

import (
	"context"

	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"syscall"
	"time"

	"github.com/go-kit/kit/log"
	"github.com/go-kit/kit/log/level"
	"github.com/oklog/run"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	"github.com/prometheus/common/promlog"
	promlogflag "github.com/prometheus/common/promlog/flag"
	"github.com/prometheus/common/version"
	"gopkg.in/alecthomas/kingpin.v2"

	rc "xprober/pkg/server"
)

func main() {

	var (
		app        = kingpin.New(filepath.Base(os.Args[0]), "The xprober-server")
		configFile = app.Flag("config.file", "xprober configuration file path.").Default("xprober.yml").String()
	)

	promlogConfig := promlog.Config{}

	app.Version(version.Print("xprober-server"))
	app.HelpFlag.Short('h')
	promlogflag.AddFlags(app, &promlogConfig)
	kingpin.MustParse(app.Parse(os.Args[1:]))

	var logger log.Logger
	logger = func(config *promlog.Config) log.Logger {
		var (
			l  log.Logger
			le level.Option
		)
		if config.Format.String() == "logfmt" {
			l = log.NewLogfmtLogger(log.NewSyncWriter(os.Stderr))
		} else {
			l = log.NewJSONLogger(log.NewSyncWriter(os.Stderr))
		}

		switch config.Level.String() {
		case "debug":
			le = level.AllowDebug()
		case "info":
			le = level.AllowInfo()
		case "warn":
			le = level.AllowWarn()
		case "error":
			le = level.AllowError()
		}
		l = level.NewFilter(l, le)
		l = log.With(l, "ts", log.TimestampFormat(
			func() time.Time { return time.Now().Local() },
			"2006-01-02T15:04:05.000Z07:00",
		), "caller", log.DefaultCaller)
		return l
	}(&promlogConfig)

	// new grpc manager
	ctxAll, cancelAll := context.WithCancel(context.Background())

	sConfig, err := rc.LoadFile(*configFile, logger)
	if err != nil {
		level.Error(logger).Log("msg", "load_config_file_error", "err", err)
		return
	}
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
