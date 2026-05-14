// Package k8s wraps the narrow K8s API surface the reconciler needs:
// in-cluster client construction and Secret read/upsert in the install
// namespace.
package k8s

import (
	"context"
	"fmt"

	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
)

// Client is a thin wrapper around the typed corev1 client scoped to a
// single namespace.
type Client struct {
	cs        kubernetes.Interface
	namespace string
}

// NewInCluster constructs a Client from the pod's in-cluster service
// account.
func NewInCluster(namespace string) (*Client, error) {
	cfg, err := rest.InClusterConfig()
	if err != nil {
		return nil, fmt.Errorf("in-cluster config: %w", err)
	}
	cs, err := kubernetes.NewForConfig(cfg)
	if err != nil {
		return nil, fmt.Errorf("kubernetes client: %w", err)
	}
	return &Client{cs: cs, namespace: namespace}, nil
}

// ReadSecret returns the named Secret's data, or nil-data + nil-error
// if the Secret doesn't exist. Other errors are returned as-is.
func (c *Client) ReadSecret(ctx context.Context, name string) (map[string][]byte, error) {
	s, err := c.cs.CoreV1().Secrets(c.namespace).Get(ctx, name, metav1.GetOptions{})
	if err != nil {
		if apierrors.IsNotFound(err) {
			return nil, nil
		}
		return nil, err
	}
	return s.Data, nil
}

// UpsertSecretKey creates or patches the named Secret so that key=val.
// Other keys on an existing Secret are preserved.
func (c *Client) UpsertSecretKey(ctx context.Context, name, key string, val []byte) error {
	existing, err := c.cs.CoreV1().Secrets(c.namespace).Get(ctx, name, metav1.GetOptions{})
	if apierrors.IsNotFound(err) {
		_, err := c.cs.CoreV1().Secrets(c.namespace).Create(ctx, &corev1.Secret{
			ObjectMeta: metav1.ObjectMeta{
				Name:      name,
				Namespace: c.namespace,
				Labels: map[string]string{
					"app.kubernetes.io/managed-by": "authstack-reconciler",
				},
			},
			Type: corev1.SecretTypeOpaque,
			Data: map[string][]byte{key: val},
		}, metav1.CreateOptions{})
		return err
	}
	if err != nil {
		return err
	}
	if existing.Data == nil {
		existing.Data = map[string][]byte{}
	}
	existing.Data[key] = val
	if existing.Labels == nil {
		existing.Labels = map[string]string{}
	}
	if _, set := existing.Labels["app.kubernetes.io/managed-by"]; !set {
		existing.Labels["app.kubernetes.io/managed-by"] = "authstack-reconciler"
	}
	_, err = c.cs.CoreV1().Secrets(c.namespace).Update(ctx, existing, metav1.UpdateOptions{})
	return err
}
