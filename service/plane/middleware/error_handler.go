package middleware

import (
	"context"
	"encoding/json"
	"net/http"

	"github.com/atomyze-foundation/hlf-control-plane/proto"
	"github.com/grpc-ecosystem/grpc-gateway/v2/runtime"
	"go.uber.org/zap"
	"google.golang.org/grpc/status"
)

func ErrorHandler(logger *zap.Logger) runtime.ServeMuxOption {
	return runtime.WithErrorHandler(func(ctx context.Context, mux *runtime.ServeMux, marshaler runtime.Marshaler, writer http.ResponseWriter, request *http.Request, err error) {
		resp := &proto.ErrorResponse{}
		s, ok := status.FromError(err)

		writer.Header().Del("Trailer")
		writer.Header().Del("Transfer-Encoding")
		writer.Header().Set("Content-Type", "application/json")

		if ok {
			writer.WriteHeader(runtime.HTTPStatusFromCode(s.Code()))
			resp.Error = s.String()
		} else {
			writer.WriteHeader(http.StatusInternalServerError)
			resp.Error = err.Error()
		}

		we, err := json.Marshal(resp)
		if err != nil {
			logger.Error("response json marshal error", zap.Error(err))
		}
		if _, err = writer.Write(we); err != nil {
			logger.Error("response write error", zap.Error(err))
		}
		logger.Error("request error", zap.String("error", resp.Error))
	})
}
