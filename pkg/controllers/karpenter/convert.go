package karpenter

import (
	"encoding/json"
	"fmt"

	corev1 "k8s.io/api/core/v1"
	coreac "k8s.io/client-go/applyconfigurations/core/v1"
)

func toApplyConfiguration[In any, Out any](in In) (*Out, error) {
	data, err := json.Marshal(in)
	if err != nil {
		return nil, fmt.Errorf("marshal: %w", err)
	}
	var out Out
	if err := json.Unmarshal(data, &out); err != nil {
		return nil, fmt.Errorf("unmarshal: %w", err)
	}
	return &out, nil
}

func applyConfigurations[In any, Out any](items []In, field string) ([]*Out, error) {
	out := make([]*Out, len(items))
	for i, item := range items {
		converted, err := toApplyConfiguration[In, Out](item)
		if err != nil {
			return nil, fmt.Errorf("convert %s[%d]: %w", field, i, err)
		}
		out[i] = converted
	}
	return out, nil
}

func envVars(vars []corev1.EnvVar) ([]*coreac.EnvVarApplyConfiguration, error) {
	return applyConfigurations[corev1.EnvVar, coreac.EnvVarApplyConfiguration](vars, "env")
}

func volumes(vols []corev1.Volume) ([]*coreac.VolumeApplyConfiguration, error) {
	return applyConfigurations[corev1.Volume, coreac.VolumeApplyConfiguration](vols, "volume")
}

func volumeMounts(mounts []corev1.VolumeMount) ([]*coreac.VolumeMountApplyConfiguration, error) {
	return applyConfigurations[corev1.VolumeMount, coreac.VolumeMountApplyConfiguration](mounts, "volumeMount")
}

func initContainers(containers []corev1.Container) ([]*coreac.ContainerApplyConfiguration, error) {
	return applyConfigurations[corev1.Container, coreac.ContainerApplyConfiguration](containers, "initContainer")
}
