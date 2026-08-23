# Implementation Lineage and Design Decisions

This document compares `zfstools-go` with the implementations that influenced
it or share the `zfs-auto-snapshot` name. They are related historically, but
they are not interchangeable and do not define one compatibility target.

## Source Lineage

### Sun ZFS Automatic Snapshot SMF service

- Repository: [aszeszo/zfs-auto-snapshot](https://github.com/aszeszo/zfs-auto-snapshot)
- Origin: Sun Microsystems' 2008-2009 OpenSolaris implementation, later
  modified in this repository for Solaris 10.
- Primary sources: `README.zfs-auto-snapshot.txt`,
  `src/var/svc/manifest/system/filesystem/auto-snapshot.xml`, and
  `src/lib/svc/method/zfs-auto-snapshot`.

This is the historical original found during this evaluation. It is a Solaris
SMF service whose instances install cron entries. It is not a portable
standalone command and is not the direct source of `zfstools-go`.

### OpenIndiana Time Slider

- Repository: [OpenIndiana/time-slider](https://github.com/OpenIndiana/time-slider)
- Origin: Sun/OpenSolaris desktop snapshot service, now maintained by
  OpenIndiana.
- Primary sources: `lib/svc/manifest/system/filesystem/auto-snapshot.xml` and
  `usr/share/time-slider/lib/time_slider/`.

Time Slider succeeded the original SMF method while preserving its service
name, ZFS properties, and automatic snapshot naming family. It moved scheduling
into a Python daemon and added capacity-driven cleanup, replication plugins,
desktop integration, and snapshot browsing.

### Ruby zfstools

- Original repository: [bdrewery/zfstools](https://github.com/bdrewery/zfstools)
- Maintained fork used for this port: [swills/zfstools](https://github.com/swills/zfstools)
- Primary sources: `bin/zfs-auto-snapshot`, `bin/zfs-cleanup-snapshots`, and
  `lib/zfstools.rb`.

Ruby `zfstools` is the direct ancestor of `zfstools-go`. It adapted automatic
snapshots to a stateless, externally scheduled FreeBSD command and introduced
the three command interfaces, database-aware snapshots, automatic recursive
partitioning, and standalone zero-size cleanup used here.

### ZFS-on-Linux shell rewrite

- Repository: [zfsonlinux/zfs-auto-snapshot](https://github.com/zfsonlinux/zfs-auto-snapshot)
- Origin: independent Linux shell implementation started by Darik Horn.
- Primary sources: `src/zfs-auto-snapshot.sh` and
  `src/zfs-auto-snapshot.8`.

This implementation was initially mistaken for the original. It is an
independent rewrite with a different command line, target model, default
eligibility policy, naming controls, and hook system. See
[`shell-rewrite-compat-eval.md`](shell-rewrite-compat-eval.md) for the focused
comparison.

## Behavioral Comparison

| Area | Sun original | Time Slider | Ruby zfstools | Linux shell rewrite | Current Go |
| --- | --- | --- | --- | --- | --- |
| Runtime model | SMF method plus cron | SMF Python daemon | External scheduler invokes scripts | External scheduler invokes shell command | External scheduler invokes multi-call binary |
| Primary platform | Solaris/OpenSolaris | illumos/OpenIndiana | FreeBSD | Linux and other OpenZFS systems | FreeBSD |
| Dataset selection | SMF target or `//` properties | Property-driven | Property-driven, optional pool | Positional targets or `//` | Property-driven, optional pool |
| Default property policy | Documented opt-in; implementation has unset-property quirks | Exact `true`, with interval override | Exact `true`, `mysql`, or `postgresql` | Opt-out unless `--default-exclude` | Ruby-derived opt-in policy |
| Filesystem mount requirement | No | No | Yes | No | Yes |
| Naming | `zfs-auto-snap_<label>-YYYY-MM-DD-HHMM` | `zfs-auto-snap_<schedule>-YYYY-MM-DD-HHhMM` | Time Slider-style; optional `U` | Configurable prefix, separator, and label; UTC | Ruby/Time Slider-style; optional `U` |
| Retention ordering | ZFS creation time | ZFS creation time | Lexical name order | Usually ZFS creation time | Numeric ZFS creation time |
| Zero-size pruning | No | Yes | Yes | No | Yes, fail-closed |
| `KEEP=0` | Not a defined safe mode | Schedule configuration, not CLI policy | Cleanup-only | Rejected; keep must be positive | Explicit cleanup-only |
| Database coordination | No | No | MySQL and PostgreSQL from the general property | Arbitrary hooks | Ruby-derived general-property coordination |
| Dry run | No | No general CLI dry run | Mutation suppressed; output can be quiet | Complete command output | Complete, race-safe command output |
| Creation failure safety | Prunes before creation | Can purge after an unobserved command failure | Cleanup still runs | Skips target retention after failure | Retention only for confirmed targets |
| Error policy | SMF maintenance plus several nonfatal quirks | Mixed daemon/plugin behavior | Many command errors ignored | Mixed; final status often remains success | Discovery and mutation failures propagate |

## Historical Original

The Sun implementation establishes the historical service conventions, but its
SMF architecture is not a compatibility specification for this command.

Relevant conventions:

- `com.sun:auto-snapshot` and `com.sun:auto-snapshot:<label>` control inherited
  dataset policy.
- A label-specific value overrides the general property.
- Recursive roots are split where excluded descendants make `zfs snapshot -r`
  unsafe.
- `keep` describes the total number of snapshots after successful creation.
- ZFS creation time determines retention age.
- Event descriptions can be stored in `com.sun:auto-snapshot-desc`.

Implementation behavior not adopted:

- SMF, cron installation, RBAC, `pfexec`, and Solaris packaging.
- Pruning to `keep-1` before attempting creation. A creation failure can remove
  a recovery point without replacing it.
- Prefix-only retention matching across labels.
- Best-effort destruction where documented maintenance behavior is not always
  enforced.
- Unvalidated zero, negative, or nonnumeric retention values.
- Shell and regular-expression handling of dataset names.

The original documentation describes property selection as opt-in. The checked
out method has quirks where fully unset properties can still be included, and
where disabling automatic pool initialization does not reliably exclude those
datasets. Those are implementation defects, not policies to reproduce.

## Time Slider

Time Slider is useful as a successor implementation and as evidence for the
interoperability conventions around properties and names.

Ideas retained by `zfstools-go`:

- Interval-specific property precedence over the general property.
- Recursive partitioning around excluded descendants.
- `zfs-auto-snap_<interval>-YYYY-MM-DD-HHhMM` naming.
- Numeric creation-time ordering.
- Preserving the newest snapshot while removing older zero-size snapshots.
- Restricting normal retention to eligible datasets.

Features outside this project's scope:

- Internal calendar scheduling and missed-schedule catch-up.
- SMF, D-Bus, GLib, RBAC, and desktop configuration.
- Capacity thresholds that override configured retention counts.
- `zfs send` and removable-media rsync plugins.
- Snapshot browsing, manual snapshot, and file-version GUIs.

Time Slider also demonstrates why creation results must be authoritative: its
snapshot command can fail without raising an error to the scheduler, after
which retention and plugins can still run. `zfstools-go` deliberately does not
follow that behavior.

## Direct Ruby Compatibility

The Go implementation retains the core Ruby command model:

- The `zfs-auto-snapshot`, `zfs-cleanup-snapshots`, and
  `zfs-snapshot-mysql` command names.
- Required `INTERVAL KEEP` arguments for automatic snapshots.
- Optional pool restriction and automatic discovery of eligible datasets.
- Exact `true`, `mysql`, and `postgresql` property values.
- Mounted-filesystem or volume eligibility.
- Automatic partitioning into individual and recursive targets.
- MySQL and PostgreSQL snapshot coordination selected by the general property.
- `KEEP=0` as cleanup-only.
- Generated-snapshot and standalone manual zero-size cleanup.
- Feature-based multi-target snapshot commands and optional parallelism.

It is not bug-for-bug compatible. Deliberate safety differences include:

- Strict flag and `KEEP` validation.
- Direct process execution instead of reconstructed shell commands where the
  external interface permits it.
- Fail-closed parsing and discovery.
- Unknown snapshot size is not zero.
- Exact generated-name matching.
- Numeric creation-time ordering.
- Per-target creation success controls retention eligibility.
- Serial destruction failures stop later cleanup; parallel destruction failures
  are joined. Independent snapshot creation continues to collect per-target
  results.
- Context cancellation remains visible before, during, and after cleanup.
- Dry run always prints planned mutations and never reaches a mutation runner.

## Current Policy Decisions

The following are intentional current policies, not unresolved compatibility
questions:

1. **Ruby lineage, not shell CLI compatibility.** Existing cron entries for
   another `zfs-auto-snapshot` implementation must not be redirected to this
   binary without migration.
2. **Opt-in datasets.** A mounted filesystem or volume must resolve to `true`,
   `mysql`, or `postgresql`; interval-specific eligibility is considered first.
   Database workflow selection currently comes from the general property.
3. **Automatic safe recursion.** The command discovers all eligible datasets
   and chooses recursive roots only when no excluded descendant would be
   included.
4. **Ruby/Time Slider naming.** Local time is the default; UTC names carry a
   trailing `U`. Retention accepts both exact forms.
5. **Final-count retention.** `KEEP` is the number left after the operation,
   including a newly created snapshot. `KEEP=0` creates nothing and removes all
   matching snapshots from included datasets.
6. **Zero-size cleanup remains enabled by default.** The newest snapshot is
   protected, every size decision is completed before mutation, and unknown
   sizes abort cleanup.
7. **Creation gates retention.** Independent successful targets can rotate;
   failed or indeterminate targets retain existing recovery points. Context
   cancellation skips retention.
8. **Creation metadata is authoritative.** Numeric ZFS creation time, not a
   timestamp-looking name, determines age.
9. **Operational failures are visible.** Discovery, parsing, size, creation,
   destruction, and cancellation failures produce nonzero status.
10. **Scheduling remains external.** This project does not absorb Time Slider's
    daemon, SMF, GUI, replication, or emergency capacity policy.

## Known Limitations

### Custom prefixes and standalone cleanup

`zfs-auto-snapshot -s <prefix>` creates and rotates snapshots with the custom
prefix correctly. The standalone `zfs-cleanup-snapshots` command has no prefix
option and conservatively excludes only snapshot components beginning with
`zfs-auto-snap_`. It therefore treats custom-prefix snapshots as manual and may
prune older zero-size instances. Do not combine custom automatic prefixes with
standalone zero-size cleanup until the cleanup command has an explicit prefix
policy.

### Database property resolution

Eligibility considers `com.sun:auto-snapshot:<interval>` before the general
property, but the `mysql` or `postgresql` workflow marker is currently derived
only from `com.sun:auto-snapshot`. An interval-specific database value can
include a dataset without selecting database coordination.

Recursive roots also carry only one database marker. Descendants requiring
different database workflows cannot both be represented by one recursive
snapshot command. PostgreSQL coordination is best-effort: the current sequence
still attempts the snapshot and stop command if the start-backup command fails.

### Positional argument compatibility

Flags and `KEEP` are validated strictly, but positional argument counts retain
Ruby-compatible behavior in several paths. Missing required arguments print
usage and return success, and extra automatic-snapshot or MySQL positional
arguments are ignored. Callers should not treat this as a general strict CLI
contract.

## Possible Future Features

These require explicit product decisions rather than compatibility assumptions:

- Exact positional dataset targeting.
- Pool health or scrub-aware creation policy.
- Snapshot event metadata.
- Pre/post hooks with a clearly defined non-shell or intentionally shell-based
  interface.
- Minimum-written-data thresholds.
- Migration support for historical separators or labels.
- Replication as a separate command or service.
- Capacity-driven emergency cleanup with explicit hold, clone, and recovery
  guarantees.
