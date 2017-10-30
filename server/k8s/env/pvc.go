package env

import (
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/pkg/api/v1"
)

// PersistentVolumeClaimInput provides the PersistentVolumeClaim function with information to produce a Kubernetes PersistentVolumeClaim.
type PersistentVolumeClaimInput struct {
	Namespace string
	Name      string
	Storage   string
}

// PersistentVolumeClaim is used for creating a new PersistentVolumeClaim object.
func PersistentVolumeClaim(input PersistentVolumeClaimInput) *v1.PersistentVolumeClaim {
	return &v1.PersistentVolumeClaim{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: input.Namespace,
			Name:      input.Name,
			Annotations: map[string]string{
				// Setting this storage class to "cache" allows system admins to register any type of
				// storage backend for "cache" claims.
				"volume.beta.kubernetes.io/storage-class": "cache",
				"author": "m8s",
			},
		},
		Spec: v1.PersistentVolumeClaimSpec{
			AccessModes: []v1.PersistentVolumeAccessMode{
				v1.ReadWriteMany,
			},
			Resources: v1.ResourceRequirements{
				Requests: v1.ResourceList{
					v1.ResourceStorage: resource.MustParse(input.Storage),
				},
			},
		},
	}
}
