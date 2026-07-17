# Reproducible Linux lab

This directory defines an optional reproducible development container. It is not required for the primary kernel MVP path, which runs RewindBPF directly inside the disposable Ubuntu VM.

## Safety boundary

The `kernel` Compose profile is intentionally privileged because OverlayFS, namespaces, and eBPF integration require kernel capabilities. It is optional and must run only inside the disposable Ubuntu VM created for this project.

Do not run the `kernel` profile directly on the personal macOS host, a production Linux host, or against bind-mounted personal directories. The Compose file uses named volumes and does not mount the host home directory.

Docker Desktop on macOS is suitable for the non-privileged `userspace` profile and Compose syntax validation. It is not the approved boundary for kernel integration.

## Primary VM workflow

1. Create or open the disposable Ubuntu VM in UTM.
2. Install the direct Linux toolchain inside that VM.
3. Copy or clone this repository into the VM. Do not bind-mount the personal Mac home directory.
4. Run read-only capability checks.
5. Stop for a safety review before any mount, eBPF load, or destructive operation.

## Optional Compose workflow

If containerized tooling is useful later:

1. Install Docker Engine and the Compose plugin inside the Ubuntu VM.
2. Validate the Compose file without starting containers:

   ```bash
   docker compose -f infra/compose.yaml --profile kernel config
   ```

3. Only after an explicit safety review, build and start the optional kernel lab:

   ```bash
   docker compose -f infra/compose.yaml --profile kernel build
   docker compose -f infra/compose.yaml --profile kernel up -d
   docker compose -f infra/compose.yaml --profile kernel exec rewind-lab bash
   ```

The build command runs Go tests while building the image. The `up` command starts an intentionally privileged container and is therefore a separate safety gate.

## Userspace-only mode

For safe local tooling checks, use the non-privileged profile:

```bash
docker compose -f infra/compose.yaml --profile userspace config
docker compose -f infra/compose.yaml --profile userspace build
```

No kernel, mount, eBPF, or destructive operation is performed by the userspace image build itself.
