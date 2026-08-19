# Future Synology SPK packaging

`roborock-mqtt-loxone` is prepared for, but does not yet ship as, a native
Synology `.spk` package. The Roborock/Loxone engine, health model, watchdog,
version reporting, update policy and data paths are independent of Docker.

## Existing platform boundary

- `supervisor.Runtime` reports the active platform and turns an unrecoverable
  watchdog decision into a single external restart request. Docker implements
  that request by exiting non-zero under `restart: unless-stopped`. A DSM
  package can use the same exit contract under its service script.
- `supervisor.UpdateSupervisor` is the transactional update boundary:
  current artifact, fetch, replace, health validation, version validation,
  rollback and finalization. `updater.DockerEngine` is one implementation; an
  SPK updater must implement the same interface without a Docker socket.
- Health (`/api/health`, `/api/live`, `/api/ready`), version metadata, stdout
  logging and the configured data directory do not depend on Docker APIs.
- Update settings and operation state are versioned files in the data directory.

Set `ROBOROCK_SUPERVISOR=synology` in the native service environment so the UI
and diagnostics identify the correct host supervisor.

## SPK work still required

1. Create the DSM package tree (`INFO`, `package.tgz`, `scripts/`, wizard and
   privilege definitions) for the minimum supported DSM release.
2. Build and package signed `linux/amd64` and `linux/arm64` binaries. Map
   Synology platform names to these architectures and reject unsupported NAS
   models explicitly.
3. Create a dedicated low-privilege package user. Give it ownership only of the
   package data/log/run directories; do not run the Roborock bridge as root.
4. Implement `start-stop-status` using DSM service conventions, PID files and a
   bounded graceful-stop timeout. DSM must restart a non-zero watchdog exit,
   while an administrator stop must remain stopped.
5. Expose port 8080 through DSM's application portal/reverse proxy and package
   firewall rules. Preserve the same LAN-only guidance for Direct Loxone.
6. Map the persistent directory to the package volume and import an existing
   Docker data directory without changing `config.json`, `.session/`, room
   overrides, schedules, update settings or backups. Take a backup before the
   first migration.
7. Implement a Synology `UpdateSupervisor`. It should download a signed SPK or
   signed release artifact into a staging directory, validate checksum/signature
   and architecture, back up data, invoke only the fixed DSM package lifecycle,
   verify `/api/live` and `/api/system/status`, and restore the previous package
   payload on failure. It must not accept arbitrary commands or package names.
8. Decide whether releases are distributed through a Synology package source
   or manual signed SPK downloads; implement DSM-compatible signing and release
   metadata for the selected route.
9. Route stdout/stderr into DSM Log Center or package-owned rotated files while
   preserving redaction. Add collection/export controls without credentials.
10. Test install, upgrade, downgrade/rollback, uninstall-with-data-preservation,
    reboot, volume relocation, expired Roborock session, network loss and disk
    full on real DSM hardware for both architectures.

## Migration principle

The data directory is the portable boundary. Stop the Docker deployment, back
up its complete data volume, copy it to the SPK data directory with the native
package user's ownership and `0600` secret files, then start the SPK. Never run
Docker and SPK instances simultaneously against the same Roborock account or
Loxone inputs.
