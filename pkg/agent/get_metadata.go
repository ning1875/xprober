package agent

import (
	"net"
	"net/http"
	"io/ioutil"
	"github.com/go-kit/kit/log"
	"github.com/go-kit/kit/log/level"
)

var (
	LocalRegion string
	LocalIp     string
)
// TODO get real id func

func GetLocalRegionByEc2(logger log.Logger) bool {
	addr := "http://169.254.169.254/latest/meta-data/placement/availability-zone"
	req, err := http.NewRequest("GET", addr, nil)
	var resp *http.Response
	resp, err = http.DefaultClient.Do(req)
	if err != nil {
		level.Error(logger).Log("msg", "GetLocalRegionByEc2_http_error1", "error", err)
		return false
	}
	if resp.StatusCode != http.StatusOK {
		level.Error(logger).Log("msg", "GetLocalRegionByEc2_http_error_rc_ne_200", "resp.StatusCode", resp.StatusCode)
		return false
	}

	respBytes, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		level.Error(logger).Log("msg", "GetLocalRegionByEc2_http_error_read_response_body", "err", err)
		return false
	}
	defer resp.Body.Close()
	dataStr := string(respBytes)
	region := dataStr[:len(dataStr)-1]
	LocalRegion = region
	return true
}

func GetLocalIp(logger log.Logger) bool {

	addrs, err := net.InterfaceAddrs()
	if err != nil {
		level.Error(logger).Log("msg", "GetLocalIp_net.InterfaceAddrs", "err", err)
		return false
	}

	for _, addr := range addrs {
		if ipnet, ok := addr.(*net.IPNet); ok && !ipnet.IP.IsLoopback() {
			if ipnet.IP.To4() != nil {
				addr := ipnet.IP.String()
				LocalIp = addr
				return true
			}
		}
	}
	return false

}
