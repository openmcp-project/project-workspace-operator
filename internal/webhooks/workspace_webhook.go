package webhooks

import (
	"context"
	"fmt"
	"strings"

	"k8s.io/apimachinery/pkg/runtime"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/webhook/admission"

	"github.com/openmcp-project/controller-utils/pkg/logging"

	pwv1alpha1 "github.com/openmcp-project/platform-service-project-workspace/api/core/v1alpha1"
	"github.com/openmcp-project/platform-service-project-workspace/internal/controller/config"
)

const WorkspaceWebhookName = "workspace-webhook"

// +kubebuilder:object:generate=false
type WorkspaceWebhook struct {
	client.Client

	// Identity is the name of the entity (usually a service account) the platform-service-project-workspace uses to access the onboarding cluster.
	// It is required to exclude the operator's own identity from validation checks.
	Identity          string
	SharedInformation config.SharedInformation
}

func SetupWorkspaceWebhookWithManager(ctx context.Context, mgr ctrl.Manager, identity string, si config.SharedInformation) error {
	wswh := &WorkspaceWebhook{
		Client:            mgr.GetClient(),
		SharedInformation: si,
		Identity:          identity,
	}

	return ctrl.NewWebhookManagedBy(mgr, &pwv1alpha1.Workspace{}).
		WithDefaulter(wswh).
		WithValidator(wswh).
		Complete()
}

// +kubebuilder:webhook:path=/mutate-core-openmcp-cloud-v1alpha1-workspace,mutating=true,failurePolicy=fail,sideEffects=None,groups=core.openmcp.cloud,resources=workspaces,verbs=create;update,versions=v1alpha1,name=mworkspace.openmcp.cloud,admissionReviewVersions=v1

var _ admission.Defaulter[*pwv1alpha1.Workspace] = &WorkspaceWebhook{}

// Default implements admission.Defaulter so a webhook will be registered for the type
func (w *WorkspaceWebhook) Default(ctx context.Context, obj *pwv1alpha1.Workspace) error {
	workspace, err := expectWorkspace(obj)
	if err != nil {
		return err
	}

	req, err := admission.RequestFromContext(ctx)
	if err != nil {
		return err
	}

	setCreatedBy(workspace, req)

	return nil
}

// +kubebuilder:webhook:path=/validate-core-openmcp-cloud-v1alpha1-workspace,mutating=false,failurePolicy=fail,sideEffects=None,groups=core.openmcp.cloud,resources=workspaces,verbs=create;update;delete,versions=v1alpha1,name=vworkspace.openmcp.cloud,admissionReviewVersions=v1

var _ admission.Validator[*pwv1alpha1.Workspace] = &WorkspaceWebhook{}

// ValidateCreate implements admission.Validator so a webhook will be registered for the type
func (v *WorkspaceWebhook) ValidateCreate(ctx context.Context, obj *pwv1alpha1.Workspace) (warnings admission.Warnings, err error) {
	log := logging.FromContextOrPanic(ctx).WithName(WorkspaceWebhookName)
	ctx = logging.NewContext(ctx, log)
	workspace, err := expectWorkspace(obj)
	if err != nil {
		return
	}
	log.Info("Validate create")

	userInfo, err := userInfoFromContext(ctx)
	if err != nil {
		return
	}
	validRole, err := v.ensureValidRole(ctx, workspace)
	if err != nil {
		return warnings, err
	}
	if !validRole {
		return warnings, errRequestingUserNoAccess(userInfo.Username)
	}

	return
}

// ValidateUpdate implements admission.Validator so a webhook will be registered for the type
func (v *WorkspaceWebhook) ValidateUpdate(ctx context.Context, oldObj, newObj *pwv1alpha1.Workspace) (warnings admission.Warnings, err error) {
	log := logging.FromContextOrPanic(ctx).WithName(WorkspaceWebhookName)
	ctx = logging.NewContext(ctx, log)
	oldWorkspace, err := expectWorkspace(oldObj)
	if err != nil {
		return
	}

	newWorkspace, err := expectWorkspace(newObj)
	if err != nil {
		return
	}

	log.Info("Validate update")

	if err = verifyCreatedByUnchanged(oldWorkspace, newWorkspace); err != nil {
		return
	}

	userInfo, err := userInfoFromContext(ctx)
	if err != nil {
		return
	}
	validRole, err := v.ensureValidRole(ctx, oldWorkspace)
	if err != nil {
		return warnings, err
	}
	if !validRole {
		return warnings, errRequestingUserNoAccess(userInfo.Username)
	}
	// ensure the user can't remove themselves from the member list
	validNewRole, err := v.ensureValidRole(ctx, newWorkspace)
	if err != nil {
		return warnings, err
	}
	if !validNewRole {
		return warnings, errRequestingUserNoAccess(userInfo.Username)
	}

	return
}

// ValidateDelete implements admission.Validator so a webhook will be registered for the type
func (v *WorkspaceWebhook) ValidateDelete(ctx context.Context, obj *pwv1alpha1.Workspace) (warnings admission.Warnings, err error) {
	log := logging.FromContextOrPanic(ctx).WithName(WorkspaceWebhookName)
	ctx = logging.NewContext(ctx, log)
	workspace, err := expectWorkspace(obj)
	if err != nil {
		return
	}

	log.Info("Validate delete")

	if validRole, err := v.ensureValidRole(ctx, workspace); !validRole {
		return warnings, err
	}
	return
}

// expectWorkspace casts the given runtime.Object to *Workspace. Returns an error in case the object can't be casted.
func expectWorkspace(obj runtime.Object) (*pwv1alpha1.Workspace, error) {
	workspace, ok := obj.(*pwv1alpha1.Workspace)
	if !ok {
		return nil, fmt.Errorf("expected a Workspace but got a %T", obj)
	}
	return workspace, nil
}

func (v *WorkspaceWebhook) ensureValidRole(ctx context.Context, workspace *pwv1alpha1.Workspace) (bool, error) {
	userInfo, err := userInfoFromContext(ctx)
	if err != nil {
		return false, fmt.Errorf("failed to get userInfo")
	}
	if workspace.UserInfoHasRole(userInfo, pwv1alpha1.WorkspaceRoleAdmin) || userInfo.Username == v.Identity {
		return true, nil
	}

	overrides, err := v.SharedInformation.MemberOverrides(ctx)
	if err != nil {
		return false, fmt.Errorf("failed to get member overrides: %w", err)
	}

	if !overrides.HasAdminOverrideForResource(&userInfo, workspace.Name, workspace.Kind) {
		return false, nil
	}
	// slightly hacky way to get parent project name
	projectName, ok := strings.CutPrefix(workspace.Namespace, "project-")
	if !ok || projectName == "" {
		return false, fmt.Errorf("failed to get Workspace Project name")
	}

	projectGVK := pwv1alpha1.GroupVersion.WithKind("Project")

	// the subject must have admin access for the parent project as well.
	if overrides.HasAdminOverrideForResource(&userInfo, projectName, projectGVK.Kind) {
		return true, nil
	}

	return false, nil
}
