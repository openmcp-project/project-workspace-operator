package webhooks

import (
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	pwv1alpha1 "github.com/openmcp-project/project-workspace-operator/api/core/v1alpha1"
)

var _ = Describe("Project Webhook", func() {
	BeforeEach(func() {
		sharedInformationForTests.MemberOverridesData = nil
	})

	Context("When creating a Project", func() {
		It("Should allow to create the project by the admin user", func() {
			var err error

			project := &pwv1alpha1.Project{
				ObjectMeta: metav1.ObjectMeta{
					Name: uniqueName(),
				},
				Spec: pwv1alpha1.ProjectSpec{
					Members: []pwv1alpha1.ProjectMember{
						{
							Subject: pwv1alpha1.Subject{
								Kind: "User",
								Name: "admin",
							},
							Roles: []pwv1alpha1.ProjectMemberRole{
								pwv1alpha1.ProjectRoleAdmin,
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

			project := &pwv1alpha1.Project{
				ObjectMeta: metav1.ObjectMeta{
					Name: uniqueName(),
				},
				Spec: pwv1alpha1.ProjectSpec{
					Members: []pwv1alpha1.ProjectMember{
						{
							Subject: pwv1alpha1.Subject{
								Kind:      "ServiceAccount",
								Name:      "admin",
								Namespace: "kube-system",
							},
							Roles: []pwv1alpha1.ProjectMemberRole{
								pwv1alpha1.ProjectRoleAdmin,
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

			project := &pwv1alpha1.Project{
				ObjectMeta: metav1.ObjectMeta{
					Name: uniqueName(),
				},
				Spec: pwv1alpha1.ProjectSpec{
					Members: []pwv1alpha1.ProjectMember{
						{
							Subject: pwv1alpha1.Subject{
								Kind: "User",
								Name: "unknown",
							},
							Roles: []pwv1alpha1.ProjectMemberRole{
								pwv1alpha1.ProjectRoleAdmin,
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
							Name: projectName,
						},
					},
				},
			}

			project := &pwv1alpha1.Project{
				ObjectMeta: metav1.ObjectMeta{
					Name: projectName,
				},
				Spec: pwv1alpha1.ProjectSpec{
					Members: []pwv1alpha1.ProjectMember{
						{
							Subject: pwv1alpha1.Subject{
								Kind: "User",
								Name: "second-admin",
							},
							Roles: []pwv1alpha1.ProjectMemberRole{
								pwv1alpha1.ProjectRoleAdmin,
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
							Name: projectName,
						},
					},
				},
			}

			project := &pwv1alpha1.Project{
				ObjectMeta: metav1.ObjectMeta{
					Name: projectName,
				},
				Spec: pwv1alpha1.ProjectSpec{
					Members: []pwv1alpha1.ProjectMember{
						{
							Subject: pwv1alpha1.Subject{
								Kind:      "ServiceAccount",
								Name:      "second-admin",
								Namespace: "kube-system",
							},
							Roles: []pwv1alpha1.ProjectMemberRole{
								pwv1alpha1.ProjectRoleAdmin,
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
							Name: projectName,
						},
					},
				},
			}

			project := &pwv1alpha1.Project{
				ObjectMeta: metav1.ObjectMeta{
					Name: projectName,
				},
				Spec: pwv1alpha1.ProjectSpec{
					Members: []pwv1alpha1.ProjectMember{
						{
							Subject: pwv1alpha1.Subject{
								Kind: "User",
								Name: "second-admin",
							},
							Roles: []pwv1alpha1.ProjectMemberRole{
								pwv1alpha1.ProjectRoleAdmin,
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
							Name: projectName,
						},
					},
				},
			}

			project := &pwv1alpha1.Project{
				ObjectMeta: metav1.ObjectMeta{
					Name: projectName,
				},
				Spec: pwv1alpha1.ProjectSpec{
					Members: []pwv1alpha1.ProjectMember{
						{
							Subject: pwv1alpha1.Subject{
								Kind: "User",
								Name: "second-admin",
							},
							Roles: []pwv1alpha1.ProjectMemberRole{
								pwv1alpha1.ProjectRoleAdmin,
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

			project := &pwv1alpha1.Project{
				ObjectMeta: metav1.ObjectMeta{
					Name: uniqueName(),
				},
				Spec: pwv1alpha1.ProjectSpec{
					Members: []pwv1alpha1.ProjectMember{
						{
							Subject: pwv1alpha1.Subject{
								Kind: "User",
								Name: "admin",
							},
							Roles: []pwv1alpha1.ProjectMemberRole{
								pwv1alpha1.ProjectRoleAdmin,
							},
						},
					},
				},
			}

			err = realUserClient.Create(ctx, project)
			Expect(err).ShouldNot(HaveOccurred())

			project.Spec.Members = []pwv1alpha1.ProjectMember{}

			err = realUserClient.Update(ctx, project)
			Expect(err).To(HaveOccurred())
		})

		It("Should allow to update the project by a user in MemberOverrides", func() {
			var err error
			var projectName = uniqueName()

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
							Name: projectName,
						},
					},
				},
			}

			project := &pwv1alpha1.Project{
				ObjectMeta: metav1.ObjectMeta{
					Name: projectName,
				},
				Spec: pwv1alpha1.ProjectSpec{
					Members: []pwv1alpha1.ProjectMember{
						{
							Subject: pwv1alpha1.Subject{
								Kind: "User",
								Name: "second-admin",
							},
							Roles: []pwv1alpha1.ProjectMemberRole{
								pwv1alpha1.ProjectRoleAdmin,
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
