package main

import (
	"bytes"
	"context"
	"fmt"
	"html/template"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/kubernetes/scheme"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/clientcmd"
	"k8s.io/client-go/tools/remotecommand"
)

const (
	namespace    = "ollama"
	port         = "8080"
	totalVRAMGiB = 24.0
	editURL      = "https://github.com/redhat-ai-dev/rosa-gitops/edit/main/ollama/ollama-models-config.yaml"
)

var (
	clientset *kubernetes.Clientset
	config    *rest.Config
	htmlTpl   = `<!DOCTYPE html>
<html lang="en">
<head>
    <meta charset="UTF-8">
    <meta name="viewport" content="width=device-width, initial-scale=1.0">
    <title>Ollama Model Viewer</title>
    <link href="https://cdn.jsdelivr.net/npm/bootstrap@5.3.0/dist/css/bootstrap.min.css" rel="stylesheet">
</head>
<body>
    <nav class="navbar navbar-expand-lg navbar-dark bg-dark">
        <div class="container-fluid">
            <a class="navbar-brand" href="#">Ollama Model Viewer</a>
        </div>
    </nav>
    <div class="container mt-4">
        {{ if .ErrorMessage }}
        <div class="alert alert-danger" role="alert">
            <strong>Error:</strong> {{ .ErrorMessage }}
        </div>
        {{ end }}

        <div class="card mb-4">
            <div class="card-header">
                vRAM Usage on <strong>g5.2xlarge</strong> node
            </div>
            <div class="card-body">
                <p class="mb-1">Total vRAM: <strong>{{ .TotalVRAM }} GiB</strong></p>
                <div class="progress" style="height: 25px;">
                    <div class="progress-bar" role="progressbar" style="width: {{ .UsedVRAMPercentage }}%;" aria-valuenow="{{ .UsedVRAM }}" aria-valuemin="0" aria-valuemax="{{ .TotalVRAM }}">
                        {{ .UsedVRAM }} GiB Used
                    </div>
                </div>
                <p class="mt-1">Remaining vRAM: <strong>{{ .RemainingVRAM }} GiB</strong></p>
            </div>
        </div>

        <div class="card">
            <div class="card-header d-flex justify-content-between align-items-center">
                <div>Loaded Models in <strong>{{ .Namespace }}</strong> namespace (from pod: <strong>{{ .PodName }}</strong>)</div>
                <a href="{{ .EditURL }}" class="btn btn-secondary btn-sm" target="_blank">Edit in GitHub</a>
            </div>
            <div class="card-body">
                <table class="table table-striped table-hover table-sm">
                    <thead>
                        <tr>
                            <th scope="col">NAME</th>
                            <th scope="col">ID</th>
                            <th scope="col">SIZE</th>
                            <th scope="col">PROCESSOR</th>
                            <th scope="col">UNTIL</th>
                        </tr>
                    </thead>
                    <tbody>
                        {{ if .Models }}
                            {{ range .Models }}
                            <tr>
                                <td>{{ .Name }}</td>
                                <td>{{ .ID }}</td>
                                <td>{{ .Size }}</td>
                                <td>{{ .Processor }}</td>
                                <td>{{ .Until }}</td>
                            </tr>
                            {{ end }}
                        {{ else }}
                            <tr>
                                <td colspan="5" class="text-center">No models loaded or found.</td>
                            </tr>
                        {{ end }}
                    </tbody>
                </table>
            </div>
        </div>
    </div>
</body>
</html>`
)

// OllamaPsModel holds the parsed data for a single model from 'ollama ps'
type OllamaPsModel struct {
	Name      string
	ID        string
	Size      string
	Processor string
	Until     string
}

// PageData is the data structure passed to the HTML template
type PageData struct {
	Models             []OllamaPsModel
	TotalVRAM          string
	UsedVRAM           string
	RemainingVRAM      string
	UsedVRAMPercentage float64
	EditURL            string
	Namespace          string
	PodName            string
	ErrorMessage       string
}

func main() {
	var err error
	config, err = getKubeConfig()
	if err != nil {
		log.Fatalf("Failed to get Kubernetes config: %v", err)
	}

	clientset, err = kubernetes.NewForConfig(config)
	if err != nil {
		log.Fatalf("Failed to create Kubernetes clientset: %v", err)
	}

	http.HandleFunc("/", viewHandler)

	log.Printf("Server starting on port %s", port)
	if err := http.ListenAndServe(":"+port, nil); err != nil {
		log.Fatalf("Failed to start server: %v", err)
	}
}

func viewHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	data := PageData{
		TotalVRAM: fmt.Sprintf("%.1f", totalVRAMGiB),
		EditURL:   editURL,
		Namespace: namespace,
	}

	pods, err := clientset.CoreV1().Pods(namespace).List(context.Background(), metav1.ListOptions{LabelSelector: "app=ollama-serve"})
	if err != nil {
		data.ErrorMessage = fmt.Sprintf("Failed to list pods: %v", err)
		renderTemplate(w, data)
		return
	}
	if len(pods.Items) == 0 {
		data.ErrorMessage = "No ollama pod found with label 'app=ollama-serve'."
		renderTemplate(w, data)
		return
	}
	ollaPod := pods.Items[0]
	data.PodName = ollaPod.Name

	psOutput, err := execInPod(ollaPod.Name, namespace, []string{"ollama", "ps"})
	if err != nil {
		data.ErrorMessage = fmt.Sprintf("Failed to execute 'ollama ps': %v", err)
		renderTemplate(w, data)
		return
	}

	models, usedGiB := parseOllamaPs(psOutput)
	data.Models = models
	remainingGiB := totalVRAMGiB - usedGiB
	data.UsedVRAM = fmt.Sprintf("%.2f", usedGiB)
	data.RemainingVRAM = fmt.Sprintf("%.2f", remainingGiB)
	if totalVRAMGiB > 0 {
		data.UsedVRAMPercentage = (usedGiB / totalVRAMGiB) * 100
	}

	renderTemplate(w, data)
}

func renderTemplate(w http.ResponseWriter, data PageData) {
	tmpl, err := template.New("index").Parse(htmlTpl)
	if err != nil {
		http.Error(w, "Failed to parse template", http.StatusInternalServerError)
		return
	}
	tmpl.Execute(w, data)
}

func execInPod(podName, namespace string, command []string) (string, error) {
	req := clientset.CoreV1().RESTClient().Post().
		Resource("pods").
		Name(podName).
		Namespace(namespace).
		SubResource("exec").
		VersionedParams(&v1.PodExecOptions{
			Command: command,
			Stdin:   false,
			Stdout:  true,
			Stderr:  true,
			TTY:     false,
		}, scheme.ParameterCodec)

	exec, err := remotecommand.NewSPDYExecutor(config, "POST", req.URL())
	if err != nil {
		return "", fmt.Errorf("failed to create executor: %w", err)
	}

	var stdout, stderr bytes.Buffer
	err = exec.Stream(remotecommand.StreamOptions{
		Stdout: &stdout,
		Stderr: &stderr,
	})

	if err != nil {
		return "", fmt.Errorf("failed to stream command execution: %w (stderr: %s)", err, stderr.String())
	}

	return stdout.String(), nil
}

func parseOllamaPs(output string) ([]OllamaPsModel, float64) {
	output = strings.ReplaceAll(output, "\r\n", "\n") // Sanitize line endings
	lines := strings.Split(strings.TrimSpace(output), "\n")
	if len(lines) <= 1 {
		return nil, 0.0
	}

	header := strings.ToUpper(lines[0])
	nameIndex := strings.Index(header, "NAME")
	idIndex := strings.Index(header, "ID")
	sizeIndex := strings.Index(header, "SIZE")
	processorIndex := strings.Index(header, "PROCESSOR")
	untilIndex := strings.Index(header, "UNTIL")

	if nameIndex == -1 || idIndex == -1 || sizeIndex == -1 || processorIndex == -1 || untilIndex == -1 {
		log.Printf("Could not parse 'ollama ps' header. Header was: %s", header)
		return nil, 0.0
	}

	var models []OllamaPsModel
	var totalSizeGiB float64

	for _, line := range lines[1:] {
		if len(line) < untilIndex {
			continue
		}
		name := strings.TrimSpace(line[nameIndex:idIndex])
		id := strings.TrimSpace(line[idIndex:sizeIndex])
		sizeStr := strings.TrimSpace(line[sizeIndex:processorIndex])
		processor := strings.TrimSpace(line[processorIndex:untilIndex])
		until := strings.TrimSpace(line[untilIndex:])

		models = append(models, OllamaPsModel{
			Name:      name,
			ID:        id,
			Size:      sizeStr,
			Processor: processor,
			Until:     until,
		})

		totalSizeGiB += parseSizeToGiB(sizeStr)
	}

	return models, totalSizeGiB
}

func parseSizeToGiB(sizeStr string) float64 {
	sizeStr = strings.ToUpper(strings.TrimSpace(sizeStr))

	var valueStr string
	var multiplier float64

	if strings.HasSuffix(sizeStr, "GIB") {
		valueStr = strings.TrimSuffix(sizeStr, "GIB")
		multiplier = 1.0
	} else if strings.HasSuffix(sizeStr, "GB") {
		valueStr = strings.TrimSuffix(sizeStr, "GB")
		multiplier = 0.931323 // GB to GiB
	} else if strings.HasSuffix(sizeStr, "MIB") {
		valueStr = strings.TrimSuffix(sizeStr, "MIB")
		multiplier = 1.0 / 1024.0 // MiB to GiB
	} else if strings.HasSuffix(sizeStr, "MB") {
		valueStr = strings.TrimSuffix(sizeStr, "MB")
		multiplier = (1.0 / 1024.0) * 0.931323 // MB to GB to GiB
	} else {
		log.Printf("Unknown size unit in '%s'", sizeStr)
		return 0.0
	}

	val, err := strconv.ParseFloat(strings.TrimSpace(valueStr), 64)
	if err != nil {
		log.Printf("Could not parse size value from '%s' (extracted value: '%s'): %v", sizeStr, valueStr, err)
		return 0.0
	}

	return val * multiplier
}

func getKubeConfig() (*rest.Config, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return nil, fmt.Errorf("failed to get user home directory: %w", err)
	}
	kubeconfigPath := filepath.Join(home, ".kube", "config")

	if _, err := os.Stat(kubeconfigPath); err == nil {
		cfg, err := clientcmd.BuildConfigFromFlags("", kubeconfigPath)
		if err == nil {
			log.Println("Using local kubeconfig.")
			return cfg, nil
		}
		log.Printf("Warning: could not use local kubeconfig: %v", err)
	}

	log.Println("Local kubeconfig not found or unusable, trying in-cluster config.")
	cfg, err := rest.InClusterConfig()
	if err != nil {
		return nil, fmt.Errorf("in-cluster config not available: %w. And failed to use local kubeconfig", err)
	}
	log.Println("Using in-cluster config.")
	return cfg, nil
}
