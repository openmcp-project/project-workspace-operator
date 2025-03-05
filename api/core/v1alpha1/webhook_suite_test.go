package v1alpha1

import (
	"context"
	"crypto/tls"
	"encoding/hex"
	"fmt"
	"net"
	"path/filepath"
	"testing"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	//+kubebuilder:scaffold:imports

	"github.com/google/uuid"
	envtestutil "github.com/openmcp-project/controller-utils/pkg/envtest"
	admissionv1 "k8s.io/api/admission/v1"
	corev1 "k8s.io/api/core/v1"
	rbacv1 "k8s.io/api/rbac/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	apimachineryruntime "k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/rest"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/envtest"
	logf "sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/log/zap"
	metricsserver "sigs.k8s.io/controller-runtime/pkg/metrics/server"
	"sigs.k8s.io/controller-runtime/pkg/webhook"
)

// These tests use Ginkgo (BDD-style Go testing framework). Refer to
// http://onsi.github.io/ginkgo/ to learn more about Ginkgo.

var cfg *rest.Config
var k8sClient client.Client
var saClient client.Client
var realUserClient client.Client
var testEnv *envtest.Environment
var ctx context.Context
var cancel context.CancelFunc
var testScheme *apimachineryruntime.Scheme

func TestAPIs(t *testing.T) {
	RegisterFailHandler(Fail)

	RunSpecs(t, "Webhook Suite")
}

var _ = BeforeSuite(func() {
	logf.SetLogger(zap.New(zap.WriteTo(GinkgoWriter), zap.UseDevMode(true)))

	ctx, cancel = context.WithCancel(context.TODO())

	By("bootstrapping test environment")
	var err error
	err = envtestutil.Install("latest")
	Expect(err).NotTo(HaveOccurred())

	testEnv = &envtest.Environment{
		CRDDirectoryPaths:     []string{filepath.Join("..", "..", "..", "config", "crd", "bases")},
		ErrorIfCRDPathMissing: false,
		WebhookInstallOptions: envtest.WebhookInstallOptions{
			Paths: []string{filepath.Join("..", "..", "..", "config", "webhook")},
		},
	}

	// cfg is defined in this file globally.
	cfg, err = testEnv.Start()
	Expect(err).NotTo(HaveOccurred())
	Expect(cfg).NotTo(BeNil())

	testScheme = apimachineryruntime.NewScheme()
	err = AddToScheme(testScheme)
	Expect(err).NotTo(HaveOccurred())

	err = admissionv1.AddToScheme(testScheme)
	Expect(err).NotTo(HaveOccurred())

	err = rbacv1.AddToScheme(testScheme)
	Expect(err).NotTo(HaveOccurred())

	err = corev1.AddToScheme(testScheme)
	Expect(err).NotTo(HaveOccurred())
	//+kubebuilder:scaffold:scheme

	k8sClient, err = client.New(cfg, client.Options{Scheme: testScheme})
	Expect(err).NotTo(HaveOccurred())
	Expect(k8sClient).NotTo(BeNil())

	// start webhook server using Manager
	webhookInstallOptions := &testEnv.WebhookInstallOptions
	mgr, err := ctrl.NewManager(cfg, ctrl.Options{
		Scheme: testScheme,
		WebhookServer: webhook.NewServer(webhook.Options{
			Host:    webhookInstallOptions.LocalServingHost,
			Port:    webhookInstallOptions.LocalServingPort,
			CertDir: webhookInstallOptions.LocalServingCertDir,
		}),
		LeaderElection: false,
		Metrics:        metricsserver.Options{BindAddress: "0"},
	})
	Expect(err).NotTo(HaveOccurred())

	err = (&Project{}).SetupWebhookWithManager(mgr, "test-override")
	Expect(err).NotTo(HaveOccurred())

	err = (&Workspace{}).SetupWebhookWithManager(mgr, "test-override")
	Expect(err).NotTo(HaveOccurred())

	//+kubebuilder:scaffold:webhook

	go func() {
		defer GinkgoRecover()
		err = mgr.Start(ctx)
		Expect(err).NotTo(HaveOccurred())
	}()

	// wait for the webhook server to get ready
	dialer := &net.Dialer{Timeout: time.Second}
	addrPort := fmt.Sprintf("%s:%d", webhookInstallOptions.LocalServingHost, webhookInstallOptions.LocalServingPort)
	Eventually(func() error {
		conn, err := tls.DialWithDialer(dialer, "tcp", addrPort, &tls.Config{InsecureSkipVerify: true})
		if err != nil {
			return err
		}
		conn.Close()
		return nil
	}).Should(Succeed())

	saClient = impersonate("system:serviceaccount:kube-system:admin", []string{
		"system:authenticated",
		"system:admin",
	})

	realUserClient = impersonate("admin", []string{
		"system:authenticated",
		"system:admin",
	})

	clusterRole := &rbacv1.ClusterRole{
		ObjectMeta: metav1.ObjectMeta{
			Name: "system:admin",
		},
		Rules: []rbacv1.PolicyRule{
			{
				APIGroups: []string{GroupVersion.Group},
				Resources: []string{"projects", "workspaces"},
				Verbs:     []string{"*"},
			},
		},
	}

	err = k8sClient.Create(ctx, clusterRole)
	if err != nil {
		if apierrors.IsAlreadyExists(err) {
			err = k8sClient.Update(ctx, clusterRole)
			Expect(err).ShouldNot(HaveOccurred())
		} else {
			Fail("Failed to create/update cluster role")
		}
	}

	clusterRoleBinding := &rbacv1.ClusterRoleBinding{
		ObjectMeta: metav1.ObjectMeta{
			Name: "system:admin",
		},
		RoleRef: rbacv1.RoleRef{
			Kind:     "ClusterRole",
			Name:     "system:admin",
			APIGroup: "rbac.authorization.k8s.io",
		},
		Subjects: []rbacv1.Subject{
			{
				Kind:     "User",
				Name:     "admin",
				APIGroup: "rbac.authorization.k8s.io",
			},
			{
				Kind:      "ServiceAccount",
				Name:      "admin",
				Namespace: "kube-system",
			},
		},
	}

	err = k8sClient.Create(ctx, clusterRoleBinding)
	if err != nil {
		if apierrors.IsAlreadyExists(err) {
			err = k8sClient.Update(ctx, clusterRoleBinding)
			Expect(err).ShouldNot(HaveOccurred())
		} else {
			Fail("Failed to create/update cluster role binding")
		}
	}
})

var _ = AfterSuite(func() {
	cancel()
	By("tearing down the test environment")
	err := testEnv.Stop()
	Expect(err).NotTo(HaveOccurred())
})

func impersonate(userName string, groups []string) client.Client {
	cfg.Impersonate = rest.ImpersonationConfig{
		UserName: userName,
		Groups:   groups,
	}

	client, err := client.New(cfg, client.Options{Scheme: testScheme})
	Expect(err).NotTo(HaveOccurred())

	return client
}

func uniqueName() string {
	uuidBin, err := uuid.New().MarshalBinary()
	Expect(err).ToNot(HaveOccurred())
	uuidHex := make([]byte, 32)
	hex.Encode(uuidHex, uuidBin)
	uuidStr := string(uuidHex[:25])
	return uuidStr
}
