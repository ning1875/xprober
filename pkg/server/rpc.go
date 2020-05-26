package server

import (
	"net"
	"context"

	"github.com/go-kit/kit/log"
	"github.com/go-kit/kit/log/level"
	"google.golang.org/grpc"

	"xprober/pkg/pb"
)

var (
	GRM *GRpcServerManager
)

type GRpcServerManager struct {
	Logger            log.Logger
	GrpcListenAddress string
	Server            *grpc.Server
}

func NewManagager(logger log.Logger, addr string) {
	gp := GRpcServerManager{
		Logger:            logger,
		GrpcListenAddress: addr,
	}

	GRM = &gp
}

func (gs *GRpcServerManager) Run(ctx context.Context, logger log.Logger) error {

	lis, err := net.Listen("tcp", gs.GrpcListenAddress)
	if err != nil {
		level.Error(gs.Logger).Log("msg", "grpc failed to listen: ", "err", err)
	}
	s := grpc.NewServer()
	gs.Server = s

	// register service
	pb.RegisterGetProberTargetServer(s, &PServer{logger: logger})
	pb.RegisterPushProberResultServer(s, &PResult{logger: logger})
	pb.RegisterProberAgentIpReportServer(s, &PAgentR{logger: logger})
	level.Info(gs.Logger).Log("msg", "grpc success to serve", "addr", gs.GrpcListenAddress)
	if err := s.Serve(lis); err != nil {
		level.Error(gs.Logger).Log("msg", "grpc failed to serve err", "err", err)
		return err
	}

	return nil
}

type PServer struct {
	pb.UnimplementedGetProberTargetServer
	logger log.Logger
}

type PResult struct {
	pb.UnimplementedPushProberResultServer
	logger log.Logger
}

type PAgentR struct {
	pb.UnimplementedProberAgentIpReportServer
	logger log.Logger
}

// GetProberTargets implements GetProberTargets
func (s *PServer) GetProberTargets(ctx context.Context, in *pb.ProberTargetsGetRequest) (*pb.ProberTargetsGetResponse, error) {
	level.Info(s.logger).Log("msg", "GetProberTargets receive", "region", in.LocalRegion, "ip", in.LocalIp)
	// TODO real get region
	region := in.LocalRegion
	tgs := GetTargetsByRegion(region)
	return &pb.ProberTargetsGetResponse{Targets: tgs}, nil
}

func GetProbeResultUid(prr *pb.ProberResultOne) (uid string) {
	uid = prr.WorkerName + prr.MetricName + prr.SourceRegion + prr.TargetRegion + prr.ProbeType + prr.TargetAddr
	return

}

func (pr *PResult) PushProberResults(ctx context.Context, in *pb.ProberResultPushRequest) (*pb.ProberResultPushResponse, error) {

	level.Debug(pr.logger).Log("msg", "PushProberResult receive", "args", in)
	suNum := 0
	for _, prr := range in.ProberResults {
		uid := GetProbeResultUid(prr)
		switch prr.ProbeType {
		case `icmp`:
			IcmpDataMap.Store(uid, prr)
		case `http`:
			HttpDataMap.Store(uid, prr)
		}
		suNum += 1

	}
	return &pb.ProberResultPushResponse{SuccessNum: int32(suNum)}, nil
}

func (pr *PAgentR) ProberAgentIpReports(ctx context.Context, in *pb.ProberAgentIpReportRequest) (*pb.ProberAgentIpReportResponse, error) {

	level.Debug(pr.logger).Log("msg", "ProberAgentIpReports receive", "args", in)

	AgentIpRegionMap.Store(in.Ip, in.Region)

	return &pb.ProberAgentIpReportResponse{IsSuccess: true}, nil
}
