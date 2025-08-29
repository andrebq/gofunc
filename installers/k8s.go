package installers

import (
	"context"
	"fmt"
	"io"
	"os"
	"text/template"
)

func K8S(ctx context.Context, output io.Writer, yamlTemplateFile string, name string, namespace string, image string) error {
	in, err := openTemplate(yamlTemplateFile)
	if err != nil {
		return fmt.Errorf("failed to open template file %q: %w", yamlTemplateFile, err)
	}
	defer in.Close()
	txt, err := io.ReadAll(in)
	if err != nil {
		return fmt.Errorf("failed to read template file %q: %w", yamlTemplateFile, err)
	}

	tmpl, err := template.New("k8s").Parse(string(txt))
	if err != nil {
		return fmt.Errorf("failed to parse template file %q: %w", yamlTemplateFile, err)
	}

	return tmpl.Execute(output, struct {
		Name         string
		Image        string
		Namespace    string
		ControlPort  string
		ControlAddr  string
		AdminSubpath string
	}{
		Name:         name,
		Image:        image,
		Namespace:    namespace,
		ControlPort:  "9000",
		ControlAddr:  "0.0.0.0",
		AdminSubpath: "/_admin/",
	})
}

func openTemplate(yamlTemplateFile string) (io.ReadCloser, error) {
	if yamlTemplateFile == "-" {
		return io.NopCloser(os.Stdin), nil
	}
	fd, err := os.Open(yamlTemplateFile)
	if err != nil {
		return nil, fmt.Errorf("failed to open template file %q: %w", yamlTemplateFile, err)
	}
	return fd, nil
}
