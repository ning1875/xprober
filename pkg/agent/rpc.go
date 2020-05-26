package agent

import (
	"context"
	"time"

	"github.com/flyaways/pool"
	"google.golang.org/grpc"
	"github.com/go-kit/kit/log"
	"github.com/go-kit/kit/log/level"

	"xprober/pkg/pb"
)

var (
	GrpcPool *pool.GRPCPool
)

const (
	RefreshInterval = 60 * time.Second
	PushInterval    = 15 * time.Second
	ReportInterval  = 60 * time.Second
)

func InitRpcPool(serverAddr string, logger log.Logger) bool {

	options := &pool.Options{
		InitTargets:  []string{serverAddr},
		InitCap:      5,
		MaxCap:       30,
		DialTimeout:  time.Second * 5,
		IdleTimeout:  time.Second * 60,
		ReadTimeout:  time.Second * 5,
		WriteTimeout: time.Second * 5,
	}

	//初始化连接池
	var err error
	GrpcPool, err = pool.NewGRPCPool(options, grpc.WithInsecure())

	if err != nil {
		level.Error(logger).Log("init_rpc_pool_failed_error", err)
		return false
	}

	if GrpcPool == nil {
		level.Error(logger).Log("msg", "init_GrpcPool_nil")
		return false
	}
	return true

}
func reportAgentIp(logger log.Logger) {
	level.Info(logger).Log("msg", "reportAgentIp run...", )
	conn, err := GrpcPool.Get()
	if err != nil {
		level.Error(logger).Log("get_rpc_conn_from_pool_err", err)
		return
	}

	defer conn.Close()
	c := pb.NewProberAgentIpReportClient(conn)
	t := pb.ProberAgentIpReportRequest{Ip: LocalIp, Region: LocalRegion}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	r, err := c.ProberAgentIpReports(ctx, &t)
	if err != nil {
		level.Error(logger).Log("msg", "could_not_reportAgentIp", "Ip", LocalIp, "Region:", LocalRegion)
		return
	}

	level.Info(logger).Log("reportAgentIpResult", r)

}
func getProberTarget(logger log.Logger) {
	level.Info(logger).Log("msg", "getProberTarget run...", )
	conn, err := GrpcPool.Get()
	if err != nil {
		level.Error(logger).Log("get_rpc_conn_from_pool_err", err)
		return
	}

	defer conn.Close()
	c := pb.NewGetProberTargetClient(conn)

	// Contact the server and print out its response.
	name := LocalRegion
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	r, err := c.GetProberTargets(ctx, &pb.ProberTargetsGetRequest{LocalRegion: LocalRegion, LocalIp: LocalIp})
	if err != nil {
		level.Error(logger).Log("msg", "could_not_get_target", "name", name, "error:", err)
		return
	}
	level.Info(logger).Log("getProberTargetresult", r)
	if len(r.Targets) > 0 {
		TargetUpdateChan <- r
	} else {
		level.Info(logger).Log("msg", "receive_empty_targets")
	}

}

func pushPbResults(logger log.Logger) {

	var prs []*pb.ProberResultOne
	f := func(k, v interface{}) bool {

		va := v.([]*pb.ProberResultOne)
		//key := k.(string)
		//fmt.Println(key, va)
		prs = append(prs, va...)
		return true
	}
	PbResMap.Range(f)
	if len(prs) == 0 {
		level.Info(logger).Log("msg", "empty_result_list_not_to_push")
		return
	}

	conn, err := GrpcPool.Get()
	if err != nil {
		level.Error(logger).Log("get_rpc_conn_from_pool_err", err)
		return
	}

	defer conn.Close()
	c := pb.NewPushProberResultClient(conn)
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	r, err := c.PushProberResults(ctx, &pb.ProberResultPushRequest{ProberResults: prs})
	if err != nil {
		level.Error(logger).Log("msg", "could_not_push_result ", "prs", prs, "error:", err)
	}
	level.Info(logger).Log("pushPbResults", r)
}
