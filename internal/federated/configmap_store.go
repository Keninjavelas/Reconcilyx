package federated

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

const (
	DefaultConfigMapName      = "reconcilyx-federated-global-model"
	DefaultConfigMapNamespace = "reconcilyx-system"
	ConfigMapDataKey          = "global_model.json"
	ConfigMapVersionKey       = "version"
	ConfigMapUpdatedAtKey     = "updated_at"
)

func SaveGlobalModel(ctx context.Context, c client.Client, namespace, name string, snap *GlobalQTable) error {
	if snap == nil {
		return errors.New("federated: snapshot is nil")
	}
	payload, err := json.Marshal(snap)
	if err != nil {
		return fmt.Errorf("federated: marshal global model: %w", err)
	}
	var existing corev1.ConfigMap
	key := client.ObjectKey{Namespace: namespace, Name: name}
	err = c.Get(ctx, key, &existing)
	switch {
	case err == nil:
		if existing.Data == nil {
			existing.Data = make(map[string]string)
		}
		existing.Data[ConfigMapDataKey] = string(payload)
		existing.Data[ConfigMapVersionKey] = itoa(int(snap.Version))
		existing.Data[ConfigMapUpdatedAtKey] = snap.CreatedAt.UTC().Format(time.RFC3339)
		return c.Update(ctx, &existing)
	case apierrors.IsNotFound(err):
		cm := &corev1.ConfigMap{
			ObjectMeta: metav1.ObjectMeta{
				Name:      name,
				Namespace: namespace,
			},
			Data: map[string]string{
				ConfigMapDataKey:      string(payload),
				ConfigMapVersionKey:   itoa(int(snap.Version)),
				ConfigMapUpdatedAtKey: snap.CreatedAt.UTC().Format(time.RFC3339),
			},
		}
		return c.Create(ctx, cm)
	default:
		return err
	}
}

func LoadGlobalModel(ctx context.Context, c client.Client, namespace, name string) (*GlobalQTable, error) {
	var existing corev1.ConfigMap
	key := client.ObjectKey{Namespace: namespace, Name: name}
	if err := c.Get(ctx, key, &existing); err != nil {
		if apierrors.IsNotFound(err) {
			return &GlobalQTable{Entries: make(map[string]GlobalQEntry), CreatedAt: time.Now().UTC()}, nil
		}
		return nil, err
	}
	raw, ok := existing.Data[ConfigMapDataKey]
	if !ok || raw == "" {
		return &GlobalQTable{Entries: make(map[string]GlobalQEntry), CreatedAt: time.Now().UTC()}, nil
	}
	var out GlobalQTable
	if err := json.Unmarshal([]byte(raw), &out); err != nil {
		return nil, fmt.Errorf("federated: unmarshal global model: %w", err)
	}
	if out.Entries == nil {
		out.Entries = make(map[string]GlobalQEntry)
	}
	return &out, nil
}
