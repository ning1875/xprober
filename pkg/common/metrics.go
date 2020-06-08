package common

/*
   metics name should be `module_type_unit`
*/
const (
	// ping
	MetricsNamePingLatency       = `ping_latency_millonseconds`
	MetricsNamePingPackageDrop   = `ping_packageDrop_rate`
	MetricsNamePingTargetSuccess = `ping_target_success`

	// http
	MetricsNameHttpResolvedurationMillonseconds    = `http_resolveDuration_millonseconds`
	MetricsNameHttpTlsDurationMillonseconds        = `http_tlsDuration_millonseconds`
	MetricsNameHttpConnectDurationMillonseconds    = `http_connectDuration_millonseconds`
	MetricsNameHttpProcessingDurationMillonseconds = `http_processingDuration_millonseconds`
	MetricsNameHttpTransferDurationMillonseconds   = `http_transferDuration_millonseconds`
	MetricsNameHttpInterfaceSuccess                = `http_interface_success`
)
