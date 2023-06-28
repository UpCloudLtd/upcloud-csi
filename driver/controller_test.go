package driver

import (
	"context"
	"github.com/container-storage-interface/spec/lib/go/csi"
	"testing"
)

func TestDriver_CreateSnapshot(t *testing.T) {
	type args struct {
		ctx          context.Context
		req          *csi.CreateSnapshotRequest
		volExists    bool
		volBackingUp bool
	}
	tests := []struct {
		name    string
		args    args
		want    *csi.CreateSnapshotResponse
		wantErr bool
	}{
		{
			name: "test",
			args: args{
				ctx: context.Background(),
				req: &csi.CreateSnapshotRequest{
					SourceVolumeId: "d470fcb8-14ba-11ee-8c6e-fe2faec4b636",
					Name:           "snappy",
				},
				volExists:    false,
				volBackingUp: false,
			},
			wantErr: false,
		},
		{
			name: "test with vol",
			args: args{
				ctx: context.Background(),
				req: &csi.CreateSnapshotRequest{
					SourceVolumeId: "d470fcb8-14ba-11ee-8c6e-fe2faec4b636",
					Name:           "snappy",
				},
				volExists:    true,
				volBackingUp: false,
			},
			wantErr: false,
		},
		{
			name: "test with vol",
			args: args{
				ctx: context.Background(),
				req: &csi.CreateSnapshotRequest{
					SourceVolumeId: "d470fcb8-14ba-11ee-8c6e-fe2faec4b636",
					Name:           "snappy",
				},
				volExists:    true,
				volBackingUp: true,
			},
			wantErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			svc := &mockUpCloudService{storageSize: 10, cloneStorageSize: 10, volumeUUIDExists: tt.args.volExists}
			d := NewMockDriver(svc)

			_, err := d.CreateSnapshot(tt.args.ctx, tt.args.req)
			if !tt.wantErr && err != nil {
				t.Fatalf("CreateSnapshot(%v, %v)", tt.args.ctx, tt.args.req)
				return
			}
		})
	}
}
