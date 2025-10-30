package core

import (
	"context"
	"errors"
	"testing"
	"time"

	"k8s.io/apimachinery/pkg/util/json"

	"github.com/stretchr/testify/assert"
	corev1 "k8s.io/api/core/v1"
	rbacv1 "k8s.io/api/rbac/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/utils/ptr"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
	"sigs.k8s.io/controller-runtime/pkg/client/interceptor"
	"sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	pwv1alpha1 "github.com/openmcp-project/project-workspace-operator/api/core/v1alpha1"
)

const (
	maxReconcileCycles = 10
)

var (
	sampleProject = &pwv1alpha1.Project{
		ObjectMeta: metav1.ObjectMeta{
			Name: "sample",
		},
		Spec: pwv1alpha1.ProjectSpec{
			Members: []pwv1alpha1.ProjectMember{
				{
					Subject: pwv1alpha1.Subject{
						Kind: rbacv1.UserKind,
						Name: "user@example.com",
					},
					Roles: []pwv1alpha1.ProjectMemberRole{pwv1alpha1.ProjectRoleAdmin},
				},
				{
					Subject: pwv1alpha1.Subject{
						Kind: rbacv1.GroupKind,
						Name: "some-group",
					},
					Roles: []pwv1alpha1.ProjectMemberRole{pwv1alpha1.ProjectRoleAdmin},
				},
				{
					Subject: pwv1alpha1.Subject{
						Kind:      "ServiceAccount",
						Name:      "default",
						Namespace: "default",
					},
					Roles: []pwv1alpha1.ProjectMemberRole{pwv1alpha1.ProjectRoleView},
				},
			},
		},
	}
	sampleProjectDeleted = &pwv1alpha1.Project{
		ObjectMeta: metav1.ObjectMeta{
			Name:              "sample",
			DeletionTimestamp: ptr.To(metav1.Now()),
			Finalizers: []string{
				deleteFinalizer,
			},
		},
		Status: pwv1alpha1.ProjectStatus{
			Namespace: "project-sample",
		},
	}

	errFake = errors.New("fake")
)

func Test_ProjectReconciler_Reconcile(t *testing.T) {
	testCases := []struct {
		desc             string
		initObjs         []client.Object
		interceptorFuncs interceptor.Funcs
		expectedResult   ctrl.Result
		expectedErr      error
		validate         func(t *testing.T, ctx context.Context, c client.Client) error
	}{
		{
			desc: "CO-1154 should not return error when not found",
			initObjs: []client.Object{
				sampleProject,
			},
			interceptorFuncs: interceptor.Funcs{
				Get: func(ctx context.Context, client client.WithWatch, key client.ObjectKey, obj client.Object, opts ...client.GetOption) error {
					return apierrors.NewNotFound(pwv1alpha1.GroupVersion.WithResource("projects").GroupResource(), sampleProject.Name)
				},
			},
			expectedResult: reconcile.Result{},
			expectedErr:    nil,
		},
		{
			desc: "CO-1154 should return error when unknown error occurs",
			initObjs: []client.Object{
				sampleProject,
			},
			interceptorFuncs: interceptor.Funcs{
				Get: func(ctx context.Context, client client.WithWatch, key client.ObjectKey, obj client.Object, opts ...client.GetOption) error {
					return errFake
				},
			},
			expectedResult: reconcile.Result{},
			expectedErr:    errFake,
		},
		{
			desc: "CO-1154 should create namespace and RBAC resources",
			initObjs: []client.Object{
				sampleProject,
			},
			expectedResult: reconcile.Result{},
			expectedErr:    nil,
			validate: func(t *testing.T, ctx context.Context, c client.Client) error {
				// check project status
				p := &pwv1alpha1.Project{}
				assert.NoErrorf(t, c.Get(ctx, client.ObjectKeyFromObject(sampleProject), p), "GET failed unexpectedly")
				assert.Equal(t, "project-sample", p.Status.Namespace)
				assert.Contains(t, p.Finalizers, deleteFinalizer)

				namespaceCreatedForProject(t, ctx, c, p, true)

				expectedAdmins := []rbacv1.Subject{
					{
						APIGroup: rbacv1.GroupName,
						Kind:     rbacv1.UserKind,
						Name:     "user@example.com",
					},
					{
						APIGroup: rbacv1.GroupName,
						Kind:     rbacv1.GroupKind,
						Name:     "some-group",
					},
				}

				clusterRoleCreatedForProject(t, ctx, c, p, pwv1alpha1.ProjectRoleAdmin, true, 2)
				clusterRoleBindingCreatedForProject(t, ctx, c, p, pwv1alpha1.ProjectRoleAdmin, true, expectedAdmins)
				roleBindingCreatedForProject(t, ctx, c, p, pwv1alpha1.ProjectRoleAdmin, true, expectedAdmins)

				expectedViewers := []rbacv1.Subject{
					{
						Kind:      rbacv1.ServiceAccountKind,
						Name:      "default",
						Namespace: "default",
					},
				}

				clusterRoleCreatedForProject(t, ctx, c, p, pwv1alpha1.ProjectRoleView, true, 2)
				clusterRoleBindingCreatedForProject(t, ctx, c, p, pwv1alpha1.ProjectRoleView, true, expectedViewers)
				roleBindingCreatedForProject(t, ctx, c, p, pwv1alpha1.ProjectRoleView, true, expectedViewers)

				return nil
			},
		},
		{
			desc: "CO-1154 should delete namespace",
			initObjs: []client.Object{
				sampleProjectDeleted,
				&corev1.Namespace{
					ObjectMeta: metav1.ObjectMeta{
						Name: sampleProjectDeleted.Status.Namespace,
					},
				},
			},
			expectedResult: reconcile.Result{},
			expectedErr:    nil,
			validate: func(t *testing.T, ctx context.Context, c client.Client) error {
				// check project status
				p := &pwv1alpha1.Project{}
				err := c.Get(ctx, client.ObjectKeyFromObject(sampleProjectDeleted), p)
				assert.True(t, apierrors.IsNotFound(err))

				namespaceCreatedForProject(t, ctx, c, sampleProjectDeleted, false)

				return nil
			},
		},
		{
			desc: "CO-1154 should not delete namespace when deletion is blocked by resources",
			initObjs: []client.Object{
				sampleProjectDeleted,
				&corev1.Namespace{
					ObjectMeta: metav1.ObjectMeta{
						Name: sampleProjectDeleted.Status.Namespace,
					},
				},
				&corev1.Secret{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "blocking",
						Namespace: sampleProjectDeleted.Status.Namespace,
					},
				},
			},
			expectedResult: reconcile.Result{RequeueAfter: 3 * time.Second},
			expectedErr:    nil,
			validate: func(t *testing.T, ctx context.Context, c client.Client) error {
				// check workspace status
				p := &pwv1alpha1.Project{}
				err := c.Get(ctx, client.ObjectKeyFromObject(sampleProjectDeleted), p)
				assert.NoError(t, err)

				assert.Len(t, p.Status.Conditions, 1)
				assert.Equal(t, pwv1alpha1.ConditionTypeContentRemaining, p.Status.Conditions[0].Type)
				assert.Equal(t, pwv1alpha1.ConditionStatusTrue, p.Status.Conditions[0].Status)
				assert.Equal(t, pwv1alpha1.ConditionReasonResourcesRemaining, p.Status.Conditions[0].Reason)
				assert.NotEmpty(t, p.Status.Conditions[0].Message)
				assert.NotNil(t, p.Status.Conditions[0].Details)

				var remainingResources []pwv1alpha1.RemainingContentResource
				assert.NoError(t, json.Unmarshal(p.Status.Conditions[0].Details, &remainingResources))
				assert.Len(t, remainingResources, 1)
				assert.Equal(t, "v1", remainingResources[0].APIGroup)
				assert.Equal(t, "Secret", remainingResources[0].Kind)
				assert.Equal(t, "blocking", remainingResources[0].Name)

				ns := &corev1.Namespace{}
				err = c.Get(ctx, types.NamespacedName{Name: p.Status.Namespace}, ns)
				assert.NoError(t, err)
				assert.Nil(t, ns.GetDeletionTimestamp())

				return nil
			},
		},
	}
	for _, tC := range testCases {
		t.Run(tC.desc, func(t *testing.T) {
			c := fake.NewClientBuilder().
				WithObjects(tC.initObjs...).
				WithInterceptorFuncs(tC.interceptorFuncs).
				WithStatusSubresource(tC.initObjs[0]).
				WithScheme(Scheme).
				Build()
			ctx := newContext()
			req := newRequest(tC.initObjs[0])

			sr := &ProjectReconciler{
				Client: c,
				Scheme: c.Scheme(),
				CommonReconciler: CommonReconciler{
					Client:         c,
					ControllerName: "test",
					ProjectWorkspaceConfigSpec: pwv1alpha1.ProjectWorkspaceConfigSpec{
						Project: pwv1alpha1.ProjectConfig{
							ResourcesBlockingDeletion: []metav1.GroupVersionKind{
								{
									Group:   "",
									Version: "v1",
									Kind:    "Secret",
								},
							},
						},
					},
				},
			}

			result, err := ctrl.Result{}, error(nil)
			for i := 0; i < maxReconcileCycles; i++ {
				result, err = sr.Reconcile(ctx, req)
				if result == tC.expectedResult || result.RequeueAfter == 0 || err != nil {
					break
				}
			}

			assert.Equal(t, tC.expectedResult, result)
			assert.Equal(t, tC.expectedErr, err)

			if tC.validate != nil {
				assert.NoError(t, tC.validate(t, ctx, c))
			}
		})
	}
}

func newContext() context.Context {
	ctx := context.Background()
	ctx = log.IntoContext(ctx, log.Log)
	return ctx
}

func newRequest(obj client.Object) ctrl.Request {
	return ctrl.Request{
		NamespacedName: client.ObjectKeyFromObject(obj),
	}
}

func namespaceCreatedForProject(t *testing.T, ctx context.Context, c client.Client, p *pwv1alpha1.Project, expectation bool) *corev1.Namespace {
	ns := &corev1.Namespace{}
	err := c.Get(ctx, types.NamespacedName{Name: p.Status.Namespace}, ns)
	if expectation {
		assert.NoError(t, err)
		assert.Equal(t, p.Name, ns.Labels[labelProject])
	} else {
		assert.True(t, apierrors.IsNotFound(err))
	}
	return ns
}

func clusterRoleCreatedForProject(t *testing.T, ctx context.Context, c client.Client, p *pwv1alpha1.Project, role pwv1alpha1.ProjectMemberRole, expectation bool, expectedRules int) {
	cr := &rbacv1.ClusterRole{}
	err := c.Get(ctx, types.NamespacedName{Name: clusterRoleForEntityAndRole(p, role)}, cr)
	if expectation {
		assert.NoError(t, err)
		assert.Len(t, cr.Rules, expectedRules)
		if assert.Len(t, cr.OwnerReferences, 1) {
			assert.Equal(t, p.UID, cr.OwnerReferences[0].UID)
		}
	} else {
		assert.True(t, apierrors.IsNotFound(err))
	}
}

func clusterRoleBindingCreatedForProject(t *testing.T, ctx context.Context, c client.Client, p *pwv1alpha1.Project, role pwv1alpha1.ProjectMemberRole, expectation bool, expectedSubjects []rbacv1.Subject) {
	crb := &rbacv1.ClusterRoleBinding{}
	err := c.Get(ctx, types.NamespacedName{Name: clusterRoleForEntityAndRole(p, role)}, crb)
	if expectation {
		assert.NoError(t, err)
		assert.Equal(t, expectedSubjects, crb.Subjects)
		if assert.Len(t, crb.OwnerReferences, 1) {
			assert.Equal(t, p.UID, crb.OwnerReferences[0].UID)
		}
	} else {
		assert.True(t, apierrors.IsNotFound(err))
	}
}

func roleBindingCreatedForProject(t *testing.T, ctx context.Context, c client.Client, p *pwv1alpha1.Project, role pwv1alpha1.ProjectMemberRole, expectation bool, expectedSubjects []rbacv1.Subject) {
	rb := &rbacv1.RoleBinding{}
	err := c.Get(ctx, types.NamespacedName{Name: roleBindingForRole(role), Namespace: p.Status.Namespace}, rb)
	if expectation {
		assert.NoError(t, err)
		assert.Equal(t, expectedSubjects, rb.Subjects)
	} else {
		assert.True(t, apierrors.IsNotFound(err))
	}
}
