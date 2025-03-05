package v1alpha1

import (
	"context"
	"fmt"
	"os"
	"strings"

	admissionv1 "k8s.io/api/admission/v1"
	authv1 "k8s.io/api/authentication/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/webhook/admission"
)

var (
	// errCreatedByImmutable is the error that is returned when the value of the resource creator annotation has been changed by the user.
	errCreatedByImmutable = fmt.Errorf("annotation %s is immutable", CreatedByAnnotation)

	// errRequestingUserNoAccess is the error that is returned when the user who is creating/updating a project or workspace would lock themselves out.
	errRequestingUserNoAccess = func(username string) error {
		return fmt.Errorf("requesting user %s will not be able to manage the created/updated resource. please check the list of members again or use MemberOverrides", username)
	}
)

// compareStringMapValue compares the value of string values identified by a key in two maps.
// Returns "true" if the value is the same.
func compareStringMapValue(a, b map[string]string, key string) bool {
	return a[key] == b[key]
}

// verifyCreatedByUnchanged checks if the value of the annotation that contains the name of the resource creator has been changed.
// Returns an error if the value has been changed or "nil" if it's the same.
func verifyCreatedByUnchanged(old, new metav1.Object) error {
	if compareStringMapValue(old.GetAnnotations(), new.GetAnnotations(), CreatedByAnnotation) {
		return nil
	}

	return errCreatedByImmutable
}

// setCreatedBy sets an annotation that contains the name of the user who created the resource.
// The value is only set when the "Operation" is "Create".
func setCreatedBy(obj metav1.Object, req admission.Request) {
	if req.Operation != admissionv1.Create {
		return
	}

	setMetaDataAnnotation(obj, CreatedByAnnotation, req.UserInfo.Username)
}

// setMetaDataAnnotation sets the annotation on the given object.
// If the given Object did not yet have annotations, they are initialized.
func setMetaDataAnnotation(meta metav1.Object, key, value string) { // TODO move to utils package
	labels := meta.GetAnnotations()
	if labels == nil {
		labels = make(map[string]string)
	}
	labels[key] = value
	meta.SetAnnotations(labels)
}

func isOwnServiceAccount(userinfo authv1.UserInfo) bool {
	svcAccUsername := fmt.Sprintf("system:serviceaccount:%s:%s", os.Getenv("POD_NAMESPACE"), os.Getenv("POD_SERVICE_ACCOUNT"))
	return strings.HasSuffix(userinfo.Username, svcAccUsername)
}

// userInfoFromContext extracts the authv1.UserInfo from the admission.Request available in the context. Returns an error if the request can't be found.
func userInfoFromContext(ctx context.Context) (authv1.UserInfo, error) {
	req, err := admission.RequestFromContext(ctx)
	if err != nil {
		return authv1.UserInfo{}, err
	}

	return req.UserInfo, nil
}

func ensureLabel(meta metav1.Object, label string) error {
	labels := meta.GetLabels()
	_, ok := labels[label]
	if !ok {
		return fmt.Errorf("label %s is missing", label)
	}

	return nil
}
