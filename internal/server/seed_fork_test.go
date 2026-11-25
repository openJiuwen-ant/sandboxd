// Copyright (c) 2026 Ant Group Corporation.
// SPDX-License-Identifier: Apache-2.0

package server

import (
	"errors"
	"testing"

	api "github.com/inclusionAI/sandboxd/api/runtime/v1"
	"github.com/inclusionAI/sandboxd/config"
	"github.com/inclusionAI/sandboxd/pkg/errord"
)

func TestSeedRequestAdmission(t *testing.T) {
	cfg := config.RunscConfig{SeedHelper: "/runtime/runsc-seed", Platform: "slimvm"}
	req := &api.StartRequest{TemplateID: "warm", Envs: map[string]string{"CHILD": "1"}}
	if err := validateSeedRequest(req, cfg); err != nil {
		t.Fatal(err)
	}
	if err := validateSeedRequest(req, config.RunscConfig{}); !errors.Is(err, errord.ErrFailedPrecondition) {
		t.Fatalf("disabled seed fork: %v", err)
	}
	for _, req := range []*api.StartRequest{
		{Runtime: "runc"}, {Rootfs: &api.RootfsConfig{}}, {Mounts: []*api.Mount{{Target: "/work"}}},
		{CheckpointInfo: &api.CheckpointInfo{}}, {Command: []string{"sh"}}, {ProcessUser: &api.ProcessUser{}},
	} {
		req.TemplateID = "warm"
		if err := validateSeedRequest(req, cfg); !errors.Is(err, errord.ErrInvalidArgument) {
			t.Errorf("accepted incompatible request %v: %v", req, err)
		}
	}
}
