package canon

// Frozen noise field lists from CANONICALIZATION_CONTRACT_V1.md §4.5.
// These fields are removed before computing resource_shape_hash.
// The lists are frozen — adding new noise fields requires a new canon version.

var k8sNoiseFields = map[string]bool{
	// Metadata noise
	"metadata.uid":                        true,
	"metadata.resourceVersion":            true,
	"metadata.generation":                 true,
	"metadata.creationTimestamp":          true,
	"metadata.deletionTimestamp":          true,
	"metadata.deletionGracePeriodSeconds": true,
	"metadata.managedFields":              true,
	"metadata.selfLink":                   true,
	"metadata.generateName":               true,

	// Status noise (never part of desired state)
	"status": true,

	// Annotation noise (operator-injected)
	"metadata.annotations.kubectl.kubernetes.io/last-applied-configuration": true,
	"metadata.annotations.deployment.kubernetes.io/revision":                true,
}

var k8sNoiseAnnotationPrefixes = []string{
	"kubectl.kubernetes.io/",
	"deployment.kubernetes.io/",
	"control-plane.alpha.kubernetes.io/",
}

// removeK8sNoiseFields removes noise fields from a K8s object map.
func removeK8sNoiseFields(obj map[string]interface{}) {
	// Remove top-level status
	delete(obj, "status")

	metadata, ok := obj["metadata"].(map[string]interface{})
	if !ok {
		return
	}

	// Remove metadata noise fields
	delete(metadata, "uid")
	delete(metadata, "resourceVersion")
	delete(metadata, "generation")
	delete(metadata, "creationTimestamp")
	delete(metadata, "deletionTimestamp")
	delete(metadata, "deletionGracePeriodSeconds")
	delete(metadata, "managedFields")
	delete(metadata, "selfLink")
	delete(metadata, "generateName")

	// Remove noisy annotations
	annotations, ok := metadata["annotations"].(map[string]interface{})
	if !ok {
		return
	}

	for key := range annotations {
		for _, prefix := range k8sNoiseAnnotationPrefixes {
			if len(key) >= len(prefix) && key[:len(prefix)] == prefix {
				delete(annotations, key)
				break
			}
		}
	}

	// Remove empty annotations map
	if len(annotations) == 0 {
		delete(metadata, "annotations")
	}
}
