load("//tools:defs.bzl", "go_binary")

go_binary(
    name = "runsc-seed",
    srcs = ["main.go"],
    pure = True,
    deps = [
        "//runsc/boot",
        "//runsc/config",
        "//runsc/container",
        "//runsc/flag",
        "//runsc/specutils",
        "@com_github_opencontainers_runtime_spec//specs-go:go_default_library",
    ],
)
