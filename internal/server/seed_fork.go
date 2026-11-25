// Copyright (c) 2026 Ant Group Corporation.
// SPDX-License-Identifier: Apache-2.0

package server

import (
	"fmt"

	api "github.com/inclusionAI/sandboxd/api/runtime/v1"
	"github.com/inclusionAI/sandboxd/config"
	"github.com/inclusionAI/sandboxd/pkg/errord"
)

func validateSeedRequest(req *api.StartRequest, cfg config.RunscConfig) error {
	if req.Runtime != "" && req.Runtime != config.RuntimeNameRunsc {
		return fmt.Errorf("template_id requires runsc: %w", errord.ErrInvalidArgument)
	}
	if req.Rootfs != nil || len(req.Command) != 0 || req.Cwd != "" || req.ProcessUser != nil ||
		len(req.Mounts) != 0 || req.CheckpointInfo != nil || req.InjectEntrypoint != "" ||
		len(req.XpuAllocations) != 0 || req.WritableLayerLimitBytes != 0 || req.ExtraConfig != "" || req.PreferImagePath {
		return fmt.Errorf("template_id inherits rootfs, mounts, process and runtime options and cannot be combined with restore: %w", errord.ErrInvalidArgument)
	}
	platform, err := config.NormalizeRunscPlatform(cfg.Platform)
	if cfg.SeedHelper == "" || err != nil || platform == config.RunscPlatformSystrap {
		return fmt.Errorf("template_id requires a configured seed_helper and kvm or slimvm: %w", errord.ErrFailedPrecondition)
	}
	return nil
}
