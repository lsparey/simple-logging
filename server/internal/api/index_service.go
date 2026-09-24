package api

import (
	"context"
	"errors"
	"fmt"
	"os"

	"connectrpc.com/connect"
	pb "github.com/lsparey/simple-logging/gen/simplelog/v1"
	"github.com/lsparey/simple-logging/internal/indexes"
)

func (s *LogService) ListIndexes(_ context.Context, _ *connect.Request[pb.ListIndexesRequest]) (*connect.Response[pb.ListIndexesResponse], error) {
	keys := s.indexes.List()
	resp := &pb.ListIndexesResponse{Indexes: make([]*pb.LogIndexInfo, 0, len(keys))}
	for _, key := range keys {
		resp.Indexes = append(resp.Indexes, &pb.LogIndexInfo{Key: key})
	}
	return connect.NewResponse(resp), nil
}

func (s *LogService) CreateIndex(_ context.Context, r *connect.Request[pb.CreateIndexRequest]) (*connect.Response[pb.CreateIndexResponse], error) {
	req := r.Msg
	if err := indexes.ValidateKey(req.Key); err != nil {
		return nil, connect.NewError(connect.CodeInvalidArgument, err)
	}
	if err := s.indexes.Create(req.Key); err != nil {
		return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("create index: %v", err))
	}
	return connect.NewResponse(&pb.CreateIndexResponse{Index: &pb.LogIndexInfo{Key: req.Key}}), nil
}

func (s *LogService) DeleteIndex(_ context.Context, r *connect.Request[pb.DeleteIndexRequest]) (*connect.Response[pb.DeleteIndexResponse], error) {
	req := r.Msg
	if req.Key == "" {
		return nil, connect.NewError(connect.CodeInvalidArgument, errors.New("key is required"))
	}
	if err := s.indexes.Delete(req.Key); err != nil {
		if os.IsNotExist(err) {
			return nil, connect.NewError(connect.CodeNotFound, fmt.Errorf("index %q not found", req.Key))
		}
		return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("delete index: %v", err))
	}
	return connect.NewResponse(&pb.DeleteIndexResponse{}), nil
}

func (s *LogService) ListIndexValues(_ context.Context, r *connect.Request[pb.ListIndexValuesRequest]) (*connect.Response[pb.ListIndexValuesResponse], error) {
	req := r.Msg
	if req.Key == "" {
		return nil, connect.NewError(connect.CodeInvalidArgument, errors.New("key is required"))
	}
	values, next, prev, err := s.indexes.ListValues(req.Key, int(req.PageSize), req.PageToken)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, connect.NewError(connect.CodeNotFound, fmt.Errorf("index %q not found", req.Key))
		}
		if err.Error() == "invalid page_token" {
			return nil, connect.NewError(connect.CodeInvalidArgument, errors.New("invalid page_token"))
		}
		return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("list index values: %v", err))
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
	return connect.NewResponse(resp), nil
}

func (s *LogService) GetIndexLogs(_ context.Context, r *connect.Request[pb.GetIndexLogsRequest]) (*connect.Response[pb.GetIndexLogsResponse], error) {
	req := r.Msg
	if req.Key == "" || req.Value == "" {
		return nil, connect.NewError(connect.CodeInvalidArgument, errors.New("key and value are required"))
	}

	lines, next, prev, err := s.indexes.GetLogs(req.Key, req.Value, int(req.PageSize), req.PageToken, req.LoadLastPage)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, connect.NewError(connect.CodeNotFound, fmt.Errorf("index %q not found", req.Key))
		}
		if err.Error() == "invalid page_token" {
			return nil, connect.NewError(connect.CodeInvalidArgument, errors.New("invalid page_token"))
		}
		return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("read index: %v", err))
	}
	return connect.NewResponse(&pb.GetIndexLogsResponse{
		Lines:         lines,
		NextPageToken: next,
		PrevPageToken: prev,
	}), nil
}
