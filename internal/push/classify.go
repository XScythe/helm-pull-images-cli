package push

import (
	"context"
	"errors"
	"helm-deep-pack/internal/pushspec"
	"net/http"
	"strings"

	"github.com/google/go-containerregistry/pkg/authn"
	"github.com/google/go-containerregistry/pkg/name"
	v1 "github.com/google/go-containerregistry/pkg/v1"
	"github.com/google/go-containerregistry/pkg/v1/remote"
	"github.com/google/go-containerregistry/pkg/v1/remote/transport"
)

type imageStatus string

const (
	statusPushable imageStatus = "pushable"
	statusMirrored imageStatus = "mirrored"
	statusConflict imageStatus = "conflict"
	statusUnknown  imageStatus = "unknown"
)

type classifiedImage struct {
	Spec         pushspec.ArchiveSpec
	Status       imageStatus
	RemoteDigest string
	ProbeErr     error
}

type remoteHeadFn func(name.Reference, ...remote.Option) (*v1.Descriptor, error)

func classifyOne(ctx context.Context, registry string, spec pushspec.ArchiveSpec, headFn remoteHeadFn) classifiedImage {
	ref, err := name.ParseReference(strings.TrimRight(registry, "/") + "/" + spec.Target)
	if err != nil {
		return classifiedImage{
			Spec:     spec,
			Status:   statusUnknown,
			ProbeErr: err,
		}
	}

	descriptor, err := headFn(ref, remote.WithContext(ctx), remote.WithAuthFromKeychain(authn.DefaultKeychain))
	if err != nil {
		if isNotFoundError(err) {
			return classifiedImage{
				Spec:   spec,
				Status: statusPushable,
			}
		}
		return classifiedImage{
			Spec:     spec,
			Status:   statusUnknown,
			ProbeErr: err,
		}
	}

	remoteDigest := descriptor.Digest.String()
	if remoteDigest == spec.OCIDigest {
		return classifiedImage{
			Spec:         spec,
			Status:       statusMirrored,
			RemoteDigest: remoteDigest,
		}
	}

	return classifiedImage{
		Spec:         spec,
		Status:       statusConflict,
		RemoteDigest: remoteDigest,
	}
}

func classifyImages(ctx context.Context, registry string, specs []pushspec.ArchiveSpec) []classifiedImage {
	return classifyImagesWithHead(ctx, registry, specs, remote.Head)
}

func classifyImagesWithHead(ctx context.Context, registry string, specs []pushspec.ArchiveSpec, headFn remoteHeadFn) []classifiedImage {
	result := make([]classifiedImage, len(specs))
	for i, spec := range specs {
		result[i] = classifyOne(ctx, registry, spec, headFn)
	}
	return result
}

func isNotFoundError(err error) bool {
	var transportErr *transport.Error
	if errors.As(err, &transportErr) {
		if transportErr.StatusCode == http.StatusNotFound {
			return true
		}
		for _, diag := range transportErr.Errors {
			if diag.Code == transport.ManifestUnknownErrorCode || diag.Code == transport.NameUnknownErrorCode {
				return true
			}
		}
	}

	msg := strings.ToLower(err.Error())
	return strings.Contains(msg, "not found") ||
		strings.Contains(msg, "manifest_unknown") ||
		strings.Contains(msg, "name_unknown") ||
		strings.Contains(msg, "404")
}
