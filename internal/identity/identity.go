package identity

import (
	"context"

	"github.com/container-storage-interface/spec/lib/go/csi"
	"github.com/golang/protobuf/ptypes/wrappers"
	"github.com/sirupsen/logrus"
)

type Identity struct {
	driverName string
	ready      bool
	log        *logrus.Entry
}

func NewIdentity(driverName string, l *logrus.Entry) *Identity {
	return &Identity{driverName: driverName, ready: true, log: l}
}

// GetPluginInfo returns metadata of the plugin.
func (i *Identity) GetPluginInfo(ctx context.Context, req *csi.GetPluginInfoRequest) (*csi.GetPluginInfoResponse, error) {
	return &csi.GetPluginInfoResponse{
		Name: i.driverName,
	}, nil
}

// GetPluginCapabilities returns available capabilities of the plugin.
func (i *Identity) GetPluginCapabilities(ctx context.Context, req *csi.GetPluginCapabilitiesRequest) (*csi.GetPluginCapabilitiesResponse, error) {
	return &csi.GetPluginCapabilitiesResponse{
		Capabilities: []*csi.PluginCapability{
			{
				Type: &csi.PluginCapability_Service_{
					Service: &csi.PluginCapability_Service{
						Type: csi.PluginCapability_Service_CONTROLLER_SERVICE,
					},
				},
			},
			{
				Type: &csi.PluginCapability_Service_{
					Service: &csi.PluginCapability_Service{
						Type: csi.PluginCapability_Service_VOLUME_ACCESSIBILITY_CONSTRAINTS,
					},
				},
			},
			{
				Type: &csi.PluginCapability_VolumeExpansion_{
					VolumeExpansion: &csi.PluginCapability_VolumeExpansion{
						Type: csi.PluginCapability_VolumeExpansion_OFFLINE,
					},
				},
			},
		},
	}, nil
}

// Probe returns the health and readiness of the plugin.
func (i *Identity) Probe(ctx context.Context, req *csi.ProbeRequest) (*csi.ProbeResponse, error) {
	i.log.WithField("method", "probe").Info("check whether the plugin is ready")

	return &csi.ProbeResponse{
		Ready: &wrappers.BoolValue{
			// TODO: should we check e.g. that network is available or that all the cli tools are available ?
			Value: i.ready,
		},
	}, nil
}
