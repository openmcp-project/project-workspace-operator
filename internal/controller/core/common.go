package core

import (
	"context"
	"fmt"
	"reflect"
	"time"

	"k8s.io/apimachinery/pkg/util/json"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/log"

	"github.com/openmcp-project/controller-utils/pkg/controller/smartrequeue"

	pwv1alpha1 "github.com/openmcp-project/platform-service-project-workspace/api/core/v1alpha1"
	"github.com/openmcp-project/platform-service-project-workspace/api/install"
	sharedconfig "github.com/openmcp-project/platform-service-project-workspace/internal/controller/config"
	"github.com/openmcp-project/platform-service-project-workspace/internal/controller/core/config"
	"github.com/openmcp-project/platform-service-project-workspace/internal/utils"
)

var (
	Scheme = runtime.NewScheme()

	deleteFinalizer = pwv1alpha1.GroupVersion.Group

	ControllerName = "project-workspace"
)

func init() {
	install.InstallOperatorAPIsOnboarding(Scheme)
}

type CommonReconciler struct {
	Config       sharedconfig.SharedInformation
	ProviderName string
	sr           *smartrequeue.Store // the store's key includes kind, so we can use one store for both Projects and Workspaces
}

func NewCommonReconciler(config sharedconfig.SharedInformation, providerName string) *CommonReconciler {
	return &CommonReconciler{
		Config:       config,
		ProviderName: providerName,
		sr:           smartrequeue.NewStore(5*time.Second, 24*time.Hour, 1.2),
	}
}

func (r *CommonReconciler) ensureFinalizer(ctx context.Context, o client.Object) error {
	if !controllerutil.ContainsFinalizer(o, deleteFinalizer) {
		onboardingCluster, err := r.Config.OnboardingClusterStatic(ctx)
		if err != nil {
			return fmt.Errorf("failed to get onboarding cluster access: %w", err)
		}
		controllerutil.AddFinalizer(o, deleteFinalizer)
		if err := onboardingCluster.Client().Update(ctx, o); err != nil {
			return fmt.Errorf("failed to add finalizer: %w", err)
		}
	}

	return nil
}

func (r *CommonReconciler) handleRemainingContentBeforeDelete(ctx context.Context, o client.Object) (bool, error) {
	if !utils.WasDeleted(o) {
		return false, nil
	}

	project, isProject := o.(*pwv1alpha1.Project)
	workspace, isWorkspace := o.(*pwv1alpha1.Workspace)

	if !isProject && !isWorkspace {
		return false, fmt.Errorf("object is not a Project or Workspace")
	}

	var namespace string
	var resourcesBlockingDeletion []sharedconfig.DeletionBlockingResource
	var err error

	if isProject {
		namespace = project.Status.Namespace

		resourcesBlockingDeletion, err = r.Config.ResourcesBlockingProjectDeletion(ctx)
		if err != nil {
			return false, fmt.Errorf("failed to get resources blocking project deletion: %w", err)
		}
		if len(resourcesBlockingDeletion) == 0 {
			return false, nil
		}
	} else {
		namespace = workspace.Status.Namespace

		resourcesBlockingDeletion, err = r.Config.ResourcesBlockingWorkspaceDeletion(ctx)
		if err != nil {
			return false, fmt.Errorf("failed to get resources blocking workspace deletion: %w", err)
		}
		if len(resourcesBlockingDeletion) == 0 {
			return false, nil
		}
	}

	remainingResources := make([]unstructured.Unstructured, 0)
	var remainingResourcesCondition pwv1alpha1.Condition

	log := log.FromContext(ctx)
	onboardingCluster, err := r.Config.OnboardingClusterDynamic(ctx)
	if err != nil {
		return false, fmt.Errorf("failed to get onboarding cluster access: %w", err)
	}

	for _, br := range resourcesBlockingDeletion {
		resList := &unstructured.UnstructuredList{}
		resList.SetGroupVersionKind(config.ToSchemaGVK(br.GroupVersionKind))

		if err := onboardingCluster.Client().List(ctx, resList, client.InNamespace(namespace)); err != nil {
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
func (r *CommonReconciler) handleDelete(ctx context.Context, o client.Object, deleteFunc func() error) (bool, RequeueType, error) {
	if !utils.WasDeleted(o) {
		return false, NoRequeue, nil
	}

	log := log.FromContext(ctx)
	onboardingCluster, err := r.Config.OnboardingClusterStatic(ctx)
	if err != nil {
		return false, RequeueError, fmt.Errorf("failed to get onboarding cluster access: %w", err)
	}

	if controllerutil.ContainsFinalizer(o, deleteFinalizer) {
		if err := deleteFunc(); err != nil {
			if rrErr, ok := err.(ResourcesRemainingError); ok {
				log.Info(rrErr.Error())
				return true, RequeueWithBackoff, nil
			}

			return false, RequeueError, fmt.Errorf("failed to perform cleanup operation: %w", err)
		}

		controllerutil.RemoveFinalizer(o, deleteFinalizer)
		if err := onboardingCluster.Client().Update(ctx, o); err != nil {
			return false, RequeueError, fmt.Errorf("failed to remove finalizer: %w", err)
		}
	}

	return true, NoRequeue, nil
}

func (r *CommonReconciler) applyManagementLabel(obj metav1.Object) {
	utils.SetManagementLabels(obj, r.ProviderName)
}

type RequeueType int

const (
	RequeueError RequeueType = iota
	RequeueWithMinInterval
	RequeueWithBackoff
	NoRequeue
)

var _ error = ResourcesRemainingError{}

type ResourcesRemainingError struct{}

func (err ResourcesRemainingError) Error() string {
	return "cleanup not finished yet due to remaining resources"
}

func (err ResourcesRemainingError) Is(target error) bool {
	return reflect.TypeOf(target) == reflect.TypeOf(err)
}
