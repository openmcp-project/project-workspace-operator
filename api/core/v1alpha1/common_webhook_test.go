package v1alpha1

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"

	admissionv1 "k8s.io/api/admission/v1"
	authv1 "k8s.io/api/authentication/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"sigs.k8s.io/controller-runtime/pkg/webhook/admission"
)

func TestCompareStringMapValue(t *testing.T) {
	tests := []struct {
		description    string
		mapA           map[string]string
		mapB           map[string]string
		key            string
		expectedResult bool
	}{
		{
			description: "returns 'true' if both maps contain map entries for the given key with the same value",
			mapA: map[string]string{
				"test": "test",
			},
			mapB: map[string]string{
				"test": "test",
			},
			key:            "test",
			expectedResult: true,
		},
		{
			description:    "returns 'true' if both maps don't contain a map entry for the given key",
			key:            "test",
			expectedResult: true,
		},
		{
			description: "returns 'false' if both maps contain map entries for the given key but with different values",
			mapA: map[string]string{
				"test": "test1",
			},
			mapB: map[string]string{
				"test": "test2",
			},
			key: "test",
		},
		{
			description: "returns 'false' if mapA doesn't contain a map entry for the given key",
			mapB: map[string]string{
				"test": "test",
			},
			key: "test",
		},
		{
			description: "returns false if mapB doesn't contain a map entry for the given key",
			mapA: map[string]string{
				"test": "test",
			},
			key: "test",
		},
	}

	for _, test := range tests {
		t.Run(test.description, func(t *testing.T) {
			actualResult := compareStringMapValue(test.mapA, test.mapB, test.key)

			assert.Equal(t, test.expectedResult, actualResult)
		})
	}

}

func TestVerifyCreatedByUnchanged(t *testing.T) {
	tests := []struct {
		description string
		objA        metav1.ObjectMeta
		objB        metav1.ObjectMeta
		expectError bool
	}{
		{
			description: "returns no error if objA and objB contain a CreatedByAnnotation with the same value",
			objA: metav1.ObjectMeta{
				Annotations: map[string]string{
					CreatedByAnnotation: "test",
				},
			},
			objB: metav1.ObjectMeta{
				Annotations: map[string]string{
					CreatedByAnnotation: "test",
				},
			},
		},
		{
			description: "returns no error if objA and objB don't contain a CreatedByAnnotation",
		},
		{
			description: "returns an error if objA and objB contain a CreatedByAnnotation with a different value",
			objA: metav1.ObjectMeta{
				Annotations: map[string]string{
					CreatedByAnnotation: "test1",
				},
			},
			objB: metav1.ObjectMeta{
				Annotations: map[string]string{
					CreatedByAnnotation: "test2",
				},
			},
			expectError: true,
		},
	}

	for _, test := range tests {
		t.Run(test.description, func(t *testing.T) {
			err := verifyCreatedByUnchanged(&test.objA, &test.objB)

			if test.expectError {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

func TestSetCreatedBy(t *testing.T) {
	tests := []struct {
		description         string
		request             admissionv1.AdmissionRequest
		expectedAnnotations map[string]string
	}{
		{
			description: "set's the CreatedBy annotation during create",
			request: admissionv1.AdmissionRequest{
				Operation: admissionv1.Create,
				UserInfo: authv1.UserInfo{
					Username: "john.doe@test.com",
				},
			},
			expectedAnnotations: map[string]string{
				CreatedByAnnotation: "john.doe@test.com",
			},
		},
		{
			description: "doesn't set the CreatedBy annotation if operation is NOT create",
			request: admissionv1.AdmissionRequest{
				Operation: admissionv1.Update,
				UserInfo: authv1.UserInfo{
					Username: "john.doe@test.com",
				},
			},
		},
	}

	for _, test := range tests {
		t.Run(test.description, func(t *testing.T) {
			var uut metav1.ObjectMeta

			setCreatedBy(&uut, admission.Request{
				AdmissionRequest: test.request,
			})

			assert.Equal(t, test.expectedAnnotations, uut.GetAnnotations())
		})
	}
}

func TestUserInfoFromContext(t *testing.T) {
	t.Run("returns the userinfo from the admission.Request in the context", func(t *testing.T) {
		userInfo := authv1.UserInfo{}
		req := admission.Request{
			AdmissionRequest: admissionv1.AdmissionRequest{
				UserInfo: userInfo,
			},
		}

		ctx := admission.NewContextWithRequest(context.Background(), req)

		uut, err := userInfoFromContext(ctx)

		if assert.NoError(t, err) {
			assert.Equal(t, userInfo, uut)
		}
	})
	t.Run("fails if admission.Request can't be found in the context", func(t *testing.T) {
		_, err := userInfoFromContext(context.TODO())

		assert.Error(t, err)
	})
}
