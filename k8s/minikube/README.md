# Minikube Deployment

Prerequisites
- Minikube v1.31+ with `kubectl` installed
- Docker CLI (configured to use Minikube’s Docker daemon for local images)

## Steps

### 1) Start Minikube
Start Minikube and (optionally) enable the ingress addon.

```bash
  minikube start
  # (Optional) Enable ingress addon if needed:
  minikube addons enable ingress
```

### 2) Build local images inside Minikube
   Set your shell to use Minikube's Docker daemon, then build images.
```bash
   eval "$(minikube docker-env)"
    # Build Go API and Worker images
   docker build -t go-api-service:local ./go-api-service
   docker build -t go-worker-service:local ./go-worker-service
   docker build -t conductor-server:local .
```
### 3) Deploy Kubernetes manifests
```bash
    # Apply the namespace first:
    kubectl apply -f k8s/minikube/namespace.yaml
    
    # Apply PostgreSQL deployment and service:
    kubectl apply -f k8s/minikube/postgres.yaml
   
    # Apply Conductor deployment and service:
    kubectl apply -f k8s/minikube/conductor.yaml

    # Apply Go API deployment and service:
    kubectl apply -f k8s/minikube/api.yaml

    # Apply worker deployment:
    kubectl apply -f k8s/minikube/worker.yaml
    
    # Apply conductor UI deployment:
    kubectl apply -f k8s/minikube/conductor-ui.yaml
    
    # Verify pod status:
    kubectl get pods -n conductor-demo
```

### 4) Access services
   Use minikube service for quick service access and automatic tunneling:

```bash
    # Quick access (opens service in browser / tunnels)
    minikube service conductor -n conductor-demo
    minikube service go-api-service -n conductor-demo
```
Or obtain the Minikube IP and node ports to access services without tunnels:

```bash
    MINIKUBE_IP=$(minikube ip)
    CONDUCTOR_HTTP_PORT=$(kubectl -n conductor-demo get svc conductor -o jsonpath='{.spec.ports[?(@.name=="http")].nodePort}')
    API_PORT=$(kubectl -n conductor-demo get svc go-api-service -o jsonpath='{.spec.ports[0].nodePort}')
    echo "Conductor UI: http://$MINIKUBE_IP:$CONDUCTOR_HTTP_PORT/"
    echo "Conductor API: http://$MINIKUBE_IP:$CONDUCTOR_HTTP_PORT/api"
    echo "Go API: http://$MINIKUBE_IP:$API_PORT"
```

Or port-forward to localhost:

```bash
    kubectl -n conductor-demo port-forward deploy/conductor 8080:8080
    kubectl -n conductor-demo port-forward deploy/go-api-service 8081:8081
#    kubectl -n conductor-demo port-forward svc/conductor-ui 8080:80
```

### 5) Register Conductor Metadata (Tasks + Workflow)
   After the Conductor service is ready, register task and workflow definitions:

```bash
    MINIKUBE_IP=$(minikube ip)
    CONDUCTOR_HTTP_PORT=$(kubectl -n conductor-demo get svc conductor -o jsonpath='{.spec.ports[?(@.name=="http")].nodePort}')
    CONDUCTOR=http://$MINIKUBE_IP:$CONDUCTOR_HTTP_PORT

    curl -X POST "$CONDUCTOR/api/metadata/taskdefs" \
    -H 'Content-Type: application/json' \
    --data-binary @workflow/task_defs.json
    
    # or use this if you're doing port-forwarding: 
    curl -X POST http://127.0.0.1:8080/api/metadata/taskdefs -H 'Content-Type: application/json' --data-binary @workflow/task_defs.json

    curl -X POST "$CONDUCTOR/api/metadata/workflow" \
    -H 'Content-Type: application/json' \
    --data-binary @workflow/onboard_entp_user_wf.json
    
    # or use this if you're doing port-forwarding: 
    curl -X POST http://127.0.0.1:8080/api/metadata/workflow -H 'Content-Type: application/json' --data-binary @workflow/onboard_entp_user_wf.json
```

### 6) Trigger Onboarding Workflow via Go API
   Get API NodePort and call onboarding endpoint:

```bash
    API_PORT=$(kubectl -n conductor-demo get svc go-api-service -o jsonpath='{.spec.ports[0].nodePort}')
    API_URL=http://$MINIKUBE_IP:$API_PORT

    curl -X POST "$API_URL/onboard" \
    -H 'Content-Type: application/json' \
    -d '{"entp_name":"AcmeCorp","user_name":"jdoe"}'
    
    # or use this if you're doing port-forwarding: 
    curl -X POST http://127.0.0.1:8081/onboard -H 'Content-Type: application/json' -d '{"entp_name":"AcmeCorp","user_name":"jdoe"}'
```
Response will contain workflow instance ID.

