set -e

IMG="platform-service-project-workspace:dev"

if [ ! -e "Makefile" ]; then
    echo "Please run this script from the project root."
    echo "$ ./hack/deploy-in-kind.sh"
    exit 1
fi

# kind delete cluster || true
# kind create cluster

make docker-build IMG=$IMG
kind load docker-image $IMG
make install
kubectl delete deployment -n platform-service-project-workspace-system platform-service-project-workspace-controller-manager || true
make deploy IMG=$IMG
