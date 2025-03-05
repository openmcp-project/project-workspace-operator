set -e

IMG="project-workspace-operator:dev"

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
kubectl delete deployment -n project-workspace-operator-system project-workspace-operator-controller-manager || true
make deploy IMG=$IMG
