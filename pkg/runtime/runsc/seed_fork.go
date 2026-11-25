// Copyright (c) 2026 Ant Group Corporation.
// SPDX-License-Identifier: Apache-2.0

package runsc

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"

	svc "github.com/inclusionAI/sandboxd/pkg/runtime"
)

// Fork resumes a child with its own cgroup and sandboxd TAP.
func (c *Client) Fork(ctx context.Context, bundle string, child svc.StartConfig) error {
	if c.Options.SeedHelper == "" {
		return fmt.Errorf("runsc seed helper is not configured")
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	if child.Network == nil || child.Network.Interface == nil {
		return fmt.Errorf("fork requires a TAP interface")
	}
	tap, err := OpenTAP(*child.Network.Interface)
	if err != nil {
		return err
	}
	defer tap.Close()
	netArgs, err := BuildNetworkArgs(NetworkConfig{Interface: child.Network.Interface, LinkAddress: child.Network.GuestHardwareAddr(),
		IP: child.Network.Ip, Mask: child.Network.Mask, Gateway: child.Network.Gateway}, tap)
	if err != nil {
		return err
	}
	request := struct {
		CgroupPath  string
		Resources   *svc.LinuxResources
		Annotations map[string]string
		Env         []string
		Network     *createLinksAndRoutesArgs
	}{CgroupPath: child.CgroupPath, Resources: svc.OCIResources(nil, child.Resources), Annotations: child.Annotations, Network: netArgs}
	for _, env := range child.Envs {
		request.Env = append(request.Env, env.Key+"="+env.Value)
	}
	data, err := json.Marshal(request)
	if err != nil {
		return err
	}
	for _, dir := range []string{bundle, filepath.Dir(child.Stdout), filepath.Dir(child.Stderr)} {
		if err := os.MkdirAll(dir, 0755); err != nil {
			return err
		}
	}
	flags := append(c.globalArgs(), c.Options.ExtraArgs...)
	flags = append(flags, "--network=none", "--directfs=false", "--runsc="+c.Binary, "--seed="+child.TemplateID,
		"--bundle="+bundle, "--stdout="+child.Stdout, "--stderr="+child.Stderr, "fork", child.ID)
	// Let the helper finish its gVisor transaction even if the caller cancels.
	// Start observes cancellation after success and rolls back the known child.
	cmd := exec.Command(c.Options.SeedHelper, flags...)
	cmd.ExtraFiles = []*os.File{tap}
	cmd.Stdin = bytes.NewReader(data)
	output, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("fork from template %q: %w: %s", child.TemplateID, err, output)
	}
	return nil
}
