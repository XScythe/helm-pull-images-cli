package pull

import (
	"bytes"
	"context"
	"fmt"
	"path"
	"strings"

	"helm.sh/helm/v3/pkg/chartutil"
	"helm.sh/helm/v3/pkg/engine"
	"helm.sh/helm/v3/pkg/releaseutil"
	"helm.sh/helm/v3/pkg/strvals"
)

func (r Runner) renderChartManifest(ctx context.Context, opts Options) (string, error) {
	loaded, err := r.loadChart(ctx, opts)
	if err != nil {
		return "", err
	}
	chrt := loaded.Chart

	userValues, err := renderUserValues(opts)
	if err != nil {
		return "", err
	}

	if err := chartutil.ProcessDependenciesWithMerge(chrt, chartutil.Values(userValues)); err != nil {
		return "", err
	}

	caps := chartutil.DefaultCapabilities.Copy()
	// Hardcoded release name and namespace: these don't affect image extraction
	// (which is the CLI's sole purpose) as they only influence template rendering
	// metadata. Users cannot customize these values.
	renderValues, err := chartutil.ToRenderValuesWithSchemaValidation(
		chrt,
		userValues,
		chartutil.ReleaseOptions{
			Name:      "mirror",
			Namespace: "default",
			Revision:  1,
			IsInstall: true,
		},
		caps,
		false,
	)
	if err != nil {
		return "", err
	}

	renderedFiles, err := engine.Render(chrt, renderValues)
	if err != nil {
		return "", err
	}

	removeNotesTemplates(renderedFiles)

	hooks, manifests, err := releaseutil.SortManifests(renderedFiles, nil, releaseutil.InstallOrder)
	if err != nil {
		return renderDebugManifest(renderedFiles), fmt.Errorf("sort manifests: %w", err)
	}

	var out bytes.Buffer
	for _, crd := range chrt.CRDObjects() {
		fmt.Fprintf(&out, "---\n# Source: %s\n%s\n", crd.Filename, string(crd.File.Data))
	}
	for _, hook := range hooks {
		fmt.Fprintf(&out, "---\n# Source: %s\n%s\n", hook.Path, hook.Manifest)
	}
	for _, manifest := range manifests {
		fmt.Fprintf(&out, "---\n# Source: %s\n%s\n", manifest.Name, manifest.Content)
	}
	return out.String(), nil
}

func renderUserValues(opts Options) (map[string]interface{}, error) {
	merged := map[string]interface{}{}

	for _, valuesFile := range opts.ValuesFiles {
		fileValues, err := chartutil.ReadValuesFile(valuesFile)
		if err != nil {
			return nil, fmt.Errorf("read values file %q: %w", valuesFile, err)
		}
		merged = chartutil.MergeTables(fileValues, merged)
	}

	for _, setExpr := range opts.SetValues {
		if err := strvals.ParseInto(setExpr, merged); err != nil {
			return nil, fmt.Errorf("parse --set %q: %w", setExpr, err)
		}
	}

	return merged, nil
}

func removeNotesTemplates(renderedFiles map[string]string) {
	for name := range renderedFiles {
		if path.Base(name) == "NOTES.txt" {
			delete(renderedFiles, name)
		}
	}
}

func renderDebugManifest(files map[string]string) string {
	var b bytes.Buffer
	for name, content := range files {
		if strings.TrimSpace(content) == "" {
			continue
		}
		fmt.Fprintf(&b, "---\n# Source: %s\n%s\n", name, content)
	}
	return b.String()
}
