/*
Copyright 2020 The Crossplane Authors.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package controller

import (
	"context"
	"encoding/json"
	"fmt"

	kerrors "k8s.io/apimachinery/pkg/api/errors"

	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"

	runtimev1alpha1 "github.com/crossplane/crossplane-runtime/apis/core/v1alpha1"
	"github.com/crossplane/crossplane-runtime/pkg/event"
	"github.com/crossplane/crossplane-runtime/pkg/logging"
	"github.com/crossplane/crossplane-runtime/pkg/reconciler/managed"
	"github.com/crossplane/crossplane-runtime/pkg/resource"
	"github.com/pkg/errors"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/rest"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/crossplane-contrib/provider-kubernetes/apis/resource/v1alpha1"
	helmv1alpha1 "github.com/crossplane-contrib/provider-kubernetes/apis/v1alpha1"
	"github.com/crossplane-contrib/provider-kubernetes/pkg/clients"
)

const (
	errNotKubernetesResource      = "managed resource is not a KubernetesResource custom resource"
	errProviderConfigNotSet       = "provider config is not set"
	errProviderNotRetrieved       = "provider could not be retrieved"
	errNewKubernetesClient        = "cannot create new Kubernetes client"
	errProviderSecretNotRetrieved = "secret referred in provider could not be retrieved"
	errFailedToCreateRestConfig   = "cannot create new rest config using provider secret"

	errFailedToGetSecret = "failed to get secret from namespace \"%s\""
	errSecretDataIsNil   = "secret data is nil"

	errUnmarshalTemplate = "cannot unmarshal template"
)

// SetupKubernetesResource adds a controller that reconciles KubernetesResource managed resources.
func SetupKubernetesResource(mgr ctrl.Manager, l logging.Logger) error {
	name := managed.ControllerName(v1alpha1.KubernetesResourceGroupKind)
	logger := l.WithValues("controller", name)

	r := managed.NewReconciler(mgr,
		resource.ManagedKind(v1alpha1.KubernetesResourceGroupVersionKind),
		managed.WithExternalConnecter(&connector{
			logger:          logger,
			client:          mgr.GetClient(),
			newRestConfigFn: clients.NewRestConfig,
			newKubeClientFn: clients.NewKubeClient,
		}),
		managed.WithInitializers(managed.NewNameAsExternalName(mgr.GetClient())),
		managed.WithLogger(logger),
		managed.WithRecorder(event.NewAPIRecorder(mgr.GetEventRecorderFor(name))))

	return ctrl.NewControllerManagedBy(mgr).
		Named(name).
		For(&v1alpha1.KubernetesResource{}).
		Complete(r)
}

type connector struct {
	logger          logging.Logger
	client          client.Client
	newRestConfigFn func(creds map[string][]byte) (*rest.Config, error)
	newKubeClientFn func(config *rest.Config) (client.Client, error)
}

func (c *connector) Connect(ctx context.Context, mg resource.Managed) (managed.ExternalClient, error) {
	cr, ok := mg.(*v1alpha1.KubernetesResource)
	if !ok {
		return nil, errors.New(errNotKubernetesResource)
	}
	l := c.logger.WithValues("request", cr.Name)

	l.Debug("Connecting")

	p := &helmv1alpha1.ProviderConfig{}

	if cr.GetProviderConfigReference() == nil {
		return nil, errors.New(errProviderConfigNotSet)
	}

	n := types.NamespacedName{Name: cr.GetProviderConfigReference().Name}
	if err := c.client.Get(ctx, n, p); err != nil {
		return nil, errors.Wrap(err, errProviderNotRetrieved)
	}
	key := types.NamespacedName{Namespace: p.Spec.CredentialsSecretRef.Namespace, Name: p.Spec.CredentialsSecretRef.Name}
	creds, err := getSecretData(ctx, c.client, key)
	if err != nil {
		return nil, errors.Wrap(err, errProviderSecretNotRetrieved)
	}

	// We rely on multiple keys in ProviderConfig secret, so, ignoring "p.Spec.CredentialsSecretRef.Key"
	// TODO(hasan): Consider relying only "kubeconfig" key
	rc, err := c.newRestConfigFn(creds)
	if err != nil {
		return nil, errors.Wrap(err, errFailedToCreateRestConfig)
	}

	// TODO(hasan): Remove below HACK, once https://github.com/crossplane/crossplane/issues/1687 resolved
	// HACK
	p.Status.SetConditions(runtimev1alpha1.Available())
	if err := c.client.Status().Update(ctx, p); err != nil {
		return nil, errors.Wrap(err, "Failed to update ProviderConfig status")
	}
	// END OF HACK

	k, err := c.newKubeClientFn(rc)
	if err != nil {
		return nil, errors.Wrap(err, errNewKubernetesClient)
	}

	return &kubeExternal{
		logger:    l,
		localKube: c.client,
		kube:      k,
	}, errors.Wrap(err, errNewKubernetesClient)
}

type kubeExternal struct {
	logger    logging.Logger
	localKube client.Client
	kube      client.Client
}

func (e *kubeExternal) Observe(ctx context.Context, mg resource.Managed) (managed.ExternalObservation, error) {
	cr, ok := mg.(*v1alpha1.KubernetesResource)
	if !ok {
		return managed.ExternalObservation{}, errors.New(errNotKubernetesResource)
	}

	e.logger.Debug("Observing " + cr.Name)
	t := &unstructured.Unstructured{}
	if err := json.Unmarshal(cr.Spec.ForProvider.Object.Raw, t); err != nil {
		return managed.ExternalObservation{}, errors.Wrap(err, errUnmarshalTemplate)
	}

	err := e.kube.Get(ctx, types.NamespacedName{
		Namespace: t.GetNamespace(),
		Name:      t.GetName(),
	}, t)

	if kerrors.IsNotFound(err) {
		return managed.ExternalObservation{ResourceExists: false}, nil
	}
	if err != nil {
		return managed.ExternalObservation{}, errors.Wrap(err, "failed to get")
	}
	cr.Status.AtProvider.Object.Raw, err = t.MarshalJSON()
	if err != nil {
		return managed.ExternalObservation{}, errors.Wrap(err, "failed to marshal")
	}

	return managed.ExternalObservation{
		ResourceExists:   true,
		ResourceUpToDate: false,
	}, nil
}

func (e *kubeExternal) Create(ctx context.Context, mg resource.Managed) (managed.ExternalCreation, error) {
	cr, ok := mg.(*v1alpha1.KubernetesResource)
	if !ok {
		return managed.ExternalCreation{}, errors.New(errNotKubernetesResource)
	}

	e.logger.Debug("Creating " + cr.Name)
	t := &unstructured.Unstructured{}
	if err := json.Unmarshal(cr.Spec.ForProvider.Object.Raw, t); err != nil {
		return managed.ExternalCreation{}, errors.Wrap(err, errUnmarshalTemplate)
	}

	if err := e.kube.Create(ctx, t); err != nil {
		return managed.ExternalCreation{}, errors.Wrap(err, "failed to create")
	}

	var err error
	if cr.Status.AtProvider.Object.Raw, err = t.MarshalJSON(); err != nil {
		return managed.ExternalCreation{}, errors.Wrap(err, "failed to marshal object for atProvider")
	}

	return managed.ExternalCreation{}, nil
}

func (e *kubeExternal) Update(ctx context.Context, mg resource.Managed) (managed.ExternalUpdate, error) {
	cr, ok := mg.(*v1alpha1.KubernetesResource)
	if !ok {
		return managed.ExternalUpdate{}, errors.New(errNotKubernetesResource)
	}

	e.logger.Debug("Updating " + cr.Name)
	t := &unstructured.Unstructured{}
	if err := json.Unmarshal(cr.Spec.ForProvider.Object.Raw, t); err != nil {
		return managed.ExternalUpdate{}, errors.Wrap(err, errUnmarshalTemplate)
	}

	if err := resource.NewAPIPatchingApplicator(e.kube).Apply(ctx, t); err != nil {
		return managed.ExternalUpdate{}, errors.Wrap(err, "failed to apply")
	}

	var err error
	if cr.Status.AtProvider.Object.Raw, err = t.MarshalJSON(); err != nil {
		return managed.ExternalUpdate{}, errors.Wrap(err, "failed to marshal object for atProvider")
	}

	return managed.ExternalUpdate{}, nil
}

func (e *kubeExternal) Delete(ctx context.Context, mg resource.Managed) error {
	cr, ok := mg.(*v1alpha1.KubernetesResource)
	if !ok {
		return errors.New(errNotKubernetesResource)
	}

	e.logger.Debug("Deleting " + cr.Name)
	t := &unstructured.Unstructured{}
	if err := json.Unmarshal(cr.Spec.ForProvider.Object.Raw, t); err != nil {
		return errors.Wrap(err, errUnmarshalTemplate)
	}

	if err := e.kube.Delete(ctx, t); resource.IgnoreNotFound(err) != nil {
		return errors.Wrap(err, "failed to delete")
	}

	return nil
}

func getSecretData(ctx context.Context, kube client.Client, nn types.NamespacedName) (map[string][]byte, error) {
	s := &corev1.Secret{}
	if err := kube.Get(ctx, nn, s); err != nil {
		return nil, errors.Wrap(err, fmt.Sprintf(errFailedToGetSecret, nn.Namespace))
	}
	if s.Data == nil {
		return nil, errors.New(errSecretDataIsNil)
	}
	return s.Data, nil
}
