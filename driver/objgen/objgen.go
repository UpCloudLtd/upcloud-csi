package objgen

import (
	"errors"
	"fmt"

	. "github.com/UpCloudLtd/upcloud-csi/deploy/kubernetes"

	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"sigs.k8s.io/cluster-api/cmd/clusterctl/client/yamlprocessor"
)

type Output struct {
	RawYaml             []byte
	UnstructuredObjects []*unstructured.Unstructured
	//Not yet ready
	//DaemonSet           []*appsv1.DaemonSet
	//ClusterRoleBinding  []*rbacv1.ClusterRoleBinding
	//ClusterRole         []*rbacv1.ClusterRole
	//ServiceAccount      []*v1.ServiceAccount
	//ConfigMap           []*v1.ConfigMap
	//Secret              []*v1.Secret
}

func Get(vars map[string]string) (*Output, error) {
	return GetTemplate(vars, SecretsTemplate, CRDTemplate, RbacTemplate, CSITemplate)
}

func GetTemplate(vars map[string]string, template ...string) (*Output, error) {
	if len(template) == 0 {
		return nil, errors.New("template(s) missing")
	}
	var (
		rawYaml []byte
		err     error
	)

	// First substitute variables to values
	processor := yamlprocessor.NewSimpleProcessor()

	variableValueGetter := func(key string) (string, error) {
		val, exist := vars[key]

		if !exist {
			return "", fmt.Errorf("variable %s is not defined in input variables", key)
		}
		return val, nil
	}

	processTemplate := func(template string) ([]byte, error) {
		vars, err := processor.GetVariableMap([]byte(template))
		if err != nil {
			return nil, fmt.Errorf("can't parse variable list from template: %w", err)
		}

		for key := range vars {
			if _, exist := vars[key]; !exist {
				return nil, fmt.Errorf("variable %s is not defined in input variables", key)
			}
		}

		processedTemplate, err := processor.Process([]byte(template), variableValueGetter)
		if err != nil {
			return nil, fmt.Errorf("can't substitute variables in cluster template: %w", err)
		}

		processedTemplate = append(processedTemplate, []byte("---\n")...)

		return processedTemplate, nil

	}

	output := &Output{}

	for i, t := range template {
		rawYaml, err = processTemplate(t)
		if err != nil {
			return nil, fmt.Errorf("can't parse template #%d: %w", i, err)
		}

		output.RawYaml = append(output.RawYaml, rawYaml...)
	}
	//for {
	//	u := &unstructured.Unstructured{}
	//	_, gvk, err := decoder.Decode(nil, u)
	//	if err == io.EOF {
	//		break
	//	}
	//	if runtime.IsNotRegisteredError(err) {
	//		continue
	//	}
	//	if err != nil {
	//		return nil, err
	//	}
	//
	//	switch gvk.Kind {
	//
	//	case "DaemonSet":
	//		obj := &appsv1.DaemonSet{}
	//		if err := runtime.DefaultUnstructuredConverter.FromUnstructured(u.Object, obj); err != nil {
	//			return nil, fmt.Errorf("cannot convert object to %s: %w", gvk.Kind, err)
	//		}
	//		output.DaemonSet = append(output.DaemonSet, obj)
	//
	//	case "ClusterRoleBinding":
	//		obj := &rbacv1.ClusterRoleBinding{}
	//		if err := runtime.DefaultUnstructuredConverter.FromUnstructured(u.Object, obj); err != nil {
	//			return nil, fmt.Errorf("cannot convert object to %s: %w", gvk.Kind, err)
	//		}
	//		output.ClusterRoleBinding = append(output.ClusterRoleBinding, obj)
	//
	//	case "ClusterRole":
	//		obj := &rbacv1.ClusterRole{}
	//		if err := runtime.DefaultUnstructuredConverter.FromUnstructured(u.Object, obj); err != nil {
	//			return nil, fmt.Errorf("cannot convert object to %s: %w", gvk.Kind, err)
	//		}
	//		output.ClusterRole = append(output.ClusterRole, obj)
	//
	//	case "ServiceAccount":
	//		obj := &v1.ServiceAccount{}
	//		if err := runtime.DefaultUnstructuredConverter.FromUnstructured(u.Object, obj); err != nil {
	//			return nil, fmt.Errorf("cannot convert object to %s: %w", gvk.Kind, err)
	//		}
	//		output.ServiceAccount = append(output.ServiceAccount, obj)
	//
	//	case "ConfigMap":
	//		obj := &v1.ConfigMap{}
	//		if err := runtime.DefaultUnstructuredConverter.FromUnstructured(u.Object, obj); err != nil {
	//			return nil, fmt.Errorf("cannot convert object to %s: %w", gvk.Kind, err)
	//		}
	//		output.ConfigMap = append(output.ConfigMap, obj)
	//
	//	case "Secret":
	//		obj := &v1.Secret{}
	//		if err := runtime.DefaultUnstructuredConverter.FromUnstructured(u.Object, obj); err != nil {
	//			return nil, fmt.Errorf("cannot convert object to %s: %w", gvk.Kind, err)
	//		}
	//		output.Secret = append(output.Secret, obj)
	//
	//	default:
	//		output.UnstructuredObjects = append(output.UnstructuredObjects, u)
	//	}
	//}

	return output, nil
}
