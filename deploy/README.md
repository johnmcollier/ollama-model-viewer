# OpenShift Deployment Resources

This directory contains all the necessary resources to deploy the ollama-model-viewer with OpenShift OAuth authentication.

## Prerequisites

- OpenShift cluster with admin access
- `oc` CLI tool configured and logged in

## Deployment Instructions

1. **Create the 'ollama' project if it doesn't exist:**
   ```bash
   oc new-project ollama
   ```

2. **Deploy all resources using Kustomize (Recommended):**
   ```bash
   oc apply -k deploy/
   ```

   Or using kubectl with kustomize:
   ```bash
   kubectl apply -k deploy/
   ```

3. **Alternative: Deploy using individual YAML files:**
   ```bash
   oc apply -f deploy/
   ```

   Or deploy individual resources in order:
   ```bash
   oc apply -f deploy/serviceaccount.yaml
   oc apply -f deploy/clusterrolebinding.yaml
   oc apply -f deploy/secret.yaml
   oc apply -f deploy/deployment.yaml
   oc apply -f deploy/service.yaml
   oc apply -f deploy/route.yaml
   ```

4. **Get the route URL:**
   ```bash
   oc get route ollama-model-viewer -n ollama
   ```

5. **Apply updates to existing deployment:**
   ```bash
   # Force restart pods with new configuration
   oc rollout restart deployment/ollama-model-viewer -n ollama
   
   # Check pod status
   oc get pods -n ollama -l app=ollama-model-viewer
   
   # Check OAuth proxy logs
   oc logs deployment/ollama-model-viewer -c oauth-proxy -n ollama
   ```

## Security Notes

- **IMPORTANT**: The cookie secret in `secret.yaml` is a default value and should be changed before production deployment
- Generate a new cookie secret with: `openssl rand -base64 32`
- Update the secret with: `oc create secret generic ollama-model-viewer-cookie --from-literal=cookie-secret=$(openssl rand -base64 32) --dry-run=client -o yaml | oc apply -f -`

## Troubleshooting

### OAuth Proxy Configuration

The OAuth proxy is now configured to use **HTTPS with OpenShift service serving certificates**:

**Architecture**:
- OAuth proxy runs with TLS on port 8443
- OpenShift automatically generates service serving certificates
- Route uses Reencrypt termination for end-to-end TLS security

**Key configuration**:
- `--https-address=:8443` - OAuth proxy listens on HTTPS
- `--tls-cert` and `--tls-key` point to auto-generated certificates
- Service annotation `service.beta.openshift.io/serving-cert-secret-name: proxy-tls` triggers certificate generation

**Previous HTTP-only issues**:
If you see TLS loading errors, it's because the OAuth proxy always expects TLS certificates. The solution is to provide proper certificates rather than trying to disable TLS.

### Common Issues

1. **"Application is not available" error**: 
   - **Root cause**: Route pointing to wrong service or port mismatch
   - **Solution**: Ensure route points to service with certificate annotation
   - **Check**: `oc get route ollama-model-viewer -o yaml` and verify service name

2. **Pod not starting**: Check service account permissions and namespace
   ```bash
   oc get pods -n ollama -l app=ollama-model-viewer
   oc logs deployment/ollama-model-viewer -c oauth-proxy -n ollama
   ```

3. **OAuth "server_error" on login screen**: 
   - **Root cause**: Service account missing OAuth client annotations
   - **Solution**: Service account needs `oauth-redirectreference` annotation pointing to the route
   - **Check**: `oc get serviceaccount ollama-model-viewer-sa -o yaml -n ollama`

4. **RBAC "cannot list pods" error**:
   - **Root cause**: Service account missing permissions to interact with pods
   - **Solution**: Role and RoleBinding grant list, get, and exec permissions for pods
   - **Check**: `oc get role,rolebinding -n ollama | grep ollama-model-viewer`

5. **OAuth authentication failing**: Verify the service account has `system:auth-delegator` role
   ```bash
   oc get clusterrolebinding ollama-model-viewer-auth-delegator
   ```

6. **Certificate issues**: Verify service serving certificate is generated
   ```bash
   oc get secret proxy-tls -n ollama
   oc describe service ollama-model-viewer -n ollama
   ```

## Image Registry

The deployment uses the pre-built image from `quay.io/jcollier/ollama-model-viewer:latest`. If you need to build and push your own image:

```bash
# Build the image
docker build -t quay.io/jcollier/ollama-model-viewer:latest .

# Push to registry
docker push quay.io/jcollier/ollama-model-viewer:latest
```

## Kustomize Benefits

Using Kustomize provides several advantages:

- **Declarative configuration management**: Manage all resources as a single unit
- **Easy customization**: Override settings without modifying base files
- **Consistent labeling**: Automatically applies common labels to all resources
- **Image management**: Centralized image tag management
- **Environment-specific overlays**: Support for dev/staging/prod configurations

### Customizing the Deployment

To customize the deployment for different environments:

1. **Change the image tag:**
   ```bash
   cd deploy/
   kustomize edit set image quay.io/jcollier/ollama-model-viewer:v1.2.3
   ```

2. **Add additional labels:**
   ```bash
   kustomize edit add label environment:production
   ```

3. **Create overlays for different environments:**
   ```
   overlays/
     production/
       kustomization.yaml
       patches/
   ```

## Resources Overview

- **serviceaccount.yaml**: Service account with OAuth client annotations
- **clusterrolebinding.yaml**: Grants auth-delegator permissions for OAuth proxy
- **role.yaml**: Grants permissions to list, get, and exec into pods in ollama namespace
- **rolebinding.yaml**: Binds the role to the service account
- **secret.yaml**: Contains cookie secret for OAuth proxy
- **deployment.yaml**: Main application deployment with OAuth proxy sidecar
- **service-ca.yaml**: Service with annotation to auto-generate TLS certificates (port 443→8443)
- **route.yaml**: OpenShift route with Reencrypt TLS termination pointing to the service
- **kustomization.yaml**: Kustomize configuration for managing all resources 