package controller

import (
	"strings"

	"github.com/UpCloudLtd/upcloud-go-api/v8/upcloud"
)

func upcloudLabels(labels []string) []upcloud.Label {
	r := make([]upcloud.Label, 0)
	for _, l := range labels {
		if l == "" {
			continue
		}
		c := strings.SplitN(l, "=", 2)
		if len(c) == 2 {
			r = append(r, upcloud.Label{Key: c[0], Value: c[1]})
		}
	}
	return r
}
