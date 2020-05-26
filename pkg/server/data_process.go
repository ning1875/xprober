package server

import (
	"sync"
	"time"
	"strings"
	"context"

	"github.com/go-kit/kit/log"
	"github.com/go-kit/kit/log/level"
	"github.com/prometheus/client_golang/prometheus"

	"xprober/pkg/pb"
	"xprober/pkg/common"
)

const (
	MetricCollectInterval      = 15 * time.Second
	TargetFlushManagerInterval = 60 * time.Second
	MetricOriginSeparator      = `_`
	MetricUniqueSeparator      = `#`
)

// rpc receive data
// update to local cache
// ticker data process
// expose prome http metric

var (
	IcmpDataMap         = sync.Map{}
	HttpDataMap         = sync.Map{}
	PingLatencyGaugeVec = prometheus.NewGaugeVec(prometheus.GaugeOpts{
		Name: common.MetricsNamePingLatency,
		Help: "Duration of ping prober ",
	}, []string{"source_region", "target_region"})
	PingPackageDropGaugeVec = prometheus.NewGaugeVec(prometheus.GaugeOpts{
		Name: common.MetricsNamePingPackageDrop,
		Help: "rate of ping packagedrop ",
	}, []string{"source_region", "target_region"})
	HttpInterFaceSuccessGaugeVec = prometheus.NewGaugeVec(prometheus.GaugeOpts{
		Name: common.MetricsNameHttpInterfaceSuccess,
		Help: "whether http probe success",
	}, []string{"source_region", "addr"})
	HttpHttpResolvedurationMillonsecondsGaugeVec = prometheus.NewGaugeVec(prometheus.GaugeOpts{
		Name: common.MetricsNameHttpResolvedurationMillonseconds,
		Help: "domain resole time",
	}, []string{"source_region", "addr"})
	HttpTlsDurationMillonsecondsGaugeVec = prometheus.NewGaugeVec(prometheus.GaugeOpts{
		Name: common.MetricsNameHttpTlsDurationMillonseconds,
		Help: "domain tls handshake time",
	}, []string{"source_region", "addr"})
	HttpConnectDurationMillonsecondsGaugeVec = prometheus.NewGaugeVec(prometheus.GaugeOpts{
		Name: common.MetricsNameHttpConnectDurationMillonseconds,
		Help: "http connect time",
	}, []string{"source_region", "addr"})
	HttpProcessingDurationMillonsecondsGaugeVec = prometheus.NewGaugeVec(prometheus.GaugeOpts{
		Name: common.MetricsNameHttpProcessingDurationMillonseconds,
		Help: "http process time",
	}, []string{"source_region", "addr"})
	HttpTransferDurationMillonsecondsGaugeVec = prometheus.NewGaugeVec(prometheus.GaugeOpts{
		Name: common.MetricsNameHttpTransferDurationMillonseconds,
		Help: "http transfer time",
	}, []string{"source_region", "addr"})
)

func NewMetrics() {

	prometheus.DefaultRegisterer.MustRegister(PingLatencyGaugeVec)
	prometheus.DefaultRegisterer.MustRegister(PingPackageDropGaugeVec)
	prometheus.DefaultRegisterer.MustRegister(HttpInterFaceSuccessGaugeVec)
	prometheus.DefaultRegisterer.MustRegister(HttpHttpResolvedurationMillonsecondsGaugeVec)
	prometheus.DefaultRegisterer.MustRegister(HttpTlsDurationMillonsecondsGaugeVec)
	prometheus.DefaultRegisterer.MustRegister(HttpConnectDurationMillonsecondsGaugeVec)
	prometheus.DefaultRegisterer.MustRegister(HttpProcessingDurationMillonsecondsGaugeVec)
	prometheus.DefaultRegisterer.MustRegister(HttpTransferDurationMillonsecondsGaugeVec)
}

func DataProcess(ctx context.Context, logger log.Logger) error {

	ticker := time.NewTicker(MetricCollectInterval)
	level.Info(logger).Log("msg", "DataProcessManager start....")
	defer ticker.Stop()
	for {
		select {
		case <-ticker.C:

			go IcmpDataProcess(logger)
			go HttpDataProcess(logger)

		case <-ctx.Done():
			level.Info(logger).Log("msg", "DataProcessManager exit....")
			return nil
		}
	}

	return nil
}

func HttpDataProcess(logger log.Logger) {
	level.Info(logger).Log("msg", "HttpDataProcess run....")
	var expireds []string
	resoleMap := make(map[string][]float64)
	processMap := make(map[string][]float64)
	transferMap := make(map[string][]float64)
	connMap := make(map[string][]float64)
	tlsMap := make(map[string][]float64)
	interSuccMap := make(map[string][]float64)

	f := func(k, v interface{}) bool {
		key := k.(string)
		va := v.(*pb.ProberResultOne)

		// check item expire
		now := time.Now().Unix()
		if now-va.TimeStamp > 300 {
			expireds = append(expireds, key)
		} else {
			if strings.Contains(va.MetricName, MetricOriginSeparator) {
				metricType := strings.Split(va.MetricName, MetricOriginSeparator)[1]
				uniqueKey := va.MetricName + MetricUniqueSeparator + va.SourceRegion + MetricUniqueSeparator + va.TargetAddr

				switch metricType {
				case "resolveDuration":
					old := resoleMap[uniqueKey]
					if len(old) == 0 {
						resoleMap[uniqueKey] = []float64{float64(va.Value)}
					} else {
						resoleMap[uniqueKey] = append(resoleMap[uniqueKey], float64(va.Value))
					}
				case "tlsDuration":
					old := tlsMap[uniqueKey]
					if len(old) == 0 {
						tlsMap[uniqueKey] = []float64{float64(va.Value)}
					} else {
						tlsMap[uniqueKey] = append(tlsMap[uniqueKey], float64(va.Value))
					}
				case "connectDuration":
					old := tlsMap[uniqueKey]
					if len(old) == 0 {
						connMap[uniqueKey] = []float64{float64(va.Value)}
					} else {
						connMap[uniqueKey] = append(connMap[uniqueKey], float64(va.Value))
					}
				case "processingDuration":
					old := tlsMap[uniqueKey]
					if len(old) == 0 {
						processMap[uniqueKey] = []float64{float64(va.Value)}
					} else {
						processMap[uniqueKey] = append(processMap[uniqueKey], float64(va.Value))
					}
				case "transferDuration":
					old := tlsMap[uniqueKey]
					if len(old) == 0 {
						transferMap[uniqueKey] = []float64{float64(va.Value)}
					} else {
						transferMap[uniqueKey] = append(transferMap[uniqueKey], float64(va.Value))
					}
				case "interface":
					old := tlsMap[uniqueKey]
					if len(old) == 0 {
						interSuccMap[uniqueKey] = []float64{float64(va.Value)}
					} else {
						interSuccMap[uniqueKey] = append(interSuccMap[uniqueKey], float64(va.Value))
					}

				}
			}

		}

		return true
	}

	HttpDataMap.Range(f)
	// delete  expireds
	for _, e := range expireds {
		HttpDataMap.Delete(e)
	}

	// compute data with avg or pct99
	dealWithDataMap(resoleMap, HttpHttpResolvedurationMillonsecondsGaugeVec, "http")
	dealWithDataMap(connMap, HttpConnectDurationMillonsecondsGaugeVec, "http")
	dealWithDataMap(tlsMap, HttpTlsDurationMillonsecondsGaugeVec, "http")
	dealWithDataMap(processMap, HttpProcessingDurationMillonsecondsGaugeVec, "http")
	dealWithDataMap(transferMap, HttpTransferDurationMillonsecondsGaugeVec, "http")
	dealWithDataMap(interSuccMap, HttpInterFaceSuccessGaugeVec, "http")
}

func IcmpDataProcess(logger log.Logger) {

	level.Info(logger).Log("msg", "IcmpDataProcess run....")

	var expireds []string

	latencyMap := make(map[string][]float64)
	packagedropMap := make(map[string][]float64)

	f := func(k, v interface{}) bool {
		key := k.(string)
		va := v.(*pb.ProberResultOne)

		// check item expire
		now := time.Now().Unix()
		if now-va.TimeStamp > 300 {
			expireds = append(expireds, key)
		} else {
			if strings.Contains(va.MetricName, MetricOriginSeparator) {
				metricType := strings.Split(va.MetricName, MetricOriginSeparator)[1]
				uniqueKey := va.MetricName + MetricUniqueSeparator + va.SourceRegion + MetricUniqueSeparator + va.TargetRegion

				switch metricType {
				case "latency":
					old := latencyMap[uniqueKey]
					if len(old) == 0 {
						latencyMap[uniqueKey] = []float64{float64(va.Value)}
					} else {
						latencyMap[uniqueKey] = append(latencyMap[uniqueKey], float64(va.Value))
					}
				case "packageDrop":
					old := packagedropMap[uniqueKey]
					if len(old) == 0 {
						packagedropMap[uniqueKey] = []float64{float64(va.Value)}
					} else {
						packagedropMap[uniqueKey] = append(packagedropMap[uniqueKey], float64(va.Value))
					}

				}
			}

		}

		return true
	}
	IcmpDataMap.Range(f)
	// delete  expireds
	for _, e := range expireds {
		IcmpDataMap.Delete(e)
	}

	// compute data with avg or pct99
	dealWithDataMap(latencyMap, PingLatencyGaugeVec, "icmp")
	dealWithDataMap(packagedropMap, PingPackageDropGaugeVec, "icmp")

}

func dealWithDataMap(dataM map[string][]float64, promeVec *prometheus.GaugeVec, pType string) {
	for uniqueKey, datas := range dataM {
		//MetricName := strings.Split(uniqueKey, MetricUniqueSeparator)[0]
		SourceRegion := strings.Split(uniqueKey, MetricUniqueSeparator)[1]
		TargetRegionOrAddr := strings.Split(uniqueKey, MetricUniqueSeparator)[2]
		var sum, avg float64
		num := len(datas)
		for _, ds := range datas {
			sum += ds
		}
		avg = sum / float64(num)
		switch pType {
		case "http":
			promeVec.With(prometheus.Labels{"source_region": SourceRegion, "addr": TargetRegionOrAddr}).Set(avg)
		case "icmp":
			promeVec.With(prometheus.Labels{"source_region": SourceRegion, "target_region": TargetRegionOrAddr}).Set(avg)
		}

	}
}
