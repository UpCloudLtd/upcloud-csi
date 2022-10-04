package driver

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/sirupsen/logrus"
	"google.golang.org/grpc"
)

type contextKey string

const (
	logNodeIDKey         string = "node_id"
	logVolumeIDKey       string = "volume_id"
	logVolumeNameKey     string = "volume_name"
	logRequestKey        string = "request"
	logResponseKey       string = "response"
	logServiceURLKey     string = "service_url"
	logServicePayloadKey string = "service_payload"
	logCorrelationIDKey  string = "correlation_id"
	logMethodKey         string = "method"
	logFilesystemTypeKey string = "fsType"
	logMountOptionsKey   string = "mount_options"
	logMountSourceKey    string = "source"
	logMountTargetKey    string = "target"
	logCommandKey        string = "cmd"
	logCommandArgsKey    string = "cmd_args"

	ctxCorrelationIDKey contextKey = "ctx_correlation_id"
	ctxCalledMethodKey  contextKey = "ctx_called_method"
)

type requestable interface {
	RequestURL() string
}

type contextualPublisher interface {
	GetPublishContext() map[string]string
}

func logWithServerContext(e *logrus.Entry, ctx context.Context) *logrus.Entry {
	if v := contextCorrelationID(ctx); v != "" {
		e = e.WithField(logCorrelationIDKey, v)
	}
	if v, ok := ctx.Value(ctxCalledMethodKey).(string); ok {
		e = e.WithField(logMethodKey, v)
	}
	return e
}

func logWithRequest(e *logrus.Entry, r fmt.Stringer) *logrus.Entry {
	return e.WithField(logRequestKey, r.String())
}

func logWithResponse(e *logrus.Entry, r fmt.Stringer) *logrus.Entry {
	return e.WithField(logResponseKey, r.String())
}

func logWithServiceRequest(e *logrus.Entry, r requestable) *logrus.Entry {
	return e.WithFields(logrus.Fields{
		logServiceURLKey:     r.RequestURL(),
		logServicePayloadKey: fmt.Sprintf("%+v", r),
	})
}

func serverLogMiddleware(log *logrus.Entry) grpc.UnaryServerInterceptor {
	return func(ctx context.Context, req interface{}, info *grpc.UnaryServerInfo, handler grpc.UnaryHandler) (interface{}, error) {
		var txID string
		// Assign pre existing correlation ID from publish context or generate new one
		if r, ok := req.(contextualPublisher); ok {
			txID = contextualPublisherCorrelationID(r)
		}
		if txID == "" {
			txID = correlationID()
		}
		ctx = context.WithValue(ctx, ctxCorrelationIDKey, txID)
		ctx = context.WithValue(ctx, ctxCalledMethodKey, info.FullMethod)

		// log requested method with correlation ID
		log := logWithServerContext(log, ctx)
		log.Infof("request %s", info.FullMethod)

		now := time.Now()
		resp, err := handler(ctx, req)
		log = log.WithField("execution_time_ms", time.Since(now).Milliseconds())

		if err != nil {
			if s, ok := req.(fmt.Stringer); ok && log.Level >= logrus.DebugLevel {
				// log request object only if we are debugging
				log = logWithRequest(log, s)
			}
			log.WithError(err).Error("method failed")
		} else {
			if s, ok := resp.(fmt.Stringer); ok && log.Level >= logrus.DebugLevel {
				// log response object only if we are debugging
				log = logWithResponse(log, s)
			}
			log.Infof("%s OK", info.FullMethod)
		}
		return resp, err
	}
}

func contextCorrelationID(ctx context.Context) string {
	if v, ok := ctx.Value(ctxCorrelationIDKey).(string); ok {
		return v
	}
	return ""
}

func contextualPublisherCorrelationID(ctx contextualPublisher) string {
	if v, ok := ctx.GetPublishContext()[string(ctxCorrelationIDKey)]; ok {
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
