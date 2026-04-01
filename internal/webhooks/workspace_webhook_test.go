package webhooks

import (
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	pwv1alpha1 "github.com/openmcp-project/project-workspace-operator/api/core/v1alpha1"
)

var _ = Describe("Workspace Webhook", func() {
	BeforeEach(func() {
		sharedInformationForTests.MemberOverridesData = nil
	})

	Context("When creating a Workspace", func() {
		It("Should allow to create the workspace by the admin user", func() {
			var err error

			workspace := &pwv1alpha1.Workspace{
				ObjectMeta: metav1.ObjectMeta{
					Name:      uniqueName(),
					Namespace: "default",
				},
				Spec: pwv1alpha1.WorkspaceSpec{
					Members: []pwv1alpha1.WorkspaceMember{
						{
							Subject: pwv1alpha1.Subject{
								Kind: "User",
								Name: "admin",
							},
							Roles: []pwv1alpha1.WorkspaceMemberRole{
								pwv1alpha1.WorkspaceRoleAdmin,
							},
						},
					},
				},
			}

			err = realUserClient.Create(ctx, workspace)
			Expect(err).ShouldNot(HaveOccurred())
		})

		It("Should allow to create the workspace by a serviceaccount", func() {
			var err error

			workspace := &pwv1alpha1.Workspace{
				ObjectMeta: metav1.ObjectMeta{
					Name:      uniqueName(),
					Namespace: "default",
				},
				Spec: pwv1alpha1.WorkspaceSpec{
					Members: []pwv1alpha1.WorkspaceMember{
						{
							Subject: pwv1alpha1.Subject{
								Kind:      "ServiceAccount",
								Name:      "admin",
								Namespace: "kube-system",
							},
							Roles: []pwv1alpha1.WorkspaceMemberRole{
								pwv1alpha1.WorkspaceRoleAdmin,
							},
						},
					},
				},
			}

			err = saClient.Create(ctx, workspace)
			Expect(err).ShouldNot(HaveOccurred())
		})

		It("should deny to create the workspace by a non-member user", func() {
			var err error

			workspace := &pwv1alpha1.Workspace{
				ObjectMeta: metav1.ObjectMeta{
					Name:      uniqueName(),
					Namespace: "default",
				},
				Spec: pwv1alpha1.WorkspaceSpec{
					Members: []pwv1alpha1.WorkspaceMember{
						{
							Subject: pwv1alpha1.Subject{
								Kind: "User",
								Name: "unknown",
							},
							Roles: []pwv1alpha1.WorkspaceMemberRole{
								pwv1alpha1.WorkspaceRoleAdmin,
							},
						},
					},
				},
			}

			err = realUserClient.Create(ctx, workspace)
			Expect(err).To(HaveOccurred())
		})

		It("Should allow to create the workspace by a user in MemberOverrides", func() {
			var err error
			var workspaceName = uniqueName()

			namespace := &corev1.Namespace{
				ObjectMeta: metav1.ObjectMeta{
					Name: "project-test",
				},
			}

			err = k8sClient.Create(ctx, namespace)
			Expect(err).ShouldNot(HaveOccurred())

			sharedInformationForTests.MemberOverridesData = pwv1alpha1.MemberOverridesV2{
				{
					Subject: pwv1alpha1.Subject{
						Kind: "User",
						Name: "admin",
					},
					Roles: []pwv1alpha1.OverrideRole{
						pwv1alpha1.OverrideRoleAdmin,
					},
					Resources: []pwv1alpha1.OverrideResource{
						{
							Kind: pwv1alpha1.OverrideResourceKindProject,
							Name: "test",
						},
						{
							Kind: pwv1alpha1.OverrideResourceKindWorkspace,
							Name: workspaceName,
						},
					},
				},
			}

			workspace := &pwv1alpha1.Workspace{
				ObjectMeta: metav1.ObjectMeta{
					Name:      workspaceName,
					Namespace: namespace.Name,
				},
				Spec: pwv1alpha1.WorkspaceSpec{
					Members: []pwv1alpha1.WorkspaceMember{
						{
							Subject: pwv1alpha1.Subject{
								Kind: "User",
								Name: "second-admin",
							},
							Roles: []pwv1alpha1.WorkspaceMemberRole{
								pwv1alpha1.WorkspaceRoleAdmin,
							},
						},
					},
				},
			}

			err = realUserClient.Create(ctx, workspace)
			Expect(err).ShouldNot(HaveOccurred())
		})

		It("Should allow to create the workspace by a serviceaccount in MemeberOverrides", func() {
			var err error
			var workspaceName = uniqueName()

			namespace := &corev1.Namespace{
				ObjectMeta: metav1.ObjectMeta{
					Name: "project-test-sa",
				},
			}

			err = k8sClient.Create(ctx, namespace)
			Expect(err).ShouldNot(HaveOccurred())

			sharedInformationForTests.MemberOverridesData = pwv1alpha1.MemberOverridesV2{
				{
					Subject: pwv1alpha1.Subject{
						Kind:      "ServiceAccount",
						Name:      "admin",
						Namespace: "kube-system",
					},
					Roles: []pwv1alpha1.OverrideRole{
						pwv1alpha1.OverrideRoleAdmin,
					},
					Resources: []pwv1alpha1.OverrideResource{
						{
							Kind: pwv1alpha1.OverrideResourceKindProject,
							Name: "test-sa",
						},
						{
							Kind: pwv1alpha1.OverrideResourceKindWorkspace,
							Name: workspaceName,
						},
					},
				},
			}
			workspace := &pwv1alpha1.Workspace{
				ObjectMeta: metav1.ObjectMeta{
					Name:      workspaceName,
					Namespace: namespace.Name,
				},
				Spec: pwv1alpha1.WorkspaceSpec{
					Members: []pwv1alpha1.WorkspaceMember{
						{
							Subject: pwv1alpha1.Subject{
								Kind:      "ServiceAccount",
								Name:      "second-admin",
								Namespace: "kube-system",
							},
							Roles: []pwv1alpha1.WorkspaceMemberRole{
								pwv1alpha1.WorkspaceRoleAdmin,
							},
						},
					},
				},
			}

			err = saClient.Create(ctx, workspace)
			Expect(err).ShouldNot(HaveOccurred())
		})

		It("Should allow to create the workspace by a group in MemberOverrides", func() {
			var err error
			var workspaceName = uniqueName()

			namespace := &corev1.Namespace{
				ObjectMeta: metav1.ObjectMeta{
					Name: "project-test-group",
				},
			}

			err = k8sClient.Create(ctx, namespace)
			Expect(err).ShouldNot(HaveOccurred())

			sharedInformationForTests.MemberOverridesData = pwv1alpha1.MemberOverridesV2{
				{
					Subject: pwv1alpha1.Subject{
						Kind: "Group",
						Name: "system:admin",
					},
					Roles: []pwv1alpha1.OverrideRole{
						pwv1alpha1.OverrideRoleAdmin,
					},
					Resources: []pwv1alpha1.OverrideResource{
						{
							Kind: pwv1alpha1.OverrideResourceKindProject,
							Name: "test-group",
						},
						{
							Kind: pwv1alpha1.OverrideResourceKindWorkspace,
							Name: workspaceName,
						},
					},
				},
			}

			workspace := &pwv1alpha1.Workspace{
				ObjectMeta: metav1.ObjectMeta{
					Name:      workspaceName,
					Namespace: namespace.Name,
				},
				Spec: pwv1alpha1.WorkspaceSpec{
					Members: []pwv1alpha1.WorkspaceMember{
						{
							Subject: pwv1alpha1.Subject{
								Kind: "User",
								Name: "second-admin",
							},
							Roles: []pwv1alpha1.WorkspaceMemberRole{
								pwv1alpha1.WorkspaceRoleAdmin,
							},
						},
					},
				},
			}

			err = realUserClient.Create(ctx, workspace)
			Expect(err).ShouldNot(HaveOccurred())
		})

		It("Should deny to create the workspace when a user is not a workspace member or in MemberOverrides", func() {
			var err error
			var workspaceName = uniqueName()

			namespace := &corev1.Namespace{
				ObjectMeta: metav1.ObjectMeta{
					Name: "project-test3",
				},
			}

			err = k8sClient.Create(ctx, namespace)
			Expect(err).ShouldNot(HaveOccurred())

			sharedInformationForTests.MemberOverridesData = pwv1alpha1.MemberOverridesV2{
				{
					Subject: pwv1alpha1.Subject{
						Kind: "User",
						Name: "another-admin",
					},
					Roles: []pwv1alpha1.OverrideRole{
						pwv1alpha1.OverrideRoleAdmin,
					},
					Resources: []pwv1alpha1.OverrideResource{
						{
							Kind: pwv1alpha1.OverrideResourceKindProject,
							Name: "test3",
						},
						{
							Kind: pwv1alpha1.OverrideResourceKindWorkspace,
							Name: workspaceName,
						},
					},
				},
			}

			workspace := &pwv1alpha1.Workspace{
				ObjectMeta: metav1.ObjectMeta{
					Name:      workspaceName,
					Namespace: namespace.Name,
				},
				Spec: pwv1alpha1.WorkspaceSpec{
					Members: []pwv1alpha1.WorkspaceMember{
						{
							Subject: pwv1alpha1.Subject{
								Kind: "User",
								Name: "second-admin",
							},
							Roles: []pwv1alpha1.WorkspaceMemberRole{
								pwv1alpha1.WorkspaceRoleAdmin,
							},
						},
					},
				},
			}

			err = realUserClient.Create(ctx, workspace)
			Expect(err).To(HaveOccurred())
		})
	})

	Context("When updating a Workspace", func() {
		It("should deny removing self from the workspace", func() {
			var err error

			workspace := &pwv1alpha1.Workspace{
				ObjectMeta: metav1.ObjectMeta{
					Name:      uniqueName(),
					Namespace: "default",
				},
				Spec: pwv1alpha1.WorkspaceSpec{
					Members: []pwv1alpha1.WorkspaceMember{
						{
							Subject: pwv1alpha1.Subject{
								Kind: "User",
								Name: "admin",
							},
							Roles: []pwv1alpha1.WorkspaceMemberRole{
								pwv1alpha1.WorkspaceRoleAdmin,
							},
						},
					},
				},
			}

			err = realUserClient.Create(ctx, workspace)
			Expect(err).ShouldNot(HaveOccurred())

			workspace.Spec.Members = []pwv1alpha1.WorkspaceMember{}

			err = realUserClient.Update(ctx, workspace)
			Expect(err).To(HaveOccurred())
		})

		It("should deny updates workspace with MemberOverrides if there is no admin override for the parent project", func() {

			var err error
			var workspaceName = uniqueName()

			namespace := &corev1.Namespace{
				ObjectMeta: metav1.ObjectMeta{
					Name: "project-test-parent",
				},
			}

			err = k8sClient.Create(ctx, namespace)
			Expect(err).ShouldNot(HaveOccurred())

			sharedInformationForTests.MemberOverridesData = pwv1alpha1.MemberOverridesV2{
				{
					Subject: pwv1alpha1.Subject{
						Kind: "User",
						Name: "admin",
					},
					Roles: []pwv1alpha1.OverrideRole{
						pwv1alpha1.OverrideRoleAdmin,
					},
					Resources: []pwv1alpha1.OverrideResource{
						{
							Kind: pwv1alpha1.OverrideResourceKindProject,
							Name: "test-parent",
						},
						{
							Kind: pwv1alpha1.OverrideResourceKindWorkspace,
							Name: workspaceName,
						},
					},
				},
			}

			workspace := &pwv1alpha1.Workspace{
				ObjectMeta: metav1.ObjectMeta{
					Name:      workspaceName,
					Namespace: namespace.Name,
				},
				Spec: pwv1alpha1.WorkspaceSpec{
					Members: []pwv1alpha1.WorkspaceMember{
						{
							Subject: pwv1alpha1.Subject{
								Kind: "User",
								Name: "second-admin",
							},
							Roles: []pwv1alpha1.WorkspaceMemberRole{
								pwv1alpha1.WorkspaceRoleAdmin,
							},
						},
					},
				},
			}

			err = realUserClient.Create(ctx, workspace)
			Expect(err).ShouldNot(HaveOccurred())

			sharedInformationForTests.MemberOverridesData = pwv1alpha1.MemberOverridesV2{
				{
					Subject: pwv1alpha1.Subject{
						Kind: "User",
						Name: "admin",
					},
					Roles: []pwv1alpha1.OverrideRole{
						pwv1alpha1.OverrideRoleAdmin,
					},
					Resources: []pwv1alpha1.OverrideResource{
						{
							Kind: pwv1alpha1.OverrideResourceKindWorkspace,
							Name: workspaceName,
						},
					},
				},
			}

			workspace.Labels = map[string]string{"key": "value"}
			err = realUserClient.Update(ctx, workspace)
			GinkgoLogr.Info("%v", err)
			Expect(err).To(HaveOccurred())
		})
	})
})
