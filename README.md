# Bpftrace playground

While this repository is compatible with standard Go tooling, the playground
uses `nix` to build the full image and standardize tooling. First, you can
ensure that you are in a general development enviornment:

```
nix develop
```

To build the image, use:

```
nix build
```

The image can then be published using `skopeo` (available using `nix develop`):

```
skopeo copy oci-archive:result docker://<registry>/<image>:<tag>
```
