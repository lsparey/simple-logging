package api

import (
	"context"
	"os"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	pb "github.com/lsparey/simple-logging/gen/simplelog/v1"
	"github.com/lsparey/simple-logging/internal/indexes"
)

func (s *LogService) ListIndexes(_ context.Context, _ *pb.ListIndexesRequest) (*pb.ListIndexesResponse, error) {
	keys := s.indexes.List()
	resp := &pb.ListIndexesResponse{Indexes: make([]*pb.LogIndexInfo, 0, len(keys))}
	for _, key := range keys {
		resp.Indexes = append(resp.Indexes, &pb.LogIndexInfo{Key: key})
	}
	return resp, nil
}

func (s *LogService) CreateIndex(_ context.Context, req *pb.CreateIndexRequest) (*pb.CreateIndexResponse, error) {
	if err := indexes.ValidateKey(req.Key); err != nil {
		return nil, status.Error(codes.InvalidArgument, err.Error())
	}
	if err := s.indexes.Create(req.Key); err != nil {
		return nil, status.Errorf(codes.Internal, "create index: %v", err)
	}
	return &pb.CreateIndexResponse{Index: &pb.LogIndexInfo{Key: req.Key}}, nil
}

func (s *LogService) DeleteIndex(_ context.Context, req *pb.DeleteIndexRequest) (*pb.DeleteIndexResponse, error) {
	if req.Key == "" {
		return nil, status.Error(codes.InvalidArgument, "key is required")
	}
	if err := s.indexes.Delete(req.Key); err != nil {
		if os.IsNotExist(err) {
			return nil, status.Errorf(codes.NotFound, "index %q not found", req.Key)
		}
		return nil, status.Errorf(codes.Internal, "delete index: %v", err)
	}
	return &pb.DeleteIndexResponse{}, nil
}

func (s *LogService) ListIndexValues(_ context.Context, req *pb.ListIndexValuesRequest) (*pb.ListIndexValuesResponse, error) {
	if req.Key == "" {
		return nil, status.Error(codes.InvalidArgument, "key is required")
	}
	values, next, prev, err := s.indexes.ListValues(req.Key, int(req.PageSize), req.PageToken)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, status.Errorf(codes.NotFound, "index %q not found", req.Key)
		}
		if err.Error() == "invalid page_token" {
			return nil, status.Error(codes.InvalidArgument, "invalid page_token")
		}
		return nil, status.Errorf(codes.Internal, "list index values: %v", err)
	}
	resp := &pb.ListIndexValuesResponse{
		Values:        make([]*pb.LogIndexValueInfo, 0, len(values)),
		NextPageToken: next,
		PrevPageToken: prev,
	}
	for _, value := range values {
		var lastUpdatedUnixMs int64
		if !value.LastUpdated.IsZero() {
			lastUpdatedUnixMs = value.LastUpdated.UnixMilli()
		}
		resp.Values = append(resp.Values, &pb.LogIndexValueInfo{
			Value:             value.Value,
			Count:             value.Count,
			LastUpdatedUnixMs: lastUpdatedUnixMs,
		})
	}
	return resp, nil
}

func (s *LogService) GetIndexLogs(_ context.Context, req *pb.GetIndexLogsRequest) (*pb.GetIndexLogsResponse, error) {
	if req.Key == "" || req.Value == "" {
		return nil, status.Error(codes.InvalidArgument, "key and value are required")
	}

	lines, next, prev, err := s.indexes.GetLogs(req.Key, req.Value, int(req.PageSize), req.PageToken, req.LoadLastPage)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, status.Errorf(codes.NotFound, "index %q not found", req.Key)
		}
		if err.Error() == "invalid page_token" {
			return nil, status.Error(codes.InvalidArgument, "invalid page_token")
		}
		return nil, status.Errorf(codes.Internal, "read index: %v", err)
	}
	return &pb.GetIndexLogsResponse{
		Lines:         lines,
		NextPageToken: next,
		PrevPageToken: prev,
	}, nil
}
