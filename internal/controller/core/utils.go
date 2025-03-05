package core

import (
	"fmt"
	"reflect"

	"github.com/go-logr/logr"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"

	openmcpv1alpha1 "github.com/openmcp-project/project-workspace-operator/api/core/v1alpha1"
)

func namespaceForProject(project *openmcpv1alpha1.Project) string {
	return fmt.Sprintf("project-%s", project.Name)
}

func namespaceForWorkspace(workspace *openmcpv1alpha1.Workspace) string {
	return fmt.Sprintf("%s--ws-%s", workspace.Namespace, workspace.Name)
}

// wasDeleted returns true if the supplied object was deleted from the API server.
func wasDeleted(o metav1.Object) bool {
	return !o.GetDeletionTimestamp().IsZero()
}

// setMetaDataLabel sets the key value pair in the labels section of the given Object.
// If the given Object did not yet have labels, they are initialized.
func setMetaDataLabel(meta metav1.Object, key, value string) { // TODO move to utils package
	labels := meta.GetLabels()
	if labels == nil {
		labels = make(map[string]string)
	}
	labels[key] = value
	meta.SetLabels(labels)
}

func logOperationResult(log logr.Logger, obj client.Object, result controllerutil.OperationResult) {
	objType := reflect.ValueOf(obj).Elem().Type()
	if obj.GetNamespace() == "" {
		log.Info(fmt.Sprintf("%s %s %s", objType.Name(), obj.GetName(), result))
	} else {
		log.Info(fmt.Sprintf("%s %s/%s %s", objType.Name(), obj.GetNamespace(), obj.GetName(), result))
	}
}
