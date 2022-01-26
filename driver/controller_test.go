package driver

import (
	"context"

	"github.com/container-storage-interface/spec/lib/go/csi"
	"reflect"
	"testing"
)

func TestControllerService_ControllerGetCapabilities(t *testing.T) {
	type args struct {
		ctx context.Context
		req *csi.ControllerGetCapabilitiesRequest
	}
	tests := []struct {
		name    string
		args    args
		wantErr bool
	}{
		{
			name: "Get Capabilities",
			args: args{
				ctx: context.Background(),
			},
			wantErr: false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			d := newMockDriver()
			gotResp, err := d.ControllerGetCapabilities(tt.args.ctx, tt.args.req)
			if (err != nil) != tt.wantErr {
				t.Errorf("ControllerGetCapabilities() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if len(gotResp.Capabilities) == 0 {
				t.Error("ControllerGetCapabilities should not be empty")
				return
			}
		})
	}
}

func TestControllerService_ControllerPublishVolume(t *testing.T) {
	type args struct {
		ctx context.Context
		req *csi.ControllerPublishVolumeRequest
	}
	tests := []struct {
		name     string
		args     args
		wantResp *csi.ControllerPublishVolumeResponse
		wantErr  bool
	}{
		{
			name: "Test Publish Volume",
			args: args{
				ctx: context.Background(),
				req: &csi.ControllerPublishVolumeRequest{
					VolumeId: "test-volume-id",
					NodeId:   "test-node-id",
					VolumeCapability: &csi.VolumeCapability{
						AccessType: &csi.VolumeCapability_Mount{
							Mount: &csi.VolumeCapability_MountVolume{},
						},
					},
				},
			},
			wantErr: false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			d := newMockDriver()
			gotResp, err := d.ControllerPublishVolume(tt.args.ctx, tt.args.req)
			if (err != nil) != tt.wantErr {
				t.Errorf("ControllerPublishVolume() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if len(gotResp.PublishContext) == 0 {
				t.Error("empty publish context")
			}
		})
	}
}

func TestControllerService_CreateVolume(t *testing.T) {
	type args struct {
		ctx context.Context
		req *csi.CreateVolumeRequest
	}
	tests := []struct {
		name         string
		args         args
		volumeExists bool
		wantResp     *csi.CreateVolumeResponse
		wantErr      bool
	}{
		{
			name: "Test Volume Already Exists",
			args: args{
				context.Background(),
				&csi.CreateVolumeRequest{
					Name: "testVolume",
					VolumeCapabilities: []*csi.VolumeCapability{
						{
							AccessType: &csi.VolumeCapability_Mount{
								Mount: &csi.VolumeCapability_MountVolume{},
							},
							AccessMode: &csi.VolumeCapability_AccessMode{
								Mode: csi.VolumeCapability_AccessMode_SINGLE_NODE_WRITER,
							},
						},
					},
					VolumeContentSource: &csi.VolumeContentSource{
						Type: &csi.VolumeContentSource_Snapshot{
							Snapshot: &csi.VolumeContentSource_SnapshotSource{
								SnapshotId: "snapshotID",
							},
						},
					},
				},
			},
			volumeExists: true,
			wantErr:      false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {

			d := newMockDriver()

			gotResp, err := d.CreateVolume(tt.args.ctx, tt.args.req)
			if (err != nil) != tt.wantErr {
				t.Errorf("CreateVolume() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if gotResp.Volume.VolumeId == "" {
				t.Error("volume ID should not be empty")
				return
			}
		})
	}

}

func TestControllerService_DeleteVolume(t *testing.T) {
	type args struct {
		ctx context.Context
		req *csi.DeleteVolumeRequest
	}
	tests := []struct {
		name     string
		args     args
		wantResp *csi.DeleteVolumeResponse
		wantErr  bool
	}{
		{
			name: "Test Delete Volume",
			args: args{
				context.Background(),
				&csi.DeleteVolumeRequest{
					VolumeId: "testVolume",
				},
			},
			wantErr:  false,
			wantResp: &csi.DeleteVolumeResponse{},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			d := newMockDriver()
			gotResp, err := d.DeleteVolume(tt.args.ctx, tt.args.req)
			if (err != nil) != tt.wantErr {
				t.Errorf("DeleteVolume() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if !reflect.DeepEqual(gotResp, tt.wantResp) {
				t.Errorf("DeleteVolume() gotResp = %v, want %v", gotResp, tt.wantResp)
			}
		})
	}
}

func TestControllerService_ListVolumes(t *testing.T) {
	type args struct {
		ctx context.Context
		req *csi.ListVolumesRequest
	}
	tests := []struct {
		name     string
		args     args
		wantResp *csi.ListVolumesResponse
		wantErr  bool
	}{
		{
			name: "Test List Volumes",
			args: args{
				context.Background(),
				&csi.ListVolumesRequest{},
			},
			wantErr: false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			d := newMockDriver()
			gotResp, err := d.ListVolumes(tt.args.ctx, tt.args.req)
			if (err != nil) != tt.wantErr {
				t.Errorf("ListVolumes() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if len(gotResp.Entries) == 0 {
				t.Error("ListVolumes should not be empty")
				return
			}
		})
	}
}

func TestControllerService_ControllerUnpublishVolume(t *testing.T) {
	type args struct {
		ctx context.Context
		req *csi.ControllerUnpublishVolumeRequest
	}
	tests := []struct {
		name    string
		args    args
		want    *csi.ControllerUnpublishVolumeResponse
		wantErr bool
	}{
		{
			name: "Test Unpublish Volume",
			args: args{
				context.Background(),
				&csi.ControllerUnpublishVolumeRequest{
					VolumeId: "testVolume",
				},
			},
			wantErr: false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			d := newMockDriver()
			_, err := d.ControllerUnpublishVolume(tt.args.ctx, tt.args.req)
			if (err != nil) != tt.wantErr {
				t.Errorf("ControllerUnpublishVolume() error = %v, wantErr %v", err, tt.wantErr)
				return
			}

		})
	}
}

func TestControllerService_ValidateVolumeCapabilities(t *testing.T) {
	type args struct {
		ctx context.Context
		req *csi.ValidateVolumeCapabilitiesRequest
	}
	tests := []struct {
		name    string
		args    args
		want    *csi.VolumeCapability_AccessMode
		wantErr bool
	}{
		{
			name: "Test ValidateVolumeCapabilities",
			args: args{
				context.Background(),
				&csi.ValidateVolumeCapabilitiesRequest{
					VolumeId: "testVolume",
					VolumeCapabilities: []*csi.VolumeCapability{
						{
							AccessType: &csi.VolumeCapability_Mount{
								Mount: &csi.VolumeCapability_MountVolume{},
							},
						},
					},
				},
			},
			want:    supportedAccessMode,
			wantErr: false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			d := newMockDriver()
			got, err := d.ValidateVolumeCapabilities(tt.args.ctx, tt.args.req)
			if (err != nil) != tt.wantErr {
				t.Errorf("ValidateVolumeCapabilities() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if !reflect.DeepEqual(got.Confirmed.VolumeCapabilities[0].AccessMode, tt.want) {
				t.Errorf("ValidateVolumeCapabilities() got = %v, want %v", got, tt.want)
			}
		})
	}
}
