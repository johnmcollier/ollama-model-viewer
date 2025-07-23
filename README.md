# Ollama Model Viewer

This web service provides a simple frontend to view loaded Ollama models and vRAM usage in the `ollama` namespace.

## Features

- View currently loaded Ollama models
- Monitor vRAM usage on g5.2xlarge nodes
- Web-based interface with Bootstrap styling
- OpenShift OAuth integration for secure access

## How to use

### Prerequisites

- A running Kubernetes/OpenShift cluster
- `kubectl` or `oc` configured to connect to your cluster
- The `ollama` namespace must exist in your cluster
- An Ollama pod running with label `app=ollama-serve`

### Running locally

1.  **Install Go dependencies:**
    ```sh
    go mod tidy
    ```

2.  **Run the service:**
    ```sh
    go run main.go
    ```
    The service will be available at `http://localhost:8080`. It will use your local kubeconfig for authentication.

### Building and deploying with Docker

1.  **Build the Docker image:**
    ```sh
    docker build -t quay.io/jcollier/ollama-model-viewer:latest .
    ```

2.  **Run the Docker container:**
    ```sh
    docker run -p 8080:8080 quay.io/jcollier/ollama-model-viewer:latest
    ```

### Deploying to OpenShift

To deploy this service to your OpenShift cluster with OAuth authentication:

1.  **Deploy using the provided resources:**
    ```sh
    oc apply -f deploy/
    ```

2.  **Get the external route:**
    ```sh
    oc get route ollama-model-viewer -n ollama
    ```

See the [deploy/README.md](deploy/README.md) for detailed deployment instructions and security considerations.

## Architecture

The application consists of:
- **Main container**: Go application serving the web interface
- **OAuth proxy sidecar**: Handles OpenShift OAuth authentication
- **Service account**: With necessary RBAC permissions to read pods and execute commands

## Security

- Uses OpenShift OAuth for authentication
- Requires proper RBAC permissions to access the Kubernetes API
- Cookie-based session management for the OAuth proxy
