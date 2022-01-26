package driver

import (
	"context"
	"github.com/sirupsen/logrus"

	"github.com/container-storage-interface/spec/lib/go/csi"
	"github.com/golang/protobuf/ptypes/wrappers"
)

type IdentityService struct {
	Driver *Driver
	csi.IdentityServer
}

// GetPluginInfo returns metadata of the plugin
func (identity *IdentityService) GetPluginInfo(ctx context.Context, req *csi.GetPluginInfoRequest) (*csi.GetPluginInfoResponse, error) {
	resp := &csi.GetPluginInfoResponse{
		Name: identity.Driver.name,
	}

	identity.Driver.log.WithFields(logrus.Fields{
		"response": resp,
		"method":   "get_plugin_info",
	}).Info("return plugin info")

	return resp, nil
}

// GetPluginCapabilities returns available capabilities of the plugin
func (identity *IdentityService) GetPluginCapabilities(ctx context.Context, req *csi.GetPluginCapabilitiesRequest) (*csi.GetPluginCapabilitiesResponse, error) {
	resp := &csi.GetPluginCapabilitiesResponse{
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
	}

	identity.Driver.log.WithFields(logrus.Fields{
		"response": resp,
		"method":   "get_plugin_capabilities",
	}).Info("get plugin capabitilies called")

	return resp, nil
}

// Probe returns the health and readiness of the plugin
func (identity *IdentityService) Probe(ctx context.Context, req *csi.ProbeRequest) (*csi.ProbeResponse, error) {
	identity.Driver.log.WithField("method", "probe").Info("check whether the plugin is ready")
	identity.Driver.readyMu.Lock()
	defer identity.Driver.readyMu.Unlock()

	return &csi.ProbeResponse{
		Ready: &wrappers.BoolValue{
			Value: identity.Driver.ready,
		},
	}, nil
}
