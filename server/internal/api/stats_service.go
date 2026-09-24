package api

import (
	"context"

	"connectrpc.com/connect"

	pb "github.com/lsparey/simple-logging/gen/simplelog/v1"
)

// GetStats returns the server's self-metrics, summed across labels.
func (s *LogService) GetStats(_ context.Context, _ *connect.Request[pb.GetStatsRequest]) (*connect.Response[pb.GetStatsResponse], error) {
	stats := s.metrics.Snapshot()
	resp := &pb.GetStatsResponse{
		StreamsActiveFile:             stats.StreamsActiveFile,
		StreamsActiveApi:              stats.StreamsActiveAPI,
		LinesWrittenTotal:             stats.LinesWritten,
		BytesWrittenTotal:             stats.BytesWritten,
		LinesDroppedTotal:             stats.LinesDropped,
		ApiReconnectsTotal:            stats.APIReconnects,
		RetentionSegmentsDeletedTotal: stats.RetentionSegmentsDeleted,
		DiskGuardSegmentsDeletedTotal: stats.DiskGuardSegmentsDeleted,
		SearchesActive:                stats.SearchesActive,
		SearchBytesScannedTotal:       stats.SearchBytesScanned,
	}
	if !stats.StartedAt.IsZero() {
		resp.StartedAtUnixMs = stats.StartedAt.UnixMilli()
	}
	return connect.NewResponse(resp), nil
}
