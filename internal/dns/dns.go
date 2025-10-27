package dns

import (
	"context"
	"fmt"
	"time"

	"github.com/openmcp-project/controller-utils/pkg/clusters"
	"github.com/openmcp-project/controller-utils/pkg/controller"
	"github.com/openmcp-project/controller-utils/pkg/logging"
	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/utils/ptr"
	controllerruntime "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
	gatewayv1 "sigs.k8s.io/gateway-api/apis/v1"
	gatewayv1alpha2 "sigs.k8s.io/gateway-api/apis/v1alpha2"
)

const (
	DefaultGatewayName      = "default"
	DefaultGatewayNamespace = "openmcp-system"
	DNSAnnotationKey        = "dns.openmcp.cloud/base-domain"
	RequeueInterval         = 20 * time.Second
)

// Reconciler is a reconciler for managing DNS records using Gateway API resources.
type Reconciler struct {
}

// Instance represents a service instance for that the TLSRoute will be managed.
type Instance struct {
	// Namespace in which the TLSRoute will be created.
	Namespace string
	// Name of the TLSRoute.
	Name string
	// SubDomainPrefix is the prefix for the subdomain that will be created for the instance.
	SubDomainPrefix string
	// BackendName is the name of the backend service to which the TLSRoute will route traffic.
	BackendName string
	// BackendPort is the port of the backend service to which the TLSRoute will route traffic.
	BackendPort int32
}

// GatewayReconcileResult is the result of a gateway reconciliation.
// If Result.Requeue is not set, the gateway is ready and the HostName can be used.
type GatewayReconcileResult struct {
	// HostName is the hostname that was created for the instance and can be used for DNS records.
	HostName string
	// Result is the result of the reconciliation.
	reconcile.Result
}

// NewReconciler creates a new DNS reconciler.
func NewReconciler() *Reconciler {
	return &Reconciler{}
}

// ReconcileGateway ensures that the default gateway exists and retrieves the base domain from its annotations.
// It returns the full hostname for the given instance that can be used for DNS records.
// If the default gateway is not found, it will requeue after a predefined interval.
func (r *Reconciler) ReconcileGateway(ctx context.Context, instance *Instance, targetCluster *clusters.Cluster) (GatewayReconcileResult, error) {
	log := logging.FromContextOrPanic(ctx)

	var err error

	// get default gateway

	gateway := &gatewayv1.Gateway{}
	gateway.SetName(DefaultGatewayName)
	gateway.SetNamespace(DefaultGatewayNamespace)

	if err = targetCluster.Client().Get(ctx, client.ObjectKeyFromObject(gateway), gateway); err != nil {
		if errors.IsNotFound(err) {
			log.Debug("Default gateway not found, requeueing...")
			// default gateway not found
			return GatewayReconcileResult{
				Result: reconcile.Result{
					RequeueAfter: RequeueInterval,
				},
			}, nil
		}

		return GatewayReconcileResult{Result: reconcile.Result{}}, fmt.Errorf("failed to get default gateway: %w", err)
	}

	log.Debug("Default Gateway available")

	baseDomain, hasBaseDomain := getBaseDomain(gateway)
	if !hasBaseDomain {
		return GatewayReconcileResult{Result: reconcile.Result{}}, fmt.Errorf("gateway is missing the %s annotation", DNSAnnotationKey)
	}

	log.Debug("Base domain found", "baseDomain", baseDomain)

	hostName := getHostName(baseDomain, instance)

	return GatewayReconcileResult{
		HostName: hostName,
		Result:   reconcile.Result{},
	}, nil
}

// ReconcileTLSRoute ensures that a TLSRoute exists for the given instance, pointing to the default gateway.
func (r *Reconciler) ReconcileTLSRoute(ctx context.Context, instance *Instance, targetCluster *clusters.Cluster) error {
	// get default gateway

	var err error

	gateway := &gatewayv1.Gateway{}
	gateway.SetName(DefaultGatewayName)
	gateway.SetNamespace(DefaultGatewayNamespace)

	if err = targetCluster.Client().Get(ctx, client.ObjectKeyFromObject(gateway), gateway); err != nil {
		return fmt.Errorf("failed to get default gateway: %w", err)
	}

	baseDomain, hasBaseDomain := getBaseDomain(gateway)
	if !hasBaseDomain {
		return fmt.Errorf("gateway is missing the %s annotation", DNSAnnotationKey)
	}

	hostName := getHostName(baseDomain, instance)

	tlsRoute := &gatewayv1alpha2.TLSRoute{}
	tlsRoute.SetName(instance.Name)
	tlsRoute.SetNamespace(instance.Namespace)

	_, err = controllerruntime.CreateOrUpdate(ctx, targetCluster.Client(), tlsRoute, func() error {
		tlsRoute.Spec = gatewayv1alpha2.TLSRouteSpec{
			CommonRouteSpec: gatewayv1alpha2.CommonRouteSpec{
				ParentRefs: []gatewayv1alpha2.ParentReference{
					{
						Name:      gatewayv1.ObjectName(gateway.Name),
						Namespace: ptr.To(gatewayv1.Namespace(gateway.Namespace)),
					},
				},
			},
			Hostnames: []gatewayv1alpha2.Hostname{
				gatewayv1alpha2.Hostname(hostName),
			},
			Rules: []gatewayv1alpha2.TLSRouteRule{
				{
					BackendRefs: []gatewayv1alpha2.BackendRef{
						{
							BackendObjectReference: gatewayv1alpha2.BackendObjectReference{
								Name: gatewayv1.ObjectName(instance.BackendName),
								Port: ptr.To(instance.BackendPort),
							},
						},
					},
				},
			},
		}
		return nil
	})

	if err != nil {
		return fmt.Errorf("failed to create or update TLSRoute: %w", err)
	}

	return nil
}

// IsTLSRouteReady checks if the TLSRoute for the given instance is accepted by the default gateway.
func (r *Reconciler) IsTLSRouteReady(ctx context.Context, instance *Instance, targetCluster *clusters.Cluster) (bool, error) {
	log := logging.FromContextOrPanic(ctx)

	var err error

	tlsRoute := &gatewayv1alpha2.TLSRoute{}
	tlsRoute.SetName(instance.Name)
	tlsRoute.SetNamespace(instance.Namespace)

	if err = targetCluster.Client().Get(ctx, client.ObjectKeyFromObject(tlsRoute), tlsRoute); err != nil {
		return false, fmt.Errorf("failed to get TLSRoute: %w", err)
	}

	for _, parent := range tlsRoute.Status.Parents {
		if parent.ParentRef.Name == DefaultGatewayName && parent.ParentRef.Namespace != nil && *parent.ParentRef.Namespace == DefaultGatewayNamespace {
			for _, cond := range parent.Conditions {
				if cond.Type == string(gatewayv1alpha2.RouteConditionAccepted) && cond.Status == "True" {
					log.Debug("TLSRoute is accepted by the gateway")
					return true, nil
				}
			}
		}
	}

	return false, nil
}

// DeleteTLSRoute deletes the TLSRoute for the given instance.
func (r *Reconciler) DeleteTLSRoute(ctx context.Context, instance *Instance, targetCluster *clusters.Cluster) error {
	log := logging.FromContextOrPanic(ctx)

	tlsRoute := &gatewayv1alpha2.TLSRoute{}
	tlsRoute.SetName(instance.Name)
	tlsRoute.SetNamespace(instance.Namespace)

	if err := targetCluster.Client().Get(ctx, client.ObjectKeyFromObject(tlsRoute), tlsRoute); err != nil {
		if errors.IsNotFound(err) {
			log.Debug("TLSRoute already deleted")
			return nil
		}
		return fmt.Errorf("failed to get TLSRoute: %w", err)
	}

	if err := targetCluster.Client().Delete(ctx, tlsRoute); client.IgnoreNotFound(err) != nil {
		return fmt.Errorf("failed to delete TLSRoute: %w", err)
	}

	log.Info("TLSRoute deleted")

	return nil
}

func getBaseDomain(gateway *gatewayv1.Gateway) (string, bool) {
	annotations := gateway.GetAnnotations()
	if len(annotations) == 0 {
		return "", false
	}

	baseDomain, hasBaseDomain := annotations[DNSAnnotationKey]
	return baseDomain, hasBaseDomain
}

func getHostName(baseDomain string, instance *Instance) string {
	subDomain := controller.NameHashSHAKE128Base32(instance.Name, instance.Namespace)
	return fmt.Sprintf("%s-%s.%s", instance.SubDomainPrefix, subDomain, baseDomain)
}
