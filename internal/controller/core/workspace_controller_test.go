package core

import (
	"context"
	"testing"
	"time"

	"k8s.io/apimachinery/pkg/util/json"

	"github.com/openmcp-project/project-workspace-operator/internal/controller/core/config"

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
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	"github.com/openmcp-project/project-workspace-operator/api/core/v1alpha1"
)

var (
	projectNamespace = &corev1.Namespace{
		ObjectMeta: metav1.ObjectMeta{
			Name: namespaceForProject(sampleProject),
			Labels: map[string]string{
				labelProject: sampleProject.Name,
			},
		},
	}
	sampleWorkspace = &v1alpha1.Workspace{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "sample",
			Namespace: projectNamespace.Name,
		},
		Spec: v1alpha1.WorkspaceSpec{
			Members: []v1alpha1.WorkspaceMember{
				{
					Subject: v1alpha1.Subject{
						Kind: rbacv1.UserKind,
						Name: "user@example.com",
					},
					Roles: []v1alpha1.WorkspaceMemberRole{v1alpha1.WorkspaceRoleAdmin},
				},
				{
					Subject: v1alpha1.Subject{
						Kind: rbacv1.GroupKind,
						Name: "some-group",
					},
					Roles: []v1alpha1.WorkspaceMemberRole{v1alpha1.WorkspaceRoleAdmin},
				},
				{
					Subject: v1alpha1.Subject{
						Kind:      "ServiceAccount",
						Name:      "default",
						Namespace: "default",
					},
					Roles: []v1alpha1.WorkspaceMemberRole{v1alpha1.WorkspaceRoleView},
				},
			},
		},
	}
	sampleWorkspaceDeleted = &v1alpha1.Workspace{
		ObjectMeta: metav1.ObjectMeta{
			Name:              "sample",
			Namespace:         projectNamespace.Name,
			DeletionTimestamp: ptr.To(metav1.Now()),
			Finalizers: []string{
				deleteFinalizer,
			},
		},
		Status: v1alpha1.WorkspaceStatus{
			Namespace: "project-sample--ws-sample",
		},
	}
)

func Test_WorkspaceReconciler_Reconcile(t *testing.T) {
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
				sampleWorkspace,
				projectNamespace,
			},
			interceptorFuncs: interceptor.Funcs{
				Get: func(ctx context.Context, client client.WithWatch, key client.ObjectKey, obj client.Object, opts ...client.GetOption) error {
					return apierrors.NewNotFound(v1alpha1.GroupVersion.WithResource("workspaces").GroupResource(), sampleWorkspace.Name)
				},
			},
			expectedResult: reconcile.Result{},
			expectedErr:    nil,
		},
		{
			desc: "CO-1154 should return error when unknown error occurs",
			initObjs: []client.Object{
				sampleWorkspace,
				projectNamespace,
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
			desc: "CO-1154 should return error when project namespace label map is nil",
			initObjs: []client.Object{
				sampleWorkspace,
				&corev1.Namespace{
					ObjectMeta: metav1.ObjectMeta{
						Name: sampleWorkspace.Namespace,
					},
				},
			},
			expectedErr: ErrNamespaceHasNoLabels,
		},
		{
			desc: "CO-1154 should return error when project namespace has no project label",
			initObjs: []client.Object{
				sampleWorkspace,
				&corev1.Namespace{
					ObjectMeta: metav1.ObjectMeta{
						Name: sampleWorkspace.Namespace,
						Labels: map[string]string{
							"some-unrelated-label": "true",
						},
					},
				},
			},
			expectedErr: ErrNamespaceHasNoProjectLabel,
		},
		{
			desc: "CO-1154 should create namespace and RBAC resources",
			initObjs: []client.Object{
				sampleWorkspace,
				projectNamespace,
				sampleProject,
			},
			expectedResult: reconcile.Result{},
			expectedErr:    nil,
			validate: func(t *testing.T, ctx context.Context, c client.Client) error {
				// check workspace status
				ws := &v1alpha1.Workspace{}
				assert.NoErrorf(t, c.Get(ctx, client.ObjectKeyFromObject(sampleWorkspace), ws), "GET failed unexpectedly")
				assert.Equal(t, "project-sample--ws-sample", ws.Status.Namespace)
				assert.Contains(t, ws.Finalizers, deleteFinalizer)

				namespaceCreatedForWorkspace(t, ctx, c, ws, true)

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

				clusterRoleCreatedForWorkspace(t, ctx, c, sampleProject, ws, v1alpha1.WorkspaceRoleAdmin, true, 1)
				clusterRoleBindingCreatedForWorkspace(t, ctx, c, sampleProject, ws, v1alpha1.WorkspaceRoleAdmin, true, expectedAdmins)
				roleBindingCreatedForWorkspace(t, ctx, c, ws, v1alpha1.WorkspaceRoleAdmin, true, expectedAdmins)

				expectedViewers := []rbacv1.Subject{
					{
						Kind:      rbacv1.ServiceAccountKind,
						Name:      "default",
						Namespace: "default",
					},
				}

				clusterRoleCreatedForWorkspace(t, ctx, c, sampleProject, ws, v1alpha1.WorkspaceRoleView, true, 1)
				clusterRoleBindingCreatedForWorkspace(t, ctx, c, sampleProject, ws, v1alpha1.WorkspaceRoleView, true, expectedViewers)
				roleBindingCreatedForWorkspace(t, ctx, c, ws, v1alpha1.WorkspaceRoleView, true, expectedViewers)

				return nil
			},
		},
		{
			desc: "CO-1154 should delete namespace",
			initObjs: []client.Object{
				sampleWorkspaceDeleted,
				projectNamespace,
				sampleProject,
				&corev1.Namespace{
					ObjectMeta: metav1.ObjectMeta{
						Name: sampleWorkspaceDeleted.Status.Namespace,
					},
				},
			},
			expectedResult: reconcile.Result{},
			expectedErr:    nil,
			validate: func(t *testing.T, ctx context.Context, c client.Client) error {
				// check workspace status
				ws := &v1alpha1.Workspace{}
				err := c.Get(ctx, client.ObjectKeyFromObject(sampleWorkspaceDeleted), ws)
				assert.True(t, apierrors.IsNotFound(err))

				namespaceCreatedForWorkspace(t, ctx, c, sampleWorkspaceDeleted, false)

				clusterRoleCreatedForWorkspace(t, ctx, c, sampleProject, ws, v1alpha1.WorkspaceRoleAdmin, false, 0)
				clusterRoleBindingCreatedForWorkspace(t, ctx, c, sampleProject, ws, v1alpha1.WorkspaceRoleAdmin, false, nil)

				clusterRoleCreatedForWorkspace(t, ctx, c, sampleProject, ws, v1alpha1.WorkspaceRoleView, false, 0)
				clusterRoleBindingCreatedForWorkspace(t, ctx, c, sampleProject, ws, v1alpha1.WorkspaceRoleView, false, nil)

				return nil
			},
		},
		{
			desc: "CO-1154 should not delete namespace when deletion is blocked by resources",
			initObjs: []client.Object{
				sampleWorkspaceDeleted,
				projectNamespace,
				sampleProject,
				&corev1.Namespace{
					ObjectMeta: metav1.ObjectMeta{
						Name: sampleWorkspaceDeleted.Status.Namespace,
					},
				},
				&corev1.Secret{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "blocking",
						Namespace: sampleWorkspaceDeleted.Status.Namespace,
					},
				},
			},
			expectedResult: reconcile.Result{RequeueAfter: 3 * time.Second},
			expectedErr:    nil,
			validate: func(t *testing.T, ctx context.Context, c client.Client) error {
				// check workspace status
				ws := &v1alpha1.Workspace{}
				err := c.Get(ctx, client.ObjectKeyFromObject(sampleWorkspaceDeleted), ws)
				assert.NoError(t, err)
				assert.NotNil(t, ws.GetDeletionTimestamp())

				assert.Len(t, ws.Status.Conditions, 1)
				assert.Equal(t, v1alpha1.ConditionTypeContentRemaining, ws.Status.Conditions[0].Type)
				assert.Equal(t, v1alpha1.ConditionStatusTrue, ws.Status.Conditions[0].Status)
				assert.Equal(t, v1alpha1.ConditionReasonResourcesRemaining, ws.Status.Conditions[0].Reason)
				assert.NotEmpty(t, ws.Status.Conditions[0].Message)
				assert.NotNil(t, ws.Status.Conditions[0].Details)

				var remainingResources []v1alpha1.RemainingContentResource
				assert.NoError(t, json.Unmarshal(ws.Status.Conditions[0].Details, &remainingResources))
				assert.Len(t, remainingResources, 1)
				assert.Equal(t, "v1", remainingResources[0].APIGroup)
				assert.Equal(t, "Secret", remainingResources[0].Kind)
				assert.Equal(t, "blocking", remainingResources[0].Name)

				ns := &corev1.Namespace{}
				err = c.Get(ctx, types.NamespacedName{Name: ws.Status.Namespace}, ns)
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

			sr := &WorkspaceReconciler{
				Client: c,
				Scheme: c.Scheme(),
				CommonReconciler: CommonReconciler{
					Client:         c,
					ControllerName: "test",
					ProjectWorkspaceConfig: &config.ProjectWorkspaceConfig{
						Workspace: config.WorkspaceConfig{
							ResourcesBlockingDeletion: []config.GroupVersionKind{
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

func namespaceCreatedForWorkspace(t *testing.T, ctx context.Context, c client.Client, ws *v1alpha1.Workspace, expectation bool) *corev1.Namespace {
	ns := &corev1.Namespace{}
	err := c.Get(ctx, types.NamespacedName{Name: ws.Status.Namespace}, ns)
	if expectation {
		assert.NoError(t, err)
		assert.Equal(t, ws.Name, ns.Labels[labelWorkspace])
	} else {
		assert.True(t, apierrors.IsNotFound(err))
	}
	return ns
}

func clusterRoleCreatedForWorkspace(t *testing.T, ctx context.Context, c client.Client, p *v1alpha1.Project, ws *v1alpha1.Workspace, role v1alpha1.WorkspaceMemberRole, expectation bool, expectedRules int) {
	cr := &rbacv1.ClusterRole{}
	err := c.Get(ctx, types.NamespacedName{Name: clusterRoleForEntityAndRoleWithParent(ws, role, p)}, cr)
	if expectation {
		assert.NoError(t, err)
		assert.Len(t, cr.Rules, expectedRules)
	} else {
		assert.True(t, apierrors.IsNotFound(err))
	}
}

func clusterRoleBindingCreatedForWorkspace(t *testing.T, ctx context.Context, c client.Client, p *v1alpha1.Project, ws *v1alpha1.Workspace, role v1alpha1.WorkspaceMemberRole, expectation bool, expectedSubjects []rbacv1.Subject) {
	crb := &rbacv1.ClusterRoleBinding{}
	err := c.Get(ctx, types.NamespacedName{Name: clusterRoleForEntityAndRoleWithParent(ws, role, p)}, crb)
	if expectation {
		assert.NoError(t, err)
		assert.Equal(t, expectedSubjects, crb.Subjects)
	} else {
		assert.True(t, apierrors.IsNotFound(err))
	}
}

func roleBindingCreatedForWorkspace(t *testing.T, ctx context.Context, c client.Client, ws *v1alpha1.Workspace, role v1alpha1.WorkspaceMemberRole, expectation bool, expectedSubjects []rbacv1.Subject) {
	rb := &rbacv1.RoleBinding{}
	err := c.Get(ctx, types.NamespacedName{Name: roleBindingForRole(role), Namespace: ws.Status.Namespace}, rb)
	if expectation {
		assert.NoError(t, err)
		assert.Equal(t, expectedSubjects, rb.Subjects)
	} else {
		assert.True(t, apierrors.IsNotFound(err))
	}
}
