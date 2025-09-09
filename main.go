package main

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"log/slog"
	"net"
	"os"
	"os/signal"
	"syscall"

	"gprc-logsink/internal/logging"

	datav3 "github.com/envoyproxy/go-control-plane/envoy/data/accesslog/v3"
	accesslog "github.com/envoyproxy/go-control-plane/envoy/service/accesslog/v3"
	"google.golang.org/grpc"
)

type server struct {
	accesslog.UnimplementedAccessLogServiceServer
	file *os.File
}

func formatProtocol(v datav3.HTTPAccessLogEntry_HTTPVersion) string {
	switch v {
	case datav3.HTTPAccessLogEntry_HTTP10:
		return "HTTP/1.0"
	case datav3.HTTPAccessLogEntry_HTTP11:
		return "HTTP/1.1"
	case datav3.HTTPAccessLogEntry_HTTP2:
		return "HTTP/2"
	case datav3.HTTPAccessLogEntry_HTTP3:
		return "HTTP/3"
	default:
		return "UNKNOWN"
	}
}

// StreamAccessLogs handles the bidirectional gRPC stream from Envoy
func (s *server) StreamAccessLogs(stream accesslog.AccessLogService_StreamAccessLogsServer) error {
	for {
		msg, err := stream.Recv()
		if err != nil {
			return err
		}

		switch entries := msg.LogEntries.(type) {
		case *accesslog.StreamAccessLogsMessage_HttpLogs:
			for _, logEntry := range entries.HttpLogs.LogEntry {

				// Helper shortcuts
				req := logEntry.Request
				resp := logEntry.Response
				common := logEntry.CommonProperties

				var upstreamHost string
				if addr := common.UpstreamRemoteAddress; addr != nil {
					if sa := addr.GetSocketAddress(); sa != nil {
						upstreamHost = fmt.Sprintf("%s:%d", sa.Address, sa.GetPortValue())
					}
				}

				out := map[string]any{
					"start_time":       common.StartTime.AsTime().Format("2006-01-02T15:04:05.000Z07:00"),
					"method":           req.RequestMethod.String(),
					"authority":        req.Authority,
					"path":             req.Path,
					"protocol":         formatProtocol(logEntry.GetProtocolVersion()),
					"status":           resp.ResponseCode.GetValue(),
					"bytes_sent":       resp.ResponseBodyBytes,
					"bytes_received":   req.RequestBodyBytes,
					"duration":         common.TimeToLastDownstreamTxByte.AsDuration().Milliseconds(),
					"upstream_host":    upstreamHost,
					"upstream_service": common.UpstreamCluster,
					"source_ip":        common.DownstreamRemoteAddress.GetSocketAddress().GetAddress(),
					"user_agent":       req.UserAgent,
					"forwarded_for":    req.ForwardedFor,
					"request_id":       common.StreamId,
					"waf_violation":    resp.ResponseHeaders["x-waf-violation"],
				}

				data, _ := json.Marshal(out)
				s.file.Write(append(data, '\n'))
			}

		case *accesslog.StreamAccessLogsMessage_TcpLogs:
			for _, logEntry := range entries.TcpLogs.LogEntry {
				data, _ := json.Marshal(logEntry)
				s.file.Write(append(data, '\n'))
			}
		}
	}
}

func main() {
	// Setup logging
	logger := logging.Setup()
	slog.SetDefault(logger)

	port := os.Getenv("PORT")
	if port == "" {
		port = "9000"
	}

	slog.Info("=== Starting GRPC log sink server ===", slog.String("port", port))

	// Create TCP listener
	lis, err := net.Listen("tcp", ":"+port)
	if err != nil {
		slog.Error("Failed to create listener", slog.Any("error", err))
		os.Exit(1)
	}
	defer lis.Close()

	logFile := os.Getenv("LOGFILE")
	if logFile == "" {
		logFile = "/var/log/envoy/access.log"
	}

	f, err := os.OpenFile(logFile, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0644)
	if err != nil {
		log.Fatal(err)
	}
	defer f.Close()

	grpcServer := grpc.NewServer()
	accesslog.RegisterAccessLogServiceServer(grpcServer, &server{file: f})
	slog.Info("ALS gRPC server running on : " + port)
	if err := grpcServer.Serve(lis); err != nil {
		slog.Error("failed to serve: " + err.Error())
	}

	// Setup graceful shutdown
	_, cancel := context.WithCancel(context.Background())
	defer cancel()

	go func() {
		sigChan := make(chan os.Signal, 1)
		signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)
		<-sigChan
		slog.Info("Shutting down server...")
		cancel()
	}()

	slog.Info("=== Server ready - waiting for connections ===")

}
