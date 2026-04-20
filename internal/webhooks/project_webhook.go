package webhooks

import (
	"context"
	"fmt"

	"k8s.io/apimachinery/pkg/runtime"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/webhook/admission"

	"github.com/openmcp-project/controller-utils/pkg/logging"

	pwv1alpha1 "github.com/openmcp-project/platform-service-project-workspace/api/v2/core/v1alpha1"
	"github.com/openmcp-project/platform-service-project-workspace/internal/controller/config"
)

const ProjectWebhookName = "project-webhook"

// +kubebuilder:object:generate=false
type ProjectWebhook struct {
	client.Client

	// Identity is the name of the entity (usually a service account) the platform-service-project-workspace uses to access the onboarding cluster.
	// It is required to exclude the operator's own identity from validation checks.
	Identity          string
	SharedInformation config.SharedInformation
}

func SetupProjectWebhookWithManager(ctx context.Context, mgr ctrl.Manager, identity string, si config.SharedInformation) error {
	pwh := &ProjectWebhook{
		Client:            mgr.GetClient(),
		SharedInformation: si,
		Identity:          identity,
	}

	return ctrl.NewWebhookManagedBy(mgr, &pwv1alpha1.Project{}).
		WithDefaulter(pwh).
		WithValidator(pwh).
		Complete()
}

// +kubebuilder:webhook:path=/mutate-core-openmcp-cloud-v1alpha1-project,mutating=true,failurePolicy=fail,sideEffects=None,groups=core.openmcp.cloud,resources=projects,verbs=create;update,versions=v1alpha1,name=mproject.openmcp.cloud,admissionReviewVersions=v1

var _ admission.Defaulter[*pwv1alpha1.Project] = &ProjectWebhook{}

// Default implements admission.Defaulter[*Project] so a webhook will be registered for the type
func (p *ProjectWebhook) Default(ctx context.Context, obj *pwv1alpha1.Project) error {
	log := logging.FromContextOrPanic(ctx).WithName(ProjectWebhookName)
	ctx = logging.NewContext(ctx, log)
	project, err := expectProject(obj)
	if err != nil {
		return err
	}
	log.Info("Default")

	req, err := admission.RequestFromContext(ctx)
	if err != nil {
		return err
	}

	setCreatedBy(project, req)

	return nil
}

// +kubebuilder:webhook:path=/validate-core-openmcp-cloud-v1alpha1-project,mutating=false,failurePolicy=fail,sideEffects=None,groups=core.openmcp.cloud,resources=projects,verbs=create;update;delete,versions=v1alpha1,name=vproject.openmcp.cloud,admissionReviewVersions=v1

var _ admission.Validator[*pwv1alpha1.Project] = &ProjectWebhook{}

// ValidateCreate implements admission.Validator[*Project] so a webhook will be registered for the type
func (v *ProjectWebhook) ValidateCreate(ctx context.Context, obj *pwv1alpha1.Project) (warnings admission.Warnings, err error) {
	log := logging.FromContextOrPanic(ctx).WithName(ProjectWebhookName)
	ctx = logging.NewContext(ctx, log)
	project, err := expectProject(obj)
	if err != nil {
		return
	}
	log.Info("Validate create")

	userInfo, err := userInfoFromContext(ctx)
	if err != nil {
		return
	}

	validRole, err := v.ensureValidRole(ctx, project)
	if err != nil {
		return warnings, err
	}
	if !validRole {
		return warnings, errRequestingUserNoAccess(userInfo.Username)
	}

	return
}

// ValidateUpdate implements admission.Validator[*Project] so a webhook will be registered for the type
func (v *ProjectWebhook) ValidateUpdate(ctx context.Context, oldObj, newObj *pwv1alpha1.Project) (warnings admission.Warnings, err error) {
	log := logging.FromContextOrPanic(ctx).WithName(ProjectWebhookName)
	ctx = logging.NewContext(ctx, log)
	oldProject, err := expectProject(oldObj)
	if err != nil {
		return
	}

	newProject, err := expectProject(newObj)
	if err != nil {
		return
	}
	log.Info("Validate update")

	if err = verifyCreatedByUnchanged(oldProject, newProject); err != nil {
		return
	}

	userInfo, err := userInfoFromContext(ctx)
	if err != nil {
		return
	}
	validRole, err := v.ensureValidRole(ctx, oldProject)
	if err != nil {
		return warnings, err
	}
	if !validRole {
		return warnings, errRequestingUserNoAccess(userInfo.Username)
	}

	// ensure the user can't remove themselves from the member list
	validNewRole, err := v.ensureValidRole(ctx, newProject)
	if err != nil {
		return warnings, err
	}
	if !validNewRole {
		return warnings, errRequestingUserNoAccess(userInfo.Username)
	}

	return
}

// ValidateDelete implements admission.Validator[*pwv1alpha1.Project] so a webhook will be registered for the type
func (v *ProjectWebhook) ValidateDelete(ctx context.Context, obj *pwv1alpha1.Project) (warnings admission.Warnings, err error) {
	log := logging.FromContextOrPanic(ctx).WithName(ProjectWebhookName)
	ctx = logging.NewContext(ctx, log)
	project, err := expectProject(obj)
	if err != nil {
		return
	}
	log.Info("Validate delete")

	if validRole, err := v.ensureValidRole(ctx, project); !validRole {
		return warnings, err
	}
	return
}

// expectProject casts the given runtime.Object to *Project. Returns an error in case the object can't be casted.
func expectProject(obj runtime.Object) (*pwv1alpha1.Project, error) {
	project, ok := obj.(*pwv1alpha1.Project)
	if !ok {
		return nil, fmt.Errorf("expected a Project but got a %T", obj)
	}
	return project, nil
}

func (v *ProjectWebhook) ensureValidRole(ctx context.Context, project *pwv1alpha1.Project) (bool, error) {
	userInfo, err := userInfoFromContext(ctx)
	if err != nil {
		return false, fmt.Errorf("failed to get userInfo")
	}
	if project.UserInfoHasRole(userInfo, pwv1alpha1.ProjectRoleAdmin) || userInfo.Username == v.Identity {
		return true, nil
	}

	overrides, err := v.SharedInformation.MemberOverrides(ctx)
	if err != nil {
		return false, fmt.Errorf("failed to get member overrides: %w", err)
	}

	if overrides.HasAdminOverrideForResource(&userInfo, project.Name, project.Kind) {
		return true, nil
	}
	return false, nil
}
