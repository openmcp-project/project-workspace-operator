package core

import (
	"context"
	"fmt"
	"reflect"
	"strings"

	"k8s.io/apimachinery/pkg/util/json"

	"github.com/openmcp-project/project-workspace-operator/internal/controller/core/config"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	utilruntime "k8s.io/apimachinery/pkg/util/runtime"
	clientgoscheme "k8s.io/client-go/kubernetes/scheme"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	pwv1alpha1 "github.com/openmcp-project/project-workspace-operator/api/core/v1alpha1"
	"github.com/openmcp-project/project-workspace-operator/api/entities"
)

var (
	Scheme = runtime.NewScheme()

	deleteFinalizer = pwv1alpha1.GroupVersion.Group

	ControllerName = "project-workspace-operator"
)

func init() {
	utilruntime.Must(clientgoscheme.AddToScheme(Scheme))
	utilruntime.Must(pwv1alpha1.AddToScheme(Scheme))
}

type CommonReconciler struct {
	client.Client
	pwv1alpha1.ProjectWorkspaceConfigSpec
	ControllerName string
}

func (r *CommonReconciler) ensureFinalizer(ctx context.Context, o client.Object) error {
	if !controllerutil.ContainsFinalizer(o, deleteFinalizer) {
		controllerutil.AddFinalizer(o, deleteFinalizer)
		if err := r.Update(ctx, o); err != nil {
			return fmt.Errorf("failed to add finalizer: %w", err)
		}
	}

	return nil
}

func (r *CommonReconciler) handleRemainingContentBeforeDelete(ctx context.Context, o client.Object) (bool, error) {
	if !wasDeleted(o) {
		return false, nil
	}

	project, isProject := o.(*pwv1alpha1.Project)
	workspace, isWorkspace := o.(*pwv1alpha1.Workspace)

	if !isProject && !isWorkspace {
		return false, fmt.Errorf("object is not a Project or Workspace")
	}

	var namespace string
	var resourcesBlockingDeletion []metav1.GroupVersionKind

	if isProject {
		namespace = project.Status.Namespace

		if len(r.Project.ResourcesBlockingDeletion) == 0 {
			return false, nil
		}

		resourcesBlockingDeletion = r.Project.ResourcesBlockingDeletion
	} else {
		namespace = workspace.Status.Namespace

		if len(r.Workspace.ResourcesBlockingDeletion) == 0 {
			return false, nil
		}

		resourcesBlockingDeletion = r.Workspace.ResourcesBlockingDeletion
	}

	remainingResources := make([]unstructured.Unstructured, 0)
	var remainingResourcesCondition pwv1alpha1.Condition

	log := log.FromContext(ctx)

	for _, gvk := range resourcesBlockingDeletion {
		resList := &unstructured.UnstructuredList{}
		resList.SetGroupVersionKind(config.ToSchemaGVK(gvk))

		if err := r.List(ctx, resList, client.InNamespace(namespace)); err != nil {
			log.Error(err, "failed to list resources")
			return false, err
		}

		if len(resList.Items) > 0 {
			remainingResources = append(remainingResources, resList.Items...)
		}
	}

	if len(remainingResources) > 0 {
		resources := make([]pwv1alpha1.RemainingContentResource, 0, len(remainingResources))

		remainingResourcesCondition = pwv1alpha1.Condition{
			Type:    pwv1alpha1.ConditionTypeContentRemaining,
			Status:  pwv1alpha1.ConditionStatusTrue,
			Reason:  pwv1alpha1.ConditionReasonResourcesRemaining,
			Message: fmt.Sprintf("There are %d remaining resources in namespace %s that are preventing deletion", len(remainingResources), namespace),
		}

		for _, res := range remainingResources {
			resources = append(resources, pwv1alpha1.RemainingContentResource{
				APIGroup:  res.GetAPIVersion(),
				Kind:      res.GetKind(),
				Name:      res.GetName(),
				Namespace: res.GetNamespace(),
			})
		}

		resourcesMarshalled, err := json.Marshal(resources)
		if err != nil {
			log.Error(err, "failed to marshal resources")
			return false, err
		}

		remainingResourcesCondition.Details = resourcesMarshalled

		if isProject {
			project.SetOrUpdateCondition(remainingResourcesCondition)
		} else {
			workspace.SetOrUpdateCondition(remainingResourcesCondition)
		}

		return true, nil
	} else {
		if isProject {
			project.RemoveCondition(pwv1alpha1.ConditionTypeContentRemaining)
		} else {
			workspace.RemoveCondition(pwv1alpha1.ConditionTypeContentRemaining)
		}
	}

	return false, nil
}
func (r *CommonReconciler) handleDelete(ctx context.Context, o client.Object, deleteFunc func() error) (bool, ctrl.Result, error) {
	if !wasDeleted(o) {
		return false, reconcile.Result{}, nil
	}

	log := log.FromContext(ctx)

	if controllerutil.ContainsFinalizer(o, deleteFinalizer) {
		if err := deleteFunc(); err != nil {
			if rrErr, ok := err.(ResourcesRemainingError); ok {
				log.Info(rrErr.Error())
				return true, reconcile.Result(rrErr), nil
			}

			return false, reconcile.Result{}, fmt.Errorf("failed to perform cleanup operation: %w", err)
		}

		controllerutil.RemoveFinalizer(o, deleteFinalizer)
		if err := r.Update(ctx, o); err != nil {
			return false, reconcile.Result{}, fmt.Errorf("failed to remove finalizer: %w", err)
		}
	}

	return true, reconcile.Result{}, nil
}

func (r *CommonReconciler) applyManagementLabel(obj metav1.Object) {
	setManagementLabel(obj, r.ControllerName)
}

var _ error = ResourcesRemainingError{}

type ResourcesRemainingError ctrl.Result

func (err ResourcesRemainingError) Error() string {
	return fmt.Sprintf("cleanup is not finished yet because there are remaining resources. should check again in %s", err.RequeueAfter)
}

func (err ResourcesRemainingError) Is(target error) bool {
	return reflect.TypeOf(target) == reflect.TypeOf(err)
}

func clusterRoleForEntityAndRole(entity entities.AccessEntity, role entities.AccessRole) string {
	if reflect.TypeOf(entity) != reflect.TypeOf(role.EntityType()) {
		panic("AccessEntity/AccessRole mismatch")
	}
	return strings.Join([]string{
		entity.TypeIdentifier(),
		entity.GetName(),
		role.Identifier(),
	}, ":")
}

func clusterRoleForRole(role entities.AccessRole) string {
	return strings.Join([]string{
		role.EntityType().TypeIdentifier(),
		role.Identifier(),
	}, "-")
}

func roleBindingForRole(role entities.AccessRole) string {
	// Name of RoleBinding (namespaced) should be the same as ClusterRole.
	return clusterRoleForRole(role)
}

func clusterRoleForEntityAndRoleWithParent(entity entities.AccessEntity, role entities.AccessRole, parent entities.AccessEntity) string {
	if reflect.TypeOf(entity) == reflect.TypeOf(parent) {
		panic("AccessEntity/Parent must not be of same type")
	}
	return strings.Join([]string{
		parent.TypeIdentifier(),
		parent.GetName(),
		clusterRoleForEntityAndRole(entity, role),
	}, ":")
}
