#!/usr/bin/env bash
set -euo pipefail

cd "$(dirname "$0")"

kubectl apply -f namespace.yaml
kubectl apply -f postgres.yaml
kubectl apply -f conductor.yaml
kubectl apply -f api.yaml
kubectl apply -f worker.yaml

echo "Waiting for pods to become Ready..."
kubectl -n conductor-demo wait --for=condition=available --timeout=120s deployment/conductor || true
kubectl -n conductor-demo wait --for=condition=available --timeout=120s deployment/go-api-service || true
kubectl -n conductor-demo wait --for=condition=available --timeout=120s deployment/postgres || true
kubectl -n conductor-demo wait --for=condition=available --timeout=120s deployment/go-worker-service || true

kubectl get pods -n conductor-demo

MINIKUBE_IP=$(minikube ip)
CONDUCTOR_HTTP_PORT=$(kubectl -n conductor-demo get svc conductor -o jsonpath='{.spec.ports[?(@.name=="http")].nodePort}')
API_PORT=$(kubectl -n conductor-demo get svc go-api-service -o jsonpath='{.spec.ports[0].nodePort}')

echo
echo "Services without tunnel:"
echo "  Conductor UI:  http://$MINIKUBE_IP:$CONDUCTOR_HTTP_PORT/"
echo "  Conductor API: http://$MINIKUBE_IP:$CONDUCTOR_HTTP_PORT/api"
echo "  Go API:        http://$MINIKUBE_IP:$API_PORT"
echo
echo "Alternatively, you can run:"
echo "  minikube service conductor -n conductor-demo"
echo "  minikube service go-api-service -n conductor-demo"
echo "(On Docker driver this may start a local tunnel and keep the terminal open.)"
