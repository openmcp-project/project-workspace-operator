package v1alpha1

import (
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

var _ = Describe("Workspace Webhook", func() {
	BeforeEach(func() {
		// this must be cleaned with each run because it's name is passed to the webhook at startup. Creating a new one with a different name won't work.
		err := k8sClient.Delete(ctx, &MemberOverrides{
			ObjectMeta: metav1.ObjectMeta{
				Name: "test-override",
			},
		})
		Expect(err).To(Or(BeNil(), MatchError(apierrors.IsNotFound, "NotFound")))
	})

	Context("When creating a Workspace", func() {
		It("Should allow to create the workspace by the admin user", func() {
			var err error

			workspace := &Workspace{
				ObjectMeta: metav1.ObjectMeta{
					Name:      uniqueName(),
					Namespace: "default",
				},
				Spec: WorkspaceSpec{
					Members: []WorkspaceMember{
						{
							Subject: Subject{
								Kind: "User",
								Name: "admin",
							},
							Roles: []WorkspaceMemberRole{
								WorkspaceRoleAdmin,
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

			workspace := &Workspace{
				ObjectMeta: metav1.ObjectMeta{
					Name:      uniqueName(),
					Namespace: "default",
				},
				Spec: WorkspaceSpec{
					Members: []WorkspaceMember{
						{
							Subject: Subject{
								Kind:      "ServiceAccount",
								Name:      "admin",
								Namespace: "kube-system",
							},
							Roles: []WorkspaceMemberRole{
								WorkspaceRoleAdmin,
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

			project := &Workspace{
				ObjectMeta: metav1.ObjectMeta{
					Name:      uniqueName(),
					Namespace: "default",
				},
				Spec: WorkspaceSpec{
					Members: []WorkspaceMember{
						{
							Subject: Subject{
								Kind: "User",
								Name: "unknown",
							},
							Roles: []WorkspaceMemberRole{
								WorkspaceRoleAdmin,
							},
						},
					},
				},
			}

			err = realUserClient.Create(ctx, project)
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

			override := &MemberOverrides{
				ObjectMeta: metav1.ObjectMeta{
					Name: "test-override",
				},
				Spec: MemberOverridesSpec{
					MemberOverrides: []MemberOverride{
						{
							Subject: Subject{
								Kind: "User",
								Name: "admin",
							},
							Roles: []OverrideRole{
								OverrideRoleAdmin,
							},
							Resources: []OverrideResource{
								{
									Kind: "project",
									Name: "test",
								},
								{
									Kind: "workspace",
									Name: workspaceName,
								},
							},
						},
					},
				},
			}

			err = k8sClient.Create(ctx, override)
			Expect(err).ShouldNot(HaveOccurred())

			workspace := &Workspace{
				ObjectMeta: metav1.ObjectMeta{
					Name:      workspaceName,
					Namespace: namespace.Name,
				},
				Spec: WorkspaceSpec{
					Members: []WorkspaceMember{
						{
							Subject: Subject{
								Kind: "User",
								Name: "second-admin",
							},
							Roles: []WorkspaceMemberRole{
								WorkspaceRoleAdmin,
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

			override := &MemberOverrides{
				ObjectMeta: metav1.ObjectMeta{
					Name: "test-override",
				},
				Spec: MemberOverridesSpec{
					MemberOverrides: []MemberOverride{
						{
							Subject: Subject{
								Kind:      "ServiceAccount",
								Name:      "admin",
								Namespace: "kube-system",
							},
							Roles: []OverrideRole{
								OverrideRoleAdmin,
							},
							Resources: []OverrideResource{
								{
									Kind: "project",
									Name: "test-sa",
								},
								{
									Kind: "workspace",
									Name: workspaceName,
								},
							},
						},
					},
				},
			}

			err = k8sClient.Create(ctx, override)
			Expect(err).ShouldNot(HaveOccurred())
			workspace := &Workspace{
				ObjectMeta: metav1.ObjectMeta{
					Name:      workspaceName,
					Namespace: namespace.Name,
				},
				Spec: WorkspaceSpec{
					Members: []WorkspaceMember{
						{
							Subject: Subject{
								Kind:      "ServiceAccount",
								Name:      "second-admin",
								Namespace: "kube-system",
							},
							Roles: []WorkspaceMemberRole{
								WorkspaceRoleAdmin,
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

			override := &MemberOverrides{
				ObjectMeta: metav1.ObjectMeta{
					Name: "test-override",
				},
				Spec: MemberOverridesSpec{
					MemberOverrides: []MemberOverride{
						{
							Subject: Subject{
								Kind: "Group",
								Name: "system:admin",
							},
							Roles: []OverrideRole{
								OverrideRoleAdmin,
							},
							Resources: []OverrideResource{
								{
									Kind: "project",
									Name: "test-group",
								},
								{
									Kind: "workspace",
									Name: workspaceName,
								},
							},
						},
					},
				},
			}

			err = k8sClient.Create(ctx, override)
			Expect(err).ShouldNot(HaveOccurred())

			workspace := &Workspace{
				ObjectMeta: metav1.ObjectMeta{
					Name:      workspaceName,
					Namespace: namespace.Name,
				},
				Spec: WorkspaceSpec{
					Members: []WorkspaceMember{
						{
							Subject: Subject{
								Kind: "User",
								Name: "second-admin",
							},
							Roles: []WorkspaceMemberRole{
								WorkspaceRoleAdmin,
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

			override := &MemberOverrides{
				ObjectMeta: metav1.ObjectMeta{
					Name: "test-override",
				},
				Spec: MemberOverridesSpec{
					MemberOverrides: []MemberOverride{
						{
							Subject: Subject{
								Kind: "User",
								Name: "another-admin",
							},
							Roles: []OverrideRole{
								OverrideRoleAdmin,
							},
							Resources: []OverrideResource{
								{
									Kind: "project",
									Name: "test3",
								},
								{
									Kind: "workspace",
									Name: workspaceName,
								},
							},
						},
					},
				},
			}

			err = k8sClient.Create(ctx, override)
			Expect(err).ShouldNot(HaveOccurred())

			workspace := &Workspace{
				ObjectMeta: metav1.ObjectMeta{
					Name:      workspaceName,
					Namespace: namespace.Name,
				},
				Spec: WorkspaceSpec{
					Members: []WorkspaceMember{
						{
							Subject: Subject{
								Kind: "User",
								Name: "second-admin",
							},
							Roles: []WorkspaceMemberRole{
								WorkspaceRoleAdmin,
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

			workspace := &Workspace{
				ObjectMeta: metav1.ObjectMeta{
					Name:      uniqueName(),
					Namespace: "default",
				},
				Spec: WorkspaceSpec{
					Members: []WorkspaceMember{
						{
							Subject: Subject{
								Kind: "User",
								Name: "admin",
							},
							Roles: []WorkspaceMemberRole{
								WorkspaceRoleAdmin,
							},
						},
					},
				},
			}

			err = realUserClient.Create(ctx, workspace)
			Expect(err).ShouldNot(HaveOccurred())

			workspace.Spec.Members = []WorkspaceMember{}

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

			override := &MemberOverrides{
				ObjectMeta: metav1.ObjectMeta{
					Name: "test-override",
				},
				Spec: MemberOverridesSpec{
					MemberOverrides: []MemberOverride{
						{
							Subject: Subject{
								Kind: "User",
								Name: "admin",
							},
							Roles: []OverrideRole{
								OverrideRoleAdmin,
							},
							Resources: []OverrideResource{
								{
									Kind: "project",
									Name: "test-parent",
								},
								{
									Kind: "workspace",
									Name: workspaceName,
								},
							},
						},
					},
				},
			}

			err = k8sClient.Create(ctx, override)
			Expect(err).ShouldNot(HaveOccurred())

			workspace := &Workspace{
				ObjectMeta: metav1.ObjectMeta{
					Name:      workspaceName,
					Namespace: namespace.Name,
				},
				Spec: WorkspaceSpec{
					Members: []WorkspaceMember{
						{
							Subject: Subject{
								Kind: "User",
								Name: "second-admin",
							},
							Roles: []WorkspaceMemberRole{
								WorkspaceRoleAdmin,
							},
						},
					},
				},
			}

			err = realUserClient.Create(ctx, workspace)
			Expect(err).ShouldNot(HaveOccurred())

			override.Spec.MemberOverrides = []MemberOverride{
				{
					Subject: Subject{
						Kind: "User",
						Name: "admin",
					},
					Roles: []OverrideRole{
						OverrideRoleAdmin,
					},
					Resources: []OverrideResource{
						{
							Kind: "workspace",
							Name: workspaceName,
						},
					},
				},
			}
			err = k8sClient.Update(ctx, override)
			Expect(err).ShouldNot(HaveOccurred())

			workspace.Labels = map[string]string{"key": "value"}
			err = realUserClient.Update(ctx, workspace)
			GinkgoLogr.Info("%v", err)
			Expect(err).To(HaveOccurred())
		})
	})
})
