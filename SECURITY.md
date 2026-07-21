# Security Policy

RewindBPF is a safety boundary for autonomous-agent execution. Treat the
runtime, policy compiler, supervisor, eBPF programs, native helpers, and
release scripts as security-sensitive code.

## Reporting a vulnerability

Please do not publish exploit details, real credentials, personal data, or
destructive reproduction steps in a public issue. After the repository is
published, use GitHub Security Advisories (or the repository owner's private
security contact) and include:

- affected commit or release;
- platform and kernel/OS version;
- a minimal synthetic reproduction;
- expected fail-closed behavior and observed behavior; and
- whether any real data or credentials were exposed.

Use disposable fixtures only. Never attach a real `.env`, SSH key, bearer token,
private key, customer data, or host filesystem snapshot.

## Scope and limitations

The Linux OverlayFS/FUSE, Landlock, cgroup-v2, eBPF evidence, supervisor, and
policy paths are the reference security surface. Privileged acceptance belongs
inside the disposable Ubuntu VM. macOS and Windows capabilities are explicitly
limited in [`docs/PLATFORM_STATUS.md`](docs/PLATFORM_STATUS.md); do not report
their contract or smoke tests as Linux-equivalent host enforcement.

RewindBPF cannot undo external database, cloud, network, device, or arbitrary
kernel side effects. Report boundary failures separately from ordinary product
bugs so they can be triaged with the right severity.
