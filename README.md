# conductor-workflow-demo
Run conductor workflows on a local Kubernetes cluster using Minikube

Run the entire application on Minikube

Prereqs
- Minikube v1.31+ with kubectl installed
- Docker CLI (uses Minikube’s Docker daemon for local images)

1) Start Minikube
- minikube start
- Optional: enable ingress if you want to use Ingress instead of NodePort (not required by these manifests)
  - minikube addons enable ingress

2) Build local images inside Minikube
Tell your shell to use Minikube’s Docker daemon so Kubernetes can pull the local images with imagePullPolicy: Never
- eval "$(minikube docker-env)"
- docker build -t go-api-service:local ./go-api-service
- docker build -t go-worker-service:local ./go-worker-service

3) Deploy the stack
- kubectl apply -f k8s/minikube/namespace.yaml
- kubectl apply -f k8s/minikube/postgres.yaml
- kubectl apply -f k8s/minikube/conductor.yaml
- kubectl apply -f k8s/minikube/api.yaml
- kubectl apply -f k8s/minikube/worker.yaml

Check pods
- kubectl get pods -n conductor-demo

4) Access services
A) Quick method using Minikube helper (may open a tunnel on Docker driver):
- Conductor UI/API: minikube service conductor -n conductor-demo
- Go API: minikube service go-api-service -n conductor-demo
Note: On macOS with Docker driver, the command may start a local tunnel and keep the terminal open. This is expected.

B) Recommended method without a persistent tunnel (NodePort via Minikube IP):
- Get Minikube IP and nodePorts, then construct URLs:
  - MINIKUBE_IP=$(minikube ip)
  - CONDUCTOR_HTTP_PORT=$(kubectl -n conductor-demo get svc conductor -o jsonpath='{.spec.ports[?(@.name=="http")].nodePort}')
  - API_PORT=$(kubectl -n conductor-demo get svc go-api-service -o jsonpath='{.spec.ports[0].nodePort}')
  - Conductor UI:  http://$MINIKUBE_IP:$CONDUCTOR_HTTP_PORT/
  - Conductor API: http://$MINIKUBE_IP:$CONDUCTOR_HTTP_PORT/api
  - Go API:        http://$MINIKUBE_IP:$API_PORT
This works on Docker and Hyperkit drivers without keeping a tunnel open.

C) Alternative: kubectl port-forward (good for local-only access):
- kubectl -n conductor-demo port-forward deploy/conductor 8080:8080
- kubectl -n conductor-demo port-forward deploy/go-api-service 8081:8081
Then use http://127.0.0.1:8080 (UI at /, API at /api) and http://127.0.0.1:8081

D) Optional: Ingress or LoadBalancer
- You can enable the ingress addon and create an Ingress for friendlier hostnames, or switch Service type to LoadBalancer and run `minikube tunnel` in a separate terminal.

5) Register Conductor metadata (tasks + workflow)
From your host, after the Conductor service is Ready, run (using Minikube IP + NodePort):
- MINIKUBE_IP=$(minikube ip)
- CONDUCTOR_HTTP_PORT=$(kubectl -n conductor-demo get svc conductor -o jsonpath='{.spec.ports[?(@.name=="http")].nodePort}')
- CONDUCTOR=http://$MINIKUBE_IP:$CONDUCTOR_HTTP_PORT
- curl -s -X POST "$CONDUCTOR/api/metadata/taskdefs" \
  -H 'Content-Type: application/json' \
  --data-binary @workflow/task_defs.json
- curl -s -X POST "$CONDUCTOR/api/metadata/workflow" \
    -H 'Content-Type: application/json' \
    --data-binary @workflow/onboard_entp_user_wf.json

6) Trigger the workflow via the Go API
- API_PORT=$(kubectl -n conductor-demo get svc go-api-service -o jsonpath='{.spec.ports[0].nodePort}')
- API_URL=http://$MINIKUBE_IP:$API_PORT
- curl -X POST "$API_URL/onboard" \
  -H 'Content-Type: application/json' \
  -d '{"entp_name":"AcmeCorp","user_name":"jdoe"}'

You should receive a JSON response containing the Conductor workflow_id. You can inspect progress in the Conductor UI.

Notes
- Postgres credentials are user/password with database conductor, matching docker-compose and app defaults.
- Services are internal DNS names: postgres, conductor in the conductor-demo namespace. The apps use CONDUCTOR_API_URL=http://conductor:8080/api.
- For local persistence you can swap emptyDir in postgres.yaml with a proper PersistentVolumeClaim if desired.
