package v1alpha1

import (
	"context"
	"fmt"

	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	logf "sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/webhook/admission"
)

// +kubebuilder:object:generate=false
type ProjectWebhook struct {
	client.Client

	// Identity is the name of the entity (usually a service account) the project-workspace-operator uses to access the onboarding cluster.
	// It is required to exclude the operator's own identity from validation checks.
	Identity     string
	OverrideName string
}

// log is for logging in this package.
var projectlog = logf.Log.WithName("project-resource")

func (p *Project) SetupWebhookWithManager(ctx context.Context, mgr ctrl.Manager, memberOverridesName, identity string) error {
	pwh := &ProjectWebhook{
		Client:       mgr.GetClient(),
		OverrideName: memberOverridesName,
		Identity:     identity,
	}

	return ctrl.NewWebhookManagedBy(mgr, p).
		WithDefaulter(pwh).
		WithValidator(pwh).
		Complete()
}

// +kubebuilder:webhook:path=/mutate-core-openmcp-cloud-v1alpha1-project,mutating=true,failurePolicy=fail,sideEffects=None,groups=core.openmcp.cloud,resources=projects,verbs=create;update,versions=v1alpha1,name=mproject.openmcp.cloud,admissionReviewVersions=v1

var _ admission.Defaulter[*Project] = &ProjectWebhook{}

// Default implements admission.Defaulter[*Project] so a webhook will be registered for the type
func (p *ProjectWebhook) Default(ctx context.Context, obj *Project) error {
	project, err := expectProject(obj)
	if err != nil {
		return err
	}
	projectlog.Info("default", "name", project.Name)

	req, err := admission.RequestFromContext(ctx)
	if err != nil {
		return err
	}

	setCreatedBy(project, req)

	return nil
}

// +kubebuilder:webhook:path=/validate-core-openmcp-cloud-v1alpha1-project,mutating=false,failurePolicy=fail,sideEffects=None,groups=core.openmcp.cloud,resources=projects,verbs=create;update;delete,versions=v1alpha1,name=vproject.openmcp.cloud,admissionReviewVersions=v1

var _ admission.Validator[*Project] = &ProjectWebhook{}

// ValidateCreate implements admission.Validator[*Project] so a webhook will be registered for the type
func (v *ProjectWebhook) ValidateCreate(ctx context.Context, obj *Project) (warnings admission.Warnings, err error) {
	project, err := expectProject(obj)
	if err != nil {
		return
	}
	projectlog.Info("validate create", "name", project.Name)

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
func (v *ProjectWebhook) ValidateUpdate(ctx context.Context, oldObj, newObj *Project) (warnings admission.Warnings, err error) {
	oldProject, err := expectProject(oldObj)
	if err != nil {
		return
	}

	newProject, err := expectProject(newObj)
	if err != nil {
		return
	}
	projectlog.Info("validate update", "name", oldProject.Name)

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

// ValidateDelete implements admission.Validator[*Project] so a webhook will be registered for the type
func (v *ProjectWebhook) ValidateDelete(ctx context.Context, obj *Project) (warnings admission.Warnings, err error) {
	project, err := expectProject(obj)
	if err != nil {
		return
	}
	projectlog.Info("validate delete", "name", project.Name)

	if validRole, err := v.ensureValidRole(ctx, project); !validRole {
		return warnings, err
	}
	return
}

// expectProject casts the given runtime.Object to *Project. Returns an error in case the object can't be casted.
func expectProject(obj runtime.Object) (*Project, error) {
	project, ok := obj.(*Project)
	if !ok {
		return nil, fmt.Errorf("expected a Project but got a %T", obj)
	}
	return project, nil
}

func (v *ProjectWebhook) ensureValidRole(ctx context.Context, project *Project) (bool, error) {
	userInfo, err := userInfoFromContext(ctx)
	if err != nil {
		return false, fmt.Errorf("failed to get userInfo")
	}
	if project.UserInfoHasRole(userInfo, ProjectRoleAdmin) || userInfo.Username == v.Identity {
		return true, nil
	}

	if v.OverrideName == "" {
		return false, nil
	}

	overrides := &MemberOverrides{}
	if err := v.Get(ctx, types.NamespacedName{Name: v.OverrideName}, overrides); err != nil {
		return false, err
	}
	if overrides.HasAdminOverrideForResource(&userInfo, project.Name, project.Kind) {
		return true, nil
	}
	return false, nil
}
