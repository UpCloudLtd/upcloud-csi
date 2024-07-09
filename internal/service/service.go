package service

import (
	"context"
	"errors"

	"github.com/UpCloudLtd/upcloud-go-api/v8/upcloud"
	"github.com/UpCloudLtd/upcloud-go-api/v8/upcloud/request"
)

var (
	ErrStorageNotFound       = errors.New("upcloud: storage not found")
	ErrServerNotFound        = errors.New("upcloud: server not found")
	ErrServerStorageNotFound = errors.New("upcloud: server storage not found")
	ErrBackupInProgress      = errors.New("upcloud: cannot take snapshot while storage is in state backup")
)

type Service interface { //nolint:interfacebloat // Split this to smaller piece when it makes sense code wise
	GetServerByHostname(context.Context, string) (*upcloud.ServerDetails, error)
	GetStorageByUUID(context.Context, string) (*upcloud.StorageDetails, error)
	GetStorageByName(context.Context, string) ([]*upcloud.StorageDetails, error)
	ListStorage(context.Context, string) ([]upcloud.Storage, error)
	GetStorageBackupByName(context.Context, string) (*upcloud.Storage, error)
	ListStorageBackups(ctx context.Context, uuid string) ([]upcloud.Storage, error)
	RequireStorageOnline(ctx context.Context, s *upcloud.Storage) error
	CreateStorage(context.Context, *request.CreateStorageRequest) (*upcloud.StorageDetails, error)
	CloneStorage(context.Context, *request.CloneStorageRequest, ...upcloud.Label) (*upcloud.StorageDetails, error)
	DeleteStorage(context.Context, string) error
	AttachStorage(context.Context, string, string) error
	DetachStorage(context.Context, string, string) error
	ResizeStorage(ctx context.Context, uuid string, newSize int, deleteBackup bool) (*upcloud.StorageDetails, error)
	ResizeBlockDevice(ctx context.Context, uuid string, newSize int) (*upcloud.StorageDetails, error)
	CreateStorageBackup(ctx context.Context, uuid, title string) (*upcloud.StorageDetails, error)
	DeleteStorageBackup(ctx context.Context, uuid string) error
}
