package console

import (
	"context"

	"github.com/ViaQ/logerr/v2/kverrors"
	"github.com/go-logr/logr"
	consolev1 "github.com/openshift/api/console/v1"
	apiextensions "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/util/retry"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

const KibanaConsoleLinkName = "kibana-public-url"

// ConsoleLinkEqualityFunc is the type for functions that compare two consolelinks.
// Return true if two consolelinks are equal.
type ConsoleLinkEqualityFunc func(current, desired *consolev1.ConsoleLink) bool

// MutateConsoleLinkFunc is the type for functions that mutate the current consolelink
// by applying the values from the desired consolelink.
type MutateConsoleLinkFunc func(current, desired *consolev1.ConsoleLink)

// CreateOrUpdateConsoleLink attempts first to get the given consolelink. If the
// consolelink does not exist, the consolelink will be created. Otherwise,
// if the consolelink exists and the provided comparison func detects any changes
// an update is attempted. Updates are retried with backoff (See retry.DefaultRetry).
// Returns on failure an non-nil error.
func CreateOrUpdateConsoleLink(ctx context.Context, c client.Client, cl *consolev1.ConsoleLink, equal ConsoleLinkEqualityFunc, mutate MutateConsoleLinkFunc) error {
	current := &consolev1.ConsoleLink{}
	key := client.ObjectKey{Name: cl.Name}
	err := c.Get(ctx, key, current)
	if err != nil {
		if apierrors.IsNotFound(err) {
			err = c.Create(ctx, cl)

			if err == nil {
				return nil
			}

			return kverrors.Wrap(err, "failed to create consolelink",
				"name", cl.Name,
			)
		}

		return kverrors.Wrap(err, "failed to get consolelink",
			"name", cl.Name,
		)
	}

	if !equal(current, cl) {
		err := retry.RetryOnConflict(retry.DefaultRetry, func() error {
			if err := c.Get(ctx, key, current); err != nil {
				return kverrors.Wrap(err, "failed to get consolelink",
					"name", cl.Name,
				)
			}

			mutate(current, cl)
			if err := c.Update(ctx, current); err != nil {
				return err
			}
			return nil
		})
		if err != nil {
			return kverrors.Wrap(err, "failed to update consolelink",
				"name", cl.Name,
			)
		}
		return nil
	}

	return nil
}

func DeleteKibanaConsoleLink(ctx context.Context, c client.Client, log logr.Logger) error {
	if !ConsoleLinkEnabled(c, log) {
		log.Info("Console CRD is not found, skipping console link deletion")
		return nil
	}

	current := NewConsoleLink(KibanaConsoleLinkName, "", "", "", "")

	if err := c.Delete(ctx, current); err != nil {
		if !apierrors.IsNotFound(err) {
			return kverrors.Wrap(err, "failed to delete consolelink",
				"name", KibanaConsoleLinkName,
			)
		}
	}

	return nil
}

// ConsoleLinksEqual returns true all of the following are equal:
// - location
// - link text
// - link href
// - application menu section
func ConsoleLinksEqual(current, desired *consolev1.ConsoleLink) bool {
	if current.Spec.Location != desired.Spec.Location {
		return false
	}

	if current.Spec.Link.Text != desired.Spec.Link.Text {
		return false
	}

	if current.Spec.Link.Href != desired.Spec.Link.Href {
		return false
	}

	if current.Spec.ApplicationMenu.Section != desired.Spec.ApplicationMenu.Section {
		return false
	}

	return true
}

// MutateSpecOnly is a default mutate implementation that copies
// only the spec from desired to current consolelink.
func MutateConsoleLinkSpecOnly(current, desired *consolev1.ConsoleLink) {
	current.Spec = desired.Spec
}

func ConsoleLinkEnabled(client client.Client, log logr.Logger) bool {
	consoleLinkCRD := apiextensions.CustomResourceDefinition{}
	err := client.Get(context.TODO(), types.NamespacedName{Name: "consolelinks.console.openshift.io"}, &consoleLinkCRD)

	//log.Error(err, "ConsoleLinkEnabled")

	return err == nil
}
