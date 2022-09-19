package driver

import (
	"context"

	"reflect"
	"testing"

	"github.com/container-storage-interface/spec/lib/go/csi"
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
			d := NewMockDriver(nil)
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
			d := NewMockDriver(nil)
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
	caps := []*csi.VolumeCapability{
		{
			AccessType: &csi.VolumeCapability_Mount{
				Mount: &csi.VolumeCapability_MountVolume{},
			},
			AccessMode: &csi.VolumeCapability_AccessMode{
				Mode: csi.VolumeCapability_AccessMode_SINGLE_NODE_WRITER,
			},
		},
	}
	type args struct {
		ctx context.Context
		req *csi.CreateVolumeRequest
	}
	tests := []struct {
		name             string
		args             args
		volumeNameExists bool
		volumeUUIDExists bool
		wantResp         *csi.CreateVolumeResponse
		wantErr          bool
	}{
		{
			name: "Test Volume Already Exists",
			args: args{
				context.Background(),
				&csi.CreateVolumeRequest{
					Name:               "testVolume",
					VolumeCapabilities: caps,
					VolumeContentSource: &csi.VolumeContentSource{
						Type: &csi.VolumeContentSource_Snapshot{
							Snapshot: &csi.VolumeContentSource_SnapshotSource{
								SnapshotId: "snapshotID",
							},
						},
					},
				},
			},
			volumeNameExists: true,
			volumeUUIDExists: true,
			wantErr:          false,
		},
		{
			name: "Test Clone Volume Size",
			args: args{
				context.Background(),
				&csi.CreateVolumeRequest{
					Name:               "testCloneVolume",
					VolumeCapabilities: caps,
					VolumeContentSource: &csi.VolumeContentSource{
						Type: &csi.VolumeContentSource_Volume{
							Volume: &csi.VolumeContentSource_VolumeSource{
								VolumeId: "volumeID",
							},
						},
					},
				},
			},
			wantResp: &csi.CreateVolumeResponse{
				Volume: &csi.Volume{
					CapacityBytes: 10 * giB,
					VolumeId:      "testCloneVolume",
				},
			},
			volumeNameExists: false,
			volumeUUIDExists: true,
			wantErr:          false,
		},
		{
			name: "Test Clone Snapshot Volume Size",
			args: args{
				context.Background(),
				&csi.CreateVolumeRequest{
					Name:               "testCloneSnapshotVolume",
					VolumeCapabilities: caps,
					VolumeContentSource: &csi.VolumeContentSource{
						Type: &csi.VolumeContentSource_Snapshot{
							Snapshot: &csi.VolumeContentSource_SnapshotSource{
								SnapshotId: "snapshotID",
							},
						},
					},
				},
			},
			wantResp: &csi.CreateVolumeResponse{
				Volume: &csi.Volume{
					CapacityBytes: 10 * giB,
					VolumeId:      "testCloneSnapshotVolume",
				},
			},
			volumeNameExists: false,
			volumeUUIDExists: true,
			wantErr:          false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			d := NewMockDriver(&mockUpCloudDriver{
				volumeNameExists: tt.volumeNameExists,
				volumeUUIDExists: tt.volumeUUIDExists,
				storageSize:      10,
				cloneStorageSize: 9, // set smaller size so that resize is triggered
			})
			gotResp, err := d.CreateVolume(tt.args.ctx, tt.args.req)
			if (err != nil) != tt.wantErr {
				t.Errorf("CreateVolume() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if gotResp.Volume.VolumeId == "" {
				t.Error("volume ID should not be empty")
				return
			}
			if tt.wantResp != nil {
				if vol := tt.wantResp.GetVolume(); vol != nil {
					if vol.CapacityBytes != gotResp.Volume.CapacityBytes {
						t.Errorf("volume capacity mismatch want %d got %d", vol.CapacityBytes, gotResp.Volume.CapacityBytes)
						return
					}
				}

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
			d := NewMockDriver(nil)
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
			d := NewMockDriver(nil)
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
			d := NewMockDriver(nil)
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
			d := NewMockDriver(nil)
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

func TestControllerExpandVolume(t *testing.T) {
	d := NewMockDriver(nil)
	wantBytes := int64(30 * giB)
	r, err := d.ControllerExpandVolume(context.Background(), &csi.ControllerExpandVolumeRequest{
		VolumeId: "test-vol",
		CapacityRange: &csi.CapacityRange{
			RequiredBytes: wantBytes,
			LimitBytes:    0,
		},
		//VolumeCapability:     &csi.VolumeCapability{},
	})
	if err != nil {
		t.Errorf("ControllerExpandVolume error = %v", err)
		return
	}
	if r.CapacityBytes != wantBytes {
		t.Errorf("CapacityBytes failed want %d got %d", wantBytes, r.CapacityBytes)
	}
}
