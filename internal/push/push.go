// Package push is the image transfer engine shared by both CLI phases. During
// pull it stages remote images into a local OCI layout (ArchiveImages) and
// copies the helper binary; during push it uploads that layout to a target
// registry (PushImages). The on-disk manifest contract lives in pushspec.
package push

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"helm-deep-pack/internal/pushspec"
	"helm-deep-pack/internal/validation"
	"io"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/google/go-containerregistry/pkg/authn"
	"github.com/google/go-containerregistry/pkg/name"
	v1 "github.com/google/go-containerregistry/pkg/v1"
	"github.com/google/go-containerregistry/pkg/v1/layout"
	"github.com/google/go-containerregistry/pkg/v1/remote"
	"golang.org/x/sync/errgroup"
)

var copyImageToRegistry = copyImageToRegistryUsingGoContainerRegistry
var writeRemoteImage = remote.Write
var loadLayoutImage = func(layoutPath layout.Path, hash v1.Hash) (v1.Image, error) {
	return layoutPath.Image(hash)
}
var loadOCILayout = layout.FromPath
var resolveExecutablePath = os.Executable

type Options struct {
	Registry          string
	InputDir          string
	Concurrency       int
	All               bool
	AllowInsecureHTTP bool
	In                io.Reader
	Out               io.Writer
}

func isTerminalReader(r io.Reader) bool {
	f, ok := r.(*os.File)
	if !ok {
		return false
	}
	info, err := f.Stat()
	if err != nil {
		return false
	}
	return info.Mode()&os.ModeCharDevice != 0
}

func PushImages(ctx context.Context, opts Options, status ...io.Writer) error {
	return pushImages(ctx, opts, newRegistryProbeClient(), status...)
}

func pushImages(ctx context.Context, opts Options, probeClient *http.Client, status ...io.Writer) error {
	if opts.Registry == "" {
		resolved, err := promptForRegistry(opts.In, opts.Out)
		if err != nil {
			return err
		}
		opts.Registry = resolved
	}
	if err := validation.ValidateImageRegistryWithPath("registry argument", opts.Registry); err != nil {
		return fmt.Errorf("validate registry argument: %w", err)
	}
	if err := validation.ValidateConcurrency("--concurrency", opts.Concurrency); err != nil {
		return fmt.Errorf("validate concurrency: %w", err)
	}
	registryHost, _ := validation.SplitRegistryPath(opts.Registry)
	if err := preflightRegistry(ctx, registryHost, opts.AllowInsecureHTTP, probeClient); err != nil {
		return fmt.Errorf("preflight registry argument: %w", err)
	}
	destRegistry := strings.TrimRight(opts.Registry, "/")

	resolvedInputDir, err := resolvePushInputDir(opts.InputDir)
	if err != nil {
		return fmt.Errorf("resolve push input dir: %w", err)
	}

	manifest, err := pushspec.ReadPushManifest(resolvedInputDir)
	if err != nil {
		return fmt.Errorf("read push manifest: %w", err)
	}

	layoutDirPath, err := resolveLayoutDirPath(resolvedInputDir, manifest.LayoutDir)
	if err != nil {
		return fmt.Errorf("resolve oci image layout path: %w", err)
	}
	layoutPath, err := loadOCILayout(layoutDirPath)
	if err != nil {
		return fmt.Errorf("load oci image layout: %w", err)
	}

	var selected []pushspec.ArchiveSpec
	if opts.All {
		selected = manifest.Images
	} else {
		chosen, proceed, selectErr := selectImagesToPush(ctx, opts, destRegistry, manifest.Images)
		if selectErr != nil {
			return selectErr
		}
		if !proceed {
			return nil
		}
		selected = chosen
	}

	return pushSpecs(ctx, destRegistry, layoutPath, selected, opts.Concurrency, opts.AllowInsecureHTTP, status...)
}

// selectImagesToPush runs the interactive selection workflow over specs: it
// classifies each image against the destination registry, surfaces probe
// warnings, prompts the user to choose images, and confirms any conflicting
// overwrites. The returned proceed flag is false when the caller should stop
// without pushing (terminal unavailable aside, this covers user cancellation,
// an empty selection, or a declined conflict confirmation).
func selectImagesToPush(ctx context.Context, opts Options, destRegistry string, specs []pushspec.ArchiveSpec) (selected []pushspec.ArchiveSpec, proceed bool, err error) {
	if opts.In == nil || !isTerminalReader(opts.In) || opts.Out == nil || !isTerminalWriter(opts.Out) {
		return nil, false, fmt.Errorf("interactive selection requires terminal input and output; re-run with --all to push every image non-interactively")
	}

	classified := classifyImages(ctx, destRegistry, opts.AllowInsecureHTTP, specs)

	for _, item := range classified {
		if item.Status == statusUnknown {
			if _, err := fmt.Fprintf(opts.Out, "warning: could not check %s in registry: %v\n", item.Spec.Image, item.ProbeErr); err != nil {
				return nil, false, fmt.Errorf("write warning output: %w", err)
			}
		}
	}

	selected, cancelled, err := runSelect(opts.In, opts.Out, classified, destRegistry)
	if err != nil {
		return nil, false, fmt.Errorf("select images: %w", err)
	}
	if cancelled {
		return nil, false, nil
	}

	if len(selected) == 0 {
		if _, err := fmt.Fprintln(opts.Out, "nothing selected; no images pushed"); err != nil {
			return nil, false, fmt.Errorf("write empty-selection output: %w", err)
		}
		return nil, false, nil
	}

	conflicts := selectedConflicts(selected, classified)
	if len(conflicts) > 0 {
		confirmed, confirmErr := confirmConflictSelection(opts.In, opts.Out, conflicts)
		if confirmErr != nil {
			return nil, false, fmt.Errorf("confirm conflict selection: %w", confirmErr)
		}
		if !confirmed {
			if _, err := fmt.Fprintln(opts.Out, "push cancelled; no images pushed"); err != nil {
				return nil, false, fmt.Errorf("write cancellation output: %w", err)
			}
			return nil, false, nil
		}
	}

	return selected, true, nil
}

func newRegistryProbeClient() *http.Client {
	return &http.Client{Timeout: 5 * time.Second}
}

func preflightRegistry(ctx context.Context, registry string, allowInsecureHTTP bool, probeClient *http.Client) (err error) {
	registry = strings.TrimRight(registry, "/")
	scheme := "https"
	if allowInsecureHTTP {
		scheme = "http"
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, scheme+"://"+registry+"/v2/", nil)
	if err != nil {
		return fmt.Errorf("build registry probe request: %w", err)
	}
	if probeClient == nil {
		probeClient = newRegistryProbeClient()
	}
	resp, err := probeClient.Do(req)
	if err != nil {
		if !allowInsecureHTTP && isHTTPServerOnHTTPSProbe(err) {
			return fmt.Errorf(
				"registry %q appears to serve plain HTTP, but push defaults to HTTPS; re-run with --allow-insecure-http for HTTP registries",
				registry,
			)
		}
		return fmt.Errorf("registry %q is not reachable: %w", registry, err)
	}
	defer func() {
		if closeErr := resp.Body.Close(); closeErr != nil && err == nil {
			err = fmt.Errorf("close registry probe response: %w", closeErr)
		}
	}()

	body, err := io.ReadAll(io.LimitReader(resp.Body, 4096))
	if err != nil {
		return fmt.Errorf("read registry probe response: %w", err)
	}
	if looksLikeWebsiteContent(resp.Header.Get("Content-Type"), body) {
		return fmt.Errorf("registry %q is reachable but looks like a website, not an image registry", registry)
	}

	switch resp.StatusCode {
	case http.StatusOK, http.StatusUnauthorized, http.StatusForbidden:
		return nil
	default:
		return fmt.Errorf("registry %q is reachable but did not expose the container registry API at /v2/ (status %s)", registry, resp.Status)
	}
}

func looksLikeWebsiteContent(contentType string, body []byte) bool {
	if strings.Contains(strings.ToLower(contentType), "text/html") {
		return true
	}

	lowerBody := strings.ToLower(string(body))
	return strings.Contains(lowerBody, "<!doctype html") || strings.Contains(lowerBody, "<html")
}

func pushSpecs(ctx context.Context, registry string, layoutPath layout.Path, specs []pushspec.ArchiveSpec, concurrency int, allowInsecureHTTP bool, status ...io.Writer) error {
	progress := newTransferProgress(statusWriter(status...), "pushing", len(specs))
	defer progress.Finish()

	group, groupCtx := errgroup.WithContext(ctx)
	group.SetLimit(normalizeConcurrency(concurrency))
	for _, spec := range specs {
		spec := spec
		group.Go(func() error {
			progress.Begin(spec.Image)
			defer progress.End(spec.Image)

			if err := copyImageToRegistry(groupCtx, registry, allowInsecureHTTP, layoutPath, spec.Image, spec.Target, spec.OCIDigest); err != nil {
				return fmt.Errorf("push %s: %w", spec.Image, err)
			}
			return nil
		})
	}
	if err := group.Wait(); err != nil {
		return fmt.Errorf("push images: %w", err)
	}

	return nil
}

func resolvePushInputDir(inputDir string) (string, error) {
	if inputDir != "" {
		return inputDir, nil
	}

	executable, err := resolveExecutablePath()
	if err != nil {
		return "", fmt.Errorf("resolve executable path: %w", err)
	}
	executableDir := filepath.Dir(executable)
	foundInExecutableDir, err := pushManifestExists(executableDir)
	if err != nil {
		return "", fmt.Errorf("check push manifest in executable dir: %w", err)
	}
	if foundInExecutableDir {
		return executableDir, nil
	}

	cwd, err := os.Getwd()
	if err != nil {
		return "", fmt.Errorf("resolve current working directory: %w", err)
	}
	foundInWorkingDir, err := pushManifestExists(cwd)
	if err != nil {
		return "", fmt.Errorf("check push manifest in current working directory: %w", err)
	}
	if foundInWorkingDir {
		return cwd, nil
	}

	return "", fmt.Errorf(
		"push manifest %q not found in executable dir %q or current working directory %q (pass --input-dir)",
		pushspec.PushManifestFileName(),
		executableDir,
		cwd,
	)
}

func resolveLayoutDirPath(inputDir, layoutDir string) (string, error) {
	if strings.TrimSpace(layoutDir) == "" {
		layoutDir = pushspec.OCILayoutDirName()
	}
	cleanLayoutDir := filepath.Clean(layoutDir)
	if filepath.IsAbs(cleanLayoutDir) {
		return "", fmt.Errorf("push manifest layoutDir must be relative: %q", layoutDir)
	}
	if cleanLayoutDir == ".." || strings.HasPrefix(cleanLayoutDir, ".."+string(filepath.Separator)) {
		return "", fmt.Errorf("push manifest layoutDir must stay within input dir: %q", layoutDir)
	}

	cleanInputDir := filepath.Clean(inputDir)
	layoutDirPath := filepath.Join(cleanInputDir, cleanLayoutDir)
	rel, err := filepath.Rel(cleanInputDir, layoutDirPath)
	if err != nil {
		return "", fmt.Errorf("resolve layoutDir relative path: %w", err)
	}
	if rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
		return "", fmt.Errorf("push manifest layoutDir must stay within input dir: %q", layoutDir)
	}
	return layoutDirPath, nil
}

func pushManifestExists(dir string) (bool, error) {
	_, err := os.Stat(filepath.Join(dir, pushspec.PushManifestFileName()))
	if err == nil {
		return true, nil
	}
	if errors.Is(err, os.ErrNotExist) {
		return false, nil
	}
	return false, err
}

func selectedConflicts(selected []pushspec.ArchiveSpec, classified []classifiedImage) []classifiedImage {
	selectedKeys := make(map[string]struct{}, len(selected))
	for _, spec := range selected {
		selectedKeys[selectionKey(spec)] = struct{}{}
	}

	conflicts := make([]classifiedImage, 0)
	for _, item := range classified {
		if item.Status != statusConflict {
			continue
		}
		if _, ok := selectedKeys[selectionKey(item.Spec)]; ok {
			conflicts = append(conflicts, item)
		}
	}
	return conflicts
}

func selectionKey(spec pushspec.ArchiveSpec) string {
	return spec.Image + "|" + spec.Target + "|" + spec.OCIDigest
}

func confirmConflictSelection(in io.Reader, out io.Writer, conflicts []classifiedImage) (bool, error) {
	conflictCount := len(conflicts)
	if in == nil {
		return false, fmt.Errorf("missing input stream")
	}
	if out == nil {
		out = io.Discard
	}

	orange := ""
	reset := ""
	if isTerminalWriter(out) {
		orange = "\x1b[38;5;208m"
		reset = "\x1b[0m"
	}

	if _, err := fmt.Fprintf(out, "\r\n%sWARNING:%s %d selected image(s) are marked [conflict] because the destination already exists with a different digest.\r\n", orange, reset, conflictCount); err != nil {
		return false, err
	}
	if _, err := fmt.Fprintf(out, "%sPotential risks:%s\r\n", orange, reset); err != nil {
		return false, err
	}
	if _, err := fmt.Fprintf(out, "- Existing tags/references in the target registry will be overwritten.\r\n"); err != nil {
		return false, err
	}
	for _, conflict := range conflicts {
		if _, err := fmt.Fprintf(out, "  * %s: current=%s staged=%s\r\n", conflict.Spec.Target, conflict.RemoteDigest, conflict.Spec.OCIDigest); err != nil {
			return false, err
		}
	}
	if _, err := fmt.Fprintf(out, "- Running workloads pulling those tags may change behavior unexpectedly.\r\n"); err != nil {
		return false, err
	}
	if _, err := fmt.Fprintf(out, "- Rollback/debugging can become harder if consumers rely on mutable tags.\r\n"); err != nil {
		return false, err
	}

	reader := bufio.NewReader(in)
	for {
		if _, err := fmt.Fprintf(out, "%sContinue and overwrite those references with the staged digest?%s [yes/no]: ", orange, reset); err != nil {
			return false, err
		}
		response, err := reader.ReadString('\n')
		if err != nil && !errors.Is(err, io.EOF) {
			return false, err
		}

		answer := strings.ToLower(strings.TrimSpace(response))
		switch answer {
		case "yes", "y":
			return true, nil
		case "no", "n", "":
			return false, nil
		}

		if _, writeErr := fmt.Fprint(out, "Please type yes or no.\r\n"); writeErr != nil {
			return false, writeErr
		}
		if errors.Is(err, io.EOF) {
			return false, nil
		}
	}
}

func copyImageToRegistryUsingGoContainerRegistry(ctx context.Context, registry string, allowInsecureHTTP bool, layoutPath layout.Path, sourceImage, target, ociDigest string) error {
	if _, err := name.ParseReference(sourceImage); err != nil {
		return fmt.Errorf("parse source image %q: %w", sourceImage, err)
	}

	hash, err := v1.NewHash(ociDigest)
	if err != nil {
		return fmt.Errorf("parse oci digest %q: %w", ociDigest, err)
	}

	registry = strings.TrimRight(registry, "/")

	img, err := loadLayoutImage(layoutPath, hash)
	if err != nil {
		return fmt.Errorf("load image %q from oci layout: %w", sourceImage, err)
	}

	destRefOpts := make([]name.Option, 0, 1)
	if allowInsecureHTTP {
		destRefOpts = append(destRefOpts, name.Insecure)
	}
	destRef, err := name.ParseReference(fmt.Sprintf("%s/%s", registry, target), destRefOpts...)
	if err != nil {
		return fmt.Errorf("parse destination reference %q: %w", registry+"/"+target, err)
	}

	if err := writeRemoteImage(destRef, img, remote.WithContext(ctx), remote.WithAuthFromKeychain(authn.DefaultKeychain)); err != nil {
		if looksLikeWebsite(err) {
			return fmt.Errorf("registry %q does not look like a container registry", registry)
		}
		return fmt.Errorf("push image %q to registry %q: %w", sourceImage, registry, err)
	}

	return nil
}

func isHTTPServerOnHTTPSProbe(err error) bool {
	var urlErr *url.Error
	if errors.As(err, &urlErr) {
		return strings.Contains(strings.ToLower(urlErr.Err.Error()), "server gave http response to https client")
	}
	return strings.Contains(strings.ToLower(err.Error()), "server gave http response to https client")
}

func looksLikeWebsite(err error) bool {
	msg := strings.ToLower(err.Error())
	switch {
	case strings.Contains(msg, "text/html"):
		return true
	case strings.Contains(msg, "invalid character '<'"):
		return true
	case strings.Contains(msg, "<!doctype"):
		return true
	case strings.Contains(msg, "<html"):
		return true
	default:
		return false
	}
}
