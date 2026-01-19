package core

import (
	"context"
	"errors"
	"fmt"

	"io/fs"
	"testing"
	"time"

	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
	"sigs.k8s.io/controller-runtime/pkg/client/interceptor"

	openmcpv1alpha1 "github.com/openmcp-project/project-workspace-operator/api/core/v1alpha1"
	"github.com/openmcp-project/project-workspace-operator/internal/controller/config"

	"github.com/stretchr/testify/assert"
)

func Test_ResourcesRemainingError_Is(t *testing.T) {
	testCases := []struct {
		desc     string
		a        error
		b        error
		expected bool
	}{
		{
			desc:     "should return true for same error",
			a:        ResourcesRemainingError{RequeueAfter: 10 * time.Second},
			b:        ResourcesRemainingError{RequeueAfter: 10 * time.Second},
			expected: true,
		},
		{
			desc:     "should return true for similar error",
			a:        ResourcesRemainingError{RequeueAfter: 10 * time.Second},
			b:        ResourcesRemainingError{RequeueAfter: 20 * time.Second},
			expected: true,
		},
		{
			desc:     "should return false for different error",
			a:        ResourcesRemainingError{RequeueAfter: 10 * time.Second},
			b:        fs.ErrNotExist,
			expected: false,
		},
		{
			desc:     "should return false for different error (swapped)",
			a:        fs.ErrNotExist,
			b:        ResourcesRemainingError{RequeueAfter: 10 * time.Second},
			expected: false,
		},
	}
	for _, tC := range testCases {
		t.Run(tC.desc, func(t *testing.T) {
			assert.Equal(t, tC.expected, errors.Is(tC.a, tC.b))
		})
	}
}

func Test_CommonReconciler_handleDelete(t *testing.T) {
	fakeTime := time.Now()
	testProject := &openmcpv1alpha1.Project{
		ObjectMeta: metav1.ObjectMeta{
			Name:              "test-project",
			DeletionTimestamp: &metav1.Time{Time: fakeTime},
			Finalizers:        []string{deleteFinalizer},
		},
	}

	type exp struct {
		b      bool
		result ctrl.Result
		err    error
	}

	test := []struct {
		name             string
		obj              client.Object
		interceptorFuncs interceptor.Funcs
		deleteFunc       func() error
		expected         exp
		validateFunc     func(ctx context.Context, c client.Client) error
	}{
		{
			name: "should return false if object was not deleted",
			obj: &openmcpv1alpha1.Project{
				ObjectMeta: metav1.ObjectMeta{Name: "test-project"},
			},
			deleteFunc: func() error {
				return nil
			},
			expected: exp{
				b:      false,
				result: ctrl.Result{},
				err:    nil,
			},
		},
		{
			name: "Resources are still remaining in the cluster",
			obj:  testProject.DeepCopy(),
			deleteFunc: func() error {
				return ResourcesRemainingError{RequeueAfter: 3 * time.Second}
			},
			expected: exp{
				b:      true,
				result: ctrl.Result{RequeueAfter: 3 * time.Second},
				err:    nil,
			},
		},
		{
			name: "Failed to perform clean up operation",
			obj:  testProject.DeepCopy(),
			deleteFunc: func() error {
				return errors.New("some error")
			},
			expected: exp{
				b:      false,
				result: ctrl.Result{},
				err:    fmt.Errorf("failed to perform cleanup operation: %w", errors.New("some error")),
			},
		},
		{
			name: "Failed to remove finalizer",
			obj:  testProject.DeepCopy(),
			interceptorFuncs: interceptor.Funcs{
				Update: func(ctx context.Context, client client.WithWatch, obj client.Object, opts ...client.UpdateOption) error {
					if _, ok := obj.(*openmcpv1alpha1.Project); ok {
						return errors.New("some update error")
					}
					return client.Update(ctx, obj, opts...)
				},
			},
			deleteFunc: func() error {
				return nil
			},
			expected: exp{
				b:      false,
				result: ctrl.Result{},
				err:    fmt.Errorf("failed to remove finalizer: %w", errors.New("some update error")),
			},
		},
		{
			name: "Finalizer removed successfully",
			obj:  testProject.DeepCopy(),
			deleteFunc: func() error {
				return nil
			},
			expected: exp{
				b:      true,
				result: ctrl.Result{},
				err:    nil,
			},
			validateFunc: func(ctx context.Context, c client.Client) error {
				project := &openmcpv1alpha1.Project{}
				err := c.Get(ctx, client.ObjectKeyFromObject(testProject), project)
				assert.True(t, apierrors.IsNotFound(err))
				return nil
			},
		},
	}

	for _, tt := range test {
		t.Run(tt.name, func(t *testing.T) {
			ctx := context.TODO()
			fakeClient := fake.NewClientBuilder().WithScheme(Scheme).WithObjects(tt.obj).WithInterceptorFuncs(tt.interceptorFuncs).Build()
			r := &CommonReconciler{
				Config:         config.NewFakeSharedInformation(fakeClient, nil, nil, nil, nil),
				ControllerName: "test-controller",
			}
			assert.NoError(t, fakeClient.Get(ctx, client.ObjectKeyFromObject(tt.obj), tt.obj))

			b, result, err := r.handleDelete(ctx, tt.obj, tt.deleteFunc)
			assert.Equal(t, tt.expected.b, b)
			assert.Equal(t, tt.expected.result, result)
			assert.Equal(t, tt.expected.err, err)

			if tt.validateFunc != nil {
				if err := tt.validateFunc(ctx, fakeClient); err != nil {
					t.Errorf("validation failed unexpectedly: %v", err)
				}
			}
		})
	}
}

func Test_CommonReconciler_ensureFinalizer(t *testing.T) {
	test := []struct {
		name             string
		obj              client.Object
		interceptorFuncs interceptor.Funcs
		expectedErr      error
		validateFunc     func(ctx context.Context, c client.Client) error
	}{
		{
			name: "Finalizer already exists",
			obj: &openmcpv1alpha1.Project{
				ObjectMeta: metav1.ObjectMeta{
					Name:       "test-project",
					Finalizers: []string{deleteFinalizer},
				},
			},
			expectedErr: nil,
		},
		{
			name: "Failed to add finalizer",
			obj: &openmcpv1alpha1.Project{
				ObjectMeta: metav1.ObjectMeta{
					Name: "test-project",
				},
			},
			interceptorFuncs: interceptor.Funcs{
				Update: func(ctx context.Context, client client.WithWatch, obj client.Object, opts ...client.UpdateOption) error {
					if _, ok := obj.(*openmcpv1alpha1.Project); ok {
						return errors.New("some error")
					}
					return client.Update(ctx, obj, opts...)
				},
			},
			expectedErr: fmt.Errorf("failed to add finalizer: %w", errors.New("some error")),
		},
		{
			name: "Finalizer added successfully",
			obj: &openmcpv1alpha1.Project{
				ObjectMeta: metav1.ObjectMeta{
					Name: "test-project",
				},
			},
			expectedErr: nil,
			validateFunc: func(ctx context.Context, c client.Client) error {
				project := &openmcpv1alpha1.Project{
					ObjectMeta: metav1.ObjectMeta{
						Name: "test-project",
					},
				}
				if err := c.Get(ctx, client.ObjectKeyFromObject(project), project); err != nil {
					return err
				}
				assert.Contains(t, project.Finalizers, deleteFinalizer)
				return nil
			},
		},
	}

	for _, tt := range test {
		t.Run(tt.name, func(t *testing.T) {
			ctx := context.TODO()
			fakeClient := fake.NewClientBuilder().WithScheme(Scheme).WithObjects(tt.obj).WithInterceptorFuncs(tt.interceptorFuncs).Build()
			r := &CommonReconciler{
				Config:         config.NewFakeSharedInformation(fakeClient, nil, nil, nil, nil),
				ControllerName: "test-controller",
			}
			assert.NoError(t, fakeClient.Get(ctx, client.ObjectKeyFromObject(tt.obj), tt.obj))

			err := r.ensureFinalizer(ctx, tt.obj)
			assert.Equal(t, tt.expectedErr, err)

			if tt.validateFunc != nil {
				err := tt.validateFunc(ctx, fakeClient)
				assert.NoErrorf(t, err, "validation failed unexpectedly")
			}
		})
	}
}
