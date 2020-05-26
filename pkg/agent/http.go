// Copyright 2016 The Prometheus Authors
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
// http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package agent

import (
	"context"
	"io"
	"net"
	"net/http"
	"net/http/cookiejar"
	"net/http/httptrace"
	"net/url"
	"strings"
	"sync"
	"time"

	"github.com/go-kit/kit/log"
	"github.com/go-kit/kit/log/level"
	pconfig "github.com/prometheus/common/config"
	"golang.org/x/net/publicsuffix"

	"xprober/pkg/pb"
	"xprober/pkg/common"
)

// roundTripTrace holds timings for a single HTTP roundtrip.
type roundTripTrace struct {
	tls           bool
	start         time.Time
	dnsDone       time.Time
	connectDone   time.Time
	gotConn       time.Time
	responseStart time.Time
	end           time.Time
}

// transport is a custom transport keeping traces for each HTTP roundtrip.
type transport struct {
	Transport             http.RoundTripper
	NoServerNameTransport http.RoundTripper
	firstHost             string
	logger                log.Logger

	mu      sync.Mutex
	traces  []*roundTripTrace
	current *roundTripTrace
}

func newTransport(rt, noServerName http.RoundTripper, logger log.Logger) *transport {
	return &transport{
		Transport:             rt,
		NoServerNameTransport: noServerName,
		logger:                logger,
		traces:                []*roundTripTrace{},
	}
}

// RoundTrip switches to a new trace, then runs embedded RoundTripper.
func (t *transport) RoundTrip(req *http.Request) (*http.Response, error) {
	level.Info(t.logger).Log("msg", "Making HTTP request", "url", req.URL.String(), "host", req.Host)

	trace := &roundTripTrace{}
	if req.URL.Scheme == "https" {
		trace.tls = true
	}
	t.current = trace
	t.traces = append(t.traces, trace)

	if t.firstHost == "" {
		t.firstHost = req.URL.Host
	}

	if t.firstHost != req.URL.Host {
		// This is a redirect to something other than the initial host,
		// so TLS ServerName should not be set.
		level.Info(t.logger).Log("msg", "Address does not match first address, not sending TLS ServerName", "first", t.firstHost, "address", req.URL.Host)
		return t.NoServerNameTransport.RoundTrip(req)
	}

	return t.Transport.RoundTrip(req)
}

func (t *transport) DNSStart(_ httptrace.DNSStartInfo) {
	t.mu.Lock()
	defer t.mu.Unlock()
	t.current.start = time.Now()
}
func (t *transport) DNSDone(_ httptrace.DNSDoneInfo) {
	t.mu.Lock()
	defer t.mu.Unlock()
	t.current.dnsDone = time.Now()
}
func (ts *transport) ConnectStart(_, _ string) {
	ts.mu.Lock()
	defer ts.mu.Unlock()
	t := ts.current
	// No DNS resolution because we connected to IP directly.
	if t.dnsDone.IsZero() {
		t.start = time.Now()
		t.dnsDone = t.start
	}
}
func (t *transport) ConnectDone(net, addr string, err error) {
	t.mu.Lock()
	defer t.mu.Unlock()
	t.current.connectDone = time.Now()
}
func (t *transport) GotConn(_ httptrace.GotConnInfo) {
	t.mu.Lock()
	defer t.mu.Unlock()
	t.current.gotConn = time.Now()
}
func (t *transport) GotFirstResponseByte() {
	t.mu.Lock()
	defer t.mu.Unlock()
	t.current.responseStart = time.Now()
}
func chooseProtocol(ctx context.Context, IPProtocol string, target string, logger log.Logger) (ip *net.IPAddr, lookupTime float64, err error) {

	if IPProtocol == "ip6" || IPProtocol == "" {
		IPProtocol = "ip6"
	} else {
		IPProtocol = "ip4"
	}

	level.Info(logger).Log("msg", "Resolving target address", "ip_protocol", IPProtocol)
	resolveStart := time.Now()
	defer func() {
		lookupTime = time.Since(resolveStart).Seconds()
	}()
	resolver := &net.Resolver{}
	ips, err := resolver.LookupIPAddr(ctx, target)
	if err != nil {
		level.Error(logger).Log("msg", "Resolution with IP protocol failed", "err", err)
		return nil, 0.0, err
	}

	// Return the IP in the requested protocol.
	var fallback *net.IPAddr
	for _, ip := range ips {
		switch IPProtocol {
		case "ip4":
			if ip.IP.To4() != nil {
				level.Info(logger).Log("msg", "Resolved target address", "ip", ip.String())
				return &ip, lookupTime, nil
			}

			// ip4 as fallback
			fallback = &ip

		case "ip6":
			if ip.IP.To4() == nil {
				level.Info(logger).Log("msg", "Resolved target address", "ip", ip.String())
				return &ip, lookupTime, nil
			}

			// ip6 as fallback
			fallback = &ip
		}
	}

	// Use fallback ip protocol.
	level.Info(logger).Log("msg", "Resolved target address", "ip", fallback.String())
	lookupTime = time.Since(resolveStart).Seconds()
	return fallback, lookupTime, nil
}

func ProbeHTTP(lt *LocalTarget) ([]*pb.ProberResultOne) {
	defer func() {
		if r := recover(); r != nil {
			resultErr, _ := r.(error)
			level.Error(lt.logger).Log("msg", "ProbeHTTP panic ...", "resultErr", resultErr)

		}
	}()

	logger := lt.logger


	ctx, cancelAll := context.WithCancel(context.Background())
	defer cancelAll()
	var (
		target string
	)
	prs := make([]*pb.ProberResultOne, 0)

	// http Success

	pSucc := pb.ProberResultOne{
		MetricName:   common.MetricsNameHttpInterfaceSuccess,
		WorkerName:   LocalIp,
		TargetAddr:   lt.Addr,
		SourceRegion: LocalRegion,
		TargetRegion: lt.TargetRegion,
		ProbeType:    lt.ProbeType,
		TimeStamp:    time.Now().Unix(),
		Value:        0,
	}

	pTls := pb.ProberResultOne{
		MetricName:   common.MetricsNameHttpTlsDurationMillonseconds,
		WorkerName:   LocalIp,
		TargetAddr:   lt.Addr,
		SourceRegion: LocalRegion,
		TargetRegion: lt.TargetRegion,
		ProbeType:    lt.ProbeType,
		TimeStamp:    time.Now().Unix(),
	}

	pConn := pb.ProberResultOne{
		MetricName:   common.MetricsNameHttpConnectDurationMillonseconds,
		WorkerName:   LocalIp,
		TargetAddr:   lt.Addr,
		SourceRegion: LocalRegion,
		TargetRegion: lt.TargetRegion,
		ProbeType:    lt.ProbeType,
		TimeStamp:    time.Now().Unix(),
	}

	pProc := pb.ProberResultOne{
		MetricName:   common.MetricsNameHttpProcessingDurationMillonseconds,
		WorkerName:   LocalIp,
		TargetAddr:   lt.Addr,
		SourceRegion: LocalRegion,
		TargetRegion: lt.TargetRegion,
		ProbeType:    lt.ProbeType,
		TimeStamp:    time.Now().Unix(),
	}

	pTran := pb.ProberResultOne{
		MetricName:   common.MetricsNameHttpTransferDurationMillonseconds,
		WorkerName:   LocalIp,
		TargetAddr:   lt.Addr,
		SourceRegion: LocalRegion,
		TargetRegion: lt.TargetRegion,
		ProbeType:    lt.ProbeType,
		TimeStamp:    time.Now().Unix(),
	}
	target = lt.Addr
	if !strings.HasPrefix(lt.Addr, "http://") && !strings.HasPrefix(lt.Addr, "https://") {
		target = "http://" + lt.Addr

	}

	targetURL, err := url.Parse(target)
	if err != nil {
		level.Error(lt.logger).Log("msg", "Could not parse target URL", "target", target, "err", err)
		prs = append(prs, &pSucc)
		return prs
	}
	targetHost, targetPort, err := net.SplitHostPort(targetURL.Host)
	// If split fails, assuming it's a hostname without port part.
	if err != nil {
		targetHost = targetURL.Host
	}
	// TODO dns lookup time

	ip, lookupTime, err := chooseProtocol(ctx, "ip4", targetHost, logger)

	if err != nil {
		level.Error(logger).Log("msg", "Error resolving address", "target", target, "err", err)
		prs = append(prs, &pSucc)
		return prs
	}
	// http resolve

	prlt := pb.ProberResultOne{
		MetricName:   common.MetricsNameHttpResolvedurationMillonseconds,
		WorkerName:   LocalIp,
		TargetAddr:   lt.Addr,
		SourceRegion: LocalRegion,
		TargetRegion: lt.TargetRegion,
		ProbeType:    lt.ProbeType,
		TimeStamp:    time.Now().Unix(),
	}
	prlt.Value = float32(lookupTime * 1000)
	prs = append(prs, &prlt)

	httpClientConfig := pconfig.HTTPClientConfig{}
	httpClientConfig.TLSConfig.ServerName = targetHost
	client, err := pconfig.NewClientFromConfig(httpClientConfig, "http_probe", true)
	if err != nil {
		level.Error(logger).Log("msg", "Error generating HTTP client", "target", target, "err", err)
		prs = append(prs, &pSucc)
		return prs
	}

	httpClientConfig.TLSConfig.ServerName = ""
	noServerName, err := pconfig.NewRoundTripperFromConfig(httpClientConfig, "http_probe", true)
	if err != nil {
		level.Error(logger).Log("msg", "Error generating HTTP client without ServerName", "target", target, "err", err)
		prs = append(prs, &pSucc)
		return prs
	}

	jar, err := cookiejar.New(&cookiejar.Options{PublicSuffixList: publicsuffix.List})
	if err != nil {
		level.Error(logger).Log("msg", "Error generating cookiejar", "target", target, "err", err)
		prs = append(prs, &pSucc)
		return prs
	}
	client.Jar = jar

	// Inject transport that tracks traces for each redirect,
	// and does not set TLS ServerNames on redirect if needed.
	tt := newTransport(client.Transport, noServerName, logger)
	client.Transport = tt

	// Replace the host field in the URL with the IP we resolved.
	origHost := targetURL.Host
	if targetPort == "" {
		if strings.Contains(ip.String(), ":") {
			targetURL.Host = "[" + ip.String() + "]"
		} else {
			targetURL.Host = ip.String()
		}
	} else {
		targetURL.Host = net.JoinHostPort(ip.String(), targetPort)
	}

	var body io.Reader

	// If a body is configured, add it to the request.

	request, err := http.NewRequest("GET", targetURL.String(), body)
	request.Host = origHost
	request = request.WithContext(ctx)
	if err != nil {
		level.Error(logger).Log("msg", "Error creating request", "target", target, "err", err)
		prs = append(prs, &pSucc)
		return prs
	}

	trace := &httptrace.ClientTrace{
		DNSStart:             tt.DNSStart,
		DNSDone:              tt.DNSDone,
		ConnectStart:         tt.ConnectStart,
		ConnectDone:          tt.ConnectDone,
		GotConn:              tt.GotConn,
		GotFirstResponseByte: tt.GotFirstResponseByte,
	}
	request = request.WithContext(httptrace.WithClientTrace(request.Context(), trace))

	resp, err := client.Do(request)
	// Err won't be nil if redirects were turned off. See https://github.com/golang/go/issues/3795
	if err != nil && resp == nil {
		level.Error(logger).Log("msg", "Error for HTTP request", "target", target, "err", err)
		prs = append(prs, &pSucc)
		return prs
	}

	level.Info(logger).Log("msg", "Received HTTP response", "target", target, "status_code", resp.StatusCode)
	if resp.StatusCode > 300 {
		prs = append(prs, &pSucc)
		return prs

	}

	// At this point body is fully read and we can write end time.
	tt.current.end = time.Now()

	if resp == nil {
		resp = &http.Response{}
	}
	tt.mu.Lock()
	defer tt.mu.Unlock()
	for i, trace := range tt.traces {
		level.Debug(logger).Log(
			"msg", "Response timings for roundtrip",
			"roundtrip", i,
			"start", trace.start,
			"dnsDone", trace.dnsDone,
			"connectDone", trace.connectDone,
			"gotConn", trace.gotConn,
			"responseStart", trace.responseStart,
			"end", trace.end,
		)
		// We get the duration for the first request from chooseProtocol.
		if i != 0 {
			prlt.Value += float32(trace.dnsDone.Sub(trace.start).Seconds() * 1000)
		}
		// Continue here if we never got a connection because a request failed.
		if trace.gotConn.IsZero() {
			continue
		}
		if trace.tls {
			// dnsDone must be set if gotConn was set.
			pConn.Value = float32(trace.connectDone.Sub(trace.dnsDone).Seconds() * 1000)
			pTls.Value = float32(trace.gotConn.Sub(trace.dnsDone).Seconds() * 1000)
		} else {
			pConn.Value = float32(trace.gotConn.Sub(trace.dnsDone).Seconds() * 1000)
		}

		// Continue here if we never got a response from the server.
		if trace.responseStart.IsZero() {
			continue
		}
		pProc.Value = float32(trace.responseStart.Sub(trace.gotConn).Seconds() * 1000)

		// Continue here if we never read the full response from the server.
		// Usually this means that request either failed or was redirected.
		if trace.end.IsZero() {
			continue
		}
		pTran.Value = float32(trace.end.Sub(trace.responseStart).Seconds() * 1000)
	}
	pSucc.Value = 1
	prs = append(prs, &pSucc)
	prs = append(prs, &pTls)
	prs = append(prs, &pConn)
	prs = append(prs, &pProc)
	prs = append(prs, &pTran)
	return prs
}
