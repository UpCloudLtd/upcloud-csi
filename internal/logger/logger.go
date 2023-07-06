package logger

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"log"
	"time"

	"github.com/google/uuid"
	"github.com/sirupsen/logrus"
	"google.golang.org/grpc"
)

type contextKey string

const (
	HostKey              string = "host"
	NodeIDKey            string = "node_id"
	VolumeIDKey          string = "volume_id"
	VolumeNameKey        string = "volume_name"
	RequestKey           string = "request"
	ResponseKey          string = "response"
	ServiceURLKey        string = "service_url"
	ServicePayloadKey    string = "service_payload"
	CorrelationIDKey     string = "correlation_id"
	MethodKey            string = "method"
	FilesystemTypeKey    string = "fs_type"
	MountOptionsKey      string = "mount_options"
	MountSourceKey       string = "source"
	MountTargetKey       string = "target"
	CommandKey           string = "cmd"
	CommandArgsKey       string = "cmd_args"
	VolumeSourceKey      string = "source_id"
	SnapshotIDKey        string = "snapshot_id"
	ListStartingTokenKey string = "starting_token"
	ListMaxEntriesKey    string = "max_entries"
	ZoneKey              string = "zone"

	CtxCorrelationIDKey contextKey = "ctx_correlation_id"
	CtxCalledMethodKey  contextKey = "ctx_called_method"
)

type requestable interface {
	RequestURL() string
}

type contextualPublisher interface {
	GetPublishContext() map[string]string
}

func WithServerContext(ctx context.Context, e *logrus.Entry) *logrus.Entry {
	if v := ContextCorrelationID(ctx); v != "" {
		e = e.WithField(CorrelationIDKey, v)
	}
	if v, ok := ctx.Value(CtxCalledMethodKey).(string); ok {
		e = e.WithField(MethodKey, v)
	}
	return e
}

func WithRequest(e *logrus.Entry, r fmt.Stringer) *logrus.Entry {
	return e.WithField(RequestKey, r.String())
}

func WithResponse(e *logrus.Entry, r fmt.Stringer) *logrus.Entry {
	return e.WithField(ResponseKey, r.String())
}

func WithServiceRequest(e *logrus.Entry, r requestable) *logrus.Entry {
	return e.WithFields(logrus.Fields{
		ServiceURLKey:     r.RequestURL(),
		ServicePayloadKey: fmt.Sprintf("%+v", r),
	})
}

func NewMiddleware(log *logrus.Entry) grpc.UnaryServerInterceptor {
	return func(ctx context.Context, req interface{}, info *grpc.UnaryServerInfo, handler grpc.UnaryHandler) (interface{}, error) {
		var txID string
		// Assign pre existing correlation ID from publish context or generate new one
		if r, ok := req.(contextualPublisher); ok {
			txID = contextualPublisherCorrelationID(r)
		}
		if txID == "" {
			txID = correlationID()
		}
		ctx = context.WithValue(ctx, CtxCorrelationIDKey, txID)
		ctx = context.WithValue(ctx, CtxCalledMethodKey, info.FullMethod)

		// log requested method with correlation ID
		log := WithServerContext(ctx, log)
		log.Infof("request %s", info.FullMethod)

		now := time.Now()
		resp, err := handler(ctx, req)
		log = log.WithField("execution_time_ms", time.Since(now).Milliseconds())

		if err != nil {
			if s, ok := req.(fmt.Stringer); ok && log.Logger.GetLevel() >= logrus.DebugLevel {
				// log request object only if we are debugging
				log = WithRequest(log, s)
			}
			log.WithError(err).Error("method failed")
		} else {
			if s, ok := resp.(fmt.Stringer); ok && log.Logger.GetLevel() >= logrus.DebugLevel {
				// log response object only if we are debugging
				log = WithResponse(log, s)
			}
			log.Infof("%s OK", info.FullMethod)
		}
		return resp, err
	}
}

func ContextCorrelationID(ctx context.Context) string {
	if v, ok := ctx.Value(CtxCorrelationIDKey).(string); ok {
		return v
	}
	return ""
}

func contextualPublisherCorrelationID(ctx contextualPublisher) string {
	if v, ok := ctx.GetPublishContext()[string(CtxCorrelationIDKey)]; ok {
		return v
	}
	return ""
}

// correlationID generates random correlation ID string.
// Currently ID is used only to distinguish actions from log so returned value doesn't have to be e.g globally unique.
// If this isn't enough use UUID or some lib to generate more unique value.
func correlationID() string {
	b := make([]byte, 6)
	if _, err := rand.Read(b); err != nil {
		// this shouldn't happen but fallback to UUID if necessary
		return uuid.NewString()
	}
	return hex.EncodeToString(b)
}

func New(logLevel string) *logrus.Logger {
	lv, err := logrus.ParseLevel(logLevel)
	if err != nil {
		log.Fatal(err)
	}
	logger := logrus.New()
	logger.SetLevel(lv)
	if logger.GetLevel() > logrus.InfoLevel {
		logger.WithField("level", logger.GetLevel().String()).Warn("using log level higher than INFO is not recommended in production")
	}
	return logger
}
