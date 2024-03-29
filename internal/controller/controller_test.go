package controller_test

import (
	"context"
	"reflect"
	"testing"

	"github.com/UpCloudLtd/upcloud-csi/internal/controller"
	"github.com/UpCloudLtd/upcloud-csi/internal/service"
	"github.com/UpCloudLtd/upcloud-csi/internal/service/mock"
	"github.com/container-storage-interface/spec/lib/go/csi"
	"github.com/google/uuid"
	"github.com/sirupsen/logrus"
)

const (
	_   = iota
	kiB = 1 << (10 * iota)
	miB
	giB
	tiB
)

func newController(svc service.Service) *controller.Controller {
	if svc == nil {
		svc = &mock.UpCloudServiceMock{StorageSize: 10, CloneStorageSize: 10, VolumeUUIDExists: true}
	}

	c, _ := controller.NewController(svc, "fi-hel2", 10, logrus.New().WithField("package", "controller_test"))
	return c
}

func TestController_ControllerGetCapabilities(t *testing.T) {
	t.Parallel()
	type args struct {
		req *csi.ControllerGetCapabilitiesRequest
	}
	tests := []struct {
		name    string
		args    args
		wantErr bool
	}{
		{
			name:    "Get Capabilities",
			args:    args{},
			wantErr: false,
		},
	}
	for _, testCase := range tests {
		tt := testCase
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			c := newController(nil)
			gotResp, err := c.ControllerGetCapabilities(context.Background(), tt.args.req)
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

func TestController_ControllerPublishVolume(t *testing.T) {
	t.Parallel()
	type args struct {
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
	for _, testCase := range tests {
		tt := testCase
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			c := newController(nil)
			gotResp, err := c.ControllerPublishVolume(context.Background(), tt.args.req)
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

func TestController_CreateVolume(t *testing.T) {
	t.Parallel()
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
				&csi.CreateVolumeRequest{
					Name:               "testVolume",
					VolumeCapabilities: caps,
					CapacityRange: &csi.CapacityRange{
						RequiredBytes: 10 * giB,
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
			volumeNameExists: true,
			volumeUUIDExists: true,
			wantErr:          false,
		},
		{
			name: "Test Clone Volume Size",
			args: args{
				&csi.CreateVolumeRequest{
					Name:               "testCloneVolume",
					VolumeCapabilities: caps,
					CapacityRange: &csi.CapacityRange{
						RequiredBytes: 10 * giB,
					},
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
				&csi.CreateVolumeRequest{
					Name:               "testCloneSnapshotVolume",
					VolumeCapabilities: caps,
					CapacityRange: &csi.CapacityRange{
						RequiredBytes: 10 * giB,
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
	for _, testCase := range tests {
		tt := testCase
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			d := newController(&mock.UpCloudServiceMock{
				VolumeNameExists: tt.volumeNameExists,
				VolumeUUIDExists: tt.volumeUUIDExists,
				StorageSize:      10,
				CloneStorageSize: 9, // set smaller size so that resize is triggered
			})
			gotResp, err := d.CreateVolume(context.Background(), tt.args.req)
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

func TestController_DeleteVolume(t *testing.T) {
	t.Parallel()
	type args struct {
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
				&csi.DeleteVolumeRequest{
					VolumeId: "testVolume",
				},
			},
			wantErr:  false,
			wantResp: &csi.DeleteVolumeResponse{},
		},
	}
	for _, testCase := range tests {
		tt := testCase
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			c := newController(nil)
			gotResp, err := c.DeleteVolume(context.Background(), tt.args.req)
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

func TestController_ListVolumes(t *testing.T) {
	t.Parallel()
	type args struct {
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
				&csi.ListVolumesRequest{},
			},
			wantErr: false,
		},
	}
	for _, testCase := range tests {
		tt := testCase
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			c := newController(nil)
			gotResp, err := c.ListVolumes(context.Background(), tt.args.req)
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

func TestController_ControllerUnpublishVolume(t *testing.T) {
	t.Parallel()
	type args struct {
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
				&csi.ControllerUnpublishVolumeRequest{
					VolumeId: "testVolume",
				},
			},
			wantErr: false,
		},
	}
	for _, testCase := range tests {
		tt := testCase
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			c := newController(nil)
			_, err := c.ControllerUnpublishVolume(context.Background(), tt.args.req)
			if (err != nil) != tt.wantErr {
				t.Errorf("ControllerUnpublishVolume() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
		})
	}
}

func TestController_ValidateVolumeCapabilities(t *testing.T) {
	t.Parallel()
	type args struct {
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
			want:    &csi.VolumeCapability_AccessMode{Mode: csi.VolumeCapability_AccessMode_SINGLE_NODE_WRITER},
			wantErr: false,
		},
	}
	for _, testCase := range tests {
		tt := testCase
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			c := newController(nil)
			got, err := c.ValidateVolumeCapabilities(context.Background(), tt.args.req)
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

func TestController_ExpandVolume(t *testing.T) {
	t.Parallel()
	c := newController(nil)
	wantBytes := int64(30 * giB)
	r, err := c.ControllerExpandVolume(context.Background(), &csi.ControllerExpandVolumeRequest{
		VolumeId: "test-vol",
		CapacityRange: &csi.CapacityRange{
			RequiredBytes: wantBytes,
			LimitBytes:    0,
		},
		// VolumeCapability:     &csi.VolumeCapability{},
	})
	if err != nil {
		t.Errorf("ControllerExpandVolume error = %v", err)
		return
	}
	if r.CapacityBytes != wantBytes {
		t.Errorf("CapacityBytes failed want %d got %d", wantBytes, r.CapacityBytes)
	}
}

func TestDriver_CreateSnapshot(t *testing.T) {
	t.Parallel()

	type args struct {
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
			name: "test without volume",
			args: args{
				req: &csi.CreateSnapshotRequest{
					SourceVolumeId: uuid.NewString(),
					Name:           "snappy",
				},
				volExists:    false,
				volBackingUp: false,
			},
			wantErr: false,
		},
		{
			name: "test with volume",
			args: args{
				req: &csi.CreateSnapshotRequest{
					SourceVolumeId: uuid.NewString(),
					Name:           "snappy",
				},
				volExists:    true,
				volBackingUp: false,
			},
			wantErr: false,
		},
		{
			name: "test with volume",
			args: args{
				req: &csi.CreateSnapshotRequest{
					SourceVolumeId: uuid.NewString(),
					Name:           "snappy",
				},
				volExists:    true,
				volBackingUp: true,
			},
			wantErr: false,
		},
		{
			name: "test without volume want err",
			args: args{
				req: &csi.CreateSnapshotRequest{
					SourceVolumeId: uuid.NewString(),
					Name:           "",
				},
				volExists:    false,
				volBackingUp: true,
			},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			d := newController(&mock.UpCloudServiceMock{
				VolumeUUIDExists: tt.args.volExists,
				StorageBackingUp: tt.args.volBackingUp,
				SourceVolumeID:   tt.args.req.SourceVolumeId,
			})

			_, err := d.CreateSnapshot(context.Background(), tt.args.req)
			if !tt.wantErr && err != nil {
				t.Fatalf("CreateSnapshot(%v) failed with %v", tt.args.req, err)
				return
			} else if tt.wantErr && err == nil {
				t.Fatalf("CreateSnapshot(%v) wanted err, but received nil", tt.args.req)
				return
			}
		})
	}
}
