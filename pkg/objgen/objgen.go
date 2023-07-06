package objgen

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"strings"

	"github.com/UpCloudLtd/upcloud-csi/deploy/kubernetes"

	snapsv1 "github.com/kubernetes-csi/external-snapshotter/client/v4/apis/volumesnapshot/v1"
	appsv1 "k8s.io/api/apps/v1"
	v1 "k8s.io/api/core/v1"
	rbacv1 "k8s.io/api/rbac/v1"
	storagev1 "k8s.io/api/storage/v1"
	extv1 "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	k8sJson "k8s.io/apimachinery/pkg/runtime/serializer/json"
	"sigs.k8s.io/cluster-api/cmd/clusterctl/client/yamlprocessor"
	"sigs.k8s.io/cluster-api/util/yaml"
)

type Output struct {
	RawYaml             []byte
	Objects             []runtime.Object
	UnstructuredObjects []*unstructured.Unstructured
}

func (o *Output) MarshalYAML() ([]byte, error) {
	serializer := k8sJson.NewSerializerWithOptions(
		k8sJson.DefaultMetaFactory, nil, nil,
		k8sJson.SerializerOptions{
			Yaml:   true,
			Pretty: true,
			Strict: true,
		},
	)
	var buf bytes.Buffer
	for _, obj := range o.Objects {
		if err := serializer.Encode(obj, &buf); err != nil {
			return buf.Bytes(), err
		}
		buf.Write([]byte("---\n"))
	}
	return buf.Bytes(), nil
}

func (o *Output) UnmarshalYAML(data []byte) error { //nolint: funlen // TODO: refactor
	o.RawYaml = data
	decoder := yaml.NewYAMLDecoder(io.NopCloser(bytes.NewReader(o.RawYaml)))
	defer decoder.Close()
	for {
		u := &unstructured.Unstructured{}
		_, gvk, err := decoder.Decode(nil, u)
		if errors.Is(err, io.EOF) {
			break
		}
		if runtime.IsNotRegisteredError(err) {
			continue
		}
		if err != nil {
			return err
		}

		var obj runtime.Object
		switch gvk.Kind {
		case "DaemonSet":
			obj = &appsv1.DaemonSet{}
		case "StatefulSet":
			obj = &appsv1.StatefulSet{}
		case "ClusterRoleBinding":
			obj = &rbacv1.ClusterRoleBinding{}
		case "ClusterRole":
			obj = &rbacv1.ClusterRole{}
		case "Role":
			obj = &rbacv1.Role{}
		case "RoleBinding":
			obj = &rbacv1.RoleBinding{}
		case "ServiceAccount":
			obj = &v1.ServiceAccount{}
		case "ConfigMap":
			obj = &v1.ConfigMap{}
		case "Secret":
			obj = &v1.Secret{}
		case "CSIDriver":
			obj = &storagev1.CSIDriver{}
		case "StorageClass":
			obj = &storagev1.StorageClass{}
		case "Deployment":
			obj = &appsv1.Deployment{}
		case "CustomResourceDefinition":
			obj = &extv1.CustomResourceDefinition{}
		case "VolumeSnapshotClass":
			obj = &snapsv1.VolumeSnapshotClass{}
		case "VolumeSnapshotContent":
			obj = &snapsv1.VolumeSnapshotContent{}
		case "VolumeSnapshots":
			obj = &snapsv1.VolumeSnapshot{}

		default:
			o.UnstructuredObjects = append(o.UnstructuredObjects, u)
			continue
		}

		if err := runtime.DefaultUnstructuredConverter.FromUnstructured(u.Object, obj); err != nil {
			return fmt.Errorf("cannot convert object to %s: %w", gvk.Kind, err)
		}

		o.Objects = append(o.Objects, obj)
	}
	return nil
}

func Get(vars map[string]string, template ...string) (*Output, error) {
	if len(template) == 0 {
		template = []string{kubernetes.SecretsTemplate, kubernetes.CRDTemplate, kubernetes.RbacTemplate, kubernetes.CSITemplate}
	}

	data, err := renderYAML(strings.Join(template, "---\n"), vars)
	if err != nil {
		return nil, fmt.Errorf("can't substitute variables in cluster template: %w", err)
	}
	output := Output{}
	return &output, output.UnmarshalYAML(data)
}

func renderYAML(template string, vars map[string]string) ([]byte, error) {
	processor := yamlprocessor.NewSimpleProcessor()
	variableValueGetter := func(key string) (string, error) {
		val, exist := vars[key]

		if !exist {
			return "", fmt.Errorf("variable %s is not defined in input variables", key)
		}
		return val, nil
	}
	return processor.Process([]byte(template), variableValueGetter)
}
