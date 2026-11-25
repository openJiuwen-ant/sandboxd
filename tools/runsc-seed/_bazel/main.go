// Copyright (c) 2026 Ant Group Corporation.
// SPDX-License-Identifier: Apache-2.0

// Command runsc-seed adapts gVisor's host Go API to sandboxd. Build this
// package with the same gVisor Bazel tree as the installed runsc and helpers.
package main

import (
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"

	specs "github.com/opencontainers/runtime-spec/specs-go"

	"gvisor.dev/gvisor/runsc/boot"
	"gvisor.dev/gvisor/runsc/config"
	"gvisor.dev/gvisor/runsc/container"
	"gvisor.dev/gvisor/runsc/flag"
	"gvisor.dev/gvisor/runsc/specutils"
)

func run() error {
	flags := flag.NewFlagSet("runsc-seed", flag.ContinueOnError)
	config.RegisterFlags(flags)
	binary := flags.String("runsc", "runsc", "matching runsc binary")
	bundle := flags.String("bundle", "", "absolute OCI bundle directory")
	seed := flags.String("seed", "", "existing template ID")
	workers := flags.Int("workers", 1, "template workers")
	stdout := flags.String("stdout", "", "application stdout file")
	stderr := flags.String("stderr", "", "application stderr file")
	if err := flags.Parse(os.Args[1:]); err != nil {
		return err
	}
	if flags.NArg() != 2 {
		return fmt.Errorf("usage: runsc-seed [flags] create|fork|delete ID")
	}
	path, err := exec.LookPath(*binary)
	if err != nil {
		return err
	}
	specutils.ExePath, err = filepath.Abs(path)
	if err != nil {
		return err
	}
	conf, err := config.NewFromFlags(flags)
	if err != nil {
		return err
	}
	action, id := flags.Arg(0), flags.Arg(1)
	if action == "delete" {
		t, err := container.LoadTemplate(conf.RootDir, id)
		if err != nil {
			return err
		}
		return t.Destroy()
	}
	if action != "create" && action != "fork" {
		return fmt.Errorf("unknown seed operation %q", action)
	}
	if !filepath.IsAbs(*bundle) {
		return fmt.Errorf("absolute --bundle is required")
	}
	out, err := os.OpenFile(*stdout, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0600)
	if err != nil {
		return err
	}
	defer out.Close()
	diagnostics, err := os.OpenFile(*stderr, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0600)
	if err != nil {
		return err
	}
	defer diagnostics.Close()
	if action == "create" {
		spec, err := specutils.ReadSpec(*bundle, specutils.SpecOpts{Conf: conf})
		if err != nil {
			return err
		}
		_, err = container.CreateTemplate(conf, container.TemplateArgs{
			ID: id, Workers: *workers, Spec: spec, BundleDir: *bundle, Stdout: out, Stderr: diagnostics,
		})
		return err
	}
	t, err := container.LoadTemplate(conf.RootDir, *seed)
	if err != nil {
		return err
	}
	var child struct {
		CgroupPath  string
		Resources   *specs.LinuxResources
		Annotations map[string]string
		Env         []string
		Network     boot.CreateLinksAndRoutesArgs
	}
	if err := json.NewDecoder(os.Stdin).Decode(&child); err != nil {
		return fmt.Errorf("decode fork request: %w", err)
	}
	// sandboxd recovers TAP and cgroup ownership from the child OCI bundle.
	source, err := container.Load(conf.RootDir, container.FullID{SandboxID: *seed, ContainerID: *seed}, container.LoadOpts{Exact: true, AllowTemplate: true, SkipCheck: true})
	if err != nil {
		return err
	}
	if source.SandboxPid() != t.PID || source.Spec == nil || source.Spec.Linux == nil {
		return fmt.Errorf("template %q changed while preparing child", *seed)
	}
	spec := source.Spec
	spec.Linux.CgroupsPath = child.CgroupPath
	spec.Linux.Resources = child.Resources
	if spec.Annotations == nil {
		spec.Annotations = make(map[string]string)
	}
	for k, v := range child.Annotations {
		spec.Annotations[k] = v
	}
	data, err := json.Marshal(spec)
	if err != nil {
		return err
	}
	if err := os.WriteFile(filepath.Join(*bundle, "config.json"), data, 0600); err != nil {
		return err
	}
	tap := os.NewFile(3, "sandboxd-tap")
	defer tap.Close()
	child.Network.FilePayload.Files = []*os.File{tap}
	_, err = t.Fork(conf, container.ForkArgs{
		ID: id, Env: child.Env, CgroupPath: child.CgroupPath, BundleDir: *bundle,
		Stdout: out, Stderr: diagnostics, NetworkArgs: &child.Network,
	})
	return err
}

func main() {
	if err := run(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
