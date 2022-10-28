package driver

import (
	"fmt"
	"strconv"
	"strings"

	"github.com/container-storage-interface/spec/lib/go/csi"

	"k8s.io/apimachinery/pkg/util/sets"
)

const (
	_   = iota
	kiB = 1 << (10 * iota)
	miB
	giB
	tiB

	// minimumVolumeSizeInBytes is used to validate that the user is not trying
	// to create a volume that is smaller than what we support.
	minimumVolumeSizeInBytes int64 = 10 * giB

	// maximumVolumeSizeInBytes is used to validate that the user is not trying
	// to create a volume that is larger than what we support.
	maximumVolumeSizeInBytes int64 = 4096 * giB

	// defaultVolumeSize is used when the user did not provide a size or
	// the size they provided did not satisfy our requirements.
	defaultVolumeSize = 10 * giB
)

var supportedAccessMode = &csi.VolumeCapability_AccessMode{Mode: csi.VolumeCapability_AccessMode_SINGLE_NODE_WRITER} //nolint: gochecknoglobals // supportedAccessMode is readonly variable

type storageRange struct {
	requiredBytes int64
	requiredSet   bool
	limitBytes    int64
	limitSet      bool
}

// TODO reword/rework
// getStorageRange extracts the storage size in bytes from the given capacity
// range. If the capacity range is not satisfied it returns the default volume
// size. If the capacity range is below or above supported sizes, it returns an
// error.
func getStorageRange(cr *csi.CapacityRange) (int64, error) {
	if cr == nil {
		return defaultVolumeSize, nil
	}

	sr := &storageRange{
		requiredBytes: cr.GetRequiredBytes(),
		requiredSet:   0 < cr.GetRequiredBytes(),
		limitBytes:    cr.GetLimitBytes(),
		limitSet:      0 < cr.GetLimitBytes(),
	}

	if !sr.requiredSet && !sr.limitSet {
		return defaultVolumeSize, nil
	}

	if sr.requiredSet && sr.limitSet && sr.limitBytes < sr.requiredBytes {
		return 0, fmt.Errorf("required bytes %d is greater than limit bytes %d", sr.requiredBytes, sr.limitBytes)
	}

	if sr.requiredSet && !sr.limitSet && sr.requiredBytes < minimumVolumeSizeInBytes {
		return 0, fmt.Errorf("required (%v) can not be less than minimum supported volume size (%v)", displayByteString(sr.requiredBytes), displayByteString(minimumVolumeSizeInBytes))
	}

	if sr.limitSet && sr.limitBytes < minimumVolumeSizeInBytes {
		return 0, fmt.Errorf("limit (%v) can not be less than minimum supported volume size (%v)", displayByteString(sr.limitBytes), displayByteString(minimumVolumeSizeInBytes))
	}

	if sr.requiredSet && sr.requiredBytes > maximumVolumeSizeInBytes {
		return 0, fmt.Errorf("required (%v) can not exceed maximum supported volume size (%v)", displayByteString(sr.requiredBytes), displayByteString(maximumVolumeSizeInBytes))
	}

	if !sr.requiredSet && sr.limitSet && sr.limitBytes > maximumVolumeSizeInBytes {
		return 0, fmt.Errorf("limit (%v) can not exceed maximum supported volume size (%v)", displayByteString(sr.limitBytes), displayByteString(maximumVolumeSizeInBytes))
	}

	if sr.requiredSet && sr.limitSet && sr.requiredBytes == sr.limitBytes {
		return sr.requiredBytes, nil
	}

	if sr.requiredSet {
		return sr.requiredBytes, nil
	}

	if sr.limitSet {
		return sr.limitBytes, nil
	}

	return defaultVolumeSize, nil
}

// displayByteString takes a byte representation of storage size and returns a human-readable string: (1 GiB).
func displayByteString(bytes int64) string {
	output := float64(bytes)
	unit := ""

	switch {
	case bytes >= tiB:
		output /= tiB
		unit = "Ti"
	case bytes >= giB:
		output /= giB
		unit = "Gi"
	case bytes >= miB:
		output /= miB
		unit = "Mi"
	case bytes >= kiB:
		output /= kiB
		unit = "Ki"
	case bytes == 0:
		return "0"
	}

	result := strconv.FormatFloat(output, 'f', 1, 64)
	result = strings.TrimSuffix(result, ".0")
	return result + unit
}

// TODO reword...
// validateCapabilities validates the requested capabilities. It returns a list
// of violations which may be empty if no violatons were found.
func validateCapabilities(capacities []*csi.VolumeCapability) []string {
	violations := sets.NewString()
	for _, capacity := range capacities {
		if capacity.GetAccessMode().GetMode() != supportedAccessMode.GetMode() {
			violations.Insert(fmt.Sprintf("unsupported access mode %s", capacity.GetAccessMode().GetMode().String()))
		}

		accessType := capacity.GetAccessType()
		switch accessType.(type) {
		case *csi.VolumeCapability_Block:
		case *csi.VolumeCapability_Mount:
		default:
			violations.Insert("unsupported access type")
		}
	}

	return violations.List()
}
