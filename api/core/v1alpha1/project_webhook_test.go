package v1alpha1

import (
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

var _ = Describe("Project Webhook", func() {
	BeforeEach(func() {
		EnforceChargingTargetLabel = false
		// this must be cleaned with each run because it's name is passed to the webhook at startup. Creating a new one with a different name won't work.
		err := k8sClient.Delete(ctx, &MemberOverrides{
			ObjectMeta: metav1.ObjectMeta{
				Name: "test-override",
			},
		})
		Expect(err).To(Or(BeNil(), MatchError(apierrors.IsNotFound, "NotFound")))
	})

	Context("When creating a Project", func() {
		It("Should allow to create the project by the admin user", func() {
			var err error

			project := &Project{
				ObjectMeta: metav1.ObjectMeta{
					Name: uniqueName(),
				},
				Spec: ProjectSpec{
					Members: []ProjectMember{
						{
							Subject: Subject{
								Kind: "User",
								Name: "admin",
							},
							Roles: []ProjectMemberRole{
								ProjectRoleAdmin,
							},
						},
					},
				},
			}

			err = realUserClient.Create(ctx, project)
			Expect(err).ShouldNot(HaveOccurred())
		})

		It("Should allow to create the project by a serviceaccount", func() {
			var err error

			project := &Project{
				ObjectMeta: metav1.ObjectMeta{
					Name: uniqueName(),
				},
				Spec: ProjectSpec{
					Members: []ProjectMember{
						{
							Subject: Subject{
								Kind:      "ServiceAccount",
								Name:      "admin",
								Namespace: "kube-system",
							},
							Roles: []ProjectMemberRole{
								ProjectRoleAdmin,
							},
						},
					},
				},
			}

			err = saClient.Create(ctx, project)
			Expect(err).ShouldNot(HaveOccurred())
		})

		It("should deny to create the project by a non-member user", func() {
			var err error

			project := &Project{
				ObjectMeta: metav1.ObjectMeta{
					Name: uniqueName(),
				},
				Spec: ProjectSpec{
					Members: []ProjectMember{
						{
							Subject: Subject{
								Kind: "User",
								Name: "unknown",
							},
							Roles: []ProjectMemberRole{
								ProjectRoleAdmin,
							},
						},
					},
				},
			}

			err = realUserClient.Create(ctx, project)
			Expect(err).To(HaveOccurred())
		})

		It("should allow to create a project with the charging-target label", func() {
			var err error

			EnforceChargingTargetLabel = true

			project := &Project{
				ObjectMeta: metav1.ObjectMeta{
					Name: uniqueName(),
					Labels: map[string]string{
						ChargingTargetLabel: "test",
					},
				},
				Spec: ProjectSpec{
					Members: []ProjectMember{
						{
							Subject: Subject{
								Kind: "User",
								Name: "admin",
							},
							Roles: []ProjectMemberRole{
								ProjectRoleAdmin,
							},
						},
					},
				},
			}

			err = realUserClient.Create(ctx, project)
			Expect(err).ShouldNot(HaveOccurred())
		})

		It("should deny to create a project without the charging-target label", func() {
			var err error

			EnforceChargingTargetLabel = true

			project := &Project{
				ObjectMeta: metav1.ObjectMeta{
					Name: uniqueName(),
				},
				Spec: ProjectSpec{
					Members: []ProjectMember{
						{
							Subject: Subject{
								Kind: "User",
								Name: "admin",
							},
							Roles: []ProjectMemberRole{
								ProjectRoleAdmin,
							},
						},
					},
				},
			}

			err = realUserClient.Create(ctx, project)
			Expect(err).To(HaveOccurred())
		})
		It("Should allow to create the project by a user in MemberOverrides", func() {
			var err error
			var projectName = uniqueName()

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
									Name: projectName,
								},
							},
						},
					},
				},
			}

			err = k8sClient.Create(ctx, override)
			Expect(err).ShouldNot(HaveOccurred())

			project := &Project{
				ObjectMeta: metav1.ObjectMeta{
					Name: projectName,
					Labels: map[string]string{
						ChargingTargetLabel: "test",
					},
				},
				Spec: ProjectSpec{
					Members: []ProjectMember{
						{
							Subject: Subject{
								Kind: "User",
								Name: "second-admin",
							},
							Roles: []ProjectMemberRole{
								ProjectRoleAdmin,
							},
						},
					},
				},
			}

			err = realUserClient.Create(ctx, project)
			Expect(err).ShouldNot(HaveOccurred())
		})

		It("Should allow to create the project by a serviceaccount in MemeberOverrides", func() {
			var err error
			var projectName = uniqueName()

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
									Name: projectName,
								},
							},
						},
					},
				},
			}

			err = k8sClient.Create(ctx, override)
			Expect(err).ShouldNot(HaveOccurred())

			project := &Project{
				ObjectMeta: metav1.ObjectMeta{
					Name: projectName,
					Labels: map[string]string{
						ChargingTargetLabel: "test",
					},
				},
				Spec: ProjectSpec{
					Members: []ProjectMember{
						{
							Subject: Subject{
								Kind:      "ServiceAccount",
								Name:      "second-admin",
								Namespace: "kube-system",
							},
							Roles: []ProjectMemberRole{
								ProjectRoleAdmin,
							},
						},
					},
				},
			}

			err = saClient.Create(ctx, project)
			Expect(err).ShouldNot(HaveOccurred())
		})

		It("Should allow to create the project by a group in MemberOverrides", func() {
			var err error
			var projectName = uniqueName()

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
									Name: projectName,
								},
							},
						},
					},
				},
			}

			err = k8sClient.Create(ctx, override)
			Expect(err).ShouldNot(HaveOccurred())

			project := &Project{
				ObjectMeta: metav1.ObjectMeta{
					Name: projectName,
					Labels: map[string]string{
						ChargingTargetLabel: "test",
					},
				},
				Spec: ProjectSpec{
					Members: []ProjectMember{
						{
							Subject: Subject{
								Kind: "User",
								Name: "second-admin",
							},
							Roles: []ProjectMemberRole{
								ProjectRoleAdmin,
							},
						},
					},
				},
			}

			err = realUserClient.Create(ctx, project)
			Expect(err).ShouldNot(HaveOccurred())
		})

		It("Should deny to create the project when a user is not project member or in MemberOverrides", func() {
			var err error
			var projectName = uniqueName()

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
									Name: projectName,
								},
							},
						},
					},
				},
			}

			err = k8sClient.Create(ctx, override)
			Expect(err).ShouldNot(HaveOccurred())

			project := &Project{
				ObjectMeta: metav1.ObjectMeta{
					Name: projectName,
					Labels: map[string]string{
						ChargingTargetLabel: "test",
					},
				},
				Spec: ProjectSpec{
					Members: []ProjectMember{
						{
							Subject: Subject{
								Kind: "User",
								Name: "second-admin",
							},
							Roles: []ProjectMemberRole{
								ProjectRoleAdmin,
							},
						},
					},
				},
			}

			err = realUserClient.Create(ctx, project)
			Expect(err).To(HaveOccurred())
		})
	})

	Context("When updating a Project", func() {
		It("should deny removing self from the project", func() {
			var err error

			project := &Project{
				ObjectMeta: metav1.ObjectMeta{
					Name: uniqueName(),
				},
				Spec: ProjectSpec{
					Members: []ProjectMember{
						{
							Subject: Subject{
								Kind: "User",
								Name: "admin",
							},
							Roles: []ProjectMemberRole{
								ProjectRoleAdmin,
							},
						},
					},
				},
			}

			err = realUserClient.Create(ctx, project)
			Expect(err).ShouldNot(HaveOccurred())

			project.Spec.Members = []ProjectMember{}

			err = realUserClient.Update(ctx, project)
			Expect(err).To(HaveOccurred())
		})

		It("should deny removing the charging-target label", func() {
			var err error

			EnforceChargingTargetLabel = true

			project := &Project{
				ObjectMeta: metav1.ObjectMeta{
					Name: uniqueName(),
					Labels: map[string]string{
						ChargingTargetLabel: "test",
					},
				},
				Spec: ProjectSpec{
					Members: []ProjectMember{
						{
							Subject: Subject{
								Kind: "User",
								Name: "admin",
							},
							Roles: []ProjectMemberRole{
								ProjectRoleAdmin,
							},
						},
					},
				},
			}

			err = realUserClient.Create(ctx, project)
			Expect(err).ShouldNot(HaveOccurred())

			project.Labels = nil

			err = realUserClient.Update(ctx, project)
			Expect(err).To(HaveOccurred())
		})

		It("Should allow to update the project by a user in MemberOverrides", func() {
			var err error
			var projectName = uniqueName()

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
									Name: projectName,
								},
							},
						},
					},
				},
			}

			err = k8sClient.Create(ctx, override)
			Expect(err).ShouldNot(HaveOccurred())

			project := &Project{
				ObjectMeta: metav1.ObjectMeta{
					Name: projectName,
				},
				Spec: ProjectSpec{
					Members: []ProjectMember{
						{
							Subject: Subject{
								Kind: "User",
								Name: "second-admin",
							},
							Roles: []ProjectMemberRole{
								ProjectRoleAdmin,
							},
						},
					},
				},
			}

			err = realUserClient.Create(ctx, project)
			Expect(err).ShouldNot(HaveOccurred())

			project.Labels = map[string]string{"key": "value"}

			err = realUserClient.Update(ctx, project)
			Expect(err).ToNot(HaveOccurred())

		})
	})
})
